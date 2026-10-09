package httpapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

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
//
// forwardURL, when not empty, is where a webhook that passed the signature check is passed on, untouched, with its signature: the same LINE
// channel can then serve the assistant too (which checks the signature again with the same channel secret). The pass-on never delays or
// changes the answer to LINE, and nothing of the body is logged.
func registerLineWebhookRoutes(router chi.Router, webhook LineWebhookAPI, forwardURL string, logger *slog.Logger) {
	forward := newLineForwarder(forwardURL, logger)
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
			forward(body, request.Header.Get("X-Line-Signature"))
			response.WriteHeader(http.StatusOK)
		}
	})
}

// newLineForwarder gives the function that passes a verified webhook on in the background: at most a few at a time, five seconds each.
func newLineForwarder(forwardURL string, logger *slog.Logger) func(body []byte, signature string) {
	if forwardURL == "" {
		return func([]byte, string) {}
	}
	client := &http.Client{Timeout: 5 * time.Second}
	slots := make(chan struct{}, 8)
	return func(body []byte, signature string) {
		select {
		case slots <- struct{}{}:
		default:
			if logger != nil {
				logger.Warn("LINE webhook not passed on", "reason", "too many in flight")
			}
			return
		}
		go func() {
			defer func() { <-slots }()
			request, err := http.NewRequest(http.MethodPost, forwardURL, bytes.NewReader(body))
			if err != nil {
				return
			}
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-Line-Signature", signature)
			response, err := client.Do(request)
			if err != nil {
				if logger != nil {
					logger.Warn("LINE webhook not passed on", "reason", "request failed")
				}
				return
			}
			_ = response.Body.Close()
			if response.StatusCode >= 300 && logger != nil {
				logger.Warn("LINE webhook passed on but refused", "status", response.StatusCode)
			}
		}()
	}
}
