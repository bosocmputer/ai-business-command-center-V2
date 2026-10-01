// Command sml-probe runs read-only SELECT statements from a file against one
// tenant's SML through JavaWS and prints the rows as JSON lines. It is for
// operators checking which fields a shop's data has. Statements are separated by
// a line containing only "---". Only a single SELECT or WITH ... SELECT is
// accepted, results are capped, and nothing is ever written.
package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/config"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/database"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/secret"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/sml"
	"github.com/google/uuid"
)

const (
	queryTimeout = 60 * time.Second
	maxRows      = 200
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "sml-probe:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return errors.New("invalid configuration")
	}
	tenantID, err := uuid.Parse(strings.TrimSpace(os.Getenv("SML_PROBE_TENANT_ID")))
	if err != nil {
		return errors.New("SML_PROBE_TENANT_ID must be a UUID")
	}
	raw, err := os.ReadFile(os.Getenv("SML_PROBE_SQL_FILE"))
	if err != nil {
		return errors.New("SML_PROBE_SQL_FILE could not be read")
	}
	statements, err := splitStatements(string(raw))
	if err != nil {
		return err
	}

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
	client := sml.NewClient(policy, queryTimeout, 8*1024*1024, maxRows)

	for index, statement := range statements {
		queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
		started := time.Now()
		rows, err := client.Query(queryCtx, connection, "select * from (\n"+statement+"\n) as probe limit "+fmt.Sprint(maxRows))
		cancel()
		fmt.Printf("-- statement %d (%d ms)\n", index+1, time.Since(started).Milliseconds())
		if err != nil {
			fmt.Printf("ERROR %v\n", err)
			continue
		}
		for _, row := range rows {
			encoded, _ := json.Marshal(row)
			fmt.Println(string(encoded))
		}
	}
	return nil
}

// splitStatements accepts only single read-only statements.
func splitStatements(text string) ([]string, error) {
	var statements []string
	for _, part := range strings.Split(text, "\n---\n") {
		statement := strings.TrimSpace(part)
		if statement == "" {
			continue
		}
		lower := strings.ToLower(statement)
		if !strings.HasPrefix(lower, "select") && !strings.HasPrefix(lower, "with") {
			return nil, errors.New("only SELECT statements are allowed")
		}
		if strings.Contains(strings.TrimSuffix(statement, ";"), ";") {
			return nil, errors.New("only one statement per block is allowed")
		}
		statements = append(statements, strings.TrimSuffix(statement, ";"))
	}
	if len(statements) == 0 {
		return nil, errors.New("no statements to run")
	}
	return statements, nil
}
