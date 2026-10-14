package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/engagement"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type EngagementStore struct{ pool *pgxpool.Pool }

func NewEngagementStore(pool *pgxpool.Pool) *EngagementStore { return &EngagementStore{pool: pool} }

const engagementColumns = `alert_channel, digest_enabled, weekly_summary_enabled, weekly_summary_weekday, weekly_summary_hour, inactivity_notice_enabled, inactivity_quiet_days, version`

func scanEngagement(row pgx.Row) (engagement.Settings, error) {
	var settings engagement.Settings
	err := row.Scan(&settings.AlertChannel, &settings.DigestEnabled, &settings.WeeklySummaryEnabled, &settings.WeeklySummaryWeekday, &settings.WeeklySummaryHour,
		&settings.InactivityNoticeEnabled, &settings.InactivityQuietDays, &settings.Version)
	return settings, err
}

// Get gives the shop's settings, or the defaults (version 0) when the admin has chosen nothing.
func (store *EngagementStore) Get(ctx context.Context, tenantID uuid.UUID) (engagement.Settings, error) {
	var exists bool
	if err := store.pool.QueryRow(ctx, `select exists(select 1 from tenants where id = $1)`, tenantID).Scan(&exists); err != nil {
		return engagement.Settings{}, fmt.Errorf("check shop for engagement settings: %w", err)
	}
	if !exists {
		return engagement.Settings{}, engagement.ErrNotFound
	}
	settings, err := scanEngagement(store.pool.QueryRow(ctx, `select `+engagementColumns+` from tenant_engagement_settings where tenant_id = $1`, tenantID))
	if errors.Is(err, pgx.ErrNoRows) {
		return engagement.Defaults(), nil
	}
	if err != nil {
		return engagement.Settings{}, fmt.Errorf("get engagement settings: %w", err)
	}
	return settings, nil
}

func (store *EngagementStore) Put(ctx context.Context, actorHash []byte, requestID string, tenantID uuid.UUID, input engagement.Settings, expectedVersion int, now time.Time) (engagement.Settings, error) {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return engagement.Settings{}, fmt.Errorf("begin engagement settings update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var exists bool
	if err := tx.QueryRow(ctx, `select exists(select 1 from tenants where id = $1)`, tenantID).Scan(&exists); err != nil {
		return engagement.Settings{}, fmt.Errorf("check shop for engagement settings: %w", err)
	}
	if !exists {
		return engagement.Settings{}, engagement.ErrNotFound
	}
	var currentVersion int
	before := engagement.Defaults()
	err = tx.QueryRow(ctx, `select version from tenant_engagement_settings where tenant_id = $1 for update`, tenantID).Scan(&currentVersion)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		if expectedVersion != 0 {
			return engagement.Settings{}, engagement.ErrConflict
		}
		_, err = tx.Exec(ctx, `
			insert into tenant_engagement_settings (tenant_id, alert_channel, digest_enabled, weekly_summary_enabled, weekly_summary_weekday, weekly_summary_hour,
			  inactivity_notice_enabled, inactivity_quiet_days, version, updated_at)
			values ($1, $2, $3, $4, $5, $6, $7, $8, 1, $9)`,
			tenantID, input.AlertChannel, input.DigestEnabled, input.WeeklySummaryEnabled, input.WeeklySummaryWeekday, input.WeeklySummaryHour,
			input.InactivityNoticeEnabled, input.InactivityQuietDays, now)
	case err != nil:
		return engagement.Settings{}, fmt.Errorf("lock engagement settings: %w", err)
	default:
		if expectedVersion != currentVersion {
			return engagement.Settings{}, engagement.ErrConflict
		}
		before, err = scanEngagement(tx.QueryRow(ctx, `select `+engagementColumns+` from tenant_engagement_settings where tenant_id = $1`, tenantID))
		if err != nil {
			return engagement.Settings{}, fmt.Errorf("read engagement settings: %w", err)
		}
		_, err = tx.Exec(ctx, `
			update tenant_engagement_settings
			set alert_channel = $2, digest_enabled = $3, weekly_summary_enabled = $4, weekly_summary_weekday = $5, weekly_summary_hour = $6,
			    inactivity_notice_enabled = $7, inactivity_quiet_days = $8, version = version + 1, updated_at = $9
			where tenant_id = $1`,
			tenantID, input.AlertChannel, input.DigestEnabled, input.WeeklySummaryEnabled, input.WeeklySummaryWeekday, input.WeeklySummaryHour,
			input.InactivityNoticeEnabled, input.InactivityQuietDays, now)
	}
	if err != nil {
		return engagement.Settings{}, fmt.Errorf("save engagement settings: %w", err)
	}
	updated, err := scanEngagement(tx.QueryRow(ctx, `select `+engagementColumns+` from tenant_engagement_settings where tenant_id = $1`, tenantID))
	if err != nil {
		return engagement.Settings{}, fmt.Errorf("read saved engagement settings: %w", err)
	}
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(updated)
	if err := insertAudit(ctx, tx, tenantID, actorHash, "TENANT_ENGAGEMENT_UPDATED", "TENANT", tenantID.String(), requestID, beforeJSON, afterJSON, now); err != nil {
		return engagement.Settings{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return engagement.Settings{}, fmt.Errorf("commit engagement settings update: %w", err)
	}
	return updated, nil
}
