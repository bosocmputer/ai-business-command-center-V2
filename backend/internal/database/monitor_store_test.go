package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/monitor"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMonitorStoreKeepsOrderedHistoryAndPrunesOldRows(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	if _, err := pool.Exec(ctx, `delete from monitor_samples`); err != nil {
		t.Fatal(err)
	}
	store := NewMonitorStore(pool)
	base := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	for index, at := range []time.Time{base.Add(2 * time.Minute), base, base.Add(time.Minute)} {
		sample := monitor.Sample{At: at, Host: monitor.HostStats{Cores: 4, CPUPercent: float64(index)}, ContainersAvailable: true,
			Containers: []monitor.ContainerStats{{Name: "api", MemoryUsedBytes: 1 << 20}}}
		if err := store.Insert(ctx, sample); err != nil {
			t.Fatal(err)
		}
	}
	// Writing the same instant twice must not fail or duplicate.
	if err := store.Insert(ctx, monitor.Sample{At: base}); err != nil {
		t.Fatalf("duplicate insert error = %v", err)
	}

	got, err := store.ListSince(ctx, base)
	if err != nil || len(got) != 3 {
		t.Fatalf("list = %d rows, %v", len(got), err)
	}
	if !got[0].At.Equal(base) || !got[2].At.Equal(base.Add(2*time.Minute)) || got[0].Host.CPUPercent != 1 || got[0].Containers[0].Name != "api" {
		t.Fatalf("rows must come back ordered and intact: %+v", got)
	}
	if got, _ = store.ListSince(ctx, base.Add(90*time.Second)); len(got) != 1 {
		t.Fatalf("since filter returned %d rows, want 1", len(got))
	}

	if err := store.Prune(ctx, base.Add(30*time.Second)); err != nil {
		t.Fatal(err)
	}
	if got, _ = store.ListSince(ctx, base.Add(-time.Hour)); len(got) != 2 {
		t.Fatalf("after prune %d rows remain, want 2", len(got))
	}
}
