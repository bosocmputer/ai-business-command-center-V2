package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/agent"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/master"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MasterStore keeps the read-only copy of a shop's customers, suppliers and items.
type MasterStore struct{ pool *pgxpool.Pool }

func NewMasterStore(pool *pgxpool.Pool) *MasterStore { return &MasterStore{pool: pool} }

// Targets lists the shops whose assistant is switched on and whose SML works, with the kinds that are due: no good copy
// yet today (shop time) and no try in the last half hour.
func (store *MasterStore) Targets(ctx context.Context, now time.Time) ([]master.Target, error) {
	rows, err := store.pool.Query(ctx, `
		select t.id, t.timezone, k.kind
		from tenants t
		join tenant_sml_connections c on c.tenant_id = t.id and c.readiness_status = 'READY'
		cross join (values ('CUSTOMER'), ('SUPPLIER'), ('ITEM')) as k(kind)
		left join master_sync s on s.tenant_id = t.id and s.kind = k.kind
		where t.status = 'ACTIVE' and t.access_ends_at > $1
		  and exists (select 1 from agent_tokens a where a.tenant_id = t.id and a.status = 'ACTIVE' and a.expires_at > $1)
		  and (s.synced_at is null or (s.synced_at at time zone t.timezone)::date < ($1::timestamptz at time zone t.timezone)::date)
		  and (s.attempted_at is null or s.attempted_at < $1::timestamptz - interval '30 minutes')
		order by t.id, k.kind`, now)
	if err != nil {
		return nil, fmt.Errorf("list master copy targets: %w", err)
	}
	defer rows.Close()
	var targets []master.Target
	for rows.Next() {
		var id uuid.UUID
		var timezone, kind string
		if err := rows.Scan(&id, &timezone, &kind); err != nil {
			return nil, err
		}
		if len(targets) == 0 || targets[len(targets)-1].TenantID != id {
			targets = append(targets, master.Target{TenantID: id, Timezone: timezone})
		}
		last := &targets[len(targets)-1]
		last.Due = append(last.Due, master.Kind(kind))
	}
	return targets, rows.Err()
}

const masterChunk = 1000

// Replace swaps one kind's copy for the new rows in a single transaction: rows are upserted with this run's time and
// every older row of the kind is removed. An empty result never replaces an existing copy.
func (store *MasterStore) Replace(ctx context.Context, tenantID uuid.UUID, kind master.Kind, records []master.Record, now time.Time) error {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin master copy: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if len(records) == 0 {
		var existing int
		if err := tx.QueryRow(ctx, `select count(*) from master_items where tenant_id = $1 and kind = $2`, tenantID, string(kind)).Scan(&existing); err != nil {
			return fmt.Errorf("count master copy: %w", err)
		}
		if existing > 0 {
			return master.ErrEmpty
		}
	}
	for start := 0; start < len(records); start += masterChunk {
		end := min(start+masterChunk, len(records))
		codes, names, phones, units, suppliers, actives := make([]string, 0, end-start), make([]string, 0, end-start), make([]string, 0, end-start), make([]string, 0, end-start), make([]string, 0, end-start), make([]bool, 0, end-start)
		for _, record := range records[start:end] {
			codes, names, phones, units, suppliers, actives = append(codes, record.Code), append(names, record.Name), append(phones, record.Phone), append(units, record.Unit), append(suppliers, record.SupplierCode), append(actives, record.Active)
		}
		if _, err := tx.Exec(ctx, `
			insert into master_items (tenant_id, kind, code, name, phone, unit, supplier_code, active, synced_at)
			select $1, $2, c, n, p, u, s, a, $8::timestamptz
			from unnest($3::text[], $4::text[], $5::text[], $6::text[], $7::text[], $9::boolean[]) as r(c, n, p, u, s, a)
			on conflict (tenant_id, kind, code) do update set
			  name = excluded.name, phone = excluded.phone, unit = excluded.unit, supplier_code = excluded.supplier_code,
			  active = excluded.active, synced_at = excluded.synced_at`,
			tenantID, string(kind), codes, names, phones, units, suppliers, now, actives); err != nil {
			return fmt.Errorf("write master copy: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `delete from master_items where tenant_id = $1 and kind = $2 and synced_at < $3::timestamptz`, tenantID, string(kind), now); err != nil {
		return fmt.Errorf("remove old master rows: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		insert into master_sync (tenant_id, kind, synced_at, attempted_at, status, row_count, error_code)
		values ($1, $2, $3::timestamptz, $3::timestamptz, 'OK', $4, null)
		on conflict (tenant_id, kind) do update set synced_at = excluded.synced_at, attempted_at = excluded.attempted_at, status = 'OK', row_count = excluded.row_count, error_code = null`,
		tenantID, string(kind), now, len(records)); err != nil {
		return fmt.Errorf("record master copy: %w", err)
	}
	return tx.Commit(ctx)
}

// MarkFailed records a failed try with a short code. The last good copy and its time stay as they were.
func (store *MasterStore) MarkFailed(ctx context.Context, tenantID uuid.UUID, kind master.Kind, code string, now time.Time) error {
	_, err := store.pool.Exec(ctx, `
		insert into master_sync (tenant_id, kind, synced_at, attempted_at, status, row_count, error_code)
		values ($1, $2, null, $3::timestamptz, 'ERROR', 0, left($4, 60))
		on conflict (tenant_id, kind) do update set attempted_at = excluded.attempted_at, status = 'ERROR', error_code = excluded.error_code`,
		tenantID, string(kind), now, code)
	return err
}

// SearchMaster finds records whose name, code or phone number contains every word of the query. Exact code first, then
// names that start with the first word, then the rest by name. It reads one shop's copy only.
func (store *MasterStore) SearchMaster(ctx context.Context, tenantID uuid.UUID, kind agent.MasterKind, words []string, limit int) (agent.MasterResult, error) {
	var result agent.MasterResult
	var syncedAt *time.Time
	if err := store.pool.QueryRow(ctx, `select synced_at from master_sync where tenant_id = $1 and kind = $2`, tenantID, string(kind)).Scan(&syncedAt); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return result, fmt.Errorf("read master copy state: %w", err)
	}
	result.SyncedAt = syncedAt
	if syncedAt == nil {
		return result, nil
	}
	lower := make([]string, len(words))
	for index, word := range words {
		lower[index] = strings.ToLower(word)
	}
	rows, err := store.pool.Query(ctx, `
		with matched as (
		  select code, name, phone, unit, supplier_code
		  from master_items m
		  where m.tenant_id = $1 and m.kind = $2 and m.active
		    and not exists (
		      select 1 from unnest($3::text[]) as w(word)
		      where strpos(lower(m.name), w.word) = 0 and strpos(lower(m.code), w.word) = 0
		        and not (char_length(regexp_replace(w.word, '\D', '', 'g')) >= 4 and strpos(regexp_replace(m.phone, '\D', '', 'g'), regexp_replace(w.word, '\D', '', 'g')) > 0)
		    )
		)
		select code, name, phone, unit, supplier_code, count(*) over () as total
		from matched
		order by (lower(code) = $4) desc, (strpos(lower(name), $4) = 1) desc, name, code
		limit $5`, tenantID, string(kind), lower, lower[0], limit)
	if err != nil {
		return result, fmt.Errorf("search master copy: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var match agent.MasterMatch
		if err := rows.Scan(&match.Code, &match.Name, &match.Phone, &match.Unit, &match.SupplierCode, &result.Total); err != nil {
			return result, err
		}
		result.Matches = append(result.Matches, match)
	}
	return result, rows.Err()
}

// MasterRecord finds one record by its exact code (inactive ones too: an old customer can still owe money).
func (store *MasterStore) MasterRecord(ctx context.Context, tenantID uuid.UUID, kind agent.MasterKind, code string) (agent.MasterMatch, bool, error) {
	var match agent.MasterMatch
	err := store.pool.QueryRow(ctx, `select code, name, phone, unit, supplier_code from master_items where tenant_id = $1 and kind = $2 and code = $3`,
		tenantID, string(kind), code).Scan(&match.Code, &match.Name, &match.Phone, &match.Unit, &match.SupplierCode)
	if errors.Is(err, pgx.ErrNoRows) {
		return agent.MasterMatch{}, false, nil
	}
	if err != nil {
		return agent.MasterMatch{}, false, fmt.Errorf("read master record: %w", err)
	}
	return match, true, nil
}
