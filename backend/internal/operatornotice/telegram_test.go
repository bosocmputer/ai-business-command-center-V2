package operatornotice

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testToken = "123456789:AAExampleExampleExampleExample0000"

func TestSendPostsOnePlainMessageToTheChat(t *testing.T) {
	var path string
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	telegram, err := NewTelegram(testToken+"\n", " -1001234 \n", server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := telegram.Send(context.Background(), "สวัสดี"); err != nil {
		t.Fatal(err)
	}
	if path != "/bot"+testToken+"/sendMessage" || body["chat_id"] != "-1001234" || body["text"] != "สวัสดี" {
		t.Fatalf("path=%q body=%v", path, body)
	}
}

func TestASendThatFailsNeverEchoesTheTokenOrTheText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) }))
	defer server.Close()
	telegram, _ := NewTelegram(testToken, "42", server.URL, nil)
	err := telegram.Send(context.Background(), "ข้อความลับของร้าน")
	if err == nil || !strings.Contains(err.Error(), "403") || strings.Contains(err.Error(), testToken) || strings.Contains(err.Error(), "ลับ") {
		t.Fatalf("err = %v", err)
	}
	if err := telegram.Send(context.Background(), "   "); err == nil {
		t.Fatal("an empty message is not sent")
	}
}

func TestATooLongMessageIsCutAndABadTokenOrChatIsRefused(t *testing.T) {
	var got string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		got = body["text"]
	}))
	defer server.Close()
	telegram, _ := NewTelegram(testToken, "42", server.URL, nil)
	_ = telegram.Send(context.Background(), strings.Repeat("ก", 5000))
	if len([]rune(got)) != maxTextRunes {
		t.Fatalf("sent %d characters", len([]rune(got)))
	}
	for _, args := range [][2]string{{"not-a-token", "42"}, {testToken, "chat-x"}, {"", ""}} {
		if _, err := NewTelegram(args[0], args[1], "", nil); err == nil {
			t.Errorf("%v should be refused", args)
		}
	}
	if _, err := NewTelegram(testToken, "42", "http://example.com", nil); err == nil {
		t.Error("a plain-http address that is not local must be refused")
	}
}
