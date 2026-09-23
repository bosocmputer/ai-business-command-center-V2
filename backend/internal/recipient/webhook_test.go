package recipient

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/line"
)

const testChannelSecret = "0123456789abcdef0123456789abcdef"

type recordedFollow struct {
	lineHash  string
	state     FollowState
	changedAt time.Time
}

type fakeFollowStore struct {
	records []recordedFollow
	err     error
}

func (store *fakeFollowStore) RecordFollowState(_ context.Context, lineHash []byte, state FollowState, changedAt, _ time.Time) (bool, error) {
	if store.err != nil {
		return false, store.err
	}
	store.records = append(store.records, recordedFollow{lineHash: string(lineHash), state: state, changedAt: changedAt})
	return true, nil
}

type prefixHasher struct{}

func (prefixHasher) HashToken(value string) []byte { return []byte("hash:" + value) }

func signedBody(body string) (string, []byte) {
	mac := hmac.New(sha256.New, []byte(testChannelSecret))
	_, _ = mac.Write([]byte(body))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil)), []byte(body)
}

func newTestWebhookService(store FollowStore, secret string) *WebhookService {
	fixed := time.Date(2026, 9, 23, 3, 0, 0, 0, time.UTC)
	return NewWebhookService(secret, store, prefixHasher{}, func() time.Time { return fixed })
}

func TestWebhookRecordsFollowAndUnfollowForUsers(t *testing.T) {
	store := &fakeFollowStore{}
	signature, body := signedBody(`{"destination":"Ubot","events":[
		{"type":"unfollow","timestamp":1790133315806,"source":{"type":"user","userId":"Ublocked"}},
		{"type":"follow","timestamp":0,"source":{"type":"user","userId":"Ufollow"}},
		{"type":"message","timestamp":1790133315806,"source":{"type":"user","userId":"Utalker"}},
		{"type":"unfollow","timestamp":1790133315806,"source":{"type":"group","userId":""}}
	]}`)

	if err := newTestWebhookService(store, testChannelSecret).HandleWebhook(context.Background(), body, signature); err != nil {
		t.Fatalf("handle webhook: %v", err)
	}
	if len(store.records) != 2 {
		t.Fatalf("recorded %d events, want follow and unfollow only: %+v", len(store.records), store.records)
	}
	blocked, followed := store.records[0], store.records[1]
	if blocked.lineHash != "hash:line-user:Ublocked" || blocked.state != FollowStateBlocked || !blocked.changedAt.Equal(time.UnixMilli(1790133315806)) {
		t.Fatalf("unexpected unfollow record: %+v", blocked)
	}
	if followed.state != FollowStateFollowing || !followed.changedAt.Equal(time.Date(2026, 9, 23, 3, 0, 0, 0, time.UTC)) {
		t.Fatalf("follow without timestamp must use the receive time: %+v", followed)
	}
}

func TestWebhookAcceptsLINEVerifyRequest(t *testing.T) {
	store := &fakeFollowStore{}
	signature, body := signedBody(`{"destination":"Ubot","events":[]}`)
	if err := newTestWebhookService(store, testChannelSecret).HandleWebhook(context.Background(), body, signature); err != nil {
		t.Fatalf("verify request rejected: %v", err)
	}
	if len(store.records) != 0 {
		t.Fatalf("verify request must not write: %+v", store.records)
	}
}

func TestWebhookRejectsBeforeTouchingTheStore(t *testing.T) {
	store := &fakeFollowStore{}
	signature, body := signedBody(`{"events":[{"type":"unfollow","source":{"type":"user","userId":"U1"}}]}`)

	if err := newTestWebhookService(store, "").HandleWebhook(context.Background(), body, signature); !errors.Is(err, ErrWebhookNotConfigured) {
		t.Fatalf("missing secret error = %v", err)
	}
	if err := newTestWebhookService(store, testChannelSecret).HandleWebhook(context.Background(), body, "c2lnbmF0dXJl"); !errors.Is(err, ErrWebhookSignatureInvalid) {
		t.Fatalf("bad signature error = %v", err)
	}
	badSignature, badBody := signedBody(`not json`)
	if err := newTestWebhookService(store, testChannelSecret).HandleWebhook(context.Background(), badBody, badSignature); !errors.Is(err, line.ErrWebhookPayloadInvalid) {
		t.Fatalf("bad payload error = %v", err)
	}
	if len(store.records) != 0 {
		t.Fatalf("rejected deliveries must not write: %+v", store.records)
	}
}

func TestWebhookSurfacesStoreFailureSoLINERetries(t *testing.T) {
	store := &fakeFollowStore{err: errors.New("database down")}
	signature, body := signedBody(`{"events":[{"type":"follow","timestamp":1,"source":{"type":"user","userId":"U1"}}]}`)
	if err := newTestWebhookService(store, testChannelSecret).HandleWebhook(context.Background(), body, signature); err == nil {
		t.Fatal("store failure was swallowed")
	}
}
