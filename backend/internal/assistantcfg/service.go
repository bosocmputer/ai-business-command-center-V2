package assistantcfg

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/secret"
	"github.com/google/uuid"
)

var (
	ErrNotConfigured   = errors.New("assistant settings are not configured")
	ErrVersionConflict = errors.New("assistant settings version conflict")
	ErrTenantNotFound  = errors.New("shop not found")
)

// ValidationError names the field and a stable code. It never carries the value that was refused.
type ValidationError struct{ Field, Code string }

func (err *ValidationError) Error() string { return err.Field + ": " + err.Code }

// Field is one of the four secrets of a shop.
type Field string

const (
	FieldOpenRouterKey Field = "openrouter-key"
	FieldTelegramToken Field = "telegram-bot-token"
	FieldLineSecret    Field = "line-channel-secret"
	FieldLineToken     Field = "line-channel-token"
)

func ParseField(raw string) (Field, bool) {
	switch field := Field(raw); field {
	case FieldOpenRouterKey, FieldTelegramToken, FieldLineSecret, FieldLineToken:
		return field, true
	}
	return "", false
}

const (
	LineNone    = "NONE"
	LineCentral = "CENTRAL"
	LineOwn     = "OWN"
)

type Stored struct {
	TenantID        uuid.UUID
	Enabled, IsTest bool
	ModelKey        string
	LineMode        string
	AssistantHost   string
	OpenRouterKey   *secret.Sealed
	OpenRouterLast4 string
	TelegramToken   *secret.Sealed
	LineSecret      *secret.Sealed
	LineToken       *secret.Sealed
	Version         int
	ConfigVersion   int64
	UpdatedAt       time.Time
}

type StoredGlobal struct {
	LineSecret, LineToken *secret.Sealed
	ConfigVersion         int64
	UpdatedAt             time.Time
}

type Patch struct {
	Enabled, IsTest    *bool
	ModelKey, LineMode *string
	AssistantHost      *string
}

// TenantGate says whether the shop may use the assistant today: switched on and not past its end date.
type TenantGate struct {
	Name   string
	Active bool
}

type Status struct {
	AppliedConfigVersion int64
	ReportedModelKey     string
	StartedAt            *time.Time
	LastSeenAt           time.Time
	LastErrorCode        string
}

type Store interface {
	Get(ctx context.Context, tenantID uuid.UUID) (Stored, error)
	Patch(ctx context.Context, actorHash []byte, requestID string, tenantID uuid.UUID, patch Patch, expectedVersion int, now time.Time) (Stored, error)
	PutSecret(ctx context.Context, actorHash []byte, requestID string, tenantID uuid.UUID, field Field, sealed *secret.Sealed, last4 string, now time.Time) (Stored, error)
	GetGlobal(ctx context.Context) (StoredGlobal, error)
	PutGlobalSecret(ctx context.Context, actorHash []byte, requestID string, field Field, sealed *secret.Sealed, now time.Time) (StoredGlobal, error)
	Gate(ctx context.Context, tenantID uuid.UUID, now time.Time) (TenantGate, error)
	RecordStatus(ctx context.Context, tenantID uuid.UUID, status Status, now time.Time) error
	GetStatus(ctx context.Context, tenantID uuid.UUID) (Status, error)
}

type Service struct {
	store Store
	box   *secret.Box
	now   func() time.Time
}

func NewService(store Store, box *secret.Box, now func() time.Time) *Service {
	return &Service{store: store, box: box, now: now}
}

// ---- views -----------------------------------------------------------------

type SecretState struct {
	IsSet bool   `json:"isSet"`
	Last4 string `json:"last4,omitempty"`
}

type ModelView struct {
	Model
	Selectable bool `json:"selectable"`
}

type StatusView struct {
	AppliedConfigVersion int64      `json:"appliedConfigVersion"`
	ReportedModelKey     string     `json:"reportedModelKey,omitempty"`
	StartedAt            *time.Time `json:"startedAt,omitempty"`
	LastSeenAt           time.Time  `json:"lastSeenAt"`
	LastErrorCode        string     `json:"lastErrorCode,omitempty"`
	UpToDate             bool       `json:"upToDate"`
}

// AdminView is everything the admin page shows. It holds no secret: a secret is "set" or "not set", and an OpenRouter key shows its
// last four characters.
type AdminView struct {
	Enabled               bool                   `json:"enabled"`
	IsTest                bool                   `json:"isTest"`
	ModelKey              string                 `json:"modelKey"`
	LineMode              string                 `json:"lineMode"`
	AssistantHost         string                 `json:"assistantHost"`
	Secrets               map[string]SecretState `json:"secrets"`
	CentralLineConfigured bool                   `json:"centralLineConfigured"`
	Version               int                    `json:"version"`
	ConfigVersion         int64                  `json:"configVersion"`
	Models                []ModelView            `json:"models"`
	Status                *StatusView            `json:"status,omitempty"`
}

type GlobalView struct {
	LineChannelSecret SecretState `json:"lineChannelSecret"`
	LineChannelToken  SecretState `json:"lineChannelToken"`
	ConfigVersion     int64       `json:"configVersion"`
}

func defaults(tenantID uuid.UUID) Stored {
	return Stored{TenantID: tenantID, ModelKey: DefaultModelKey, LineMode: LineNone, Version: 0}
}

func (service *Service) load(ctx context.Context, tenantID uuid.UUID) (Stored, error) {
	stored, err := service.store.Get(ctx, tenantID)
	if errors.Is(err, ErrNotConfigured) {
		return defaults(tenantID), nil
	}
	return stored, err
}

// ensureTenant refuses a shop that does not exist, so a mistyped id is a 404 and not a database error.
func (service *Service) ensureTenant(ctx context.Context, tenantID uuid.UUID) error {
	gate, err := service.store.Gate(ctx, tenantID, service.now().UTC())
	if err != nil {
		return err
	}
	if gate.Name == "" {
		return ErrTenantNotFound
	}
	return nil
}

func (service *Service) View(ctx context.Context, tenantID uuid.UUID) (AdminView, error) {
	if err := service.ensureTenant(ctx, tenantID); err != nil {
		return AdminView{}, err
	}
	stored, err := service.load(ctx, tenantID)
	if err != nil {
		return AdminView{}, err
	}
	return service.view(ctx, stored)
}

func (service *Service) view(ctx context.Context, stored Stored) (AdminView, error) {
	global, err := service.store.GetGlobal(ctx)
	if err != nil {
		return AdminView{}, err
	}
	view := AdminView{
		Enabled: stored.Enabled, IsTest: stored.IsTest, ModelKey: stored.ModelKey, LineMode: stored.LineMode, AssistantHost: stored.AssistantHost,
		Secrets: map[string]SecretState{
			"openrouterKey":     {IsSet: stored.OpenRouterKey != nil, Last4: stored.OpenRouterLast4},
			"telegramBotToken":  {IsSet: stored.TelegramToken != nil},
			"lineChannelSecret": {IsSet: stored.LineSecret != nil},
			"lineChannelToken":  {IsSet: stored.LineToken != nil},
		},
		CentralLineConfigured: global.LineSecret != nil && global.LineToken != nil,
		Version:               stored.Version, ConfigVersion: stored.ConfigVersion + global.ConfigVersion,
	}
	for _, model := range catalog {
		view.Models = append(view.Models, ModelView{Model: model, Selectable: model.Selectable(stored.IsTest)})
	}
	if stored.Version > 0 {
		status, statusErr := service.store.GetStatus(ctx, stored.TenantID)
		if statusErr == nil {
			view.Status = &StatusView{
				AppliedConfigVersion: status.AppliedConfigVersion, ReportedModelKey: status.ReportedModelKey, StartedAt: status.StartedAt,
				LastSeenAt: status.LastSeenAt, LastErrorCode: status.LastErrorCode, UpToDate: status.AppliedConfigVersion == view.ConfigVersion,
			}
		} else if !errors.Is(statusErr, ErrNotConfigured) {
			return AdminView{}, statusErr
		}
	}
	return view, nil
}

// ---- changing settings -------------------------------------------------------

type UpdateInput struct {
	Enabled, IsTest    *bool
	ModelKey, LineMode *string
	AssistantHost      *string
	Version            int
}

func (service *Service) Update(ctx context.Context, actorHash []byte, requestID string, tenantID uuid.UUID, input UpdateInput) (AdminView, error) {
	if err := service.ensureTenant(ctx, tenantID); err != nil {
		return AdminView{}, err
	}
	current, err := service.load(ctx, tenantID)
	if err != nil {
		return AdminView{}, err
	}
	next := current
	if input.Enabled != nil {
		next.Enabled = *input.Enabled
	}
	if input.IsTest != nil {
		next.IsTest = *input.IsTest
	}
	if input.ModelKey != nil {
		next.ModelKey = strings.TrimSpace(*input.ModelKey)
	}
	if input.LineMode != nil {
		next.LineMode = strings.TrimSpace(*input.LineMode)
	}
	if input.AssistantHost != nil {
		next.AssistantHost = strings.TrimSpace(*input.AssistantHost)
		if next.AssistantHost != "" && !hostPattern.MatchString(next.AssistantHost) {
			return AdminView{}, &ValidationError{Field: "assistantHost", Code: "INVALID_HOST"}
		}
	}
	model, ok := ModelFor(next.ModelKey)
	if !ok {
		return AdminView{}, &ValidationError{Field: "modelKey", Code: "UNKNOWN_MODEL"}
	}
	if !model.Selectable(next.IsTest) {
		return AdminView{}, &ValidationError{Field: "modelKey", Code: "MODEL_FOR_TEST_SHOPS_ONLY"}
	}
	if next.LineMode != LineNone && next.LineMode != LineCentral && next.LineMode != LineOwn {
		return AdminView{}, &ValidationError{Field: "lineMode", Code: "UNKNOWN_LINE_MODE"}
	}
	if next.Enabled {
		global, err := service.store.GetGlobal(ctx)
		if err != nil {
			return AdminView{}, err
		}
		switch {
		case next.OpenRouterKey == nil:
			return AdminView{}, &ValidationError{Field: "enabled", Code: "OPENROUTER_KEY_REQUIRED"}
		case next.LineMode == LineOwn && (next.LineSecret == nil || next.LineToken == nil):
			return AdminView{}, &ValidationError{Field: "lineMode", Code: "LINE_OWN_INCOMPLETE"}
		case next.LineMode == LineCentral && (global.LineSecret == nil || global.LineToken == nil):
			return AdminView{}, &ValidationError{Field: "lineMode", Code: "LINE_CENTRAL_NOT_CONFIGURED"}
		}
	}
	stored, err := service.store.Patch(ctx, actorHash, requestID, tenantID, Patch{Enabled: &next.Enabled, IsTest: &next.IsTest, ModelKey: &next.ModelKey, LineMode: &next.LineMode, AssistantHost: &next.AssistantHost}, input.Version, service.now().UTC())
	if err != nil {
		return AdminView{}, err
	}
	return service.view(ctx, stored)
}

// hostPattern is one DNS label: it can only name a service on the internal network, never another site.
var hostPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,60}[a-z0-9])?$`)

var (
	telegramTokenPattern = regexp.MustCompile(`^\d{6,12}:[A-Za-z0-9_-]{30,50}$`)
	lineSecretPattern    = regexp.MustCompile(`^[0-9a-f]{32}$`)
	openRouterPattern    = regexp.MustCompile(`^sk-or-[A-Za-z0-9_-]{14,190}$`)
)

// ValidateSecret checks a value's shape before it is sealed. The refusal names the field and a code, never the value.
func ValidateSecret(field Field, value string) error {
	bad := func() error { return &ValidationError{Field: string(field), Code: "INVALID_FORMAT"} }
	if value == "" || value != strings.TrimSpace(value) || strings.ContainsAny(value, " \t\r\n\x00") {
		return bad()
	}
	switch field {
	case FieldOpenRouterKey:
		if !openRouterPattern.MatchString(value) {
			return bad()
		}
	case FieldTelegramToken:
		if !telegramTokenPattern.MatchString(value) {
			return bad()
		}
	case FieldLineSecret:
		if !lineSecretPattern.MatchString(value) {
			return bad()
		}
	case FieldLineToken:
		if len(value) < 32 || len(value) > 4096 {
			return bad()
		}
	default:
		return bad()
	}
	return nil
}

func aad(scope string, field Field) []byte { return []byte(scope + ":assistant:" + string(field)) }

func last4Of(field Field, value string) string {
	if field == FieldOpenRouterKey && len(value) >= 4 {
		return value[len(value)-4:]
	}
	return ""
}

func (service *Service) SetSecret(ctx context.Context, actorHash []byte, requestID string, tenantID uuid.UUID, field Field, value string) (AdminView, error) {
	if err := ValidateSecret(field, value); err != nil {
		return AdminView{}, err
	}
	if err := service.ensureTenant(ctx, tenantID); err != nil {
		return AdminView{}, err
	}
	sealed, err := service.box.Encrypt([]byte(value), aad(tenantID.String(), field))
	if err != nil {
		return AdminView{}, errors.New("seal assistant secret")
	}
	stored, err := service.store.PutSecret(ctx, actorHash, requestID, tenantID, field, &sealed, last4Of(field, value), service.now().UTC())
	if err != nil {
		return AdminView{}, err
	}
	return service.view(ctx, stored)
}

func (service *Service) ClearSecret(ctx context.Context, actorHash []byte, requestID string, tenantID uuid.UUID, field Field) (AdminView, error) {
	if err := service.ensureTenant(ctx, tenantID); err != nil {
		return AdminView{}, err
	}
	current, err := service.load(ctx, tenantID)
	if err != nil {
		return AdminView{}, err
	}
	if field == FieldOpenRouterKey && current.Enabled {
		return AdminView{}, &ValidationError{Field: string(field), Code: "DISABLE_FIRST"}
	}
	if current.Enabled && current.LineMode == LineOwn && (field == FieldLineSecret || field == FieldLineToken) {
		return AdminView{}, &ValidationError{Field: string(field), Code: "DISABLE_FIRST"}
	}
	stored, err := service.store.PutSecret(ctx, actorHash, requestID, tenantID, field, nil, "", service.now().UTC())
	if err != nil {
		return AdminView{}, err
	}
	return service.view(ctx, stored)
}

func (service *Service) GlobalView(ctx context.Context) (GlobalView, error) {
	global, err := service.store.GetGlobal(ctx)
	if err != nil {
		return GlobalView{}, err
	}
	return GlobalView{LineChannelSecret: SecretState{IsSet: global.LineSecret != nil}, LineChannelToken: SecretState{IsSet: global.LineToken != nil}, ConfigVersion: global.ConfigVersion}, nil
}

func (service *Service) SetGlobalSecret(ctx context.Context, actorHash []byte, requestID string, field Field, value string) (GlobalView, error) {
	if field != FieldLineSecret && field != FieldLineToken {
		return GlobalView{}, &ValidationError{Field: string(field), Code: "NOT_A_GLOBAL_FIELD"}
	}
	if err := ValidateSecret(field, value); err != nil {
		return GlobalView{}, err
	}
	sealed, err := service.box.Encrypt([]byte(value), aad("global", field))
	if err != nil {
		return GlobalView{}, errors.New("seal assistant secret")
	}
	if _, err := service.store.PutGlobalSecret(ctx, actorHash, requestID, field, &sealed, service.now().UTC()); err != nil {
		return GlobalView{}, err
	}
	return service.GlobalView(ctx)
}

// ---- what the shop's assistant reads ------------------------------------------

type AgentModel struct {
	Key                string   `json:"key"`
	ModelID            string   `json:"modelId"`
	Providers          []string `json:"providers,omitempty"`
	DataCollectionDeny bool     `json:"dataCollectionDeny"`
}

type AgentSecrets struct {
	OpenRouterKey          string `json:"openrouterKey,omitempty"`
	TelegramBotToken       string `json:"telegramBotToken,omitempty"`
	LineChannelSecret      string `json:"lineChannelSecret,omitempty"`
	LineChannelAccessToken string `json:"lineChannelAccessToken,omitempty"`
}

// AgentConfig is the answer to the assistant's pull. When the assistant must not run (switched off, past the shop's end date, no key,
// a secret that cannot be read) it says so and carries no model and no secret.
type AgentConfig struct {
	ConfigVersion int64         `json:"configVersion"`
	Enabled       bool          `json:"enabled"`
	Reason        string        `json:"reason,omitempty"`
	ShopName      string        `json:"shopName,omitempty"`
	Model         *AgentModel   `json:"model,omitempty"`
	LineMode      string        `json:"lineMode,omitempty"`
	Secrets       *AgentSecrets `json:"secrets,omitempty"`
}

// ETag changes when the version changes and when the shop turns from usable to unusable by the passing of its end date.
func (config AgentConfig) ETag() string {
	return fmt.Sprintf(`"v%d-%t"`, config.ConfigVersion, config.Enabled)
}

func (service *Service) open(sealed *secret.Sealed, scope string, field Field) (string, error) {
	if sealed == nil {
		return "", nil
	}
	plain, err := service.box.Decrypt(*sealed, aad(scope, field))
	return string(plain), err
}

func (service *Service) AgentConfig(ctx context.Context, tenantID uuid.UUID) (AgentConfig, error) {
	stored, err := service.store.Get(ctx, tenantID)
	global, globalErr := service.store.GetGlobal(ctx)
	if globalErr != nil {
		return AgentConfig{}, globalErr
	}
	if errors.Is(err, ErrNotConfigured) {
		return AgentConfig{ConfigVersion: global.ConfigVersion, Reason: "NOT_CONFIGURED"}, nil
	}
	if err != nil {
		return AgentConfig{}, err
	}
	config := AgentConfig{ConfigVersion: stored.ConfigVersion + global.ConfigVersion}
	gate, err := service.store.Gate(ctx, tenantID, service.now().UTC())
	if err != nil {
		return AgentConfig{}, err
	}
	switch {
	case !stored.Enabled:
		config.Reason = "DISABLED"
		return config, nil
	case !gate.Active:
		config.Reason = "EXPIRED"
		return config, nil
	case stored.OpenRouterKey == nil:
		config.Reason = "KEY_MISSING"
		return config, nil
	}
	model, ok := ModelFor(stored.ModelKey)
	if !ok || !model.Selectable(stored.IsTest) {
		config.Reason = "MODEL_NOT_ALLOWED"
		return config, nil
	}
	scope := tenantID.String()
	secrets := &AgentSecrets{}
	if secrets.OpenRouterKey, err = service.open(stored.OpenRouterKey, scope, FieldOpenRouterKey); err != nil {
		config.Reason = "SECRET_UNREADABLE"
		return config, nil
	}
	if secrets.TelegramBotToken, err = service.open(stored.TelegramToken, scope, FieldTelegramToken); err != nil {
		config.Reason = "SECRET_UNREADABLE"
		return config, nil
	}
	switch stored.LineMode {
	case LineOwn:
		secrets.LineChannelSecret, err = service.open(stored.LineSecret, scope, FieldLineSecret)
		if err == nil {
			secrets.LineChannelAccessToken, err = service.open(stored.LineToken, scope, FieldLineToken)
		}
	case LineCentral:
		secrets.LineChannelSecret, err = service.open(global.LineSecret, "global", FieldLineSecret)
		if err == nil {
			secrets.LineChannelAccessToken, err = service.open(global.LineToken, "global", FieldLineToken)
		}
	}
	if err != nil {
		config.Reason = "SECRET_UNREADABLE"
		return config, nil
	}
	config.Enabled, config.ShopName, config.LineMode, config.Secrets = true, gate.Name, stored.LineMode, secrets
	config.Model = &AgentModel{Key: model.Key, ModelID: model.ModelID, Providers: append([]string(nil), model.Providers...), DataCollectionDeny: model.DataCollectionDeny}
	return config, nil
}

type StatusReport struct {
	ConfigVersion int64
	ModelKey      string
	StartedAt     *time.Time
	ErrorCode     string
}

var safeCode = regexp.MustCompile(`^[A-Z0-9_]{1,40}$`)

// RecordStatus keeps what the assistant says it is running. Free text from the assistant is not kept: a model key must be in the
// catalog and an error code must be a short upper-case code.
func (service *Service) RecordStatus(ctx context.Context, tenantID uuid.UUID, report StatusReport) error {
	if report.ConfigVersion < 0 {
		return &ValidationError{Field: "configVersion", Code: "INVALID"}
	}
	if report.ModelKey != "" {
		if _, ok := ModelFor(report.ModelKey); !ok {
			return &ValidationError{Field: "modelKey", Code: "UNKNOWN_MODEL"}
		}
	}
	if report.ErrorCode != "" && !safeCode.MatchString(report.ErrorCode) {
		return &ValidationError{Field: "errorCode", Code: "INVALID"}
	}
	return service.store.RecordStatus(ctx, tenantID, Status{AppliedConfigVersion: report.ConfigVersion, ReportedModelKey: report.ModelKey, StartedAt: report.StartedAt, LastSeenAt: service.now().UTC(), LastErrorCode: report.ErrorCode}, service.now().UTC())
}
