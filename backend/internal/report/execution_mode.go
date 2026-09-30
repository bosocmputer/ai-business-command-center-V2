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

// ChunkUnitThreshold is the unit count (products or customers) from which a
// report is measured as CHUNKED. It is a starting guess: the only hard data is
// that 8,080 stock rows could not be fetched in one query. Tune it from the
// first real shops.
const ChunkUnitThreshold = 3000

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

// RecommendMode maps a measured unit count to a mode.
func RecommendMode(units int) ExecutionMode {
	if units >= ChunkUnitThreshold {
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
