package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/keywatch"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// KeyWatchStore lists the shops whose assistant is switched on (their model key is in use) and remembers whom the operator was told about.
type KeyWatchStore struct{ pool *pgxpool.Pool }

func NewKeyWatchStore(pool *pgxpool.Pool) *KeyWatchStore { return &KeyWatchStore{pool: pool} }

func (store *KeyWatchStore) Shops(ctx context.Context) ([]keywatch.Shop, error) {
	rows, err := store.pool.Query(ctx, `
		select t.id, t.name from tenant_assistant_settings s
		join tenants t on t.id = s.tenant_id and t.status = 'ACTIVE' and t.access_ends_at > now()
		where s.enabled order by t.name`)
	if err != nil {
		return nil, fmt.Errorf("list shops with an assistant: %w", err)
	}
	defer rows.Close()
	shops := make([]keywatch.Shop, 0)
	for rows.Next() {
		var shop keywatch.Shop
		if err := rows.Scan(&shop.TenantID, &shop.Name); err != nil {
			return nil, err
		}
		shops = append(shops, shop)
	}
	return shops, rows.Err()
}

func (store *KeyWatchStore) LastNotice(ctx context.Context, tenantID uuid.UUID, level string) (time.Time, bool, error) {
	var at time.Time
	err := store.pool.QueryRow(ctx, `select notified_at from assistant_key_notices where tenant_id = $1 and level = $2`, tenantID, level).Scan(&at)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("read model key notice: %w", err)
	}
	return at, true, nil
}

func (store *KeyWatchStore) MarkNotice(ctx context.Context, tenantID uuid.UUID, level string, now time.Time) error {
	if _, err := store.pool.Exec(ctx, `
		insert into assistant_key_notices (tenant_id, level, notified_at) values ($1, $2, $3)
		on conflict (tenant_id, level) do update set notified_at = excluded.notified_at`, tenantID, level, now); err != nil {
		return fmt.Errorf("remember model key notice: %w", err)
	}
	return nil
}

func (store *KeyWatchStore) ClearNotices(ctx context.Context, tenantID uuid.UUID) error {
	if _, err := store.pool.Exec(ctx, `delete from assistant_key_notices where tenant_id = $1`, tenantID); err != nil {
		return fmt.Errorf("clear model key notices: %w", err)
	}
	return nil
}
