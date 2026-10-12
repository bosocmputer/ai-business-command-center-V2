// Package provision lets an admin create a shop's assistant from the admin page. AI-BCC keeps what should run (a row per shop) and the
// sealed token the new assistant needs; a small script on the server, started every minute, creates and starts the containers and says
// how it went. Nothing here can start a container: the web application has no control over the server.
package provision

import (
	"context"
	"errors"
	"regexp"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/agent"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/assistantcfg"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/secret"
	"github.com/google/uuid"
)

type Status string

const (
	StatusPending  Status = "PENDING"
	StatusCreating Status = "CREATING"
	StatusRunning  Status = "RUNNING"
	StatusStopping Status = "STOPPING"
	StatusStopped  Status = "STOPPED"
	StatusError    Status = "ERROR"

	DesiredRunning = "RUNNING"
	DesiredStopped = "STOPPED"

	hostPrefix = "assistant-"
)

var (
	ErrNotFound      = errors.New("no assistant instance for this shop")
	ErrAlreadyExists = errors.New("this shop already has an assistant instance")
	// ErrHasManualAssistant means the shop already points at an assistant that was set up by hand; creating another would take over its token.
	ErrHasManualAssistant = errors.New("this shop already has an assistant that was set up by hand")
	ErrNoToken            = errors.New("no token is waiting for this instance")
)

type ValidationError struct{ Field, Code string }

func (err *ValidationError) Error() string { return "invalid " + err.Field + ": " + err.Code }

// Instance is what the admin page shows. It never carries the token.
type Instance struct {
	TenantID     uuid.UUID  `json:"tenantId"`
	Name         string     `json:"name"`
	Host         string     `json:"host"`
	Desired      string     `json:"desired"`
	Status       Status     `json:"status"`
	StatusCode   string     `json:"statusCode,omitempty"`
	RecipientID  *uuid.UUID `json:"recipientId,omitempty"`
	TokenWaiting bool       `json:"tokenWaiting"`
	UpdatedAt    time.Time  `json:"updatedAt"`
	ReportedAt   *time.Time `json:"reportedAt,omitempty"`
}

// Stored is a row as the store keeps it.
type Stored struct {
	Instance
	Token *secret.Sealed
}

// Work is one thing the server has to do.
type Work struct {
	Name     string
	TenantID uuid.UUID
	Action   string // "start" or "stop"
}

type Store interface {
	Get(ctx context.Context, tenantID uuid.UUID) (Stored, error)
	GetByName(ctx context.Context, name string) (Stored, error)
	// Upsert writes the row of a shop (a new one, or a stopped or failed one being started again) and audits it.
	Upsert(ctx context.Context, actorHash []byte, requestID string, row Stored, now time.Time) error
	SetDesired(ctx context.Context, actorHash []byte, requestID string, tenantID uuid.UUID, desired string, status Status, now time.Time) error
	Pending(ctx context.Context) ([]Work, error)
	Report(ctx context.Context, name string, status Status, code string, now time.Time) error
	ClearToken(ctx context.Context, name string, now time.Time) error
	Adopt(ctx context.Context, name string, tenantID uuid.UUID, now time.Time) error
}

type TokenIssuer interface {
	IssueToken(ctx context.Context, actorHash []byte, requestID string, tenantID, recipientID uuid.UUID, namesVisible bool) (agent.IssuedToken, error)
}

type Settings interface {
	View(ctx context.Context, tenantID uuid.UUID) (assistantcfg.AdminView, error)
	Update(ctx context.Context, actorHash []byte, requestID string, tenantID uuid.UUID, input assistantcfg.UpdateInput) (assistantcfg.AdminView, error)
}

type Service struct {
	store    Store
	box      *secret.Box
	issuer   TokenIssuer
	settings Settings
	now      func() time.Time
}

func NewService(store Store, box *secret.Box, issuer TokenIssuer, settings Settings, now func() time.Time) *Service {
	return &Service{store: store, box: box, issuer: issuer, settings: settings, now: now}
}

// NameFor is the instance name of a shop: short, lower case, and the same every time.
func NameFor(tenantID uuid.UUID) string { return "s" + tenantID.String()[:8] }

func HostFor(name string) string { return hostPrefix + name }

func aad(tenantID uuid.UUID) []byte { return []byte(tenantID.String() + ":provision:token") }

// View is the state for the admin page; ErrNotFound when the shop has no instance (yet).
func (service *Service) View(ctx context.Context, tenantID uuid.UUID) (Instance, error) {
	row, err := service.store.Get(ctx, tenantID)
	return row.Instance, err
}

// Overview is what the admin page needs about a shop's assistant machine: the instance, or the host of an assistant someone set up by hand.
type Overview struct {
	Instance   *Instance `json:"instance"`
	ManualHost string    `json:"manualHost"`
}

func (service *Service) Overview(ctx context.Context, tenantID uuid.UUID) (Overview, error) {
	row, err := service.store.Get(ctx, tenantID)
	if err == nil {
		instance := row.Instance
		return Overview{Instance: &instance}, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Overview{}, err
	}
	view, err := service.settings.View(ctx, tenantID)
	if err != nil {
		return Overview{}, err
	}
	return Overview{ManualHost: view.AssistantHost}, nil
}

// Create (or start again) the assistant of a shop. The admin names who holds the assistant's token: the assistant answers with that
// person's report permissions, so it must be someone with the assistant switched on. The token is issued here, sealed, and handed to the
// server script; no one is shown it.
func (service *Service) Create(ctx context.Context, actorHash []byte, requestID string, tenantID, recipientID uuid.UUID, namesVisible bool) (Instance, error) {
	if recipientID == uuid.Nil {
		return Instance{}, &ValidationError{Field: "recipientId", Code: "REQUIRED"}
	}
	name, host := NameFor(tenantID), HostFor(NameFor(tenantID))
	existing, err := service.store.Get(ctx, tenantID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return Instance{}, err
	}
	if err == nil && existing.Desired == DesiredRunning && existing.Status != StatusError {
		return Instance{}, ErrAlreadyExists
	}
	view, err := service.settings.View(ctx, tenantID)
	if err != nil {
		return Instance{}, err
	}
	if view.AssistantHost != "" && view.AssistantHost != host {
		return Instance{}, ErrHasManualAssistant
	}
	if view.AssistantHost != host {
		if _, err := service.settings.Update(ctx, actorHash, requestID, tenantID, assistantcfg.UpdateInput{AssistantHost: &host, Version: view.Version}); err != nil {
			return Instance{}, err
		}
	}
	issued, err := service.issuer.IssueToken(ctx, actorHash, requestID, tenantID, recipientID, namesVisible)
	if err != nil {
		return Instance{}, err
	}
	sealed, err := service.box.Encrypt([]byte(issued.Token), aad(tenantID))
	if err != nil {
		return Instance{}, errors.New("seal the assistant token")
	}
	now := service.now().UTC()
	row := Stored{Instance: Instance{TenantID: tenantID, Name: name, Host: host, Desired: DesiredRunning, Status: StatusPending, RecipientID: &recipientID, TokenWaiting: true, UpdatedAt: now}, Token: &sealed}
	if err := service.store.Upsert(ctx, actorHash, requestID, row, now); err != nil {
		return Instance{}, err
	}
	return row.Instance, nil
}

// Stop asks the server to stop the shop's assistant. Its volume and settings stay, so it can be started again.
func (service *Service) Stop(ctx context.Context, actorHash []byte, requestID string, tenantID uuid.UUID) (Instance, error) {
	row, err := service.store.Get(ctx, tenantID)
	if err != nil {
		return Instance{}, err
	}
	status := StatusStopping
	if row.Status == StatusPending || row.Status == StatusStopped {
		status = StatusStopped // the server never started it (or already stopped it)
	}
	if err := service.store.SetDesired(ctx, actorHash, requestID, tenantID, DesiredStopped, status, service.now().UTC()); err != nil {
		return Instance{}, err
	}
	return service.View(ctx, tenantID)
}

// ---- what the server script uses --------------------------------------------------------------------------------------------

// Pending lists what the server has to start or stop.
func (service *Service) Pending(ctx context.Context) ([]Work, error) {
	return service.store.Pending(ctx)
}

// Token gives the assistant token waiting for an instance, once the server asks for it to write it into the assistant's secrets file.
func (service *Service) Token(ctx context.Context, name string) (string, error) {
	row, err := service.store.GetByName(ctx, name)
	if err != nil {
		return "", err
	}
	if row.Token == nil || row.Desired != DesiredRunning {
		return "", ErrNoToken
	}
	plain, err := service.box.Decrypt(*row.Token, aad(row.TenantID))
	if err != nil {
		return "", errors.New("open the assistant token")
	}
	return string(plain), nil
}

var safeCode = regexp.MustCompile(`^[A-Z0-9_]{0,40}$`)

// Report keeps what the server says it did. Only the states the server may report are accepted, and the code is a short upper-case word.
func (service *Service) Report(ctx context.Context, name string, status Status, code string) error {
	switch status {
	case StatusCreating, StatusRunning, StatusStopped, StatusError:
	default:
		return &ValidationError{Field: "status", Code: "INVALID"}
	}
	if !safeCode.MatchString(code) {
		return &ValidationError{Field: "code", Code: "INVALID"}
	}
	return service.store.Report(ctx, name, status, code, service.now().UTC())
}

// Ack removes the sealed token once the server has written it, so it does not stay in the database.
func (service *Service) Ack(ctx context.Context, name string) error {
	return service.store.ClearToken(ctx, name, service.now().UTC())
}

// Adopt records an assistant that was set up by hand as running, so the admin page shows it. No token is kept.
func (service *Service) Adopt(ctx context.Context, name string, tenantID uuid.UUID) error {
	return service.store.Adopt(ctx, name, tenantID, service.now().UTC())
}
