package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/line"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/recipient"
	"github.com/go-chi/chi/v5"
)

const lineWebhookBodyLimit = 1 << 20

type LineWebhookAPI interface {
	HandleWebhook(ctx context.Context, body []byte, signature string) error
}

// registerLineWebhookRoutes exposes the Messaging API webhook. It carries no
// session: the X-Line-Signature HMAC over the raw body is the authentication.
func registerLineWebhookRoutes(router chi.Router, webhook LineWebhookAPI) {
	router.Post("/api/v1/line/webhook", func(response http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(http.MaxBytesReader(response, request.Body, lineWebhookBodyLimit))
		if err != nil {
			writeProblem(response, request, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "Webhook body is too large.", false)
			return
		}
		err = webhook.HandleWebhook(request.Context(), body, request.Header.Get("X-Line-Signature"))
		switch {
		case errors.Is(err, recipient.ErrWebhookNotConfigured):
			writeProblem(response, request, http.StatusServiceUnavailable, "LINE_WEBHOOK_NOT_CONFIGURED", "LINE webhook is not configured.", false)
		case errors.Is(err, recipient.ErrWebhookSignatureInvalid):
			writeProblem(response, request, http.StatusUnauthorized, "LINE_SIGNATURE_INVALID", "Webhook signature is invalid.", false)
		case errors.Is(err, line.ErrWebhookPayloadInvalid):
			writeProblem(response, request, http.StatusBadRequest, "VALIDATION_ERROR", "Webhook body is invalid.", false)
		case err != nil:
			writeProblem(response, request, http.StatusInternalServerError, "INTERNAL_ERROR", "Unable to process the webhook.", true)
		default:
			response.WriteHeader(http.StatusOK)
		}
	})
}
