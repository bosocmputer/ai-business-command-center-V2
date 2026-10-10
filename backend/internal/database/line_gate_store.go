package database

import (
	"context"
	"fmt"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/linegate"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LineGateStore answers the LINE front gate: who the sender is, which shops they may ask about, and which one they chose.
type LineGateStore struct{ pool *pgxpool.Pool }

func NewLineGateStore(pool *pgxpool.Pool) *LineGateStore { return &LineGateStore{pool: pool} }

const lineGateShopsSQL = `
	select r.id, t.id, t.name, s.assistant_host
	from line_recipients r
	join tenant_memberships m on m.recipient_id = r.id and m.status = 'ACTIVE' and m.ai_chat_enabled
	join tenants t on t.id = m.tenant_id and t.status = 'ACTIVE' and t.access_ends_at > $2
	join tenant_assistant_settings s on s.tenant_id = t.id and s.enabled and s.line_mode = 'CENTRAL' and s.assistant_host <> ''
	where r.line_user_id_hash = $1 and r.status = 'ACTIVE'
	order by t.name, t.id`

// Shops lists the shops the sender may ask about right now. A sender who is nobody's recipient gets an empty list, the same as
// one whose shops are all switched off or expired.
func (store *LineGateStore) Shops(ctx context.Context, lineHash []byte, now time.Time) (uuid.UUID, []linegate.Shop, error) {
	rows, err := store.pool.Query(ctx, lineGateShopsSQL, lineHash, now)
	if err != nil {
		return uuid.Nil, nil, fmt.Errorf("list LINE gate shops: %w", err)
	}
	defer rows.Close()
	var recipientID uuid.UUID
	var shops []linegate.Shop
	for rows.Next() {
		var shop linegate.Shop
		if err := rows.Scan(&recipientID, &shop.TenantID, &shop.Name, &shop.Host); err != nil {
			return uuid.Nil, nil, fmt.Errorf("scan LINE gate shop: %w", err)
		}
		shops = append(shops, shop)
	}
	return recipientID, shops, rows.Err()
}

func (store *LineGateStore) Selected(ctx context.Context, recipientID uuid.UUID) (uuid.UUID, bool, error) {
	var tenantID uuid.UUID
	err := store.pool.QueryRow(ctx, `select tenant_id from line_chat_selection where recipient_id = $1`, recipientID).Scan(&tenantID)
	if err == pgx.ErrNoRows {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("read LINE gate selection: %w", err)
	}
	return tenantID, true, nil
}

func (store *LineGateStore) Select(ctx context.Context, recipientID, tenantID uuid.UUID, now time.Time) error {
	_, err := store.pool.Exec(ctx, `
		insert into line_chat_selection (recipient_id, tenant_id, selected_at) values ($1, $2, $3)
		on conflict (recipient_id) do update set tenant_id = excluded.tenant_id, selected_at = excluded.selected_at`, recipientID, tenantID, now)
	if err != nil {
		return fmt.Errorf("keep LINE gate selection: %w", err)
	}
	return nil
}
