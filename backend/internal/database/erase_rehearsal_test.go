package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestEraseRehearsalOnACopy runs a whole-shop erase against a restored COPY of the production database (never the real one) to prove that the
// real data shape does not block the delete. It runs only when ERASE_REHEARSAL_TENANT is set; it logs counts and never a name.
func TestEraseRehearsalOnACopy(t *testing.T) {
	databaseURL, tenant := os.Getenv("TEST_DATABASE_URL"), os.Getenv("ERASE_REHEARSAL_TENANT")
	if databaseURL == "" || tenant == "" {
		t.Skip("rehearsal only: needs TEST_DATABASE_URL (a copy) and ERASE_REHEARSAL_TENANT")
	}
	tenantID, err := uuid.Parse(tenant)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	store := NewEraseStore(pool)
	plan, err := store.PlanShop(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range SortedCounts(plan.Tables) {
		t.Logf("plan %-32s %d", item.Table, item.Rows)
	}
	t.Logf("status %s, people only here: %d", plan.Status, len(plan.People))
	if _, err := pool.Exec(ctx, `update tenants set status = 'DISABLED' where id = $1`, tenantID); err != nil {
		t.Fatal(err)
	}
	result, err := store.EraseShop(ctx, tenantID, true, time.Now().UTC())
	if err != nil {
		t.Fatalf("the erase failed on the copy: %v", err)
	}
	t.Logf("erased: %d tables, %d people wiped", len(result.Deleted), result.PeopleDone)
	var left int
	if err := pool.QueryRow(ctx, `select count(*) from tenants where id = $1`, tenantID).Scan(&left); err != nil || left != 0 {
		t.Fatalf("the shop is still there: %d %v", left, err)
	}
}
