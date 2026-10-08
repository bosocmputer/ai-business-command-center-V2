package database

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EvidenceStore copies the proof behind a disputed number into one row that normal retention does not touch.
type EvidenceStore struct{ pool *pgxpool.Pool }

func NewEvidenceStore(pool *pgxpool.Pool) *EvidenceStore { return &EvidenceStore{pool: pool} }

// CaseSummary is what a person sees about a case: counts and times, never the payload.
type CaseSummary struct {
	ID          uuid.UUID
	TenantID    uuid.UUID
	Reason      string
	RequestedBy string
	WindowFrom  time.Time
	WindowTo    time.Time
	CreatedAt   time.Time
	ExpiresAt   time.Time
	Calls       int
	Snapshots   int
	Cards       int
	Alerts      int
	Bytes       int
}

const (
	evidenceMaxCalls     = 2000
	evidenceMaxSnapshots = 60
)

// Capture copies, for one shop and a time window: the assistant's call log (metadata), the stored reports those calls (and the
// cards sent in the window) were answered from, with their figures, the cards sent and the alerts raised.
func (store *EvidenceStore) Capture(ctx context.Context, tenantID uuid.UUID, from, to time.Time, reason, requestedBy string, now time.Time) (CaseSummary, error) {
	var summary CaseSummary
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return CaseSummary{}, fmt.Errorf("begin evidence capture: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	err = tx.QueryRow(ctx, `
		with calls as (
		  select id, created_at, tool, report_key, period_from, period_to, outcome, snapshot_run_id
		  from agent_calls where tenant_id = $1 and created_at between $2 and $3 order by created_at limit $6
		), cards as (
		  select n.id, n.scheduled_for, n.status, n.safe_error_code, n.finished_at,
		         coalesce((select jsonb_agg(jsonb_build_object('reportKey', nr.report_key, 'reportRunId', nr.report_run_id)) from notification_run_reports nr where nr.notification_run_id = n.id), '[]'::jsonb) as reports
		  from notification_runs n where n.tenant_id = $1 and n.scheduled_for between $2 and $3 order by n.scheduled_for limit 500
		), runs as (
		  select distinct r.id from report_runs r
		  where r.tenant_id = $1 and r.id in (
		    select snapshot_run_id from calls where snapshot_run_id is not null
		    union select nr.report_run_id from notification_run_reports nr join cards c on c.id = nr.notification_run_id)
		  limit $7
		), snapshots as (
		  select r.id, r.report_key, r.period_from, r.period_to, r.finished_at, r.report_definition_version, r.query_plan_fingerprint, r.dashboard_json
		  from report_runs r join runs on runs.id = r.id
		), alerts as (
		  select rule_key, fired_on, value, threshold, message, status, created_at
		  from agent_alert_events where tenant_id = $1 and created_at between $2 and $3 order by created_at limit 500
		), built as (
		  select jsonb_build_object(
		    'calls', coalesce((select jsonb_agg(jsonb_build_object('at', created_at, 'tool', tool, 'reportKey', report_key, 'periodFrom', period_from, 'periodTo', period_to, 'outcome', outcome, 'snapshotRunId', snapshot_run_id) order by created_at) from calls), '[]'::jsonb),
		    'snapshots', coalesce((select jsonb_agg(jsonb_build_object('runId', id, 'reportKey', report_key, 'periodFrom', period_from, 'periodTo', period_to, 'finishedAt', finished_at, 'definitionVersion', report_definition_version, 'queryPlanFingerprint', query_plan_fingerprint, 'dashboard', dashboard_json)) from snapshots), '[]'::jsonb),
		    'cards', coalesce((select jsonb_agg(jsonb_build_object('notificationRunId', id, 'scheduledFor', scheduled_for, 'status', status, 'errorCode', safe_error_code, 'finishedAt', finished_at, 'reports', reports) order by scheduled_for) from cards), '[]'::jsonb),
		    'alerts', coalesce((select jsonb_agg(jsonb_build_object('rule', rule_key, 'firedOn', fired_on, 'value', value, 'threshold', threshold, 'message', message, 'status', status) order by created_at) from alerts), '[]'::jsonb)
		  ) as payload
		)
		insert into evidence_cases (tenant_id, reason, requested_by, window_from, window_to, created_at, expires_at, payload)
		select $1, $4, $5, $2, $3, $8::timestamptz, $8::timestamptz + interval '1095 days', payload from built
		returning id, created_at, expires_at, octet_length(payload::text),
		  jsonb_array_length(payload -> 'calls'), jsonb_array_length(payload -> 'snapshots'), jsonb_array_length(payload -> 'cards'), jsonb_array_length(payload -> 'alerts')`,
		tenantID, from, to, reason, requestedBy, evidenceMaxCalls, evidenceMaxSnapshots, now).
		Scan(&summary.ID, &summary.CreatedAt, &summary.ExpiresAt, &summary.Bytes, &summary.Calls, &summary.Snapshots, &summary.Cards, &summary.Alerts)
	if err != nil {
		return CaseSummary{}, fmt.Errorf("capture evidence: %w", err)
	}
	summary.TenantID, summary.Reason, summary.RequestedBy, summary.WindowFrom, summary.WindowTo = tenantID, reason, requestedBy, from, to
	after := fmt.Sprintf(`{"caseId":%q,"calls":%d,"snapshots":%d,"cards":%d,"alerts":%d}`, summary.ID, summary.Calls, summary.Snapshots, summary.Cards, summary.Alerts)
	actor := sha256.Sum256([]byte(requestedBy))
	if err := insertAuditAs(ctx, tx, tenantID, "ADMIN", actor[:], "EVIDENCE_CASE_CREATED", "EVIDENCE_CASE", summary.ID.String(), "", nil, []byte(after), now); err != nil {
		return CaseSummary{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CaseSummary{}, fmt.Errorf("commit evidence capture: %w", err)
	}
	return summary, nil
}

// List returns the cases of one shop, newest first, without their payloads.
func (store *EvidenceStore) List(ctx context.Context, tenantID uuid.UUID) ([]CaseSummary, error) {
	rows, err := store.pool.Query(ctx, `
		select id, reason, requested_by, window_from, window_to, created_at, expires_at, octet_length(payload::text),
		       jsonb_array_length(payload -> 'calls'), jsonb_array_length(payload -> 'snapshots'), jsonb_array_length(payload -> 'cards'), jsonb_array_length(payload -> 'alerts')
		from evidence_cases where tenant_id = $1 order by created_at desc limit 200`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list evidence cases: %w", err)
	}
	defer rows.Close()
	var list []CaseSummary
	for rows.Next() {
		summary := CaseSummary{TenantID: tenantID}
		if err := rows.Scan(&summary.ID, &summary.Reason, &summary.RequestedBy, &summary.WindowFrom, &summary.WindowTo, &summary.CreatedAt, &summary.ExpiresAt, &summary.Bytes, &summary.Calls, &summary.Snapshots, &summary.Cards, &summary.Alerts); err != nil {
			return nil, err
		}
		list = append(list, summary)
	}
	return list, rows.Err()
}

// Payload returns one case's copy as JSON text, for a person who needs to read the proof.
func (store *EvidenceStore) Payload(ctx context.Context, tenantID, caseID uuid.UUID) (string, error) {
	var payload string
	err := store.pool.QueryRow(ctx, `select payload::text from evidence_cases where tenant_id = $1 and id = $2`, tenantID, caseID).Scan(&payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("evidence case not found")
	}
	return payload, err
}

// Delete removes a case (on the shop's request, or when it was made by mistake).
func (store *EvidenceStore) Delete(ctx context.Context, tenantID, caseID uuid.UUID, requestedBy string, now time.Time) (bool, error) {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin evidence delete: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `delete from evidence_cases where tenant_id = $1 and id = $2`, tenantID, caseID)
	if err != nil || tag.RowsAffected() != 1 {
		return false, err
	}
	actor := sha256.Sum256([]byte(requestedBy))
	if err := insertAuditAs(ctx, tx, tenantID, "ADMIN", actor[:], "EVIDENCE_CASE_DELETED", "EVIDENCE_CASE", caseID.String(), "", nil, nil, now); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}
