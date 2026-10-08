package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/agent"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// trimNumber shows a stored numeric(20,4) the way it was said: 500000.0000 -> 500000, 12.5000 -> 12.5.
func trimNumber(text string) string {
	if strings.Contains(text, ".") {
		text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	}
	return text
}

func (store *AgentStore) Alerts(ctx context.Context, principal agent.Principal) ([]agent.StoredAlert, error) {
	rows, err := store.pool.Query(ctx, `
		select rule_key, threshold::text, enabled, last_fired_at, coalesce(last_status, '')
		from agent_alert_rules where tenant_id = $1 and recipient_id = $2 order by rule_key`, principal.TenantID, principal.RecipientID)
	if err != nil {
		return nil, fmt.Errorf("list alert rules: %w", err)
	}
	defer rows.Close()
	items := make([]agent.StoredAlert, 0)
	for rows.Next() {
		var item agent.StoredAlert
		if err := rows.Scan(&item.Rule, &item.Threshold, &item.Enabled, &item.LastFiredAt, &item.LastStatus); err != nil {
			return nil, err
		}
		item.Threshold = trimNumber(item.Threshold)
		items = append(items, item)
	}
	return items, rows.Err()
}

// UpsertAlert saves one rule. Changing the threshold or switching the rule back on starts it afresh: it is checked
// again at the next pass and may speak at once. The change is written to the audit log in the same transaction.
func (store *AgentStore) UpsertAlert(ctx context.Context, principal agent.Principal, rule agent.AlertRuleKey, threshold string, enabled bool, requestID string, now time.Time) (agent.StoredAlert, error) {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return agent.StoredAlert{}, fmt.Errorf("begin alert save: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var beforeThreshold *string
	var beforeEnabled *bool
	err = tx.QueryRow(ctx, `
		select threshold::text, enabled from agent_alert_rules
		where tenant_id = $1 and recipient_id = $2 and rule_key = $3 for update`, principal.TenantID, principal.RecipientID, rule).Scan(&beforeThreshold, &beforeEnabled)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return agent.StoredAlert{}, fmt.Errorf("lock alert rule: %w", err)
	}
	var saved agent.StoredAlert
	err = tx.QueryRow(ctx, `
		insert into agent_alert_rules (tenant_id, recipient_id, rule_key, threshold, enabled, created_at, updated_at)
		values ($1, $2, $3, $4::numeric, $5, $6, $6)
		on conflict (tenant_id, recipient_id, rule_key) do update set
		  threshold = excluded.threshold, enabled = excluded.enabled, updated_at = excluded.updated_at,
		  last_checked_on = case when agent_alert_rules.threshold is distinct from excluded.threshold
		                          or (excluded.enabled and not agent_alert_rules.enabled) then null else agent_alert_rules.last_checked_on end,
		  last_fired_at = case when agent_alert_rules.threshold is distinct from excluded.threshold
		                        or (excluded.enabled and not agent_alert_rules.enabled) then null else agent_alert_rules.last_fired_at end,
		  last_fired_value = case when agent_alert_rules.threshold is distinct from excluded.threshold
		                           or (excluded.enabled and not agent_alert_rules.enabled) then null else agent_alert_rules.last_fired_value end
		returning rule_key, threshold::text, enabled, last_fired_at, coalesce(last_status, '')`,
		principal.TenantID, principal.RecipientID, rule, threshold, enabled, now).Scan(&saved.Rule, &saved.Threshold, &saved.Enabled, &saved.LastFiredAt, &saved.LastStatus)
	if err != nil {
		return agent.StoredAlert{}, fmt.Errorf("save alert rule: %w", err)
	}
	saved.Threshold = trimNumber(saved.Threshold)
	var before []byte
	if beforeThreshold != nil && beforeEnabled != nil {
		before, _ = json.Marshal(map[string]any{"threshold": trimNumber(*beforeThreshold), "enabled": *beforeEnabled})
	}
	after, _ := json.Marshal(map[string]any{"rule": string(rule), "threshold": saved.Threshold, "enabled": saved.Enabled, "via": "assistant"})
	if err := insertAuditAs(ctx, tx, principal.TenantID, "VIEWER", principal.TokenID[:], "AGENT_ALERT_SET", "LINE_RECIPIENT", principal.RecipientID.String(), requestID, before, after, now); err != nil {
		return agent.StoredAlert{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return agent.StoredAlert{}, fmt.Errorf("commit alert save: %w", err)
	}
	return saved, nil
}

// AlertTargets lists every enabled rule whose owner can still use the assistant: the token is live, the shop is active,
// the recipient and their membership are active and they have been given the assistant.
func (store *AgentStore) AlertTargets(ctx context.Context, now time.Time) ([]agent.AlertTarget, error) {
	rows, err := store.pool.Query(ctx, `
		select r.id, r.rule_key, r.threshold::text, coalesce(to_char(r.last_checked_on, 'YYYY-MM-DD'), ''), r.last_fired_at, r.last_fired_value::text,
		       a.id, a.tenant_id, a.recipient_id, a.names_visible, t.name, t.timezone
		from agent_alert_rules r
		join agent_tokens a on a.tenant_id = r.tenant_id and a.recipient_id = r.recipient_id and a.status = 'ACTIVE' and a.expires_at > $1
		join tenants t on t.id = r.tenant_id and t.status = 'ACTIVE' and t.access_ends_at > $1
		join line_recipients lr on lr.id = r.recipient_id and lr.status = 'ACTIVE'
		join tenant_memberships m on m.tenant_id = r.tenant_id and m.recipient_id = r.recipient_id and m.status = 'ACTIVE' and m.ai_chat_enabled
		where r.enabled order by r.created_at`, now)
	if err != nil {
		return nil, fmt.Errorf("list alert targets: %w", err)
	}
	defer rows.Close()
	targets := make([]agent.AlertTarget, 0)
	for rows.Next() {
		var target agent.AlertTarget
		var threshold string
		var firedValue *string
		if err := rows.Scan(&target.RuleID, &target.Rule, &threshold, &target.LastCheckedOn, &target.LastFiredAt, &firedValue,
			&target.Principal.TokenID, &target.Principal.TenantID, &target.Principal.RecipientID, &target.Principal.NamesVisible, &target.Principal.ShopName, &target.Principal.Timezone); err != nil {
			return nil, err
		}
		var ok bool
		if target.Threshold, ok = new(big.Rat).SetString(threshold); !ok {
			continue
		}
		if firedValue != nil {
			target.LastFiredValue, _ = new(big.Rat).SetString(*firedValue)
		}
		targets = append(targets, target)
	}
	return targets, rows.Err()
}

func (store *AgentStore) MarkAlertChecked(ctx context.Context, ruleID uuid.UUID, on string, status string) error {
	_, err := store.pool.Exec(ctx, `
		update agent_alert_rules set last_status = $3, last_checked_on = case when $3 in ('OK', 'NO_ACCESS') then $2::date else last_checked_on end
		where id = $1`, ruleID, on, status)
	return err
}

// RecordAlertFire stores that a rule spoke today. A second record for the same rule and day is refused (created=false),
// which is what keeps a restarted worker from sending the same alert twice.
func (store *AgentStore) RecordAlertFire(ctx context.Context, target agent.AlertTarget, on string, value, threshold, message, status string, now time.Time) (agent.AlertEvent, bool, error) {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return agent.AlertEvent{}, false, fmt.Errorf("begin alert fire: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	event := agent.AlertEvent{RuleID: target.RuleID, TenantID: target.Principal.TenantID, Recipient: target.Principal.RecipientID, Rule: target.Rule, FiredOn: on, Message: message, Status: status}
	err = tx.QueryRow(ctx, `
		insert into agent_alert_events (rule_id, tenant_id, recipient_id, rule_key, fired_on, value, threshold, message, status, created_at, expires_at)
		values ($1, $2, $3, $4, $5::date, $6::numeric, $7::numeric, $8, $9, $10::timestamptz, $10::timestamptz + interval '365 days')
		on conflict (rule_id, fired_on) do nothing returning id`,
		target.RuleID, event.TenantID, event.Recipient, target.Rule, on, value, threshold, message, status, now).Scan(&event.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return agent.AlertEvent{}, false, nil
	}
	if err != nil {
		return agent.AlertEvent{}, false, fmt.Errorf("insert alert event: %w", err)
	}
	if _, err := tx.Exec(ctx, `update agent_alert_rules set last_fired_at = $2, last_fired_value = $3::numeric where id = $1`, target.RuleID, now, value); err != nil {
		return agent.AlertEvent{}, false, fmt.Errorf("remember alert fire: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return agent.AlertEvent{}, false, fmt.Errorf("commit alert fire: %w", err)
	}
	return event, true, nil
}

// UnsentAlerts are events still to be delivered: new ones and ones that failed, up to five tries within six hours.
func (store *AgentStore) UnsentAlerts(ctx context.Context, now time.Time) ([]agent.AlertEvent, error) {
	rows, err := store.pool.Query(ctx, `
		select id, rule_id, tenant_id, recipient_id, rule_key, to_char(fired_on, 'YYYY-MM-DD'), message, status, attempts
		from agent_alert_events
		where status in ('PENDING', 'FAILED') and attempts < 5 and created_at > $1::timestamptz - interval '6 hours'
		order by created_at limit 50`, now)
	if err != nil {
		return nil, fmt.Errorf("list unsent alerts: %w", err)
	}
	defer rows.Close()
	events := make([]agent.AlertEvent, 0)
	for rows.Next() {
		var event agent.AlertEvent
		if err := rows.Scan(&event.ID, &event.RuleID, &event.TenantID, &event.Recipient, &event.Rule, &event.FiredOn, &event.Message, &event.Status, &event.Attempts); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

// MarkAlertDelivery records one delivery try. errorCode is a short code, never the response or the message.
func (store *AgentStore) MarkAlertDelivery(ctx context.Context, id uuid.UUID, delivered bool, errorCode string, now time.Time) error {
	if delivered {
		_, err := store.pool.Exec(ctx, `update agent_alert_events set status = 'SENT', attempts = attempts + 1, sent_at = $2, last_error = null where id = $1`, id, now)
		return err
	}
	_, err := store.pool.Exec(ctx, `update agent_alert_events set status = 'FAILED', attempts = attempts + 1, last_error = $2 where id = $1`, id, errorCode)
	return err
}
