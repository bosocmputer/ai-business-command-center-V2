package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/executionmode"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrExecutionModeUnsupported = errors.New("report cannot be split into chunks")

type ReportModeStore struct{ pool *pgxpool.Pool }

func NewReportModeStore(pool *pgxpool.Pool) *ReportModeStore { return &ReportModeStore{pool: pool} }

const modeColumns = `report_key, mode, source, coalesce(reason, ''), last_rows, last_duration_ms, changed_at`

func scanMode(row pgx.Row) (report.ExecutionModeRecord, error) {
	var record report.ExecutionModeRecord
	var key, mode, source string
	var changedAt time.Time
	if err := row.Scan(&key, &mode, &source, &record.Reason, &record.LastRows, &record.LastDurationMS, &changedAt); err != nil {
		return report.ExecutionModeRecord{}, err
	}
	record.ReportKey, record.Mode, record.Source, record.ChangedAt = report.Key(key), report.ExecutionMode(mode), report.ModeSource(source), &changedAt
	return record, nil
}

// Get returns the stored record. A missing row is reported as DIRECT/DEFAULT so
// callers never need to special-case it.
func (s *ReportModeStore) Get(ctx context.Context, tenantID uuid.UUID, key report.Key) (report.ExecutionModeRecord, error) {
	record, err := scanMode(s.pool.QueryRow(ctx, `select `+modeColumns+` from report_execution_modes where tenant_id = $1 and report_key = $2`, tenantID, string(key)))
	if errors.Is(err, pgx.ErrNoRows) {
		return report.ExecutionModeRecord{ReportKey: key, Mode: report.ModeDirect, Source: report.ModeSourceDefault}, nil
	}
	return record, err
}

func (s *ReportModeStore) List(ctx context.Context, tenantID uuid.UUID) ([]report.ExecutionModeRecord, error) {
	rows, err := s.pool.Query(ctx, `select `+modeColumns+` from report_execution_modes where tenant_id = $1 order by report_key`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := []report.ExecutionModeRecord{}
	for rows.Next() {
		record, err := scanMode(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

// SeedFromTargets records the legacy HEAVY_CHUNK_TENANT_REPORTS entries so the
// switch to table-driven modes changes nothing on the first deploy. Existing rows
// win, and tenants that no longer exist are skipped.
func (s *ReportModeStore) SeedFromTargets(ctx context.Context, targets []string, now time.Time) error {
	for _, target := range targets {
		var tenantText, keyText string
		for index := range target {
			if target[index] == '/' {
				tenantText, keyText = target[:index], target[index+1:]
				break
			}
		}
		tenantID, err := uuid.Parse(tenantText)
		if err != nil || keyText == "" {
			continue
		}
		if _, err := s.pool.Exec(ctx, `
			insert into report_execution_modes (tenant_id, report_key, mode, source, reason, changed_at, updated_at)
			select $1, $2, 'CHUNKED', 'ENV_SEED', 'seeded from HEAVY_CHUNK_TENANT_REPORTS', $3, $3
			where exists (select 1 from tenants where id = $1)
			on conflict (tenant_id, report_key) do nothing`, tenantID, keyText, now); err != nil {
			return fmt.Errorf("seed report execution mode: %w", err)
		}
	}
	return nil
}

// SwitchToChunked moves a DIRECT report to CHUNKED and writes an audit row. It
// returns false when the report was already CHUNKED (another worker won).
func (s *ReportModeStore) SwitchToChunked(ctx context.Context, tenantID uuid.UUID, key report.Key, reason string, now time.Time) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var switched bool
	err = tx.QueryRow(ctx, `
		insert into report_execution_modes (tenant_id, report_key, mode, source, reason, changed_at, updated_at)
		values ($1, $2, 'CHUNKED', 'AUTO_SWITCHED', $3, $4, $4)
		on conflict (tenant_id, report_key) do update
		  set mode = 'CHUNKED', source = 'AUTO_SWITCHED', reason = excluded.reason,
		      consecutive_direct_timeouts = 0, changed_at = $4, updated_at = $4
		  where report_execution_modes.mode = 'DIRECT'
		returning true`, tenantID, string(key), reason, now).Scan(&switched)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("switch report to chunked: %w", err)
	}
	if err := insertModeAudit(ctx, tx, tenantID, "WORKER", nil, "REPORT_MODE_AUTO_SWITCHED", key, "", report.ModeDirect, report.ModeChunked, reason, now); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

// Set applies an admin's choice, or a first-time measurement when source is
// MEASURED. It refuses CHUNKED for reports that cannot be chunked.
func (s *ReportModeStore) Set(ctx context.Context, actorHash []byte, requestID string, tenantID uuid.UUID, key report.Key, mode report.ExecutionMode, source report.ModeSource, reason string, now time.Time) (report.ExecutionModeRecord, error) {
	definition, known := report.DefinitionFor(key)
	if !known || !mode.Valid() || (mode == report.ModeChunked && !definition.ChunkSafe) {
		return report.ExecutionModeRecord{}, ErrExecutionModeUnsupported
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return report.ExecutionModeRecord{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	before := report.ModeDirect
	var current string
	switch err := tx.QueryRow(ctx, `select mode from report_execution_modes where tenant_id = $1 and report_key = $2 for update`, tenantID, string(key)).Scan(&current); {
	case err == nil:
		before = report.ExecutionMode(current)
	case errors.Is(err, pgx.ErrNoRows):
	default:
		return report.ExecutionModeRecord{}, err
	}
	record, err := scanMode(tx.QueryRow(ctx, `
		insert into report_execution_modes (tenant_id, report_key, mode, source, reason, changed_at, updated_at)
		values ($1, $2, $3, $4, nullif($5, ''), $6, $6)
		on conflict (tenant_id, report_key) do update
		  set mode = excluded.mode, source = excluded.source, reason = excluded.reason,
		      consecutive_direct_timeouts = 0,
		      changed_at = case when report_execution_modes.mode = excluded.mode then report_execution_modes.changed_at else $6 end,
		      updated_at = $6
		returning `+modeColumns, tenantID, string(key), string(mode), string(source), reason, now))
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		return report.ExecutionModeRecord{}, executionmode.ErrTenantNotFound
	}
	if err != nil {
		return report.ExecutionModeRecord{}, fmt.Errorf("set report execution mode: %w", err)
	}
	if before != mode {
		action := "REPORT_MODE_SET"
		if source == report.ModeSourceMeasured {
			action = "REPORT_MODE_MEASURED"
		}
		if err := insertModeAudit(ctx, tx, tenantID, "ADMIN", actorHash, action, key, requestID, before, mode, reason, now); err != nil {
			return report.ExecutionModeRecord{}, err
		}
	}
	return record, tx.Commit(ctx)
}

// RecordMeasurement stores the latest size and duration without touching the mode.
func (s *ReportModeStore) RecordMeasurement(ctx context.Context, tenantID uuid.UUID, key report.Key, rows int, duration time.Duration, now time.Time) error {
	_, err := s.pool.Exec(ctx, `
		insert into report_execution_modes (tenant_id, report_key, last_rows, last_duration_ms, changed_at, updated_at)
		values ($1, $2, $3, $4, $5, $5)
		on conflict (tenant_id, report_key) do update
		  set last_rows = excluded.last_rows, last_duration_ms = excluded.last_duration_ms, updated_at = $5`,
		tenantID, string(key), rows, duration.Milliseconds(), now)
	return err
}

// RecordDirectSuccess stores the measurement and clears the timeout streak.
func (s *ReportModeStore) RecordDirectSuccess(ctx context.Context, tenantID uuid.UUID, key report.Key, rows int, duration time.Duration, now time.Time) error {
	_, err := s.pool.Exec(ctx, `
		insert into report_execution_modes (tenant_id, report_key, last_rows, last_duration_ms, changed_at, updated_at)
		values ($1, $2, $3, $4, $5, $5)
		on conflict (tenant_id, report_key) do update
		  set last_rows = excluded.last_rows, last_duration_ms = excluded.last_duration_ms,
		      consecutive_direct_timeouts = 0, updated_at = $5`,
		tenantID, string(key), rows, duration.Milliseconds(), now)
	return err
}

// RecordDirectTimeout counts consecutive DIRECT timeouts and returns the streak.
func (s *ReportModeStore) RecordDirectTimeout(ctx context.Context, tenantID uuid.UUID, key report.Key, now time.Time) (int, error) {
	var streak int
	err := s.pool.QueryRow(ctx, `
		insert into report_execution_modes (tenant_id, report_key, consecutive_direct_timeouts, changed_at, updated_at)
		values ($1, $2, 1, $3, $3)
		on conflict (tenant_id, report_key) do update
		  set consecutive_direct_timeouts = report_execution_modes.consecutive_direct_timeouts + 1, updated_at = $3
		returning consecutive_direct_timeouts`, tenantID, string(key), now).Scan(&streak)
	return streak, err
}

func insertModeAudit(ctx context.Context, tx pgx.Tx, tenantID uuid.UUID, actorType string, actorHash []byte, action string, key report.Key, requestID string, before, after report.ExecutionMode, reason string, now time.Time) error {
	beforeJSON, _ := json.Marshal(map[string]any{"mode": before})
	afterJSON, _ := json.Marshal(map[string]any{"mode": after, "reason": reason})
	_, err := tx.Exec(ctx, `
		insert into audit_logs (tenant_id, actor_type, actor_id_hash, action, resource_type, resource_id, request_id, before_json, after_json, result, created_at, expires_at)
		values ($1, $2, $3, $4, 'REPORT_EXECUTION_MODE', $5, nullif($6, ''), $7, $8, 'SUCCESS', $9, $10)`,
		tenantID, actorType, actorHash, action, string(key), requestID, beforeJSON, afterJSON, now, now.AddDate(1, 0, 0))
	if err != nil {
		return fmt.Errorf("insert execution mode audit: %w", err)
	}
	return nil
}

// TenantBusy reports whether a report run for the tenant is being worked on, so
// an admin measurement does not add load next to it.
func (s *ReportModeStore) TenantBusy(ctx context.Context, tenantID uuid.UUID) (bool, error) {
	var busy bool
	err := s.pool.QueryRow(ctx, `select exists (select 1 from report_runs where tenant_id = $1 and status in ('CLAIMED', 'RUNNING'))`, tenantID).Scan(&busy)
	return busy, err
}
