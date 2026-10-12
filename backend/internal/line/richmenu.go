package line

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// The shop owner's menu at the bottom of the chat: four buttons that send a short Thai message, exactly as if the owner had typed it.
// The messages are ordinary questions for the assistant (or the front gate for "change shop"), so no button needs its own code path.

const (
	DefaultRichMenuAPI  = "https://api.line.me/v2/bot"
	DefaultRichMenuData = "https://api-data.line.me/v2/bot"

	RichMenuName     = "AI-BCC owner menu"
	RichMenuWidth    = 2500
	RichMenuHeight   = 843
	MaxRichMenuImage = 1 << 20 // LINE accepts at most 1 MB
)

// MenuButton is one fourth of the menu.
type MenuButton struct {
	Label string // shown nowhere in the chat; kept short for LINE's accessibility readers (at most 20 characters)
	Text  string // what the button sends
}

// DefaultMenuButtons is the menu the owners get. The second text asks for the figure read live from the shop's system (the assistant
// treats "ตัวเลขสดล่าสุด" as an explicit request for the latest figures).
func DefaultMenuButtons() []MenuButton {
	return []MenuButton{
		{Label: "ยอดขายเมื่อวาน", Text: "ยอดขายเมื่อวาน"},
		{Label: "ยอดขายวันนี้", Text: "ยอดขายวันนี้ ตัวเลขสดล่าสุด"},
		{Label: "สินค้าใกล้หมด", Text: "สินค้าใกล้หมด"},
		{Label: "เปลี่ยนร้าน", Text: "เปลี่ยนร้าน"},
	}
}

type richMenuBounds struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

type richMenuAction struct {
	Type  string `json:"type"`
	Label string `json:"label"`
	Text  string `json:"text"`
}

type richMenuArea struct {
	Bounds richMenuBounds `json:"bounds"`
	Action richMenuAction `json:"action"`
}

// RichMenu is the body LINE wants for creating a menu.
type RichMenu struct {
	Size        richMenuSize   `json:"size"`
	Selected    bool           `json:"selected"`
	Name        string         `json:"name"`
	ChatBarText string         `json:"chatBarText"`
	Areas       []richMenuArea `json:"areas"`
}

type richMenuSize struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// BuildRichMenu lays the buttons side by side over the whole image. The menu is open when a chat opens (selected), because an owner who
// never taps the small bar would otherwise never see it.
func BuildRichMenu(buttons []MenuButton) (RichMenu, error) {
	if len(buttons) < 1 || len(buttons) > 6 {
		return RichMenu{}, errors.New("a menu has one to six buttons")
	}
	menu := RichMenu{Size: richMenuSize{Width: RichMenuWidth, Height: RichMenuHeight}, Selected: true, Name: RichMenuName, ChatBarText: "เมนู"}
	for index, button := range buttons {
		label, text := strings.TrimSpace(button.Label), strings.TrimSpace(button.Text)
		if label == "" || len([]rune(label)) > 20 || text == "" || len([]rune(text)) > 300 {
			return RichMenu{}, fmt.Errorf("button %d needs a label up to 20 characters and a text up to 300", index+1)
		}
		left := RichMenuWidth * index / len(buttons)
		right := RichMenuWidth * (index + 1) / len(buttons)
		menu.Areas = append(menu.Areas, richMenuArea{
			Bounds: richMenuBounds{X: left, Y: 0, Width: right - left, Height: RichMenuHeight},
			Action: richMenuAction{Type: "message", Label: label, Text: text},
		})
	}
	return menu, nil
}

// RichMenuInfo is what a listing tells about one menu.
type RichMenuInfo struct {
	ID   string `json:"richMenuId"`
	Name string `json:"name"`
}

type RichMenuClient struct {
	accessToken string
	api         string
	data        string
	client      *http.Client
}

func NewRichMenuClient(accessToken, api, data string, timeout time.Duration) *RichMenuClient {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	return &RichMenuClient{
		accessToken: accessToken, api: strings.TrimRight(api, "/"), data: strings.TrimRight(data, "/"),
		client: &http.Client{Transport: transport, Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("LINE rich menu redirects are not allowed")
		}},
	}
}

// do sends one request and returns the body of a 2xx answer. A refusal carries LINE's own message (it names the field that was wrong and
// holds no secret), never the token.
func (client *RichMenuClient) do(ctx context.Context, method, target, contentType string, body []byte) ([]byte, error) {
	if len(client.accessToken) < 32 {
		return nil, errors.New("the LINE channel access token is missing")
	}
	request, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+client.accessToken)
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	response, err := client.client.Do(request)
	if err != nil {
		return nil, errors.New("LINE could not be reached")
	}
	defer response.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return nil, fmt.Errorf("LINE refused the request (%d): %s", response.StatusCode, strings.TrimSpace(string(payload)))
	}
	return payload, nil
}

// List returns every menu of the channel.
func (client *RichMenuClient) List(ctx context.Context) ([]RichMenuInfo, error) {
	payload, err := client.do(ctx, http.MethodGet, client.api+"/richmenu/list", "", nil)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		RichMenus []RichMenuInfo `json:"richmenus"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return nil, errors.New("LINE answered with something that is not a menu list")
	}
	return parsed.RichMenus, nil
}

// DefaultID is the menu every user of the channel gets, or "" when none is set.
func (client *RichMenuClient) DefaultID(ctx context.Context) (string, error) {
	payload, err := client.do(ctx, http.MethodGet, client.api+"/user/all/richmenu", "", nil)
	if err != nil {
		if strings.Contains(err.Error(), "(404)") {
			return "", nil
		}
		return "", err
	}
	var parsed struct {
		ID string `json:"richMenuId"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return "", errors.New("LINE answered with something that is not a menu id")
	}
	return parsed.ID, nil
}

// Create makes the menu (no image yet) and returns its id.
func (client *RichMenuClient) Create(ctx context.Context, menu RichMenu) (string, error) {
	body, err := json.Marshal(menu)
	if err != nil {
		return "", err
	}
	payload, err := client.do(ctx, http.MethodPost, client.api+"/richmenu", "application/json", body)
	if err != nil {
		return "", err
	}
	var parsed struct {
		ID string `json:"richMenuId"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil || parsed.ID == "" {
		return "", errors.New("LINE did not return a menu id")
	}
	return parsed.ID, nil
}

// UploadImage attaches the picture; contentType is image/png or image/jpeg.
func (client *RichMenuClient) UploadImage(ctx context.Context, id, contentType string, image []byte) error {
	if contentType != "image/png" && contentType != "image/jpeg" {
		return errors.New("the menu picture must be PNG or JPEG")
	}
	if len(image) == 0 || len(image) > MaxRichMenuImage {
		return errors.New("the menu picture must be between 1 byte and 1 MB")
	}
	_, err := client.do(ctx, http.MethodPost, client.data+"/richmenu/"+url.PathEscape(id)+"/content", contentType, image)
	return err
}

// SetDefault gives the menu to every user of the channel.
func (client *RichMenuClient) SetDefault(ctx context.Context, id string) error {
	_, err := client.do(ctx, http.MethodPost, client.api+"/user/all/richmenu/"+url.PathEscape(id), "", nil)
	return err
}

// ClearDefault takes the default menu away from the channel (the menus themselves stay).
func (client *RichMenuClient) ClearDefault(ctx context.Context) error {
	_, err := client.do(ctx, http.MethodDelete, client.api+"/user/all/richmenu", "", nil)
	return err
}

func (client *RichMenuClient) Delete(ctx context.Context, id string) error {
	_, err := client.do(ctx, http.MethodDelete, client.api+"/richmenu/"+url.PathEscape(id), "", nil)
	return err
}

// ImageContentType tells PNG from JPEG by the first bytes, so a file with the wrong extension is still sent as what it is.
func ImageContentType(image []byte) string {
	switch {
	case len(image) > 8 && bytes.Equal(image[:8], []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}):
		return "image/png"
	case len(image) > 3 && image[0] == 0xff && image[1] == 0xd8 && image[2] == 0xff:
		return "image/jpeg"
	}
	return ""
}
