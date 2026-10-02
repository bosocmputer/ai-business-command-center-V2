// Package agent is the read-only API the owner-facing assistant uses. AI-BCC stays the only source of numbers
// and the only place that decides who may see what: the assistant presents a token, never a permission.
package agent

import (
	"context"
	"errors"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/viewer"
	"github.com/google/uuid"
)

type Tool string

const (
	ToolContext        Tool = "context"
	ToolGetReport      Tool = "get_report"
	ToolCompare        Tool = "compare"
	ToolLatestDelivery Tool = "latest_delivery"
)

type Outcome string

const (
	OutcomeOK            Outcome = "OK"
	OutcomePreparing     Outcome = "PREPARING"
	OutcomeUnavailable   Outcome = "UNAVAILABLE"
	OutcomeNoData        Outcome = "NO_DATA"
	OutcomeInvalidPeriod Outcome = "INVALID_PERIOD"
	OutcomeRateLimited   Outcome = "RATE_LIMITED"
)

// Every message below is shown to the owner by the assistant, so each is plain Thai.
const (
	MessageNoData       = "ไม่มีข้อมูลเรื่องนี้ให้ดู"
	MessageUnauthorized = "ผู้ช่วยยังไม่ได้รับอนุญาตให้ดูข้อมูลร้านนี้ กรุณาแจ้งผู้ดูแลระบบ"
	MessageInvalidDates = "ช่วงวันที่ไม่ถูกต้อง ช่วงต้องไม่เกิน 366 วัน ไม่เป็นวันในอนาคต และวันเริ่มต้องไม่หลังวันสิ้นสุด"
	MessagePreparing    = "กำลังดึงข้อมูลรายงานนี้จากระบบของร้าน ใช้เวลาประมาณ 1 นาที ถามใหม่อีกครั้งภายหลัง"
	MessageUnavailable  = "ตอนนี้ยังไม่มีข้อมูลรายงานนี้และยังดึงใหม่ไม่ได้ ลองถามใหม่ภายหลัง"
	MessageBudgetUsed   = "เพิ่งสั่งดึงข้อมูลไปหลายรายการแล้ว ขอรอสักครู่ (ประมาณ 5 นาที) แล้วถามใหม่"
	MessageStale        = "ข้อมูลนี้ไม่ใช่ข้อมูลล่าสุด ระบบกำลังอัปเดตให้"
	MessageRateLimited  = "ถามถี่เกินไป รอสักครู่แล้วถามใหม่"
	MessageMasked       = "ชื่อลูกค้าและผู้จำหน่ายถูกแทนด้วยรหัส เช่น ลูกค้า-7F3A เพื่อความเป็นส่วนตัว"
)

var (
	ErrUnauthorized      = errors.New("agent token is not valid")
	ErrNoData            = errors.New("no data")
	ErrInvalidPeriod     = errors.New("agent period is invalid")
	ErrDisabled          = errors.New("agent api is disabled")
	ErrAIChatDisabled    = errors.New("the recipient has not been given the assistant")
	ErrRecipientNotFound = errors.New("recipient not found")
)

// RateLimitedError says how long to wait.
type RateLimitedError struct{ RetryAfter time.Duration }

func (err *RateLimitedError) Error() string { return "agent call limit reached" }

// Principal is who a valid token stands for.
type Principal struct {
	TokenID      uuid.UUID
	TenantID     uuid.UUID
	RecipientID  uuid.UUID
	NamesVisible bool
	ShopName     string
	Timezone     string
}

// TokenInfo is what an admin sees about a recipient's token. The token itself is shown once when issued.
type TokenInfo struct {
	Status       string     `json:"status"`
	NamesVisible bool       `json:"namesVisible"`
	CreatedAt    *time.Time `json:"createdAt,omitempty"`
	ExpiresAt    *time.Time `json:"expiresAt,omitempty"`
	LastUsedAt   *time.Time `json:"lastUsedAt,omitempty"`
	Calls24h     int        `json:"calls24h"`
}

type IssuedToken struct {
	Token string    `json:"token"`
	Info  TokenInfo `json:"info"`
}

// Call is one row of the call log. It holds no values, names or question text.
type Call struct {
	TokenID, TenantID, RecipientID uuid.UUID
	Tool                           Tool
	ReportKey                      string
	PeriodFrom, PeriodTo           string
	Outcome                        Outcome
	Duration                       time.Duration
	SnapshotRunID                  *uuid.UUID
}

// Delivered is a card's report as the owner last received it.
type Delivered struct {
	Dashboard   report.Dashboard
	DeliveredAt time.Time
	CollectedAt *time.Time
	RunID       uuid.UUID
}

type Store interface {
	Authenticate(ctx context.Context, tokenHash []byte, now time.Time) (Principal, error)
	TouchToken(ctx context.Context, tokenID uuid.UUID, now time.Time) error
	CallsSince(ctx context.Context, tokenID uuid.UUID, since time.Time) (int, error)
	PreparingSince(ctx context.Context, tenantID uuid.UUID, since time.Time) (int, error)
	RecordCall(ctx context.Context, call Call, now time.Time) error
	PermittedReports(ctx context.Context, principal Principal, now time.Time) ([]report.Key, error)
	LatestDelivery(ctx context.Context, principal Principal, key report.Key) (Delivered, error)

	IssueToken(ctx context.Context, actorHash []byte, requestID string, tenantID, recipientID uuid.UUID, namesVisible bool, tokenHash []byte, expiresAt, now time.Time) (TokenInfo, error)
	RevokeToken(ctx context.Context, actorHash []byte, requestID string, tenantID, recipientID uuid.UUID, now time.Time) error
	TokenInfo(ctx context.Context, tenantID, recipientID uuid.UUID, now time.Time) (TokenInfo, error)
}

// Snapshots is the existing summary snapshot store. The assistant reads what cards and the web page read.
type Snapshots interface {
	GetExactSnapshotForPeriod(ctx context.Context, tenantID uuid.UUID, key report.Key, period report.Period, now time.Time) (viewer.DashboardSnapshot, error)
	RevalidateSnapshot(ctx context.Context, tenantID uuid.UUID, key report.Key, period report.Period, now time.Time) (viewer.ReportRevalidation, error)
}

type TokenHasher interface{ HashToken(token string) []byte }
