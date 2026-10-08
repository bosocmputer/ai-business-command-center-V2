// Package master keeps a read-only copy of a shop's master data (customers, suppliers, items) so the assistant can find a
// record by a name or phone number. Once a day the worker reads three SELECTs from SML and replaces the copy. Nothing is
// ever written back to SML, and the copy holds codes, names and phone numbers only: no business figures.
package master

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/sml"
	"github.com/google/uuid"
)

type Kind string

const (
	KindCustomer Kind = "CUSTOMER"
	KindSupplier Kind = "SUPPLIER"
	KindItem     Kind = "ITEM"
)

var Kinds = []Kind{KindCustomer, KindSupplier, KindItem}

// Record is one master-data row as kept. Phone is empty for items; Unit and SupplierCode are empty for people.
type Record struct {
	Code, Name, Phone, Unit, SupplierCode string
	Active                                bool
}

// The SELECTs are fixed text: no value from outside is ever put into them.
var queries = map[Kind]string{
	KindCustomer: `select code, coalesce(name_1, '') as name, coalesce(nullif(telephone, ''), nullif(sms_phonenumber, ''), '') as phone, coalesce(status, 0) as status from ar_customer where coalesce(code, '') <> '' order by code`,
	KindSupplier: `select code, coalesce(name_1, '') as name, coalesce(telephone, '') as phone, coalesce(status, 0) as status from ap_supplier where coalesce(code, '') <> '' order by code`,
	KindItem:     `select code, coalesce(name_1, '') as name, coalesce(unit_standard_name, '') as unit, coalesce(supplier_code, '') as supplier_code, coalesce(status, 0) as status from ic_inventory where coalesce(code, '') <> '' order by code`,
}

// Query is the SELECT for a kind, for tests and for documentation.
func Query(kind Kind) string { return queries[kind] }

// Rows turns what SML returned into records: codes trimmed, a repeated code kept once, text cut to the sizes the table
// allows. A row without a code is skipped.
func Rows(kind Kind, raw []map[string]string) []Record {
	seen := make(map[string]struct{}, len(raw))
	records := make([]Record, 0, len(raw))
	for _, row := range raw {
		code := strings.TrimSpace(row["code"])
		if code == "" || utf8.RuneCountInString(code) > 80 {
			continue
		}
		if _, repeated := seen[code]; repeated {
			continue
		}
		seen[code] = struct{}{}
		record := Record{Code: code, Name: clip(row["name"], 300), Active: strings.TrimSpace(row["status"]) == "0" || strings.TrimSpace(row["status"]) == ""}
		switch kind {
		case KindItem:
			record.Unit, record.SupplierCode = clip(row["unit"], 100), clip(row["supplier_code"], 80)
		default:
			record.Phone = clip(row["phone"], 100)
		}
		records = append(records, record)
	}
	return records
}

func clip(text string, limit int) string {
	text = strings.Join(strings.Fields(text), " ")
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	return string([]rune(text)[:limit])
}

// Target is a shop with the copies that are due today.
type Target struct {
	TenantID uuid.UUID
	Timezone string
	Due      []Kind
}

type Store interface {
	// Targets lists the shops whose assistant is on and whose SML works, with the kinds not yet copied today and not tried in the last half hour.
	Targets(ctx context.Context, now time.Time) ([]Target, error)
	// Replace swaps one kind's copy for the new rows in one step. It refuses an empty result when a copy already exists.
	Replace(ctx context.Context, tenantID uuid.UUID, kind Kind, records []Record, now time.Time) error
	MarkFailed(ctx context.Context, tenantID uuid.UUID, kind Kind, code string, now time.Time) error
}

type Source interface {
	Fetch(ctx context.Context, tenantID uuid.UUID, kind Kind) ([]Record, error)
}

// ErrEmpty is returned by Store.Replace when SML gave nothing for a kind that had rows yesterday: more likely a failure
// than a shop that deleted all its customers, so the old copy stays.
var ErrEmpty = errors.New("master data result is empty")

// SMLSource reads a kind from a shop's SML through the existing, read-only client.
type SMLSource struct {
	Connections interface {
		Open(ctx context.Context, tenantID uuid.UUID) (sml.Connection, error)
	}
	Client *sml.Client
}

func (source SMLSource) Fetch(ctx context.Context, tenantID uuid.UUID, kind Kind) ([]Record, error) {
	statement, known := queries[kind]
	if !known {
		return nil, fmt.Errorf("master kind %q is not known", kind)
	}
	connection, err := source.Connections.Open(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	raw, err := source.Client.Query(ctx, connection, statement)
	if err != nil {
		return nil, err
	}
	return Rows(kind, raw), nil
}

// Syncer copies due kinds once a day inside a morning window (before the reports are prepared).
type Syncer struct {
	Store       Store
	Source      Source
	Logger      *slog.Logger
	Now         func() time.Time
	StartMinute int
	Window      time.Duration
}

func NewSyncer(store Store, source Source, logger *slog.Logger, now func() time.Time) *Syncer {
	return &Syncer{Store: store, Source: source, Logger: logger, Now: now, StartMinute: 6*60 + 30, Window: 4 * time.Hour}
}

type Summary struct{ Copied, Failed, Rows int }

func (syncer *Syncer) RunOnce(ctx context.Context) Summary {
	var summary Summary
	now := syncer.Now().UTC()
	targets, err := syncer.Store.Targets(ctx, now)
	if err != nil {
		syncer.Logger.Error("master targets failed", "safeErrorCode", "MASTER_TARGETS_FAILED")
		summary.Failed++
		return summary
	}
	for _, target := range targets {
		location, locErr := time.LoadLocation(target.Timezone)
		if locErr != nil {
			location = time.FixedZone("Asia/Bangkok", 7*60*60)
		}
		local := now.In(location)
		minutes := local.Hour()*60 + local.Minute()
		if minutes < syncer.StartMinute || time.Duration(minutes-syncer.StartMinute)*time.Minute > syncer.Window {
			continue
		}
		for _, kind := range target.Due {
			if ctx.Err() != nil {
				return summary
			}
			records, fetchErr := syncer.Source.Fetch(ctx, target.TenantID, kind)
			if fetchErr == nil {
				fetchErr = syncer.Store.Replace(ctx, target.TenantID, kind, records, syncer.Now().UTC())
			}
			if fetchErr != nil {
				summary.Failed++
				code := safeCode(fetchErr)
				syncer.Logger.Warn("master copy failed", "kind", string(kind), "safeErrorCode", code)
				_ = syncer.Store.MarkFailed(ctx, target.TenantID, kind, code, syncer.Now().UTC())
				continue
			}
			summary.Copied++
			summary.Rows += len(records)
		}
	}
	if summary != (Summary{}) {
		syncer.Logger.Info("master copy completed", "event", "master_sync", "kinds", summary.Copied, "failed", summary.Failed, "rows", summary.Rows)
	}
	return summary
}

// safeCode keeps only a short code: never the error text, which could carry SQL or a value.
func safeCode(err error) string {
	var safe *sml.SafeError
	var connection *sml.ConnectionTestError
	switch {
	case errors.Is(err, ErrEmpty):
		return "EMPTY_RESULT"
	case errors.As(err, &safe) && safe.Code != "":
		return safe.Code
	case errors.As(err, &connection) && connection.SafeCode != "":
		return connection.SafeCode
	default:
		return "MASTER_COPY_FAILED"
	}
}
