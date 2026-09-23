package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/recipient"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// RecordFollowState applies a LINE follow/unfollow event to the matching active
// recipient. The changed-at guard makes webhook redeliveries and out-of-order
// events idempotent: only a strictly newer event can change the state.
func (store *RecipientStore) RecordFollowState(ctx context.Context, lineHash []byte, state recipient.FollowState, changedAt, now time.Time) (bool, error) {
	if state != recipient.FollowStateFollowing && state != recipient.FollowStateBlocked {
		return false, fmt.Errorf("record follow state: unsupported state %q", state)
	}
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin follow state update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var recipientID uuid.UUID
	err = tx.QueryRow(ctx, `
		update line_recipients
		set line_follow_status = $2, line_follow_changed_at = $3, updated_at = $4
		where line_user_id_hash = $1 and status = 'ACTIVE'
		  and (line_follow_changed_at is null or line_follow_changed_at < $3)
		returning id`, lineHash, string(state), changedAt, now).Scan(&recipientID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("update follow state: %w", err)
	}

	action := "LINE_RECIPIENT_FOLLOWED"
	if state == recipient.FollowStateBlocked {
		action = "LINE_RECIPIENT_BLOCKED"
	}
	afterJSON, err := json.Marshal(map[string]any{"lineFollowStatus": state, "lineFollowChangedAt": changedAt})
	if err != nil {
		return false, fmt.Errorf("encode follow state audit: %w", err)
	}
	// One audit row per store the person belongs to, so each tenant's history
	// shows it. The LINE user ID never enters the audit log.
	if _, err := tx.Exec(ctx, `
		insert into audit_logs (
		  tenant_id, actor_type, action, resource_type, resource_id,
		  after_json, result, created_at, expires_at
		)
		select membership.tenant_id, 'SYSTEM', $2, 'LINE_RECIPIENT', $6,
		       $3, 'SUCCESS', $4, $5
		from tenant_memberships membership
		where membership.recipient_id = $1 and membership.status <> 'REVOKED'`,
		recipientID, action, afterJSON, now, now.AddDate(1, 0, 0), recipientID.String()); err != nil {
		return false, fmt.Errorf("insert follow state audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit follow state update: %w", err)
	}
	return true, nil
}
