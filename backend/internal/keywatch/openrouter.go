package keywatch

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

const DefaultEndpoint = "https://openrouter.ai/api/v1/key"

// OpenRouter asks OpenRouter how much of one key's limit is left (GET /api/v1/key). It sends the key only to that endpoint.
type OpenRouter struct {
	endpoint string
	client   *http.Client
}

func NewOpenRouter(endpoint string, timeout time.Duration) *OpenRouter {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	return &OpenRouter{endpoint: strings.TrimSpace(endpoint), client: &http.Client{Transport: transport, Timeout: timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("model key check redirects are not allowed")
		}}}
}

type keyAnswer struct {
	Data struct {
		Limit          *float64 `json:"limit"`
		LimitRemaining *float64 `json:"limit_remaining"`
	} `json:"data"`
}

// Balance never returns the key or the answer body in an error.
func (client *OpenRouter) Balance(ctx context.Context, key string) (Balance, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, client.endpoint, nil)
	if err != nil {
		return Balance{}, errors.New("model key check request is invalid")
	}
	request.Header.Set("Authorization", "Bearer "+key)
	response, err := client.client.Do(request)
	if err != nil {
		return Balance{}, errors.New("model key check could not reach OpenRouter")
	}
	defer response.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	switch {
	case response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden || response.StatusCode == http.StatusPaymentRequired:
		return Balance{}, &ErrRefused{Status: response.StatusCode}
	case response.StatusCode < 200 || response.StatusCode > 299:
		return Balance{}, errors.New("model key check was not answered")
	}
	var answer keyAnswer
	if err := json.Unmarshal(payload, &answer); err != nil {
		return Balance{}, errors.New("model key check answered with something unexpected")
	}
	return Balance{Limit: answer.Data.Limit, Remaining: answer.Data.LimitRemaining}, nil
}
