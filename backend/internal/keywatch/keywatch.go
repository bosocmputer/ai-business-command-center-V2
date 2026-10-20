// Package keywatch tells the operator before a shop's model key runs out. Each shop's assistant answers through its own OpenRouter key with a
// spending limit; when the limit is reached the assistant stops answering that shop altogether (this happened to a shop during testing on
// 20 Oct 2026). OpenRouter says how much of a key's limit is left, so the worker asks every half hour and writes to the operator's Telegram
// chat when a key is low or unusable. Messages carry shop names and dollar amounts, never a key.
package keywatch

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	LevelLow      = "LOW"      // the key still works but little is left
	LevelUnusable = "UNUSABLE" // the key is refused: its limit is reached or it was revoked
)

// Thresholds: "low" is when the remainder is under LowFloor dollars or under LowShare of the key's limit, whichever is larger.
const (
	LowFloor    = 1.0
	LowShare    = 0.2
	ReminderGap = 24 * time.Hour
)

type Shop struct {
	TenantID uuid.UUID
	Name     string
}

// Balance is what OpenRouter says about one key. Limit and Remaining are nil for a key without a limit.
type Balance struct {
	Limit     *float64
	Remaining *float64
}

// ErrRefused is a key OpenRouter does not accept (limit reached, revoked, or wrong).
type ErrRefused struct{ Status int }

func (err *ErrRefused) Error() string { return fmt.Sprintf("model key refused (%d)", err.Status) }

type Store interface {
	Shops(ctx context.Context) ([]Shop, error)
	LastNotice(ctx context.Context, tenantID uuid.UUID, level string) (time.Time, bool, error)
	MarkNotice(ctx context.Context, tenantID uuid.UUID, level string, now time.Time) error
	ClearNotices(ctx context.Context, tenantID uuid.UUID) error
}

// Keys gives a shop's current model key ("" when the shop has none to use). It is the only place a key is read, and it is never logged.
type Keys interface {
	KeyFor(ctx context.Context, tenantID uuid.UUID) (string, error)
}

type Checker interface {
	Balance(ctx context.Context, key string) (Balance, error)
}

type Sender interface {
	Send(ctx context.Context, text string) error
}

type Mode string

const (
	ModeOff     Mode = "off"
	ModeObserve Mode = "observe"
	ModeSend    Mode = "send"
)

type Watcher struct {
	store   Store
	keys    Keys
	checker Checker
	sender  Sender
	mode    Mode
	now     func() time.Time
	logger  *slog.Logger
}

func NewWatcher(store Store, keys Keys, checker Checker, sender Sender, mode Mode, now func() time.Time, logger *slog.Logger) *Watcher {
	return &Watcher{store: store, keys: keys, checker: checker, sender: sender, mode: mode, now: now, logger: logger}
}

type Summary struct{ Shops, Low, Unusable, Told, Unknown int }

// Level says how a balance stands; "" means the key has plenty left or no limit.
func Level(balance Balance) string {
	if balance.Remaining == nil || balance.Limit == nil {
		return ""
	}
	threshold := LowFloor
	if share := *balance.Limit * LowShare; share > threshold {
		threshold = share
	}
	switch {
	case *balance.Remaining <= 0:
		return LevelUnusable
	case *balance.Remaining < threshold:
		return LevelLow
	}
	return ""
}

func (watcher *Watcher) RunOnce(ctx context.Context) Summary {
	var summary Summary
	if watcher.mode == ModeOff {
		return summary
	}
	shops, err := watcher.store.Shops(ctx)
	if err != nil {
		watcher.logger.Error("model key check failed", "safeErrorCode", "KEYWATCH_SHOPS_FAILED")
		return summary
	}
	summary.Shops = len(shops)
	now := watcher.now().UTC()
	for _, shop := range shops {
		if ctx.Err() != nil {
			return summary
		}
		key, err := watcher.keys.KeyFor(ctx, shop.TenantID)
		if err != nil || key == "" {
			continue
		}
		level, balance := "", Balance{}
		balance, err = watcher.checker.Balance(ctx, key)
		var refused *ErrRefused
		switch {
		case err != nil && asRefused(err, &refused):
			level = LevelUnusable
		case err != nil:
			summary.Unknown++ // OpenRouter could not be reached: say nothing rather than guess
			continue
		default:
			level = Level(balance)
		}
		switch level {
		case LevelLow:
			summary.Low++
		case LevelUnusable:
			summary.Unusable++
		default:
			if err := watcher.store.ClearNotices(ctx, shop.TenantID); err != nil {
				watcher.logger.Error("model key notices not cleared", "safeErrorCode", "KEYWATCH_CLEAR_FAILED")
			}
			continue
		}
		if watcher.mode != ModeSend || watcher.sender == nil {
			continue
		}
		if last, found, err := watcher.store.LastNotice(ctx, shop.TenantID, level); err == nil && found && now.Sub(last) < ReminderGap {
			continue
		}
		if err := watcher.sender.Send(ctx, Message(shop.Name, level, balance)); err != nil {
			watcher.logger.Warn("model key notice not sent", "safeErrorCode", "KEYWATCH_SEND_FAILED")
			continue
		}
		if err := watcher.store.MarkNotice(ctx, shop.TenantID, level, now); err != nil {
			watcher.logger.Error("model key notice not remembered", "safeErrorCode", "KEYWATCH_MARK_FAILED")
		}
		summary.Told++
	}
	if summary.Low > 0 || summary.Unusable > 0 {
		watcher.logger.Info("model key check", "event", "model_key_check", "mode", string(watcher.mode), "shops", summary.Shops, "low", summary.Low, "unusable", summary.Unusable, "told", summary.Told)
	}
	return summary
}

func asRefused(err error, target **ErrRefused) bool {
	refused, ok := err.(*ErrRefused)
	if ok {
		*target = refused
	}
	return ok
}

func dollars(value float64) string { return fmt.Sprintf("$%.2f", value) }

// Message is what the operator reads: the shop, what is left, and what to do. It never holds a key.
func Message(shop, level string, balance Balance) string {
	shop = strings.TrimSpace(shop)
	if level == LevelUnusable {
		return fmt.Sprintf("🛑 เลขา AI ของร้าน %s ตอบไม่ได้ตอนนี้ เพราะ key โมเดลใช้ไม่ได้ (วงเงินเต็มหรือถูกเพิกถอน)\nแก้ที่ openrouter.ai/keys เพิ่มวงเงินหรือเปลี่ยน key ในหน้า admin ของร้านนี้", shop)
	}
	left := ""
	if balance.Remaining != nil && balance.Limit != nil {
		left = fmt.Sprintf(" เหลือ %s จากวงเงิน %s", dollars(*balance.Remaining), dollars(*balance.Limit))
	}
	return fmt.Sprintf("⚠️ วงเงิน key โมเดลของร้าน %s ใกล้หมด%s\nเมื่อหมด เลขา AI ของร้านนี้จะหยุดตอบ เพิ่มวงเงินที่ openrouter.ai/keys ก่อน", shop, left)
}
