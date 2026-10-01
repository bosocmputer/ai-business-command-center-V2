// Package executionmode is the admin-facing side of the per-tenant report
// execution modes: list them, change one, and measure how big a shop's heavy
// reports are. The worker owns the automatic switching.
package executionmode

import (
	"context"
	"errors"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/sml"
	"github.com/google/uuid"
)

var (
	ErrUnsupported    = errors.New("report cannot be split into chunks")
	ErrTenantBusy     = errors.New("tenant has a report running")
	ErrTenantNotFound = errors.New("tenant not found")
)

const (
	maximumReasonLength = 200
	// Two manifest queries must fit inside the API's 30 second write timeout.
	measureQueryTimeout = 12 * time.Second
)

type Store interface {
	List(context.Context, uuid.UUID) ([]report.ExecutionModeRecord, error)
	Set(ctx context.Context, actorHash []byte, requestID string, tenantID uuid.UUID, key report.Key, mode report.ExecutionMode, source report.ModeSource, reason string, now time.Time) (report.ExecutionModeRecord, error)
	RecordMeasurement(context.Context, uuid.UUID, report.Key, int, time.Duration, time.Time) error
	TenantBusy(context.Context, uuid.UUID) (bool, error)
}

type Connections interface {
	Open(context.Context, uuid.UUID) (sml.Connection, error)
}

type Querier interface {
	Query(context.Context, sml.Connection, string) ([]map[string]string, error)
}

// Item is one report row on the tenant's modes page. Reports that cannot be
// chunked still appear so the page explains why they have no choice.
type Item struct {
	ReportKey      report.Key           `json:"reportKey"`
	Label          string               `json:"label"`
	Chunkable      bool                 `json:"chunkable"`
	Mode           report.ExecutionMode `json:"mode"`
	Source         report.ModeSource    `json:"source"`
	Reason         string               `json:"reason"`
	LastRows       *int                 `json:"lastRows"`
	LastDurationMS *int64               `json:"lastDurationMs"`
	ChangedAt      *time.Time           `json:"changedAt"`
	// ChunkThreshold is the measured unit count from which CHUNKED is recommended,
	// or nil for a report that cannot be chunked.
	ChunkThreshold *int `json:"chunkThreshold"`
}

// Measurement is the outcome for one chunkable report. Units is nil and
// SafeErrorCode set when the shop could not be measured.
type Measurement struct {
	ReportKey       report.Key           `json:"reportKey"`
	Units           *int                 `json:"units"`
	RecommendedMode report.ExecutionMode `json:"recommendedMode"`
	Applied         bool                 `json:"applied"`
	Threshold       int                  `json:"threshold"`
	SafeErrorCode   string               `json:"safeErrorCode,omitempty"`
}

type Service struct {
	store       Store
	connections Connections
	client      Querier
	now         func() time.Time
}

func NewService(store Store, connections Connections, client Querier, now func() time.Time) *Service {
	return &Service{store: store, connections: connections, client: client, now: now}
}

func (s *Service) List(ctx context.Context, tenantID uuid.UUID) ([]Item, error) {
	records, err := s.store.List(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	byKey := make(map[report.Key]report.ExecutionModeRecord, len(records))
	for _, record := range records {
		byKey[record.ReportKey] = record
	}
	items := make([]Item, 0, len(report.Keys()))
	for _, key := range report.Keys() {
		definition, ok := report.DefinitionFor(key)
		if !ok {
			continue
		}
		record, stored := byKey[key]
		if !stored {
			record = report.ExecutionModeRecord{ReportKey: key, Mode: report.ModeDirect, Source: report.ModeSourceDefault}
		}
		items = append(items, itemFor(definition, record))
	}
	return items, nil
}

// Set applies an admin's choice. CHUNKED is refused for reports without a
// chunk plan, because a mode the worker cannot honour would fail every run.
func (s *Service) Set(ctx context.Context, actorHash []byte, requestID string, tenantID uuid.UUID, key report.Key, mode report.ExecutionMode, reason string) (Item, error) {
	definition, known := report.DefinitionFor(key)
	if !known || !mode.Valid() || (mode == report.ModeChunked && !definition.ChunkSafe) || len(reason) > maximumReasonLength {
		return Item{}, ErrUnsupported
	}
	record, err := s.store.Set(ctx, actorHash, requestID, tenantID, key, mode, report.ModeSourceManual, reason, s.now().UTC())
	if err != nil {
		return Item{}, err
	}
	return itemFor(definition, record), nil
}

func itemFor(definition report.Definition, record report.ExecutionModeRecord) Item {
	item := Item{
		ReportKey: definition.Key, Label: definition.LabelTH, Chunkable: definition.ChunkSafe,
		Mode: record.Mode, Source: record.Source, Reason: record.Reason,
		LastRows: record.LastRows, LastDurationMS: record.LastDurationMS, ChangedAt: record.ChangedAt,
	}
	if definition.ChunkSafe {
		threshold := report.ChunkUnitThreshold(definition.Key)
		item.ChunkThreshold = &threshold
	}
	return item
}

// Measure counts the units each chunkable report would split over, records the
// size, and sets the mode for reports nobody has decided yet. Reports already
// decided by the worker or an admin only receive a recommendation.
func (s *Service) Measure(ctx context.Context, actorHash []byte, requestID string, tenantID uuid.UUID) ([]Measurement, error) {
	busy, err := s.store.TenantBusy(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if busy {
		return nil, ErrTenantBusy
	}
	records, err := s.store.List(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	current := make(map[report.Key]report.ExecutionModeRecord, len(records))
	for _, record := range records {
		current[record.ReportKey] = record
	}
	connection, err := s.connections.Open(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	businessDate := now.In(bangkok()).Format("2006-01-02")
	period := report.Period{Preset: report.AsOfRun, DateFrom: businessDate, DateTo: businessDate}
	var results []Measurement
	for _, key := range report.Keys() {
		definition, ok := report.DefinitionFor(key)
		if !ok || !definition.ChunkSafe {
			continue
		}
		results = append(results, s.measureOne(ctx, actorHash, requestID, tenantID, key, period, connection, current[key], now))
	}
	return results, nil
}

func (s *Service) measureOne(ctx context.Context, actorHash []byte, requestID string, tenantID uuid.UUID, key report.Key, period report.Period, connection sml.Connection, existing report.ExecutionModeRecord, now time.Time) Measurement {
	result := Measurement{ReportKey: key, Threshold: report.ChunkUnitThreshold(key)}
	query, _, err := report.BuildChunkManifestQuery(key, period)
	if err != nil {
		result.SafeErrorCode = "REPORT_CONTRACT_INVALID"
		return result
	}
	rendered, err := report.RenderSQL(query)
	if err != nil {
		result.SafeErrorCode = "REPORT_QUERY_RENDER_FAILED"
		return result
	}
	queryCtx, cancel := context.WithTimeout(ctx, measureQueryTimeout)
	defer cancel()
	started := s.now()
	rows, err := s.client.Query(queryCtx, connection, rendered)
	if err != nil {
		result.SafeErrorCode = safeCode(err)
		return result
	}
	elapsed := s.now().Sub(started)
	units, err := report.ChunkKeys(rows)
	if err != nil {
		result.SafeErrorCode = "REPORT_OUTPUT_INVALID"
		return result
	}
	count := len(units)
	result.Units = &count
	result.RecommendedMode = report.RecommendMode(key, count)
	if err := s.store.RecordMeasurement(ctx, tenantID, key, count, elapsed, now); err != nil {
		result.SafeErrorCode = "STORE_FAILED"
		return result
	}
	// A measurement replaces nothing but an earlier measurement or the default.
	// A switch made by the worker or a choice made by an admin is never overruled.
	if existing.Source == "" || existing.Source == report.ModeSourceDefault || existing.Source == report.ModeSourceMeasured {
		reason := "measured " + itoa(count) + " units"
		if _, err := s.store.Set(ctx, actorHash, requestID, tenantID, key, result.RecommendedMode, report.ModeSourceMeasured, reason, now); err == nil {
			result.Applied = true
		}
	}
	return result
}

func safeCode(err error) string {
	var safe *sml.SafeError
	if errors.As(err, &safe) {
		return safe.Code
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "SML_TIMEOUT"
	}
	return "SML_QUERY_FAILED"
}
