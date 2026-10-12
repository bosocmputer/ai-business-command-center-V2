package provision

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/agent"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/assistantcfg"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/secret"
	"github.com/google/uuid"
)

type fakeStore struct {
	rows     map[uuid.UUID]Stored
	upserts  int
	desired  []string
	cleared  []string
	reported []string
}

func newFakeStore() *fakeStore { return &fakeStore{rows: map[uuid.UUID]Stored{}} }

func (store *fakeStore) Get(_ context.Context, tenantID uuid.UUID) (Stored, error) {
	row, ok := store.rows[tenantID]
	if !ok {
		return Stored{}, ErrNotFound
	}
	return row, nil
}
func (store *fakeStore) GetByName(_ context.Context, name string) (Stored, error) {
	for _, row := range store.rows {
		if row.Name == name {
			return row, nil
		}
	}
	return Stored{}, ErrNotFound
}
func (store *fakeStore) Upsert(_ context.Context, _ []byte, _ string, row Stored, _ time.Time) error {
	store.upserts++
	row.TokenWaiting = row.Token != nil
	store.rows[row.TenantID] = row
	return nil
}
func (store *fakeStore) SetDesired(_ context.Context, _ []byte, _ string, tenantID uuid.UUID, desired string, status Status, _ time.Time) error {
	row, ok := store.rows[tenantID]
	if !ok {
		return ErrNotFound
	}
	row.Desired, row.Status = desired, status
	if desired == DesiredStopped {
		row.Token, row.TokenWaiting = nil, false
	}
	store.rows[tenantID] = row
	store.desired = append(store.desired, desired+"/"+string(status))
	return nil
}
func (store *fakeStore) Pending(context.Context) ([]Work, error) {
	var work []Work
	for _, row := range store.rows {
		switch {
		case row.Desired == DesiredRunning && (row.Status == StatusPending || row.Status == StatusCreating):
			work = append(work, Work{Name: row.Name, TenantID: row.TenantID, Action: "start"})
		case row.Desired == DesiredStopped && row.Status == StatusStopping:
			work = append(work, Work{Name: row.Name, TenantID: row.TenantID, Action: "stop"})
		}
	}
	return work, nil
}
func (store *fakeStore) Report(_ context.Context, name string, status Status, code string, _ time.Time) error {
	for id, row := range store.rows {
		if row.Name == name {
			row.Status, row.StatusCode = status, code
			store.rows[id] = row
			store.reported = append(store.reported, string(status)+"/"+code)
			return nil
		}
	}
	return ErrNotFound
}
func (store *fakeStore) ClearToken(_ context.Context, name string, _ time.Time) error {
	for id, row := range store.rows {
		if row.Name == name {
			row.Token, row.TokenWaiting = nil, false
			store.rows[id] = row
			store.cleared = append(store.cleared, name)
		}
	}
	return nil
}
func (store *fakeStore) Adopt(_ context.Context, name string, tenantID uuid.UUID, _ time.Time) error {
	store.rows[tenantID] = Stored{Instance: Instance{TenantID: tenantID, Name: name, Host: HostFor(name), Desired: DesiredRunning, Status: StatusRunning}}
	return nil
}

type fakeIssuer struct {
	err    error
	issued int
}

const issuedToken = "abcc_TESTTOKEN0123456789"

func (issuer *fakeIssuer) IssueToken(_ context.Context, _ []byte, _ string, _, _ uuid.UUID, _ bool) (agent.IssuedToken, error) {
	if issuer.err != nil {
		return agent.IssuedToken{}, issuer.err
	}
	issuer.issued++
	return agent.IssuedToken{Token: issuedToken}, nil
}

type fakeSettings struct {
	view    assistantcfg.AdminView
	updates []string
}

func (settings *fakeSettings) View(context.Context, uuid.UUID) (assistantcfg.AdminView, error) {
	return settings.view, nil
}
func (settings *fakeSettings) Update(_ context.Context, _ []byte, _ string, _ uuid.UUID, input assistantcfg.UpdateInput) (assistantcfg.AdminView, error) {
	if input.AssistantHost != nil {
		settings.view.AssistantHost = *input.AssistantHost
		settings.updates = append(settings.updates, *input.AssistantHost)
	}
	return settings.view, nil
}

func newTestService(t *testing.T) (*Service, *fakeStore, *fakeIssuer, *fakeSettings) {
	t.Helper()
	box, err := secret.NewBox(bytes.Repeat([]byte{7}, 32), "test-key", bytes.NewReader(bytes.Repeat([]byte{9}, 4096)))
	if err != nil {
		t.Fatal(err)
	}
	store, issuer, settings := newFakeStore(), &fakeIssuer{}, &fakeSettings{}
	return NewService(store, box, issuer, settings, func() time.Time { return time.Date(2026, 10, 12, 5, 0, 0, 0, time.UTC) }), store, issuer, settings
}

var (
	tenantID  = uuid.MustParse("9d36e282-0355-4caf-91e0-daf0857c935b")
	recipient = uuid.New()
	actor     = []byte("actor")
)

func TestCreatingAnInstanceSetsTheHostSealsTheTokenAndWaitsForTheServer(t *testing.T) {
	service, store, issuer, settings := newTestService(t)
	instance, err := service.Create(context.Background(), actor, "r", tenantID, recipient, true)
	if err != nil {
		t.Fatal(err)
	}
	if instance.Name != "s9d36e282" || instance.Host != "assistant-s9d36e282" || instance.Status != StatusPending || !instance.TokenWaiting {
		t.Fatalf("instance = %+v", instance)
	}
	if len(settings.updates) != 1 || settings.updates[0] != "assistant-s9d36e282" || issuer.issued != 1 {
		t.Fatalf("host updates %v, tokens issued %d", settings.updates, issuer.issued)
	}
	sealed := store.rows[tenantID].Token
	if sealed == nil || strings.Contains(string(sealed.Ciphertext), "TESTTOKEN") {
		t.Fatal("the token must be kept sealed")
	}
	view, err := service.View(context.Background(), tenantID)
	if err != nil || strings.Contains(strings.ToLower(view.Name+view.Host+view.StatusCode), "abcc_") {
		t.Fatalf("the admin view must never carry the token: %+v %v", view, err)
	}
	work, _ := service.Pending(context.Background())
	if len(work) != 1 || work[0].Action != "start" || work[0].Name != "s9d36e282" {
		t.Fatalf("work = %+v", work)
	}
}

func TestTheServerGetsTheTokenOnceThenAcknowledgesAndItIsGone(t *testing.T) {
	service, _, _, _ := newTestService(t)
	ctx := context.Background()
	if _, err := service.Create(ctx, actor, "r", tenantID, recipient, false); err != nil {
		t.Fatal(err)
	}
	token, err := service.Token(ctx, "s9d36e282")
	if err != nil || token != issuedToken {
		t.Fatalf("token = %q %v", token, err)
	}
	if err := service.Report(ctx, "s9d36e282", StatusCreating, ""); err != nil {
		t.Fatal(err)
	}
	if err := service.Report(ctx, "s9d36e282", StatusRunning, ""); err != nil {
		t.Fatal(err)
	}
	if err := service.Ack(ctx, "s9d36e282"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Token(ctx, "s9d36e282"); !errors.Is(err, ErrNoToken) {
		t.Fatalf("after the acknowledgement no token may remain: %v", err)
	}
	view, _ := service.View(ctx, tenantID)
	if view.Status != StatusRunning || view.TokenWaiting {
		t.Fatalf("view = %+v", view)
	}
	if work, _ := service.Pending(ctx); len(work) != 0 {
		t.Fatalf("a running instance has no work: %+v", work)
	}
}

func TestAShopThatAlreadyHasAHandMadeAssistantIsNotTakenOver(t *testing.T) {
	service, store, issuer, settings := newTestService(t)
	settings.view.AssistantHost = "assistant"
	if _, err := service.Create(context.Background(), actor, "r", tenantID, recipient, true); !errors.Is(err, ErrHasManualAssistant) {
		t.Fatalf("err = %v", err)
	}
	if issuer.issued != 0 || store.upserts != 0 || len(settings.updates) != 0 {
		t.Fatal("nothing may be changed: a new token would revoke the running assistant's token")
	}
}

func TestAnInstanceThatIsRunningCannotBeCreatedAgainButAFailedOrStoppedOneCanBeStarted(t *testing.T) {
	service, store, issuer, _ := newTestService(t)
	ctx := context.Background()
	if _, err := service.Create(ctx, actor, "r", tenantID, recipient, true); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(ctx, actor, "r", tenantID, recipient, true); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("a second create while it is wanted: %v", err)
	}
	_ = service.Report(ctx, "s9d36e282", StatusError, "START_TIMEOUT")
	if _, err := service.Create(ctx, actor, "r", tenantID, recipient, true); err != nil {
		t.Fatalf("a failed one may be tried again: %v", err)
	}
	if store.rows[tenantID].Status != StatusPending || store.rows[tenantID].StatusCode != "" || issuer.issued != 2 {
		t.Fatalf("row = %+v issued %d", store.rows[tenantID], issuer.issued)
	}
	if _, err := service.Stop(ctx, actor, "r", tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(ctx, actor, "r", tenantID, recipient, true); err != nil {
		t.Fatalf("a stopped one may be started again: %v", err)
	}
}

func TestStoppingAnInstanceThatNeverStartedNeedsNoWorkFromTheServer(t *testing.T) {
	service, store, _, _ := newTestService(t)
	ctx := context.Background()
	if _, err := service.Create(ctx, actor, "r", tenantID, recipient, true); err != nil {
		t.Fatal(err)
	}
	stopped, err := service.Stop(ctx, actor, "r", tenantID)
	if err != nil || stopped.Status != StatusStopped || stopped.TokenWaiting {
		t.Fatalf("stopped = %+v %v", stopped, err)
	}
	if work, _ := service.Pending(ctx); len(work) != 0 {
		t.Fatal("nothing to stop on the server")
	}
	// A running one needs the server to stop it, and loses any token still waiting.
	_ = service.Report(ctx, "s9d36e282", StatusRunning, "")
	store.rows[tenantID] = Stored{Instance: Instance{TenantID: tenantID, Name: "s9d36e282", Host: "assistant-s9d36e282", Desired: DesiredRunning, Status: StatusRunning}}
	if running, err := service.Stop(ctx, actor, "r", tenantID); err != nil || running.Status != StatusStopping {
		t.Fatalf("running one: %+v %v", running, err)
	}
	if work, _ := service.Pending(ctx); len(work) != 1 || work[0].Action != "stop" {
		t.Fatalf("work = %+v", work)
	}
}

func TestOnlyTheStatesAndCodesTheServerMayReportAreAccepted(t *testing.T) {
	service, _, _, _ := newTestService(t)
	ctx := context.Background()
	_, _ = service.Create(ctx, actor, "r", tenantID, recipient, true)
	for _, bad := range []Status{StatusPending, StatusStopping, "WHATEVER"} {
		var invalid *ValidationError
		if err := service.Report(ctx, "s9d36e282", bad, ""); !errors.As(err, &invalid) {
			t.Fatalf("%s must be refused: %v", bad, err)
		}
	}
	var invalid *ValidationError
	if err := service.Report(ctx, "s9d36e282", StatusError, "has spaces and Secret=abc"); !errors.As(err, &invalid) {
		t.Fatalf("free text must be refused: %v", err)
	}
	if err := service.Report(ctx, "nobody", StatusRunning, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown instance: %v", err)
	}
}

func TestAnIssuingFailureLeavesNoInstanceBehind(t *testing.T) {
	service, store, issuer, _ := newTestService(t)
	issuer.err = agent.ErrAIChatDisabled
	if _, err := service.Create(context.Background(), actor, "r", tenantID, recipient, true); !errors.Is(err, agent.ErrAIChatDisabled) {
		t.Fatalf("err = %v", err)
	}
	if store.upserts != 0 {
		t.Fatal("no row without a token")
	}
	if _, err := service.Create(context.Background(), actor, "r", tenantID, uuid.Nil, true); err == nil {
		t.Fatal("a token holder is required")
	}
}
