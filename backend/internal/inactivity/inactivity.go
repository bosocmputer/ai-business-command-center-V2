// Package inactivity tells the operator when an owner has stopped using the assistant (or never started), so the operator can reach out
// before the owner forgets it exists. It counts only what the system already records about use (cards and reports opened, messages and
// menu buttons in LINE) and says nothing about words or figures.
package inactivity

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Mode string

const (
	ModeOff     Mode = "off"
	ModeObserve Mode = "observe" // looks and counts, tells nobody and remembers nothing
	ModeSend    Mode = "send"
)

type Kind string

const (
	// Inactive: used before, nothing for the quiet period.
	Inactive Kind = "INACTIVE"
	// NeverUsed: invited to talk to the assistant, and has not used anything since.
	NeverUsed Kind = "NEVER_USED"
)

type Candidate struct {
	TenantID     uuid.UUID
	RecipientID  uuid.UUID
	Shop         string
	Kind         Kind
	LastActivity *time.Time // for Inactive
	Joined       time.Time  // for NeverUsed: when the person joined the shop
}

type Store interface {
	Candidates(ctx context.Context, now time.Time, quiet, never time.Duration) ([]Candidate, error)
	MarkNotified(ctx context.Context, candidate Candidate, now time.Time) error
}

// Names gives the name an operator knows the person by ("" when it cannot be read).
type Names interface {
	DisplayName(ctx context.Context, recipientID uuid.UUID) string
}

type Sender interface {
	Send(ctx context.Context, text string) error
}

type Notifier struct {
	store     Store
	names     Names
	sender    Sender
	mode      Mode
	logger    *slog.Logger
	now       func() time.Time
	Quiet     time.Duration // days without use before an owner counts as quiet
	Never     time.Duration // days since joining before "never used" is worth saying
	StartHour int           // the shop day's hour from which the check may speak
}

func NewNotifier(store Store, names Names, sender Sender, mode Mode, now func() time.Time, logger *slog.Logger) *Notifier {
	return &Notifier{store: store, names: names, sender: sender, mode: mode, now: now, logger: logger, Quiet: 5 * 24 * time.Hour, Never: 7 * 24 * time.Hour, StartHour: 9}
}

type Summary struct{ Candidates, Told, Failed int }

var bangkok = time.FixedZone("Asia/Bangkok", 7*60*60)

// RunOnce looks for owners to tell the operator about. Each owner is told about once per quiet spell: a notice is remembered, and a person who
// is active again can be told about next time they go quiet.
func (notifier *Notifier) RunOnce(ctx context.Context) Summary {
	var summary Summary
	if notifier.mode == ModeOff {
		return summary
	}
	now := notifier.now().UTC()
	if now.In(bangkok).Hour() < notifier.StartHour {
		return summary
	}
	candidates, err := notifier.store.Candidates(ctx, now, notifier.Quiet, notifier.Never)
	if err != nil {
		notifier.logger.Error("inactivity check failed", "safeErrorCode", "INACTIVITY_CANDIDATES_FAILED")
		return summary
	}
	summary.Candidates = len(candidates)
	if notifier.mode == ModeObserve || notifier.sender == nil {
		if summary.Candidates > 0 {
			notifier.logger.Info("inactivity candidates", "event", "owner_inactivity", "mode", string(notifier.mode), "candidates", summary.Candidates)
		}
		return summary
	}
	for _, candidate := range candidates {
		if ctx.Err() != nil {
			return summary
		}
		name := notifier.names.DisplayName(ctx, candidate.RecipientID)
		if err := notifier.sender.Send(ctx, Message(candidate, name, now)); err != nil {
			summary.Failed++
			notifier.logger.Warn("inactivity notice not sent", "safeErrorCode", "INACTIVITY_SEND_FAILED")
			continue // not remembered: tried again next time
		}
		if err := notifier.store.MarkNotified(ctx, candidate, now); err != nil {
			notifier.logger.Error("inactivity notice not remembered", "safeErrorCode", "INACTIVITY_MARK_FAILED")
		}
		summary.Told++
	}
	if summary.Candidates > 0 {
		notifier.logger.Info("owner inactivity check", "event", "owner_inactivity", "mode", string(notifier.mode), "candidates", summary.Candidates, "told", summary.Told, "failed", summary.Failed)
	}
	return summary
}

var thaiMonths = []string{"ม.ค.", "ก.พ.", "มี.ค.", "เม.ย.", "พ.ค.", "มิ.ย.", "ก.ค.", "ส.ค.", "ก.ย.", "ต.ค.", "พ.ย.", "ธ.ค."}

func thaiDate(at time.Time) string {
	local := at.In(bangkok)
	return fmt.Sprintf("%d %s %d", local.Day(), thaiMonths[local.Month()-1], local.Year()+543)
}

func wholeDays(from, to time.Time) int {
	return int(to.Sub(from).Hours() / 24)
}

// Message is what the operator reads: who, which shop, how long, and what to do. No figures and no words of the owner's.
func Message(candidate Candidate, name string, now time.Time) string {
	who := strings.TrimSpace(name)
	if who == "" {
		who = "ผู้รับรายหนึ่ง"
	}
	shop := strings.TrimSpace(candidate.Shop)
	switch candidate.Kind {
	case NeverUsed:
		return fmt.Sprintf("👀 ยังไม่เคยใช้เลขา AI: %s (ร้าน %s) เข้าร่วมมา %d วันแล้ว ยังไม่เคยเปิดการ์ด เปิดรายงาน หรือถามเลขาเลย\nลองทักไปถามว่าติดตรงไหน หรือช่วยตั้งเกณฑ์เตือนด้วยกัน", who, shop, wholeDays(candidate.Joined, now))
	default:
		last := "ไม่ทราบ"
		days := 0
		if candidate.LastActivity != nil {
			last, days = thaiDate(*candidate.LastActivity), wholeDays(*candidate.LastActivity, now)
		}
		return fmt.Sprintf("👀 เจ้าของหยุดใช้เลขา AI: %s (ร้าน %s) ไม่มีการเปิดการ์ด เปิดรายงาน หรือถามเลขามา %d วัน (ครั้งล่าสุด %s)\nลองทักไปถามว่าติดตรงไหน หรือมีอะไรที่อยากให้เลขาช่วยเพิ่ม", who, shop, days, last)
	}
}
