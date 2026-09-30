package monitor

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

const (
	sampleInterval  = 5 * time.Second
	persistInterval = 30 * time.Second
	retention       = 48 * time.Hour
	pruneInterval   = time.Hour

	DefaultHistoryMinutes = 60
	MaxHistoryMinutes     = 24 * 60
	maxHistoryPoints      = 288
)

var ErrInvalidRange = errors.New("monitor history range is invalid")

type Store interface {
	Insert(context.Context, Sample) error
	ListSince(context.Context, time.Time) ([]Sample, error)
	Prune(context.Context, time.Time) error
}

type Sampler interface {
	Collect() (Sample, error)
}

type Service struct {
	sampler Sampler
	store   Store
	now     func() time.Time
	logger  *slog.Logger

	mu     sync.RWMutex
	latest *Sample
}

func NewService(sampler Sampler, store Store, now func() time.Time, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{sampler: sampler, store: store, now: now, logger: logger}
}

// Run samples until ctx ends. The first sample only primes the CPU baseline, so
// CPU shows 0 until the second tick.
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(sampleInterval)
	defer ticker.Stop()
	var lastPersist, lastPrune time.Time
	for {
		s.tick(ctx, &lastPersist, &lastPrune)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) tick(ctx context.Context, lastPersist, lastPrune *time.Time) {
	sample, err := s.sampler.Collect()
	if err != nil {
		s.logger.Warn("monitor sample failed", "error_category", "collect")
		return
	}
	s.mu.Lock()
	s.latest = &sample
	s.mu.Unlock()
	if sample.At.Sub(*lastPersist) >= persistInterval {
		*lastPersist = sample.At
		if err := s.store.Insert(ctx, sample); err != nil && ctx.Err() == nil {
			s.logger.Warn("monitor sample not stored", "error_category", "store")
		}
	}
	if sample.At.Sub(*lastPrune) >= pruneInterval {
		*lastPrune = sample.At
		if err := s.store.Prune(ctx, sample.At.Add(-retention)); err != nil && ctx.Err() == nil {
			s.logger.Warn("monitor prune failed", "error_category", "store")
		}
	}
}

// Current returns the newest sample, or false before the first one exists.
func (s *Service) Current() (Sample, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.latest == nil {
		return Sample{}, false
	}
	return *s.latest, true
}

// History returns at most maxHistoryPoints points covering the last minutes.
func (s *Service) History(ctx context.Context, minutes int) ([]HistoryPoint, error) {
	if minutes < 5 || minutes > MaxHistoryMinutes {
		return nil, ErrInvalidRange
	}
	samples, err := s.store.ListSince(ctx, s.now().UTC().Add(-time.Duration(minutes)*time.Minute))
	if err != nil {
		return nil, err
	}
	return reduce(samples), nil
}

func reduce(samples []Sample) []HistoryPoint {
	stride := 1
	if len(samples) > maxHistoryPoints {
		stride = (len(samples) + maxHistoryPoints - 1) / maxHistoryPoints
	}
	points := make([]HistoryPoint, 0, len(samples)/stride+1)
	for index := 0; index < len(samples); index += stride {
		// Average each bucket so a short spike is not lost by sampling one row.
		end := min(index+stride, len(samples))
		points = append(points, average(samples[index:end]))
	}
	return points
}

func average(bucket []Sample) HistoryPoint {
	last := bucket[len(bucket)-1]
	point := HistoryPoint{At: last.At, Containers: []ContainerStats{}}
	memory := 0.0
	type sum struct {
		cpu, used float64
		limit     int64
		count     int
	}
	sums := map[string]*sum{}
	var order []string
	for _, sample := range bucket {
		point.CPUPercent += sample.Host.CPUPercent
		point.Load1 += sample.Host.Load1
		if sample.Host.MemoryTotalBytes > 0 {
			memory += float64(sample.Host.MemoryTotalBytes-sample.Host.MemoryAvailableBytes) / float64(sample.Host.MemoryTotalBytes) * 100
		}
		for _, container := range sample.Containers {
			entry := sums[container.Name]
			if entry == nil {
				entry = &sum{}
				sums[container.Name] = entry
				order = append(order, container.Name)
			}
			entry.cpu += container.CPUPercent
			entry.used += float64(container.MemoryUsedBytes)
			entry.limit = container.MemoryLimitBytes
			entry.count++
		}
	}
	count := float64(len(bucket))
	point.CPUPercent = round1(point.CPUPercent / count)
	point.Load1 = float64(int64(point.Load1/count*100+0.5)) / 100
	point.MemoryUsedPercent = round1(memory / count)
	for _, name := range order {
		entry := sums[name]
		n := float64(entry.count)
		point.Containers = append(point.Containers, ContainerStats{Name: name, CPUPercent: round1(entry.cpu / n), MemoryUsedBytes: int64(entry.used / n), MemoryLimitBytes: entry.limit})
	}
	return point
}
