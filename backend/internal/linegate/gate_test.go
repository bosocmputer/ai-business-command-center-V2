package linegate

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/line"
	"github.com/google/uuid"
)

const testSecret = "0123456789abcdef0123456789abcdef"

type fakeStore struct {
	mu        sync.Mutex
	recipient uuid.UUID
	shops     []Shop
	selected  map[uuid.UUID]uuid.UUID
	err       error
}

func (store *fakeStore) Shops(context.Context, []byte, time.Time) (uuid.UUID, []Shop, error) {
	return store.recipient, store.shops, store.err
}
func (store *fakeStore) Selected(_ context.Context, recipientID uuid.UUID) (uuid.UUID, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	tenantID, ok := store.selected[recipientID]
	return tenantID, ok, nil
}
func (store *fakeStore) Select(_ context.Context, recipientID, tenantID uuid.UUID, _ time.Time) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.selected[recipientID] = tenantID
	return nil
}

type fakeReplier struct{ replies []json.RawMessage }

func (replier *fakeReplier) Reply(_ context.Context, _ string, messages ...json.RawMessage) error {
	replier.replies = append(replier.replies, messages...)
	return nil
}

type sentWebhook struct {
	host      string
	body      []byte
	signature string
}

type fakeForwarder struct {
	sent []sentWebhook
	err  error
}

func (forwarder *fakeForwarder) Forward(_ context.Context, host string, body []byte, signature string) error {
	forwarder.sent = append(forwarder.sent, sentWebhook{host, body, signature})
	return forwarder.err
}

type fakeHasher struct{}

func (fakeHasher) HashToken(value string) []byte { return []byte(value) }

type timeoutError struct{}

func (timeoutError) Error() string   { return "timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

func newGate(store *fakeStore, forwarder *fakeForwarder) (*Gate, *fakeReplier) {
	replier := &fakeReplier{}
	return New(testSecret, store, fakeHasher{}, replier, forwarder, nil, func() time.Time { return time.Unix(1_800_000_000, 0) }), replier
}

func shopsOf(names ...string) []Shop {
	shops := make([]Shop, 0, len(names))
	for _, name := range names {
		shops = append(shops, Shop{TenantID: uuid.New(), Name: name, Host: "assistant-" + strings.ToLower(name)})
	}
	return shops
}

func textEvent(user, value string) string {
	return `{"destination":"Udest","events":[{"type":"message","timestamp":1,"replyToken":"reply-token-1","source":{"type":"user","userId":"` + user + `"},"message":{"type":"text","id":"1","text":"` + value + `"}}]}`
}

func postbackEvent(user, data string) string {
	return `{"destination":"Udest","events":[{"type":"postback","replyToken":"reply-token-2","source":{"type":"user","userId":"` + user + `"},"postback":{"data":"` + data + `"}}]}`
}

func replyText(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var message struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &message); err != nil {
		t.Fatal(err)
	}
	return message.Text
}

func forwardedText(t *testing.T, sent sentWebhook) string {
	t.Helper()
	webhook, err := line.ParseWebhook(sent.body)
	if err != nil || len(webhook.Events) != 1 || webhook.Events[0].Message == nil {
		t.Fatalf("forwarded body is not one message event: %v", err)
	}
	return webhook.Events[0].Message.Text
}

func TestSenderWithNoShopIsToldItIsNotEnabledAndNothingIsPassedOn(t *testing.T) {
	forwarder := &fakeForwarder{}
	gate, replier := newGate(&fakeStore{selected: map[uuid.UUID]uuid.UUID{}}, forwarder)
	gate.Process(context.Background(), []byte(textEvent("Ustranger", "hello")))
	if len(forwarder.sent) != 0 || len(replier.replies) != 1 || replyText(t, replier.replies[0]) != MessageNotEnabled {
		t.Fatalf("sent=%d replies=%d", len(forwarder.sent), len(replier.replies))
	}
}

func TestSingleShopPassesTheMessageOnSignedAndWithoutAPrefix(t *testing.T) {
	shops := shopsOf("A")
	forwarder := &fakeForwarder{}
	gate, replier := newGate(&fakeStore{recipient: uuid.New(), shops: shops, selected: map[uuid.UUID]uuid.UUID{}}, forwarder)
	gate.Process(context.Background(), []byte(textEvent("Uone", "ยอดขายวันนี้")))
	if len(forwarder.sent) != 1 || len(replier.replies) != 0 {
		t.Fatalf("sent=%d replies=%d", len(forwarder.sent), len(replier.replies))
	}
	sent := forwarder.sent[0]
	if sent.host != shops[0].Host || forwardedText(t, sent) != "ยอดขายวันนี้" {
		t.Fatalf("host=%q text=%q", sent.host, forwardedText(t, sent))
	}
	if !line.VerifyWebhookSignature(testSecret, sent.body, sent.signature) {
		t.Fatal("the assistant could not verify the re-signed body")
	}
}

func TestTwoShopsAskWhichOneBeforeAnythingIsPassedOn(t *testing.T) {
	forwarder := &fakeForwarder{}
	gate, replier := newGate(&fakeStore{recipient: uuid.New(), shops: shopsOf("A", "B"), selected: map[uuid.UUID]uuid.UUID{}}, forwarder)
	gate.Process(context.Background(), []byte(textEvent("Umany", "ยอดขาย")))
	if len(forwarder.sent) != 0 || len(replier.replies) != 1 {
		t.Fatalf("sent=%d replies=%d", len(forwarder.sent), len(replier.replies))
	}
	var message struct {
		QuickReply struct {
			Items []struct {
				Action struct{ Data string } `json:"action"`
			} `json:"items"`
		} `json:"quickReply"`
	}
	if err := json.Unmarshal(replier.replies[0], &message); err != nil || len(message.QuickReply.Items) != 2 || !strings.HasPrefix(message.QuickReply.Items[0].Action.Data, selectPrefix) {
		t.Fatalf("chooser is wrong: %s", replier.replies[0])
	}
}

func TestChoosingAShopIsKeptAndLaterQuestionsGoThereWithTheShopNamed(t *testing.T) {
	shops := shopsOf("A", "B")
	store := &fakeStore{recipient: uuid.New(), shops: shops, selected: map[uuid.UUID]uuid.UUID{}}
	forwarder := &fakeForwarder{}
	gate, replier := newGate(store, forwarder)
	gate.Process(context.Background(), []byte(postbackEvent("Umany", selectPrefix+shops[1].TenantID.String())))
	if store.selected[store.recipient] != shops[1].TenantID || len(replier.replies) != 1 || !strings.Contains(replyText(t, replier.replies[0]), "B") {
		t.Fatalf("selection not kept: %v", store.selected)
	}
	gate.Process(context.Background(), []byte(textEvent("Umany", "ยอดขาย")))
	if len(forwarder.sent) != 1 || forwarder.sent[0].host != shops[1].Host || forwardedText(t, forwarder.sent[0]) != "[ถามร้าน B] ยอดขาย" {
		t.Fatalf("sent=%+v", forwarder.sent)
	}
	if !line.VerifyWebhookSignature(testSecret, forwarder.sent[0].body, forwarder.sent[0].signature) {
		t.Fatal("re-signed body does not verify")
	}
}

func TestAShopTheSenderMayNotAskAboutCannotBeChosen(t *testing.T) {
	store := &fakeStore{recipient: uuid.New(), shops: shopsOf("A", "B"), selected: map[uuid.UUID]uuid.UUID{}}
	gate, replier := newGate(store, &fakeForwarder{})
	for _, data := range []string{selectPrefix + uuid.NewString(), selectPrefix + "not-a-uuid", "gate=other"} {
		gate.Process(context.Background(), []byte(postbackEvent("Umany", data)))
	}
	if len(store.selected) != 0 || len(replier.replies) != 3 || replyText(t, replier.replies[0]) != MessageSelectFail {
		t.Fatalf("selected=%v replies=%d", store.selected, len(replier.replies))
	}
}

func TestAStoredChoiceOfAShopNoLongerAllowedAsksAgain(t *testing.T) {
	store := &fakeStore{recipient: uuid.New(), shops: shopsOf("A", "B"), selected: map[uuid.UUID]uuid.UUID{}}
	store.selected[store.recipient] = uuid.New()
	forwarder := &fakeForwarder{}
	gate, replier := newGate(store, forwarder)
	gate.Process(context.Background(), []byte(textEvent("Umany", "ยอดขาย")))
	if len(forwarder.sent) != 0 || len(replier.replies) != 1 {
		t.Fatalf("sent=%d replies=%d", len(forwarder.sent), len(replier.replies))
	}
}

func TestSwitchWordShowsTheChoicesEvenWhenOneIsKept(t *testing.T) {
	shops := shopsOf("A", "B")
	store := &fakeStore{recipient: uuid.New(), shops: shops, selected: map[uuid.UUID]uuid.UUID{}}
	store.selected[store.recipient] = shops[0].TenantID
	forwarder := &fakeForwarder{}
	gate, replier := newGate(store, forwarder)
	gate.Process(context.Background(), []byte(textEvent("Umany", "เปลี่ยนร้าน")))
	if len(forwarder.sent) != 0 || len(replier.replies) != 1 || replyText(t, replier.replies[0]) != MessageChoose {
		t.Fatalf("sent=%d replies=%d", len(forwarder.sent), len(replier.replies))
	}
}

func TestOtherPostbacksGoToTheShopLikeMessages(t *testing.T) {
	shops := shopsOf("A")
	forwarder := &fakeForwarder{}
	gate, _ := newGate(&fakeStore{recipient: uuid.New(), shops: shops, selected: map[uuid.UUID]uuid.UUID{}}, forwarder)
	gate.Process(context.Background(), []byte(postbackEvent("Uone", "assistant-answer=abc")))
	if len(forwarder.sent) != 1 {
		t.Fatalf("sent=%d", len(forwarder.sent))
	}
}

func TestEventsThatAreNotPersonalMessagesAreIgnored(t *testing.T) {
	forwarder := &fakeForwarder{}
	gate, replier := newGate(&fakeStore{recipient: uuid.New(), shops: shopsOf("A"), selected: map[uuid.UUID]uuid.UUID{}}, forwarder)
	gate.Process(context.Background(), []byte(`{"destination":"U","events":[
		{"type":"follow","source":{"type":"user","userId":"Uone"}},
		{"type":"message","replyToken":"reply-token-9","source":{"type":"group","groupId":"G1","userId":"Uone"},"message":{"type":"text","text":"hi"}}]}`))
	gate.Process(context.Background(), []byte(`not json`))
	if len(forwarder.sent) != 0 || len(replier.replies) != 0 {
		t.Fatalf("sent=%d replies=%d", len(forwarder.sent), len(replier.replies))
	}
}

func TestAnAssistantThatIsDownGetsANoticeButASlowOneDoesNot(t *testing.T) {
	shops := shopsOf("A")
	store := &fakeStore{recipient: uuid.New(), shops: shops, selected: map[uuid.UUID]uuid.UUID{}}
	down := &fakeForwarder{err: errors.New("connection refused")}
	gate, replier := newGate(store, down)
	gate.Process(context.Background(), []byte(textEvent("Uone", "x")))
	if len(replier.replies) != 1 || replyText(t, replier.replies[0]) != MessageUnavailable {
		t.Fatalf("replies=%d", len(replier.replies))
	}
	slow := &fakeForwarder{err: timeoutError{}}
	gate, replier = newGate(store, slow)
	gate.Process(context.Background(), []byte(textEvent("Uone", "x")))
	if len(replier.replies) != 0 {
		t.Fatalf("a timeout must not be reported to the person: replies=%d", len(replier.replies))
	}
}

func TestAStoreFailureTellsThePersonToRetryAndPassesNothingOn(t *testing.T) {
	forwarder := &fakeForwarder{}
	gate, replier := newGate(&fakeStore{err: errors.New("db down"), selected: map[uuid.UUID]uuid.UUID{}}, forwarder)
	gate.Process(context.Background(), []byte(textEvent("Uone", "x")))
	if len(forwarder.sent) != 0 || len(replier.replies) != 1 || replyText(t, replier.replies[0]) != MessageUnavailable {
		t.Fatalf("sent=%d replies=%d", len(forwarder.sent), len(replier.replies))
	}
}

func TestChooserKeepsToLinesLimits(t *testing.T) {
	names := make([]string, 0, 20)
	for index := 0; index < 20; index++ {
		names = append(names, strings.Repeat("ร้านยาวมาก", 6))
	}
	var message struct {
		QuickReply struct {
			Items []struct {
				Action struct{ Label string } `json:"action"`
			} `json:"items"`
		} `json:"quickReply"`
	}
	if err := json.Unmarshal(chooser(shopsOf(names...)), &message); err != nil || len(message.QuickReply.Items) != maxChoices {
		t.Fatalf("items=%d err=%v", len(message.QuickReply.Items), err)
	}
	if label := message.QuickReply.Items[0].Action.Label; len([]rune(label)) != maxLabelRunes {
		t.Fatalf("label has %d characters", len([]rune(label)))
	}
}

func TestHTTPForwarderPostsTheSignedBodyAndReportsRefusals(t *testing.T) {
	var gotSignature, gotPath string
	status := http.StatusOK
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		gotSignature, gotPath = request.Header.Get("X-Line-Signature"), request.URL.Path
		response.WriteHeader(status)
	}))
	defer server.Close()
	forwarder, err := NewHTTPForwarder(strings.Replace(server.URL, "127.0.0.1", "{host}", 1) + "/line/webhook")
	if err != nil {
		t.Fatal(err)
	}
	if err := forwarder.Forward(context.Background(), "127.0.0.1", []byte(`{}`), "sig"); err != nil || gotSignature != "sig" || gotPath != "/line/webhook" {
		t.Fatalf("err=%v signature=%q path=%q", err, gotSignature, gotPath)
	}
	status = http.StatusUnauthorized
	if err := forwarder.Forward(context.Background(), "127.0.0.1", []byte(`{}`), "sig"); err == nil {
		t.Fatal("a refusal must be an error")
	}
	for _, bad := range []string{"http://nohost/line", "https://{host}/x", "http://{host}/{host}", "http://u:p@{host}/x"} {
		if _, err := NewHTTPForwarder(bad); err == nil {
			t.Fatalf("template %q must be refused", bad)
		}
	}
}
