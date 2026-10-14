// Package engagement holds the per-shop options for how an owner is looked after: where alerts reach them, whether the assistant's own
// morning digest is sent, the weekly summary, and whether the operator is told when the owner stops using the assistant. The admin chooses;
// a shop with nothing chosen gets Defaults.
package engagement

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

const (
	ChannelTelegram = "TELEGRAM"
	ChannelLine     = "LINE"
	ChannelOff      = "OFF"
)

var (
	ErrInvalid  = errors.New("engagement settings are invalid")
	ErrConflict = errors.New("engagement settings changed in another session")
	ErrNotFound = errors.New("shop not found")
)

type Settings struct {
	AlertChannel            string `json:"alertChannel"`
	DigestEnabled           bool   `json:"digestEnabled"`
	WeeklySummaryEnabled    bool   `json:"weeklySummaryEnabled"`
	WeeklySummaryWeekday    int    `json:"weeklySummaryWeekday"` // 1 = Monday ... 7 = Sunday
	WeeklySummaryHour       int    `json:"weeklySummaryHour"`    // the shop's local hour
	InactivityNoticeEnabled bool   `json:"inactivityNoticeEnabled"`
	InactivityQuietDays     int    `json:"inactivityQuietDays"`
	Version                 int    `json:"version"` // 0 until the admin has saved something for the shop
}

// Defaults is what a shop has until the admin chooses: the alerts and digest as they were, the weekly summary off, the operator told after
// five quiet days.
func Defaults() Settings {
	return Settings{AlertChannel: ChannelTelegram, DigestEnabled: true, WeeklySummaryWeekday: 1, WeeklySummaryHour: 8, InactivityNoticeEnabled: true, InactivityQuietDays: 5}
}

func (settings Settings) Validate() error {
	switch settings.AlertChannel {
	case ChannelTelegram, ChannelLine, ChannelOff:
	default:
		return ErrInvalid
	}
	if settings.WeeklySummaryWeekday < 1 || settings.WeeklySummaryWeekday > 7 || settings.WeeklySummaryHour < 0 || settings.WeeklySummaryHour > 23 ||
		settings.InactivityQuietDays < 2 || settings.InactivityQuietDays > 60 {
		return ErrInvalid
	}
	return nil
}

type Store interface {
	Get(ctx context.Context, tenantID uuid.UUID) (Settings, error)
	// Put saves the settings when expectedVersion is the stored one (0 when nothing is stored yet) and writes the audit record.
	Put(ctx context.Context, actorHash []byte, requestID string, tenantID uuid.UUID, settings Settings, expectedVersion int, now time.Time) (Settings, error)
}

type Service struct {
	store Store
	now   func() time.Time
}

func NewService(store Store, now func() time.Time) *Service { return &Service{store: store, now: now} }

func (service *Service) Get(ctx context.Context, tenantID uuid.UUID) (Settings, error) {
	return service.store.Get(ctx, tenantID)
}

// Put validates and saves. input.Version is the version the admin saw.
func (service *Service) Put(ctx context.Context, actorHash []byte, requestID string, tenantID uuid.UUID, input Settings) (Settings, error) {
	if err := input.Validate(); err != nil {
		return Settings{}, err
	}
	return service.store.Put(ctx, actorHash, requestID, tenantID, input, input.Version, service.now().UTC())
}
