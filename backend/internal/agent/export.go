package agent

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/viewer"
	"github.com/google/uuid"
)

const (
	// exportPageRows is how many rows one answer carries; exportMaxRows is the most rows one file is built from.
	exportPageRows = 500
	exportMaxRows  = 20000
	// exportReadPage is the page the viewer's own row reader allows.
	exportReadPage = 100

	MessageExportPreparing = "กำลังดึงรายละเอียดทุกแถวของรายงานนี้จากระบบของร้าน ใช้เวลาตั้งแต่ 1 ถึงหลายนาที รายงานใหญ่นานกว่า ถามใหม่อีกครั้งภายหลัง"
	MessageExportExpired   = "รายละเอียดของรายงานนี้หมดอายุแล้ว ขอดึงใหม่อีกครั้ง"
	MessageExportCapped    = "ไฟล์นี้มีเฉพาะ 20,000 แถวแรก รายงานนี้มีมากกว่านั้น ให้เลือกช่วงวันที่สั้นลงเพื่อได้ครบทุกแถว"
)

var ErrExportUnavailable = errors.New("report export is not available")

// ExportSource is the path a person's own web page uses to run a report in detail and read its rows. The assistant goes
// through it with the person's own permission, so an export can never carry a row the web page would not show them.
type ExportSource interface {
	OwnDetailRun(ctx context.Context, recipientID, tenantID uuid.UUID, key report.Key, period report.Period) (viewer.DashboardSnapshot, error)
	Create(ctx context.Context, recipientID, tenantID uuid.UUID, key report.Key, idempotencyKey string, input viewer.CreateReportRunInput) (report.Run, error)
	Get(ctx context.Context, recipientID, tenantID uuid.UUID, key report.Key, runID uuid.UUID) (report.Run, error)
	ListRows(ctx context.Context, recipientID, tenantID uuid.UUID, key report.Key, runID uuid.UUID, cursor string, pageSize int) (viewer.ReportRows, error)
}

// ConfigureExports turns on the export tool. Without it the tool answers as if there were no such thing.
func (service *Service) ConfigureExports(source ExportSource) *Service {
	service.exports = source
	return service
}

// ExportResponse is one page of a report's detail rows. READY carries rows; follow nextCursor until it is empty to get
// them all. PREPARING says the rows are being fetched from the shop's system.
type ExportResponse struct {
	ReportKey         report.Key     `json:"reportKey"`
	Label             string         `json:"label"`
	Status            string         `json:"status"`
	Message           string         `json:"message,omitempty"`
	Period            *PeriodView    `json:"period,omitempty"`
	CollectedAt       string         `json:"collectedAt,omitempty"`
	Columns           []ExportColumn `json:"columns,omitempty"`
	Rows              [][]string     `json:"rows,omitempty"`
	TotalRows         int            `json:"totalRows,omitempty"`
	NextCursor        string         `json:"nextCursor,omitempty"`
	Truncated         bool           `json:"truncated,omitempty"`
	Notes             []string       `json:"notes,omitempty"`
	RetryAfterSeconds int            `json:"retryAfterSeconds,omitempty"`
}

// Export gives the detail rows of a report for a period, a page at a time. The first call (no cursor) finds the person's
// own finished run of the report or asks for one; later calls follow the cursor through that same run.
func (service *Service) Export(ctx context.Context, principal Principal, rawKey, dateFrom, dateTo, cursor string) (ExportResponse, error) {
	started := service.now()
	now := started.UTC()
	if service.exports == nil {
		return ExportResponse{}, ErrExportUnavailable
	}
	definition, err := service.authorize(ctx, principal, rawKey, now)
	columns := exportColumns[definition.Key]
	if err != nil || len(columns) == 0 {
		service.record(ctx, principal, ToolExport, "", report.Period{}, OutcomeNoData, started, nil)
		return ExportResponse{}, ErrNoData
	}
	response := ExportResponse{ReportKey: definition.Key, Label: definition.LabelTH, Columns: columns}
	if cursor != "" {
		return service.exportContinue(ctx, principal, definition, response, cursor)
	}
	period, err := resolvePeriod(definition, locationOf(principal), now, dateFrom, dateTo)
	if err != nil {
		service.record(ctx, principal, ToolExport, string(definition.Key), report.Period{}, OutcomeInvalidPeriod, started, nil)
		return ExportResponse{}, err
	}
	response.Period = &PeriodView{DateFrom: period.DateFrom, DateTo: period.DateTo}
	runID, state, retry := service.exportRun(ctx, principal, definition, period, now)
	if state != stateReady {
		response.Status, response.RetryAfterSeconds = state.status(), retry
		response.Message = MessageExportPreparing
		if state == stateUnavailable {
			response.Message = MessageUnavailable
		}
		service.record(ctx, principal, ToolExport, string(definition.Key), period, outcomeOf(state), started, nil)
		return response, nil
	}
	response, err = service.exportPage(ctx, principal, definition, response, runID, 0, "")
	if err != nil {
		return ExportResponse{}, err
	}
	service.record(ctx, principal, ToolExport, string(definition.Key), period, outcomeOf(stateOf(response.Status)), started, &runID)
	return response, nil
}

func stateOf(status string) snapshotState {
	switch status {
	case "READY":
		return stateReady
	case "PREPARING":
		return statePreparing
	default:
		return stateUnavailable
	}
}

// exportRun finds the person's own detail run for the period, or starts one. A run started for the same report and
// period within the hour is the same run, so asking again while it works does not start another fetch from SML.
func (service *Service) exportRun(ctx context.Context, principal Principal, definition report.Definition, period report.Period, now time.Time) (uuid.UUID, snapshotState, int) {
	own, err := service.exports.OwnDetailRun(ctx, principal.RecipientID, principal.TenantID, definition.Key, period)
	have := err == nil
	if have && own.FreshnessStatus == viewer.FreshnessFresh {
		return own.RunID, stateReady, 0
	}
	if err != nil && !errors.Is(err, report.ErrRunNotFound) {
		return uuid.Nil, stateUnavailable, 60
	}
	input := viewer.CreateReportRunInput{PeriodPreset: period.Preset}
	if period.Preset == report.Custom {
		from, to := period.DateFrom, period.DateTo
		input.DateFrom, input.DateTo = &from, &to
	}
	key := fmt.Sprintf("agent-export-%s-%s-%s-%s-%d", principal.RecipientID.String()[:8], definition.Key, period.DateFrom, period.DateTo, now.Unix()/3600)
	run, err := service.exports.Create(ctx, principal.RecipientID, principal.TenantID, definition.Key, key, input)
	switch {
	case err == nil:
	case errors.Is(err, report.ErrRunConcurrencyLimit), errors.Is(err, report.ErrRunCircuitOpen):
		if have && own.FreshnessStatus != viewer.FreshnessExpired {
			return own.RunID, stateReady, 120
		}
		return uuid.Nil, stateUnavailable, 120
	default:
		return uuid.Nil, stateUnavailable, 60
	}
	switch run.Status {
	case report.StatusSucceeded:
		return run.ID, stateReady, 0
	case report.StatusQueued, report.StatusRunning:
		return uuid.Nil, statePreparing, 45
	}
	if have && own.FreshnessStatus != viewer.FreshnessExpired {
		return own.RunID, stateReady, 120
	}
	return uuid.Nil, stateUnavailable, 300
}

// exportContinue reads the next page of a run an earlier call found. The cursor is "run.count.position": the run, how
// many rows were already given, and where the viewer's own row reader stands.
func (service *Service) exportContinue(ctx context.Context, principal Principal, definition report.Definition, response ExportResponse, cursor string) (ExportResponse, error) {
	parts := strings.SplitN(cursor, ".", 3)
	if len(parts) != 3 {
		return ExportResponse{}, ErrInvalidPeriod
	}
	runID, idErr := uuid.Parse(parts[0])
	count, countErr := strconv.Atoi(parts[1])
	if idErr != nil || countErr != nil || count < 0 || count > exportMaxRows {
		return ExportResponse{}, ErrInvalidPeriod
	}
	return service.exportPage(ctx, principal, definition, response, runID, count, parts[2])
}

func (service *Service) exportPage(ctx context.Context, principal Principal, definition report.Definition, response ExportResponse, runID uuid.UUID, given int, position string) (ExportResponse, error) {
	run, err := service.exports.Get(ctx, principal.RecipientID, principal.TenantID, definition.Key, runID)
	if err != nil || run.Status != report.StatusSucceeded {
		return ExportResponse{}, ErrNoData
	}
	location := locationOf(principal)
	response.Period = &PeriodView{DateFrom: run.Period.DateFrom, DateTo: run.Period.DateTo}
	response.TotalRows = run.RowCount
	if run.SourceFinishedAt != nil {
		response.CollectedAt = run.SourceFinishedAt.In(location).Format(time.RFC3339)
	}
	nameColumns := map[string]bool{}
	if !principal.NamesVisible {
		for _, key := range exportNameColumns[definition.Key] {
			nameColumns[key] = true
		}
	}
	more := true
	for more && len(response.Rows) < exportPageRows && given+len(response.Rows) < exportMaxRows {
		want := min(exportReadPage, exportPageRows-len(response.Rows), exportMaxRows-given-len(response.Rows))
		page, err := service.exports.ListRows(ctx, principal.RecipientID, principal.TenantID, definition.Key, runID, position, want)
		if err != nil {
			if errors.Is(err, report.ErrRunRowsExpired) {
				response.Status, response.Message = "UNAVAILABLE", MessageExportExpired
				response.Rows, response.NextCursor = nil, ""
				return response, nil
			}
			return ExportResponse{}, err
		}
		for _, row := range page.Rows {
			cells := make([]string, len(response.Columns))
			for index, column := range response.Columns {
				value := row[column.Key]
				if nameColumns[column.Key] && strings.TrimSpace(value) != "" {
					value = service.alias(principal.TenantID, value)
				}
				cells[index] = value
			}
			response.Rows = append(response.Rows, cells)
		}
		position, more = page.NextCursor, page.HasMore
	}
	response.Status = "READY"
	total := given + len(response.Rows)
	if more {
		response.NextCursor = fmt.Sprintf("%s.%d.%s", runID, total, position)
		if total >= exportMaxRows {
			response.NextCursor = ""
			response.Truncated = true
			response.Notes = append(response.Notes, MessageExportCapped)
		}
	}
	if run.IsTruncated {
		response.Truncated = true
		response.Notes = append(response.Notes, "ระบบของร้านส่งแถวมาไม่ครบทั้งหมด ข้อมูลในไฟล์อาจไม่ครบ ให้เลือกช่วงวันที่สั้นลง")
	}
	if note, ok := exportNotes[definition.Key]; ok {
		response.Notes = append(response.Notes, note)
	}
	if !principal.NamesVisible && len(exportNameColumns[definition.Key]) > 0 {
		response.Notes = append(response.Notes, MessageMasked)
	}
	return response, nil
}
