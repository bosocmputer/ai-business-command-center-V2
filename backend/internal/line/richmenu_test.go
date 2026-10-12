package line

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const richMenuTestToken = "0123456789abcdef0123456789abcdef0123"

func TestBuildRichMenuSplitsTheWidthIntoEqualButtonsThatSendTheirText(t *testing.T) {
	menu, err := BuildRichMenu(DefaultMenuButtons())
	if err != nil {
		t.Fatal(err)
	}
	if len(menu.Areas) != 4 || !menu.Selected || menu.Size.Width != 2500 || menu.Size.Height != 843 {
		t.Fatalf("menu = %+v", menu)
	}
	end := 0
	for _, area := range menu.Areas {
		if area.Bounds.X != end || area.Bounds.Y != 0 || area.Bounds.Height != 843 || area.Action.Type != "message" || area.Action.Text == "" {
			t.Fatalf("area = %+v (expected to start at %d)", area, end)
		}
		end = area.Bounds.X + area.Bounds.Width
	}
	if end != 2500 {
		t.Fatalf("the buttons cover %d of 2500", end)
	}
	body, _ := json.Marshal(menu)
	for _, want := range []string{`"width":2500`, `"height":843`, `"chatBarText"`} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("body lacks %s: %s", want, body)
		}
	}
}

func TestBuildRichMenuRefusesBadButtons(t *testing.T) {
	for _, buttons := range [][]MenuButton{
		nil,
		{{Label: "", Text: "x"}},
		{{Label: strings.Repeat("ก", 21), Text: "x"}},
		{{Label: "ok", Text: strings.Repeat("ก", 301)}},
	} {
		if _, err := BuildRichMenu(buttons); err == nil {
			t.Fatalf("buttons %+v should be refused", buttons)
		}
	}
}

func TestRichMenuClientCallsLINEWithTheTokenAndNeverEchoesIt(t *testing.T) {
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+richMenuTestToken {
			t.Errorf("missing token on %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		seen = append(seen, r.Method+" "+r.URL.Path+" "+r.Header.Get("Content-Type")+" "+string(body))
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/bot/richmenu":
			_, _ = w.Write([]byte(`{"richMenuId":"richmenu-1"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/bot/richmenu/list":
			_, _ = w.Write([]byte(`{"richmenus":[{"richMenuId":"richmenu-0","name":"AI-BCC owner menu"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/bot/user/all/richmenu":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"no default"}`))
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer server.Close()
	client := NewRichMenuClient(richMenuTestToken, server.URL+"/bot", server.URL+"/data-bot", 5*time.Second)
	ctx := context.Background()

	menu, _ := BuildRichMenu(DefaultMenuButtons())
	id, err := client.Create(ctx, menu)
	if err != nil || id != "richmenu-1" {
		t.Fatalf("Create = %q, %v", id, err)
	}
	png := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 1, 2, 3}
	if err := client.UploadImage(ctx, id, ImageContentType(png), png); err != nil {
		t.Fatal(err)
	}
	if err := client.SetDefault(ctx, id); err != nil {
		t.Fatal(err)
	}
	menus, err := client.List(ctx)
	if err != nil || len(menus) != 1 || menus[0].Name != RichMenuName {
		t.Fatalf("List = %+v, %v", menus, err)
	}
	if current, err := client.DefaultID(ctx); err != nil || current != "" {
		t.Fatalf("DefaultID = %q, %v (a 404 means no default)", current, err)
	}
	if err := client.Delete(ctx, "richmenu-0"); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(seen, "\n")
	for _, want := range []string{"POST /bot/richmenu application/json", "POST /data-bot/richmenu/richmenu-1/content image/png", "POST /bot/user/all/richmenu/richmenu-1", "DELETE /bot/richmenu/richmenu-0"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("no request %q in:\n%s", want, joined)
		}
	}
}

func TestRichMenuClientRefusalNamesTheStatusButNotTheToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"The request body has 1 error(s)"}`))
	}))
	defer server.Close()
	client := NewRichMenuClient(richMenuTestToken, server.URL, server.URL, 5*time.Second)
	_, err := client.Create(context.Background(), RichMenu{})
	if err == nil || !strings.Contains(err.Error(), "400") || strings.Contains(err.Error(), richMenuTestToken) {
		t.Fatalf("err = %v", err)
	}
	if err := NewRichMenuClient("short", server.URL, server.URL, time.Second).Delete(context.Background(), "x"); err == nil {
		t.Fatal("a missing token must be refused before any request")
	}
}

func TestImageContentTypeReadsTheFirstBytes(t *testing.T) {
	if ImageContentType([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0}) != "image/png" || ImageContentType([]byte{0xff, 0xd8, 0xff, 0xe0}) != "image/jpeg" || ImageContentType([]byte("GIF89a....")) != "" {
		t.Fatal("content types are wrong")
	}
}
