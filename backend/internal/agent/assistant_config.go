package agent

import (
	"context"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/assistantcfg"
	"github.com/google/uuid"
)

// AssistantConfigSource is where the settings of a shop's assistant are kept. The assistant of a shop reads its own with its own
// token, so a token can only ever ask for the shop it belongs to.
type AssistantConfigSource interface {
	AgentConfig(ctx context.Context, tenantID uuid.UUID) (assistantcfg.AgentConfig, error)
	RecordStatus(ctx context.Context, tenantID uuid.UUID, report assistantcfg.StatusReport) error
}

// ConfigureAssistantConfig turns on the settings pull. Without it the routes answer like a missing report.
func (service *Service) ConfigureAssistantConfig(source AssistantConfigSource) *Service {
	service.assistant = source
	return service
}

// AssistantConfig gives the caller's shop its settings, secrets included, or says it must not run. It is not a counted call and not
// written to the call log: the assistant asks every minute whether anything changed.
func (service *Service) AssistantConfig(ctx context.Context, principal Principal) (assistantcfg.AgentConfig, error) {
	if service.assistant == nil {
		return assistantcfg.AgentConfig{}, ErrNoData
	}
	return service.assistant.AgentConfig(ctx, principal.TenantID)
}

// AssistantStatus keeps what the shop's assistant says it is running.
func (service *Service) AssistantStatus(ctx context.Context, principal Principal, report assistantcfg.StatusReport) error {
	if service.assistant == nil {
		return ErrNoData
	}
	return service.assistant.RecordStatus(ctx, principal.TenantID, report)
}
