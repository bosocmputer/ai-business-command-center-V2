package viewer

import (
	"context"
	"crypto/hmac"
	"errors"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/auth"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/line"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/recipient"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
	"github.com/google/uuid"
)

var (
	ErrSessionInvalid             = errors.New("viewer session is invalid")
	ErrIdentityForbidden          = errors.New("LINE identity is not an active recipient")
	ErrDeliveryReferenceForbidden = errors.New("delivery reference does not belong to LINE identity")
	ErrReportForbidden            = errors.New("viewer report access is forbidden")
)

type SessionRecord struct {
	TokenHash   []byte
	RecipientID uuid.UUID
	CSRFHash    []byte
	ExpiresAt   time.Time
	RevokedAt   *time.Time
}

type TenantAccess struct {
	ID         uuid.UUID    `json:"id"`
	Name       string       `json:"name"`
	Timezone   string       `json:"timezone"`
	ReportKeys []report.Key `json:"reportKeys"`
}

type ReportAccess struct {
	Key         report.Key           `json:"reportKey"`
	Version     string               `json:"version"`
	Label       string               `json:"label"`
	Category    string               `json:"category"`
	IsSensitive bool                 `json:"isSensitive"`
	PeriodMode  report.ParameterKind `json:"periodMode"`
	// DrillLinks lists the other reports a row can open. Only links whose target
	// the viewer may also open are included.
	DrillLinks []report.DrillLink `json:"drillLinks"`
}

type Store interface {
	CreateSession(context.Context, SessionRecord) error
	FindSession(context.Context, []byte, time.Time) (SessionRecord, error)
	RevokeSession(context.Context, []byte, time.Time) error
	ResolveDeliveryReference(context.Context, []byte, uuid.UUID, *uuid.UUID, time.Time) (DeliveryContext, error)
	GetDeliveryContext(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, time.Time) (DeliveryContext, error)
	ListTenants(context.Context, uuid.UUID, time.Time) ([]TenantAccess, error)
	ListReports(context.Context, uuid.UUID, uuid.UUID, time.Time) ([]ReportAccess, error)
	CanAccessReport(context.Context, uuid.UUID, uuid.UUID, report.Key, time.Time) (bool, error)
}

type IdentityVerifier interface {
	Verify(context.Context, string) (line.Identity, error)
}

type RecipientResolver interface {
	ResolveIdentity(context.Context, line.Identity, string) (recipient.Recipient, error)
	Get(context.Context, uuid.UUID) (recipient.Recipient, error)
}

type Service struct {
	identityVerifier IdentityVerifier
	recipients       RecipientResolver
	store            Store
	tokens           *auth.SessionManager
	now              func() time.Time
}

type ExchangeResult struct {
	RawToken                 string
	CSRFToken                string
	RecipientID              uuid.UUID
	DisplayName              string
	ExpiresAt                time.Time
	DeliveryContext          *DeliveryContext
	DeliveryContextErrorCode string
}

type AuthenticatedViewer struct {
	TokenHash   []byte
	RecipientID uuid.UUID
	DisplayName string
	CSRFHash    []byte
	ExpiresAt   time.Time
}

func NewService(identityVerifier IdentityVerifier, recipients RecipientResolver, store Store, tokens *auth.SessionManager, now func() time.Time) *Service {
	return &Service{identityVerifier: identityVerifier, recipients: recipients, store: store, tokens: tokens, now: now}
}

func (service *Service) Exchange(ctx context.Context, idToken, invitationReference, deliveryReference string, expectedTenantID *uuid.UUID) (ExchangeResult, error) {
	if service.identityVerifier == nil {
		return ExchangeResult{}, &line.SafeError{Code: "LINE_LOGIN_NOT_CONFIGURED", Retryable: false}
	}
	identity, err := service.identityVerifier.Verify(ctx, idToken)
	if err != nil {
		return ExchangeResult{}, err
	}
	resolved, err := service.recipients.ResolveIdentity(ctx, identity, invitationReference)
	if errors.Is(err, recipient.ErrRecipientNotFound) || errors.Is(err, recipient.ErrInvitationInvalid) {
		return ExchangeResult{}, ErrIdentityForbidden
	}
	if err != nil {
		return ExchangeResult{}, err
	}
	if resolved.Status != recipient.StatusActive {
		return ExchangeResult{}, ErrIdentityForbidden
	}
	issued, err := service.tokens.Issue(24 * time.Hour)
	if err != nil {
		return ExchangeResult{}, err
	}
	if err := service.store.CreateSession(ctx, SessionRecord{
		TokenHash: issued.TokenHash, RecipientID: resolved.ID, CSRFHash: issued.CSRFHash, ExpiresAt: issued.ExpiresAt,
	}); err != nil {
		return ExchangeResult{}, err
	}
	result := ExchangeResult{
		RawToken: issued.RawToken, CSRFToken: issued.CSRFToken, RecipientID: resolved.ID,
		DisplayName: resolved.DisplayName, ExpiresAt: issued.ExpiresAt,
	}
	if deliveryReference != "" {
		contextItem, contextErr := service.ResolveDeliveryContext(ctx, AuthenticatedViewer{RecipientID: resolved.ID}, deliveryReference, expectedTenantID)
		if contextErr != nil {
			if errors.Is(contextErr, ErrDeliveryContextPermissionChanged) {
				result.DeliveryContextErrorCode = "DELIVERY_CONTEXT_PERMISSION_CHANGED"
			} else {
				result.DeliveryContextErrorCode = "DELIVERY_CONTEXT_UNAVAILABLE"
			}
		} else {
			result.DeliveryContext = &contextItem
		}
	}
	return result, nil
}

func (service *Service) Authenticate(ctx context.Context, rawToken string) (AuthenticatedViewer, error) {
	if rawToken == "" {
		return AuthenticatedViewer{}, ErrSessionInvalid
	}
	tokenHash := service.tokens.HashToken(rawToken)
	session, err := service.store.FindSession(ctx, tokenHash, service.now().UTC())
	if errors.Is(err, ErrSessionInvalid) {
		return AuthenticatedViewer{}, ErrSessionInvalid
	}
	if err != nil {
		return AuthenticatedViewer{}, err
	}
	resolved, err := service.recipients.Get(ctx, session.RecipientID)
	if errors.Is(err, recipient.ErrRecipientNotFound) || resolved.Status != recipient.StatusActive {
		return AuthenticatedViewer{}, ErrSessionInvalid
	}
	if err != nil {
		return AuthenticatedViewer{}, err
	}
	return AuthenticatedViewer{
		TokenHash: tokenHash, RecipientID: session.RecipientID, DisplayName: resolved.DisplayName,
		CSRFHash: session.CSRFHash, ExpiresAt: session.ExpiresAt,
	}, nil
}

func (service *Service) ValidateCSRF(viewer AuthenticatedViewer, token string) error {
	if token == "" || !hmac.Equal(service.tokens.HashToken(token), viewer.CSRFHash) {
		return auth.ErrInvalidCSRF
	}
	return nil
}

func (service *Service) Logout(ctx context.Context, viewer AuthenticatedViewer) error {
	return service.store.RevokeSession(ctx, viewer.TokenHash, service.now().UTC())
}

func (service *Service) ListTenants(ctx context.Context, recipientID uuid.UUID) ([]TenantAccess, error) {
	return service.store.ListTenants(ctx, recipientID, service.now().UTC())
}

func (service *Service) ListReports(ctx context.Context, recipientID, tenantID uuid.UUID) ([]ReportAccess, error) {
	items, err := service.store.ListReports(ctx, recipientID, tenantID, service.now().UTC())
	if err != nil {
		return nil, err
	}
	permitted := make(map[report.Key]struct{}, len(items))
	for _, item := range items {
		permitted[item.Key] = struct{}{}
	}
	for index := range items {
		definition, ok := report.DefinitionFor(items[index].Key)
		if !ok {
			return nil, ErrReportForbidden
		}
		items[index].PeriodMode = definition.ParameterKind
		items[index].DrillLinks = permittedDrillLinks(items[index].Key, permitted)
	}
	return items, nil
}

// permittedDrillLinks keeps the drill links of a report whose target the viewer
// is also permitted to open. The target report checks permission again when it is
// opened, so this only keeps buttons from appearing that would lead nowhere.
func permittedDrillLinks(key report.Key, permitted map[report.Key]struct{}) []report.DrillLink {
	links := make([]report.DrillLink, 0)
	for _, link := range report.DrillLinksFor(key) {
		if _, ok := permitted[link.TargetReport]; ok {
			links = append(links, link)
		}
	}
	return links
}

func (service *Service) CanAccessReport(ctx context.Context, recipientID, tenantID uuid.UUID, reportKey report.Key) (bool, error) {
	if _, ok := report.DefinitionFor(reportKey); !ok {
		return false, ErrReportForbidden
	}
	return service.store.CanAccessReport(ctx, recipientID, tenantID, reportKey, service.now().UTC())
}
