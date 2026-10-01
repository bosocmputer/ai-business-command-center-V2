package report

import "time"

// ExecutionMode says how a heavy report is fetched from SML.
type ExecutionMode string

const (
	ModeDirect  ExecutionMode = "DIRECT"
	ModeChunked ExecutionMode = "CHUNKED"
)

// ModeSource records who chose the current mode.
type ModeSource string

const (
	ModeSourceDefault      ModeSource = "DEFAULT"
	ModeSourceEnvSeed      ModeSource = "ENV_SEED"
	ModeSourceMeasured     ModeSource = "MEASURED"
	ModeSourceAutoSwitched ModeSource = "AUTO_SWITCHED"
	ModeSourceManual       ModeSource = "MANUAL"
)

// ChunkUnitThreshold returns the unit count (products or customers) from which a
// measurement recommends CHUNKED for a report. It is an estimate anchored on two
// observations per report, not a measured limit:
//
//   - stock balance: one shop fetched 190 items in a single query in about a
//     second, while another could not fetch 8,080 stock rows at all (the response
//     XML was cut off), so the line sits between them;
//   - receivable movement: a shop fetched 3,972 customers and about 50,000 rows
//     in a single query in about five seconds. One query may return at most
//     200,000 rows, and a shop has been seen at about 13 rows per customer, so
//     the line is set well below that.
//
// The worker still switches a report to CHUNKED by itself when a direct fetch
// fails on size, so a threshold that is too high costs one failed run, not
// missing data. Recorded sizes and durations (last_rows, last_duration_ms) are
// there to tune these numbers.
func ChunkUnitThreshold(key Key) int {
	switch key {
	case StockBalance:
		return 5000
	case ARCustomerMovement:
		return 8000
	}
	return 0
}

// ExecutionModeRecord is one tenant/report row. Missing rows mean DIRECT.
type ExecutionModeRecord struct {
	ReportKey      Key           `json:"reportKey"`
	Mode           ExecutionMode `json:"mode"`
	Source         ModeSource    `json:"source"`
	Reason         string        `json:"reason,omitempty"`
	LastRows       *int          `json:"lastRows"`
	LastDurationMS *int64        `json:"lastDurationMs"`
	ChangedAt      *time.Time    `json:"changedAt"`
}

func (mode ExecutionMode) Valid() bool { return mode == ModeDirect || mode == ModeChunked }

// RecommendMode maps a measured unit count to a mode for one report. A report
// that cannot be chunked is always DIRECT.
func RecommendMode(key Key, units int) ExecutionMode {
	if threshold := ChunkUnitThreshold(key); threshold > 0 && units >= threshold {
		return ModeChunked
	}
	return ModeDirect
}

// IsSizeSignal reports whether a failed DIRECT run says the data was too big for
// one response, as opposed to the shop being down or misconfigured. validation
// is the JavaWS parser's result-validation code when one was recorded.
func IsSizeSignal(code, validation string) bool {
	switch code {
	case "SML_RESPONSE_TOO_LARGE", "SML_ZIP_TOO_LARGE":
		return true
	case "SML_RESULT_INVALID":
		switch validation {
		case "XML_MALFORMED", "ROW_LIMIT_EXCEEDED", "FIELD_VALUE_TOO_LARGE":
			return true
		}
	}
	return false
}
