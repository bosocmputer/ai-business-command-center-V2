package monitor

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeSampler struct {
	samples []Sample
	err     error
}

func (f *fakeSampler) Collect() (Sample, error) {
	if f.err != nil {
		return Sample{}, f.err
	}
	sample := f.samples[0]
	f.samples = f.samples[1:]
	return sample, nil
}

type fakeStore struct {
	inserted []Sample
	pruned   []time.Time
	listed   []Sample
	since    time.Time
}

func (f *fakeStore) Insert(_ context.Context, sample Sample) error {
	f.inserted = append(f.inserted, sample)
	return nil
}
func (f *fakeStore) ListSince(_ context.Context, since time.Time) ([]Sample, error) {
	f.since = since
	return f.listed, nil
}
func (f *fakeStore) Prune(_ context.Context, before time.Time) error {
	f.pruned = append(f.pruned, before)
	return nil
}

func TestServiceStoresEveryThirtySecondsAndPrunesHourly(t *testing.T) {
	base := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	var samples []Sample
	for index := 0; index < 8; index++ {
		samples = append(samples, Sample{At: base.Add(time.Duration(index) * 5 * time.Second)})
	}
	store := &fakeStore{}
	service := NewService(&fakeSampler{samples: samples}, store, time.Now, nil)
	var persist, prune time.Time
	for range samples {
		service.tick(context.Background(), &persist, &prune)
	}
	if len(store.inserted) != 2 {
		t.Fatalf("inserted %d samples in 35s, want 2 (first and +30s)", len(store.inserted))
	}
	if len(store.pruned) != 1 || !store.pruned[0].Equal(base.Add(-retention)) {
		t.Fatalf("pruned = %v", store.pruned)
	}
	if current, ok := service.Current(); !ok || !current.At.Equal(base.Add(35*time.Second)) {
		t.Fatalf("current = %+v, %v", current, ok)
	}
}

func TestServiceKeepsLastSampleWhenCollectFails(t *testing.T) {
	service := NewService(&fakeSampler{err: errors.New("no proc")}, &fakeStore{}, time.Now, nil)
	var persist, prune time.Time
	service.tick(context.Background(), &persist, &prune)
	if _, ok := service.Current(); ok {
		t.Fatal("a failed collect must not publish a sample")
	}
}

func TestHistoryValidatesRangeAndAveragesBuckets(t *testing.T) {
	base := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	store := &fakeStore{}
	for index := 0; index < 576; index++ {
		store.listed = append(store.listed, Sample{
			At:         base.Add(time.Duration(index) * 30 * time.Second),
			Host:       HostStats{CPUPercent: float64(index % 2 * 40), MemoryTotalBytes: 1000, MemoryAvailableBytes: 750, Load1: 1},
			Containers: []ContainerStats{{Name: "api", CPUPercent: 10, MemoryUsedBytes: 100, MemoryLimitBytes: 500}},
		})
	}
	now := base.Add(5 * time.Hour)
	service := NewService(&fakeSampler{}, store, func() time.Time { return now }, nil)

	for _, minutes := range []int{0, 4, MaxHistoryMinutes + 1} {
		if _, err := service.History(context.Background(), minutes); !errors.Is(err, ErrInvalidRange) {
			t.Fatalf("History(%d) error = %v, want ErrInvalidRange", minutes, err)
		}
	}
	points, err := service.History(context.Background(), 300)
	if err != nil {
		t.Fatal(err)
	}
	if !store.since.Equal(now.Add(-300 * time.Minute)) {
		t.Fatalf("since = %v", store.since)
	}
	if len(points) > maxHistoryPoints || len(points) < 200 {
		t.Fatalf("%d points, want between 200 and %d", len(points), maxHistoryPoints)
	}
	first := points[0]
	if first.CPUPercent != 20 || first.MemoryUsedPercent != 25 || first.Load1 != 1 {
		t.Fatalf("bucket average = %+v; alternating 0/40 CPU must average 20 and memory 25%%", first)
	}
	if len(first.Containers) != 1 || first.Containers[0].MemoryUsedBytes != 100 {
		t.Fatalf("container average = %+v", first.Containers)
	}
}
