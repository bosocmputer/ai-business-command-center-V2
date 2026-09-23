package recipient

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/line"
)

type FollowState string

const (
	FollowStateFollowing FollowState = "FOLLOWING"
	FollowStateBlocked   FollowState = "BLOCKED"
)

var (
	ErrWebhookNotConfigured    = errors.New("LINE webhook channel secret is not configured")
	ErrWebhookSignatureInvalid = errors.New("LINE webhook signature is invalid")
)

// FollowStore records whether a known recipient currently follows the
// official account. It reports false when no active recipient matches or a
// newer event already won, so replays and out-of-order delivery are no-ops.
type FollowStore interface {
	RecordFollowState(ctx context.Context, lineHash []byte, state FollowState, changedAt, now time.Time) (bool, error)
}

type lineUserHasher interface {
	HashToken(string) []byte
}

type WebhookService struct {
	channelSecret string
	store         FollowStore
	tokens        lineUserHasher
	now           func() time.Time
}

func NewWebhookService(channelSecret string, store FollowStore, tokens lineUserHasher, now func() time.Time) *WebhookService {
	return &WebhookService{channelSecret: channelSecret, store: store, tokens: tokens, now: now}
}

// HandleWebhook verifies and applies one LINE webhook delivery. Events from
// people who are not recipients are dropped without storing anything about them.
func (service *WebhookService) HandleWebhook(ctx context.Context, body []byte, signature string) error {
	if service.channelSecret == "" {
		return ErrWebhookNotConfigured
	}
	if !line.VerifyWebhookSignature(service.channelSecret, body, signature) {
		return ErrWebhookSignatureInvalid
	}
	webhook, err := line.ParseWebhook(body)
	if err != nil {
		return err
	}
	now := service.now().UTC()
	for _, event := range webhook.Events {
		state, ok := followStateFor(event)
		if !ok {
			continue
		}
		changedAt := now
		if event.Timestamp > 0 {
			changedAt = time.UnixMilli(event.Timestamp).UTC()
		}
		lineHash := service.tokens.HashToken("line-user:" + event.Source.UserID)
		if _, err := service.store.RecordFollowState(ctx, lineHash, state, changedAt, now); err != nil {
			return fmt.Errorf("record LINE follow state: %w", err)
		}
	}
	return nil
}

func followStateFor(event line.WebhookEvent) (FollowState, bool) {
	if event.Source.Type != "user" || len(event.Source.UserID) < 2 || len(event.Source.UserID) > 128 {
		return "", false
	}
	switch event.Type {
	case line.WebhookEventFollow:
		return FollowStateFollowing, true
	case line.WebhookEventUnfollow:
		return FollowStateBlocked, true
	default:
		return "", false
	}
}
