// Command mode-compare runs one chunkable report for a shop twice, once as a
// single query and once split into chunks, and says whether the results are
// identical. It is read-only and prints only counts and field names, never row
// values. Run it before trusting the automatic DIRECT to CHUNKED switch on a
// new report or a new kind of shop.
package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/config"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/database"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/secret"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/sml"
	"github.com/google/uuid"
)

const queryTimeout = 5 * time.Minute

type stepRows = map[string][]map[string]string

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("mode comparison failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return errors.New("invalid configuration")
	}
	tenantID, err := uuid.Parse(strings.TrimSpace(os.Getenv("MODE_COMPARE_TENANT_ID")))
	if err != nil {
		return errors.New("MODE_COMPARE_TENANT_ID must be a UUID")
	}
	key := report.Key(strings.TrimSpace(os.Getenv("MODE_COMPARE_REPORT")))
	definition, ok := report.DefinitionFor(key)
	if !ok || !definition.ChunkSafe {
		return errors.New("MODE_COMPARE_REPORT must be a chunkable report")
	}
	chunkSize := 0
	if raw := os.Getenv("MODE_COMPARE_CHUNK_SIZE"); raw != "" {
		if chunkSize, err = strconv.Atoi(raw); err != nil || chunkSize < report.MinimumChunkSize {
			return fmt.Errorf("MODE_COMPARE_CHUNK_SIZE must be a number of at least %d", report.MinimumChunkSize)
		}
	}
	date := time.Now().UTC().Add(7 * time.Hour).Format(time.DateOnly)
	if raw := os.Getenv("MODE_COMPARE_DATE"); raw != "" {
		if _, err := time.Parse(time.DateOnly, raw); err != nil {
			return errors.New("MODE_COMPARE_DATE must use YYYY-MM-DD")
		}
		date = raw
	}
	period := report.Period{Preset: report.AsOfRun, DateFrom: date, DateTo: date}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	pool, err := database.OpenPool(ctx, cfg.DatabaseURL, cfg.DatabaseMaxConnections, cfg.DatabaseMinConnections)
	if err != nil {
		return errors.New("database is unavailable")
	}
	defer pool.Close()
	box, err := secret.NewBox(cfg.EncryptionMasterKey, cfg.EncryptionKeyID, rand.Reader)
	if err != nil {
		return errors.New("encryption configuration is invalid")
	}
	policy := sml.EndpointPolicy{AllowedPrefixes: cfg.SMLAllowedPrefixes, AllowedHosts: cfg.SMLAllowedHosts, AllowPublicEndpoints: cfg.SMLAllowPublicEndpoints, AllowedPorts: cfg.SMLAllowedPorts}
	connection, err := sml.NewConnectionService(database.NewSMLConnectionStore(pool), box, policy, nil, time.Now).Open(ctx, tenantID)
	if err != nil {
		return errors.New("tenant SML connection could not be opened")
	}
	client := sml.NewClient(policy, queryTimeout, 32*1024*1024, 200_000)
	query := func(q report.Query) ([]map[string]string, error) {
		rendered, err := report.RenderSQL(q)
		if err != nil {
			return nil, err
		}
		queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
		defer cancel()
		return client.Query(queryCtx, connection, rendered)
	}

	allEqual := true
	for _, projection := range []report.ResultKind{report.ResultDetail, report.ResultSummary} {
		started := time.Now()
		direct, err := runDirect(query, key, period, projection)
		if err != nil {
			return fmt.Errorf("direct %s run: %w", projection, err)
		}
		directMs := time.Since(started).Milliseconds()
		started = time.Now()
		chunked, chunks, units, err := runChunked(query, key, period, projection, chunkSize)
		if err != nil {
			return fmt.Errorf("chunked %s run: %w", projection, err)
		}
		chunkedMs := time.Since(started).Milliseconds()

		differences := compare(key, period, projection, direct, chunked)
		equal := len(differences) == 0
		allEqual = allEqual && equal
		logger.Info("mode comparison", "reportKey", key, "projection", projection, "equal", equal,
			"directRows", countRows(direct), "chunkedRows", countRows(chunked), "units", units, "chunks", chunks,
			"directMs", directMs, "chunkedMs", chunkedMs, "differences", differences)
	}
	if !allEqual {
		return errors.New("direct and chunked results differ")
	}
	logger.Info("direct and chunked results are identical", "reportKey", key)
	return nil
}

func runDirect(query func(report.Query) ([]map[string]string, error), key report.Key, period report.Period, projection report.ResultKind) (stepRows, error) {
	plan, err := report.BuildQueryPlanForProjection(key, period, projection)
	if err != nil {
		return nil, err
	}
	steps := stepRows{}
	for _, step := range plan.Steps {
		rows, err := query(step.Query)
		if err != nil {
			return nil, err
		}
		steps[step.Name] = rows
	}
	return steps, nil
}

func runChunked(query func(report.Query) ([]map[string]string, error), key report.Key, period report.Period, projection report.ResultKind, override int) (stepRows, int, int, error) {
	manifestQuery, size, err := report.BuildChunkManifestQuery(key, period)
	if err != nil {
		return nil, 0, 0, err
	}
	if override > 0 {
		size = override
	}
	manifestRows, err := query(manifestQuery)
	if err != nil {
		return nil, 0, 0, err
	}
	units, err := report.ChunkKeys(manifestRows)
	if err != nil {
		return nil, 0, 0, err
	}
	var chunks []stepRows
	for start := 0; start < len(units); start += size {
		end := min(start+size, len(units))
		plan, err := report.BuildChunkQueryPlan(key, period, projection, units[start:end])
		if err != nil {
			return nil, 0, 0, err
		}
		chunk := stepRows{}
		for _, step := range plan.Steps {
			rows, err := query(step.Query)
			if err != nil {
				return nil, 0, 0, err
			}
			chunk[step.Name] = rows
		}
		chunks = append(chunks, chunk)
	}
	merged, err := report.MergeChunkedSteps(key, projection, chunks)
	return merged, len(chunks), len(units), err
}

// compare returns only the names of what differs, never values.
func compare(key report.Key, period report.Period, projection report.ResultKind, direct, chunked stepRows) []string {
	var differences []string
	if projection == report.ResultDetail {
		if !sameRows(direct["rows"], chunked["rows"]) {
			differences = append(differences, "detail rows")
		}
		// Chunking may reorder detail rows, which is not a difference; sort both
		// sides so the summary and dashboard comparison below ignores order too.
		direct, chunked = sortedRows(direct), sortedRows(chunked)
	}
	directSummary, err1 := report.Summarize(key, direct)
	chunkedSummary, err2 := report.Summarize(key, chunked)
	if err1 != nil || err2 != nil {
		return append(differences, "summary could not be built")
	}
	differences = append(differences, summaryDifferences(directSummary, chunkedSummary)...)
	comparison, err := report.ResolveComparisonPeriod(period)
	if err == nil {
		empty := stepRows{"rows": {}}
		directDashboard, e1 := report.BuildDashboard(key, period, comparison, direct, empty)
		chunkedDashboard, e2 := report.BuildDashboard(key, period, comparison, chunked, empty)
		if e1 == nil && e2 == nil && !sameJSON(directDashboard, chunkedDashboard) {
			differences = append(differences, "dashboard")
		}
	}
	return differences
}

// summaryDifferences names which top-level summary fields differ and, for the
// row list, whether only the order differs. Values are never returned.
func summaryDifferences(direct, chunked report.SummaryResult) []string {
	var left, right map[string]json.RawMessage
	leftBytes, _ := json.Marshal(direct)
	rightBytes, _ := json.Marshal(chunked)
	if json.Unmarshal(leftBytes, &left) != nil || json.Unmarshal(rightBytes, &right) != nil {
		return []string{"summary"}
	}
	var differences []string
	for name, value := range left {
		if string(value) == string(right[name]) {
			continue
		}
		if name == "Rows" {
			var leftRows, rightRows []map[string]string
			if json.Unmarshal(value, &leftRows) == nil && json.Unmarshal(right[name], &rightRows) == nil {
				if sameRows(leftRows, rightRows) {
					differences = append(differences, "summary.Rows (same rows, different order)")
					continue
				}
				if sameRows(canonicalRows(leftRows), canonicalRows(rightRows)) {
					differences = append(differences, "summary.Rows (same numbers, different formatting)")
					continue
				}
			}
			var leftRows, rightRows []map[string]string
			detail := "different rows"
			if json.Unmarshal(value, &leftRows) == nil && json.Unmarshal(right[name], &rightRows) == nil {
				detail = describeRowDifference(leftRows, rightRows)
			}
			differences = append(differences, "summary.Rows ("+detail+")")
			continue
		}
		differences = append(differences, "summary."+name)
	}
	sort.Strings(differences)
	return differences
}

// describeRowDifference matches rows by customer code and document sort and says
// how many rows exist on one side only and which fields differ on the rest.
func describeRowDifference(direct, chunked []map[string]string) string {
	identity := func(row map[string]string) string { return row["cust_code"] + "|" + row["doc_sort"] }
	right := make(map[string]map[string]string, len(chunked))
	for _, row := range chunked {
		right[identity(row)] = row
	}
	onlyDirect, matched := 0, 0
	fields := map[string]bool{}
	for _, row := range direct {
		other, ok := right[identity(row)]
		if !ok {
			onlyDirect++
			continue
		}
		matched++
		delete(right, identity(row))
		for name, value := range row {
			if other[name] != value {
				fields[name] = true
			}
		}
	}
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	return fmt.Sprintf("only in direct: %d, only in chunked: %d, matched: %d, differing fields on matched: %v", onlyDirect, len(right), matched, names)
}

// canonicalRows rewrites every numeric value in a canonical form, so 12.50 and
// 12.5000 compare equal. Non-numeric values are left untouched.
func canonicalRows(rows []map[string]string) []map[string]string {
	canonical := make([]map[string]string, len(rows))
	for index, row := range rows {
		copied := make(map[string]string, len(row))
		for name, value := range row {
			// Only decimals are rewritten: codes such as 001 are text and must stay as is.
			if number, ok := new(big.Rat).SetString(value); ok && strings.Contains(value, ".") {
				copied[name] = number.RatString()
			} else {
				copied[name] = value
			}
		}
		canonical[index] = copied
	}
	return canonical
}

func sortedRows(steps stepRows) stepRows {
	sorted := make(stepRows, len(steps))
	for name, rows := range steps {
		copied := append([]map[string]string(nil), rows...)
		sort.Slice(copied, func(i, j int) bool {
			a, _ := json.Marshal(copied[i])
			b, _ := json.Marshal(copied[j])
			return string(a) < string(b)
		})
		sorted[name] = copied
	}
	return sorted
}

func countRows(steps stepRows) int {
	total := 0
	for _, rows := range steps {
		total += len(rows)
	}
	return total
}

func sameJSON(left, right any) bool {
	a, errA := json.Marshal(left)
	b, errB := json.Marshal(right)
	return errA == nil && errB == nil && string(a) == string(b)
}

// sameRows compares as unordered sets of rows: chunking may reorder rows, and
// any ordering the report cares about is applied later by Summarize.
func sameRows(left, right []map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	counts := make(map[string]int, len(left))
	for _, row := range left {
		encoded, _ := json.Marshal(row)
		counts[string(encoded)]++
	}
	for _, row := range right {
		encoded, _ := json.Marshal(row)
		counts[string(encoded)]--
		if counts[string(encoded)] < 0 {
			return false
		}
	}
	return true
}
