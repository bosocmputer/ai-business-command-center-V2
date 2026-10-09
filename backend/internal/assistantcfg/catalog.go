// Package assistantcfg keeps the settings of each shop's assistant: whether it is on, which model it uses, the keys of the shop's
// OpenRouter account, Telegram bot and LINE channel. The admin sets them; the assistant of the shop pulls them with its own token.
package assistantcfg

// ModelStatus says how far a model has been checked. Only CERTIFIED has passed everything the shop's real use needs.
type ModelStatus string

const (
	StatusCertified ModelStatus = "CERTIFIED" // used by a real shop: answers, safety, pictures, files all checked
	StatusUntested  ModelStatus = "UNTESTED"  // one scripted question passed; safety, pictures, files and long use not yet checked
	StatusSlow      ModelStatus = "SLOW"      // works but too slow for a chat
	StatusTestOnly  ModelStatus = "TEST_ONLY" // may keep or train on what it is sent; test shops only
)

// Measured is what one scripted question showed (9 Oct 2026, one run per model), not a benchmark.
type Measured struct {
	Seconds float64 `json:"seconds"`
	USD     float64 `json:"usd"`
}

// Model is one choice in the admin page. Providers are pinned to hosts OpenRouter lists as not keeping data (ZDR); a model is never
// sent to "any provider".
type Model struct {
	Key       string   `json:"key"`
	ModelID   string   `json:"modelId"`
	Providers []string `json:"-"`
	// DataCollectionDeny asks OpenRouter to refuse any provider that may store prompts. Not possible for the free router.
	DataCollectionDeny bool        `json:"-"`
	Label              string      `json:"label"`
	Summary            string      `json:"summary"`
	Status             ModelStatus `json:"status"`
	Vision             bool        `json:"vision"`
	Measured           *Measured   `json:"measured,omitempty"`
	Default            bool        `json:"default,omitempty"`
}

// DefaultModelKey is the model every shop starts with: the one the pilot shop has used.
const DefaultModelKey = "gemini-3.1-flash-lite"

var catalog = []Model{
	{Key: "gemini-3.1-flash-lite", ModelID: "google/gemini-3.1-flash-lite", Providers: []string{"google-vertex"}, DataCollectionDeny: true,
		Label: "Gemini 3.1 Flash Lite", Status: StatusCertified, Vision: true, Default: true, Measured: &Measured{Seconds: 31, USD: 0.0033},
		Summary: "ตัวที่ร้านสินทวีใช้จริงมาแล้ว ตอบกระชับ ผ่านการใช้งานจริง ความปลอดภัย รูปภาพ และไฟล์ ค่าเริ่มต้นที่แนะนำ"},
	{Key: "glm-5.3-flash", ModelID: "z-ai/glm-5.3-flash", Providers: []string{"deepinfra", "together", "fireworks"}, DataCollectionDeny: true,
		Label: "GLM 5.3 Flash", Status: StatusUntested, Vision: true, Measured: &Measured{Seconds: 24, USD: 0.0008},
		Summary: "ถูกที่สุด ราว 1 ใน 4 ของ Gemini และเร็วกว่า ผ่านคำถามทดสอบ 1 ข้อ ยังไม่ได้ทดสอบความปลอดภัย การอ่านรูป และไฟล์"},
	{Key: "deepseek-v4.1-flash", ModelID: "deepseek/deepseek-v4.1-flash", Providers: []string{"deepinfra", "together", "fireworks"}, DataCollectionDeny: true,
		Label: "DeepSeek V4.1 Flash", Status: StatusUntested, Vision: true, Measured: &Measured{Seconds: 21, USD: 0.0014},
		Summary: "ถูกและเร็ว ผ่านคำถามทดสอบ 1 ข้อ ยังไม่ได้ทดสอบความปลอดภัย การอ่านรูป และไฟล์ ใช้เฉพาะผู้ให้บริการที่ไม่เก็บข้อมูล ไม่ใช้เซิร์ฟเวอร์ของ DeepSeek เอง"},
	{Key: "qwen-3.8-27b", ModelID: "qwen/qwen3.8-27b", Providers: []string{"deepinfra", "parasail"}, DataCollectionDeny: true,
		Label: "Qwen 3.8 27B", Status: StatusUntested, Vision: true, Measured: &Measured{Seconds: 20, USD: 0.0022},
		Summary: "เร็ว ราคากลาง ใกล้เคียง Gemini ผ่านคำถามทดสอบ 1 ข้อ ยังไม่ได้ทดสอบความปลอดภัย การอ่านรูป และไฟล์"},
	{Key: "claude-haiku-5.5", ModelID: "anthropic/claude-haiku-5.5", Providers: []string{"google-vertex", "amazon-bedrock"}, DataCollectionDeny: true,
		Label: "Claude Haiku 5.5", Status: StatusUntested, Vision: true, Measured: &Measured{Seconds: 15, USD: 0.006},
		Summary: "เร็วที่สุด ตอบละเอียดที่สุด แต่เขียนยาวจึงแพงกว่า Gemini เกือบ 2 เท่าในการทดสอบ ผ่านคำถามทดสอบ 1 ข้อ ยังไม่ได้ทดสอบความปลอดภัย การอ่านรูป และไฟล์"},
	{Key: "gpt-5-nano", ModelID: "openai/gpt-5-nano", Providers: []string{"azure"}, DataCollectionDeny: true,
		Label: "GPT-5 nano", Status: StatusSlow, Vision: true, Measured: &Measured{Seconds: 55, USD: 0.0023},
		Summary: "คิดก่อนตอบ ช้าที่สุดในการทดสอบ (55 วินาที) ช้าเกินไปสำหรับคุยในไลน์ ไม่แนะนำ"},
	{Key: "openrouter-free", ModelID: "openrouter/free", Providers: nil, DataCollectionDeny: false,
		Label: "OpenRouter Free (ทดสอบเท่านั้น)", Status: StatusTestOnly, Vision: true,
		Summary: "ฟรี แต่ผู้ให้บริการอาจเก็บหรือนำข้อความไปใช้ ห้ามใช้กับข้อมูลลูกค้าจริง เลือกได้เฉพาะร้านที่ตั้งเป็นร้านทดสอบ"},
}

// Catalog gives the models in the order the admin page shows them.
func Catalog() []Model { return append([]Model(nil), catalog...) }

// ModelFor finds a model by its key.
func ModelFor(key string) (Model, bool) {
	for _, model := range catalog {
		if model.Key == key {
			return model, true
		}
	}
	return Model{}, false
}

// Selectable says whether a shop may use the model: a test-only model is for test shops only.
func (model Model) Selectable(isTest bool) bool {
	return model.Status != StatusTestOnly || isTest
}
