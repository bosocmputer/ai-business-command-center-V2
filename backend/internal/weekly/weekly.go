// Package weekly sends each owner of a shop that switched it on one short LINE message a week: how much the assistant was used in the last
// seven days. It states only counts the system already records (figures fetched for the owner, reports and cards opened, alerts sent) and no
// business figure, name or question. A week with nothing to count sends nothing.
package weekly

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Due is one owner of one shop whose summary is to be sent today.
type Due struct {
	TenantID    uuid.UUID
	RecipientID uuid.UUID
	Shop        string
	LocalDay    string // the shop's date, the key that keeps a summary from going out twice
	Asked       int    // messages to the assistant and menu buttons pressed in LINE
	Opened      int    // cards and reports opened
	Alerts      int    // alerts sent to the owner (the morning digest is not an alert: it is sent every day)
}

func (due Due) Total() int { return due.Asked + due.Opened + due.Alerts }

type Store interface {
	// Due lists the owners of shops whose chosen weekday and hour has come and who have not had today's summary.
	Due(ctx context.Context, now time.Time) ([]Due, error)
	// Claim reserves today's summary; false when another pass already did.
	Claim(ctx context.Context, due Due, now time.Time) (bool, error)
	// Release gives the reservation back after a failed send, so the next pass tries again the same day.
	Release(ctx context.Context, due Due) error
}

type Sender interface {
	SendText(ctx context.Context, recipientID uuid.UUID, retryKey uuid.UUID, text string) error
}

type Notifier struct {
	store  Store
	sender Sender
	now    func() time.Time
	logger *slog.Logger
}

func NewNotifier(store Store, sender Sender, now func() time.Time, logger *slog.Logger) *Notifier {
	return &Notifier{store: store, sender: sender, now: now, logger: logger}
}

type Summary struct{ Due, Skipped, Sent, Failed int }

// RetryKey is the same for the same owner, shop and day, so LINE takes a repeated push of one summary only once.
func RetryKey(due Due) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("weekly-summary:"+due.TenantID.String()+":"+due.RecipientID.String()+":"+due.LocalDay))
}

func (notifier *Notifier) RunOnce(ctx context.Context) Summary {
	var summary Summary
	now := notifier.now().UTC()
	list, err := notifier.store.Due(ctx, now)
	if err != nil {
		notifier.logger.Error("weekly summary list failed", "safeErrorCode", "WEEKLY_DUE_FAILED")
		return summary
	}
	summary.Due = len(list)
	for _, due := range list {
		if ctx.Err() != nil {
			return summary
		}
		if due.Total() == 0 {
			summary.Skipped++
			continue
		}
		claimed, err := notifier.store.Claim(ctx, due, now)
		if err != nil {
			summary.Failed++
			notifier.logger.Error("weekly summary claim failed", "safeErrorCode", "WEEKLY_CLAIM_FAILED")
			continue
		}
		if !claimed {
			continue
		}
		if err := notifier.sender.SendText(ctx, due.RecipientID, RetryKey(due), Message(due)); err != nil {
			summary.Failed++
			notifier.logger.Warn("weekly summary not sent", "safeErrorCode", "WEEKLY_SEND_FAILED")
			if releaseErr := notifier.store.Release(ctx, due); releaseErr != nil {
				notifier.logger.Error("weekly summary not released", "safeErrorCode", "WEEKLY_RELEASE_FAILED")
			}
			continue
		}
		summary.Sent++
	}
	if summary.Sent > 0 || summary.Failed > 0 {
		notifier.logger.Info("weekly summary pass", "event", "weekly_summary", "due", summary.Due, "skipped", summary.Skipped, "sent", summary.Sent, "failed", summary.Failed)
	}
	return summary
}

// shopLabel puts "ร้าน" before a name that does not already start with it.
func shopLabel(name string) string {
	name = strings.TrimSpace(name)
	if strings.HasPrefix(name, "ร้าน") {
		return name
	}
	return "ร้าน " + name
}

// Message is what the owner reads: the shop, three counts at most (a zero is left out) and an invitation to ask.
func Message(due Due) string {
	lines := []string{fmt.Sprintf("📊 สรุปสัปดาห์ของเลขา AI (%s)", shopLabel(due.Shop)), "7 วันที่ผ่านมา"}
	if due.Asked > 0 {
		lines = append(lines, fmt.Sprintf("• คุณคุยกับเลขาหรือกดเมนูใน LINE %d ครั้ง", due.Asked))
	}
	if due.Opened > 0 {
		lines = append(lines, fmt.Sprintf("• เปิดการ์ดและรายงาน %d ครั้ง", due.Opened))
	}
	if due.Alerts > 0 {
		lines = append(lines, fmt.Sprintf("• แจ้งเตือนที่ส่งให้ %d เรื่อง", due.Alerts))
	}
	lines = append(lines, "พิมพ์ถามเลขาต่อได้เลย หรือกดเมนูด้านล่าง")
	return strings.Join(lines, "\n")
}
