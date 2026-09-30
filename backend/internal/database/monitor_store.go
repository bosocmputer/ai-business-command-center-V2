package database

import (
	"context"
	"encoding/json"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/monitor"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MonitorStore struct{ pool *pgxpool.Pool }

func NewMonitorStore(pool *pgxpool.Pool) *MonitorStore { return &MonitorStore{pool: pool} }

func (s *MonitorStore) Insert(ctx context.Context, sample monitor.Sample) error {
	payload, err := json.Marshal(sample)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		insert into monitor_samples (sampled_at, payload) values ($1, $2)
		on conflict (sampled_at) do nothing`, sample.At, payload)
	return err
}

func (s *MonitorStore) ListSince(ctx context.Context, since time.Time) ([]monitor.Sample, error) {
	rows, err := s.pool.Query(ctx, `select payload from monitor_samples where sampled_at >= $1 order by sampled_at`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	samples := []monitor.Sample{}
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var sample monitor.Sample
		if err := json.Unmarshal(payload, &sample); err != nil {
			continue
		}
		samples = append(samples, sample)
	}
	return samples, rows.Err()
}

func (s *MonitorStore) Prune(ctx context.Context, before time.Time) error {
	_, err := s.pool.Exec(ctx, `delete from monitor_samples where sampled_at < $1`, before)
	return err
}
