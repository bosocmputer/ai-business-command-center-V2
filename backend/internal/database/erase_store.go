package database

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EraseStore removes a shop, or one person, from the system on the operator's request (deploy/erase.sh). A plan only counts; an erase runs in one
// transaction, checks that nothing of the shop is left, and writes an audit record that holds counts and no personal data. What it cannot reach is
// reported by the command: the assistant's own memory (assistant/forget.sh) and the database backups, which age out on their own.
type EraseStore struct{ pool *pgxpool.Pool }

func NewEraseStore(pool *pgxpool.Pool) *EraseStore { return &EraseStore{pool: pool} }

var (
	ErrEraseNotFound   = errors.New("nothing with that id")
	ErrEraseShopActive = errors.New("the shop is still active: switch it off in the admin page first")
	ErrEraseHolder     = errors.New("the person holds a shop's assistant: use the holder flag, the assistant will then have no holder")
	ErrEraseLeftovers  = errors.New("something of the shop would be left: nothing was deleted")
)

type TableCount struct {
	Table string
	Rows  int64
}

type ShopPlan struct {
	TenantID uuid.UUID
	Status   string
	Tables   []TableCount // tables holding rows of the shop (counts only)
	People   []uuid.UUID  // people who belong to no other shop and would keep their LINE ID after the shop goes
}

type PersonPlan struct {
	RecipientID     uuid.UUID
	Status          string
	Tables          []TableCount
	HoldsAssistants int64
}

type ShopResult struct {
	Deleted    []TableCount
	PeopleDone int
}

func tenantTables(ctx context.Context, tx pgx.Tx) ([]string, error) {
	rows, err := tx.Query(ctx, `select table_name from information_schema.columns where table_schema = 'public' and column_name = 'tenant_id' order by table_name`)
	if err != nil {
		return nil, fmt.Errorf("list tables of a shop: %w", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

func countRows(ctx context.Context, tx pgx.Tx, table, column string, id uuid.UUID) (int64, error) {
	var count int64
	query := fmt.Sprintf("select count(*) from %s where %s = $1", pgx.Identifier{table}.Sanitize(), pgx.Identifier{column}.Sanitize())
	if err := tx.QueryRow(ctx, query, id).Scan(&count); err != nil {
		return 0, fmt.Errorf("count rows: %w", err)
	}
	return count, nil
}

func shopCounts(ctx context.Context, tx pgx.Tx, tenantID uuid.UUID) ([]TableCount, error) {
	tables, err := tenantTables(ctx, tx)
	if err != nil {
		return nil, err
	}
	var counts []TableCount
	for _, table := range tables {
		count, err := countRows(ctx, tx, table, "tenant_id", tenantID)
		if err != nil {
			return nil, err
		}
		if count > 0 {
			counts = append(counts, TableCount{Table: table, Rows: count})
		}
	}
	return counts, nil
}

const peopleOfShopOnly = `
	select m.recipient_id from tenant_memberships m
	where m.tenant_id = $1
	  and not exists (select 1 from tenant_memberships other where other.recipient_id = m.recipient_id and other.tenant_id <> $1 and other.status <> 'REVOKED')
	order by m.recipient_id`

func peopleLeft(ctx context.Context, tx pgx.Tx, tenantID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := tx.Query(ctx, peopleOfShopOnly, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list people of the shop: %w", err)
	}
	defer rows.Close()
	var people []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		people = append(people, id)
	}
	return people, rows.Err()
}

func shopStatus(ctx context.Context, tx pgx.Tx, tenantID uuid.UUID) (string, error) {
	var status string
	err := tx.QueryRow(ctx, `select status from tenants where id = $1 for update`, tenantID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrEraseNotFound
	}
	if err != nil {
		return "", fmt.Errorf("read shop: %w", err)
	}
	return status, nil
}

// PlanShop counts what erasing the shop would remove. It changes nothing.
func (store *EraseStore) PlanShop(ctx context.Context, tenantID uuid.UUID) (ShopPlan, error) {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return ShopPlan{}, fmt.Errorf("begin plan: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	status, err := shopStatus(ctx, tx, tenantID)
	if err != nil {
		return ShopPlan{}, err
	}
	plan := ShopPlan{TenantID: tenantID, Status: status}
	if plan.Tables, err = shopCounts(ctx, tx, tenantID); err != nil {
		return ShopPlan{}, err
	}
	if plan.People, err = peopleLeft(ctx, tx, tenantID); err != nil {
		return ShopPlan{}, err
	}
	return plan, nil
}

// EraseShop deletes the shop (the database cascades its rows). With people it also erases the LINE ID of everyone who belonged to this shop only.
// A shop that is still active is refused. After the delete every table is checked for leftovers; if any remain the whole transaction is undone.
func (store *EraseStore) EraseShop(ctx context.Context, tenantID uuid.UUID, withPeople bool, now time.Time) (ShopResult, error) {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return ShopResult{}, fmt.Errorf("begin erase: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	status, err := shopStatus(ctx, tx, tenantID)
	if err != nil {
		return ShopResult{}, err
	}
	if status == "ACTIVE" {
		return ShopResult{}, ErrEraseShopActive
	}
	before, err := shopCounts(ctx, tx, tenantID)
	if err != nil {
		return ShopResult{}, err
	}
	people, err := peopleLeft(ctx, tx, tenantID)
	if err != nil {
		return ShopResult{}, err
	}
	if _, err := tx.Exec(ctx, `delete from tenants where id = $1`, tenantID); err != nil {
		return ShopResult{}, fmt.Errorf("delete shop: %w", err)
	}
	left, err := shopCounts(ctx, tx, tenantID)
	if err != nil {
		return ShopResult{}, err
	}
	if len(left) > 0 {
		return ShopResult{}, ErrEraseLeftovers
	}
	result := ShopResult{Deleted: before}
	if withPeople {
		for _, person := range people {
			if _, err := erasePerson(ctx, tx, person, true); err != nil {
				return ShopResult{}, err
			}
			result.PeopleDone++
		}
	}
	if err := writeEraseAudit(ctx, tx, "TENANT_ERASED", "TENANT", tenantID, map[string]any{"tables": tableMap(before), "peopleErased": result.PeopleDone, "peopleLeftWithLineId": len(people) - result.PeopleDone}, now); err != nil {
		return ShopResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ShopResult{}, fmt.Errorf("commit erase: %w", err)
	}
	return result, nil
}

// Tables with a recipient_id that hold what a person did or was given. The rows that only point at the person (a delivery, a report run) stay: once the
// person's name and LINE ID are gone they identify nobody.
var personTables = []string{
	"agent_calls", "agent_alert_events", "agent_alert_rules", "agent_tokens", "owner_weekly_summaries", "owner_inactivity_notices",
	"report_view_events", "viewer_sessions", "delivery_access_links", "line_chat_selection", "recipient_report_permissions",
}

func personCounts(ctx context.Context, tx pgx.Tx, recipientID uuid.UUID) ([]TableCount, error) {
	var counts []TableCount
	for _, table := range personTables {
		count, err := countRows(ctx, tx, table, "recipient_id", recipientID)
		if err != nil {
			return nil, err
		}
		if count > 0 {
			counts = append(counts, TableCount{Table: table, Rows: count})
		}
	}
	return counts, nil
}

func holdsAssistants(ctx context.Context, tx pgx.Tx, recipientID uuid.UUID) (int64, error) {
	return countRows(ctx, tx, "assistant_instances", "recipient_id", recipientID)
}

// PlanPerson counts what erasing the person would remove. It changes nothing.
func (store *EraseStore) PlanPerson(ctx context.Context, recipientID uuid.UUID) (PersonPlan, error) {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return PersonPlan{}, fmt.Errorf("begin plan: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	plan := PersonPlan{RecipientID: recipientID}
	err = tx.QueryRow(ctx, `select status from line_recipients where id = $1`, recipientID).Scan(&plan.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return PersonPlan{}, ErrEraseNotFound
	}
	if err != nil {
		return PersonPlan{}, fmt.Errorf("read person: %w", err)
	}
	if plan.Tables, err = personCounts(ctx, tx, recipientID); err != nil {
		return PersonPlan{}, err
	}
	if plan.HoldsAssistants, err = holdsAssistants(ctx, tx, recipientID); err != nil {
		return PersonPlan{}, err
	}
	return plan, nil
}

// ErasePerson removes what the person did and was given, ends their memberships and wipes their name and LINE ID, keeping the (now empty) row so that
// delivery history still points at something. A person who holds a shop's assistant is refused unless allowed.
func (store *EraseStore) ErasePerson(ctx context.Context, recipientID uuid.UUID, allowHolder bool, now time.Time) ([]TableCount, error) {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin erase: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var status string
	err = tx.QueryRow(ctx, `select status from line_recipients where id = $1 for update`, recipientID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrEraseNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read person: %w", err)
	}
	counts, err := erasePerson(ctx, tx, recipientID, allowHolder)
	if err != nil {
		return nil, err
	}
	if err := writeEraseAudit(ctx, tx, "RECIPIENT_ERASED", "LINE_RECIPIENT", recipientID, map[string]any{"tables": tableMap(counts)}, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit erase: %w", err)
	}
	return counts, nil
}

func erasePerson(ctx context.Context, tx pgx.Tx, recipientID uuid.UUID, allowHolder bool) ([]TableCount, error) {
	held, err := holdsAssistants(ctx, tx, recipientID)
	if err != nil {
		return nil, err
	}
	if held > 0 && !allowHolder {
		return nil, ErrEraseHolder
	}
	counts, err := personCounts(ctx, tx, recipientID)
	if err != nil {
		return nil, err
	}
	for _, table := range personTables {
		query := fmt.Sprintf("delete from %s where recipient_id = $1", pgx.Identifier{table}.Sanitize())
		if _, err := tx.Exec(ctx, query, recipientID); err != nil {
			return nil, fmt.Errorf("erase %s: %w", table, err)
		}
	}
	for _, statement := range []string{
		`delete from recipient_invitations where pending_recipient_id = $1`,
		`update assistant_instances set recipient_id = null, updated_at = now() where recipient_id = $1`,
		`update tenant_memberships set status = 'REVOKED', ai_chat_enabled = false, updated_at = now() where recipient_id = $1 and (status <> 'REVOKED' or ai_chat_enabled)`,
		`update line_recipients set line_user_id_hash = null, line_user_id_ciphertext = null, line_user_id_nonce = null,
		   display_name_ciphertext = null, display_name_nonce = null, line_follow_status = null, line_follow_changed_at = null, status = 'REVOKED', updated_at = now()
		 where id = $1`,
	} {
		if _, err := tx.Exec(ctx, statement, recipientID); err != nil {
			return nil, fmt.Errorf("erase person: %w", err)
		}
	}
	return counts, nil
}

func tableMap(counts []TableCount) map[string]int64 {
	result := make(map[string]int64, len(counts))
	for _, item := range counts {
		result[item.Table] = item.Rows
	}
	return result
}

// writeEraseAudit records that an erase happened, with counts only: no name, no LINE ID, no figure.
func writeEraseAudit(ctx context.Context, tx pgx.Tx, action, resourceType string, id uuid.UUID, detail map[string]any, now time.Time) error {
	payload, err := json.Marshal(detail)
	if err != nil {
		return fmt.Errorf("encode erase audit: %w", err)
	}
	actor := sha256.Sum256([]byte("erase-tool"))
	if _, err := tx.Exec(ctx, `
		insert into audit_logs (tenant_id, actor_type, actor_id_hash, action, resource_type, resource_id, request_id, after_json, result, created_at, expires_at)
		values (null, 'SYSTEM', $1, $2, $3, $4, 'erase-tool', $5, 'SUCCESS', $6, $7)`,
		actor[:], action, resourceType, id.String(), payload, now, now.AddDate(1, 0, 0)); err != nil {
		return fmt.Errorf("write erase audit: %w", err)
	}
	return nil
}

// SortedCounts is for printing: biggest first, then by name.
func SortedCounts(counts []TableCount) []TableCount {
	sorted := append([]TableCount(nil), counts...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Rows != sorted[j].Rows {
			return sorted[i].Rows > sorted[j].Rows
		}
		return sorted[i].Table < sorted[j].Table
	})
	return sorted
}
