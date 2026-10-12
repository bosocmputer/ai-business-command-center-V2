package line

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"
)

// FooterButton is one extra button under a card. Pressing it sends Text to the chat, exactly as if the person had typed it.
type FooterButton struct {
	Label string // at most 20 characters (LINE's limit)
	Text  string // at most 300 characters
}

// WithFooterButtons adds buttons under the "open details" button of a single-card Flex message made by RenderFlex.
func WithFooterButtons(message json.RawMessage, buttons []FooterButton) (json.RawMessage, error) {
	var decoded map[string]any
	if err := json.Unmarshal(message, &decoded); err != nil {
		return nil, ErrFlexInputInvalid
	}
	bubble, ok := decoded["contents"].(map[string]any)
	if !ok || bubble["type"] != "bubble" {
		return nil, errors.New("only a single card can take extra buttons")
	}
	footer, ok := bubble["footer"].(map[string]any)
	if !ok {
		return nil, ErrFlexInputInvalid
	}
	contents, _ := footer["contents"].([]any)
	for _, button := range buttons {
		label, text := strings.TrimSpace(button.Label), strings.TrimSpace(button.Text)
		if label == "" || utf8.RuneCountInString(label) > 20 || text == "" || utf8.RuneCountInString(text) > 300 {
			return nil, errors.New("a footer button needs a label up to 20 characters and a text up to 300")
		}
		contents = append(contents, map[string]any{
			"type": "button", "style": "secondary", "height": "sm", "margin": "sm",
			"action": map[string]any{"type": "message", "label": label, "text": text},
		})
	}
	footer["contents"] = contents
	encoded, err := json.Marshal(decoded)
	if err != nil || len(encoded) >= maximumFlexPayloadBytes {
		return nil, ErrFlexInputInvalid
	}
	return encoded, nil
}
