// Package operatornotice sends a short message to the operator's own Telegram chat (the one that already receives the system's incident
// alerts). It is for things only the operator should read, never for owners.
package operatornotice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var (
	tokenPattern = regexp.MustCompile(`^[0-9]{5,16}:[A-Za-z0-9_-]{24,240}$`)
	chatPattern  = regexp.MustCompile(`^-?[0-9]{1,20}$`)
)

const maxTextRunes = 3500

type Telegram struct {
	token, chatID, baseURL string
	client                 *http.Client
}

func NewTelegram(token, chatID, baseURL string, client *http.Client) (*Telegram, error) {
	token, chatID, baseURL = strings.TrimSpace(token), strings.TrimSpace(chatID), strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.telegram.org"
	}
	if !tokenPattern.MatchString(token) {
		return nil, errors.New("Telegram token is invalid")
	}
	if !chatPattern.MatchString(chatID) {
		return nil, errors.New("Telegram chat identifier is invalid")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "localhost") {
		return nil, errors.New("Telegram API base URL is invalid")
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &Telegram{token: token, chatID: chatID, baseURL: baseURL, client: client}, nil
}

// Send posts one plain-text message. The error never carries the token or the text.
func (telegram *Telegram) Send(ctx context.Context, text string) error {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) == 0 {
		return errors.New("nothing to send")
	}
	if len(runes) > maxTextRunes {
		runes = runes[:maxTextRunes]
	}
	body, _ := json.Marshal(map[string]any{"chat_id": telegram.chatID, "text": string(runes), "disable_web_page_preview": true})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, telegram.baseURL+"/bot"+telegram.token+"/sendMessage", bytes.NewReader(body))
	if err != nil {
		return errors.New("Telegram request could not be built")
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := telegram.client.Do(request)
	if err != nil {
		return errors.New("Telegram could not be reached")
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return fmt.Errorf("Telegram refused the message (%d)", response.StatusCode)
	}
	return nil
}
