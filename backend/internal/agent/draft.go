package agent

import (
	"context"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/thaifmt"
)

// A draft is text the owner can copy and send to someone themselves. AI-BCC writes it from its own figures, word for
// word and number for number; the model only hands it over. Nothing is sent, nothing is written to SML, and the call
// log records that a draft was made (never its text or the customer's name).

const (
	DraftToneFriendly = "friendly"
	DraftToneFormal   = "formal"

	MessageDraftNeedsNames = "ร่างข้อความถึงลูกค้าต้องใช้ชื่อลูกค้า แต่การเข้าถึงนี้ปิดการแสดงชื่อไว้ จึงร่างให้ไม่ได้"
	MessageDraftNote       = "นี่เป็นร่างที่ยังไม่ได้ส่งให้ใคร ระบบไม่ส่งเอง เจ้าของคัดลอกไปส่งเองและควรตรวจยอดกับใบแจ้งหนี้ก่อน"
	MessageDraftStale      = "ยอดที่ใช้ในร่างเป็นข้อมูลชุดก่อนหน้า อาจไม่ใช่ยอดล่าสุด ควรตรวจยอดก่อนส่ง"
)

// draftTopOverdue is how many of the customers owing the most past their due date a draft can be made for.
const draftTopOverdue = 10

// InvalidDraftError says, in Thai the assistant can pass on, what was wrong with the request.
type InvalidDraftError struct{ Message string }

func (err *InvalidDraftError) Error() string { return "draft request is not valid" }

type DraftRequest struct {
	Customer string `json:"customer"`
	Tone     string `json:"tone"`
}

type DraftResponse struct {
	Status            string   `json:"status"`
	Message           string   `json:"message,omitempty"`
	Customer          string   `json:"customer,omitempty"`
	OverdueAmount     string   `json:"overdueAmount,omitempty"`
	MaxDaysPastDue    int      `json:"maxDaysPastDue,omitempty"`
	Tone              string   `json:"tone,omitempty"`
	Draft             string   `json:"draft,omitempty"`
	CollectedAt       string   `json:"collectedAt,omitempty"`
	Freshness         string   `json:"freshness,omitempty"`
	Candidates        []string `json:"candidates,omitempty"`
	Notes             []string `json:"notes,omitempty"`
	RetryAfterSeconds int      `json:"retryAfterSeconds,omitempty"`
}

// DraftCollection writes a payment reminder for one of the customers who owe the most past their due date, read from
// the same receivable report the owner sees. A recipient without the receivable report gets the uniform "no data"
// answer; one who may not see names gets a plain refusal, because a reminder without a name is not a reminder.
func (service *Service) DraftCollection(ctx context.Context, principal Principal, request DraftRequest) (DraftResponse, error) {
	started := service.now()
	now := started.UTC()
	definition, err := service.authorize(ctx, principal, string(report.ARAging), now)
	if err != nil {
		service.record(ctx, principal, ToolDraft, "", report.Period{}, OutcomeNoData, started, nil)
		return DraftResponse{}, err
	}
	key := string(definition.Key)
	query := normalizeName(request.Customer)
	tone := strings.TrimSpace(request.Tone)
	if tone == "" {
		tone = DraftToneFriendly
	}
	if utf8.RuneCountInString(query) < 2 || utf8.RuneCountInString(query) > 100 || (tone != DraftToneFriendly && tone != DraftToneFormal) {
		service.record(ctx, principal, ToolDraft, key, report.Period{}, OutcomeInvalidDraft, started, nil)
		return DraftResponse{}, &InvalidDraftError{Message: "ต้องระบุชื่อลูกค้า (อย่างน้อย 2 ตัวอักษร) และน้ำเสียงเป็น friendly (สุภาพเป็นกันเอง) หรือ formal (ทางการ)"}
	}
	if !principal.NamesVisible {
		service.record(ctx, principal, ToolDraft, key, report.Period{}, OutcomeUnavailable, started, nil)
		return DraftResponse{Status: "UNAVAILABLE", Message: MessageDraftNeedsNames}, nil
	}
	period, err := resolvePeriod(definition, locationOf(principal), now, "", "")
	if err != nil {
		service.record(ctx, principal, ToolDraft, key, report.Period{}, OutcomeInvalidPeriod, started, nil)
		return DraftResponse{}, err
	}
	snapshot := service.snapshot(ctx, principal, definition, period, now)
	if snapshot.state != stateReady {
		service.record(ctx, principal, ToolDraft, key, period, outcomeOf(snapshot.state), started, snapshot.runID())
		return DraftResponse{Status: snapshot.state.status(), Message: snapshot.message(), RetryAfterSeconds: snapshot.retryAfter}, nil
	}
	debtors := overdueDebtors(snapshot.snapshot.Dashboard)
	var matches []overdueDebtor
	for _, debtor := range debtors {
		if strings.Contains(normalizeName(debtor.Name), query) {
			matches = append(matches, debtor)
		}
	}
	response := DraftResponse{}
	if finished := snapshot.snapshot.SourceFinishedAt; finished != nil {
		response.CollectedAt = finished.In(locationOf(principal)).Format(time.RFC3339)
	}
	response.Freshness = "FRESH"
	if snapshot.stale {
		response.Freshness = "STALE"
	}
	switch {
	case len(matches) == 0:
		response.Status = "NOT_FOUND"
		response.Message = fmt.Sprintf("ร่างข้อความได้เฉพาะลูกหนี้เลยกำหนดที่ค้างสูงสุด %d รายแรก และไม่พบชื่อที่ระบุในกลุ่มนี้", draftTopOverdue)
		for _, debtor := range debtors {
			response.Candidates = append(response.Candidates, debtor.Name)
		}
		service.record(ctx, principal, ToolDraft, key, period, OutcomeNoData, started, snapshot.runID())
		return response, nil
	case len(matches) > 1:
		response.Status = "AMBIGUOUS"
		response.Message = "ชื่อที่ระบุตรงกับลูกหนี้หลายราย ให้เจ้าของเลือกชื่อเต็มของรายที่ต้องการ"
		for _, debtor := range matches {
			response.Candidates = append(response.Candidates, debtor.Name)
		}
		service.record(ctx, principal, ToolDraft, key, period, OutcomeOK, started, snapshot.runID())
		return response, nil
	}
	debtor := matches[0]
	asOf, parseErr := time.ParseInLocation(time.DateOnly, snapshot.snapshot.Dashboard.Period.DateTo, locationOf(principal))
	if parseErr != nil {
		asOf = now.In(locationOf(principal))
	}
	response.Status, response.Tone = "READY", tone
	response.Customer, response.OverdueAmount, response.MaxDaysPastDue = debtor.Name, debtor.Amount.FloatString(2), debtor.DaysPastDue
	response.Draft = collectionDraft(tone, principal.ShopName, debtor, asOf)
	response.Notes = []string{MessageDraftNote}
	if snapshot.stale {
		response.Notes = append(response.Notes, MessageDraftStale)
	}
	service.record(ctx, principal, ToolDraft, key, period, OutcomeOK, started, snapshot.runID())
	return response, nil
}

type overdueDebtor struct {
	Name        string
	Amount      *big.Rat
	DaysPastDue int
}

// overdueDebtors reads the two charts the receivable report builds side by side: the amount each customer owes past
// the due date, and how many days the oldest of those documents is past due. Same customers, same order.
func overdueDebtors(dashboard report.Dashboard) []overdueDebtor {
	var amounts, days *report.DashboardVisualization
	for index := range dashboard.Visualizations {
		switch dashboard.Visualizations[index].Key {
		case "overdue_debtors":
			amounts = &dashboard.Visualizations[index]
		case "overdue_debtor_days":
			days = &dashboard.Visualizations[index]
		}
	}
	if amounts == nil || len(amounts.Series) == 0 {
		return nil
	}
	list := make([]overdueDebtor, 0, len(amounts.Categories))
	for index, name := range amounts.Categories {
		if index >= len(amounts.Series[0].Values) || index >= draftTopOverdue {
			break
		}
		amount, ok := new(big.Rat).SetString(amounts.Series[0].Values[index])
		if !ok || amount.Sign() <= 0 {
			continue
		}
		item := overdueDebtor{Name: name, Amount: amount}
		if days != nil && len(days.Series) > 0 && index < len(days.Series[0].Values) && index < len(days.Categories) && days.Categories[index] == name {
			item.DaysPastDue, _ = strconv.Atoi(days.Series[0].Values[index])
		}
		list = append(list, item)
	}
	return list
}

func normalizeName(text string) string {
	return strings.ToLower(strings.Join(strings.Fields(text), " "))
}

// collectionDraft is a short, polite reminder. It states only what the report knows: the overdue amount and how long
// the oldest overdue document has been overdue. It sets no deadline, adds no fee and threatens nothing.
func collectionDraft(tone, shop string, debtor overdueDebtor, asOf time.Time) string {
	amount, date := thaifmt.Baht(debtor.Amount), thaifmt.Date(asOf)
	age := ""
	if debtor.DaysPastDue > 0 {
		age = fmt.Sprintf(" โดยรายการที่เลยกำหนดนานที่สุดเลยมาแล้ว %d วัน", debtor.DaysPastDue)
	}
	if tone == DraftToneFormal {
		return fmt.Sprintf("เรื่อง แจ้งยอดค้างชำระ\n\nเรียน %s\n\n%s ขอเรียนแจ้งว่า ณ วันที่ %s ท่านมียอดค้างชำระที่เลยกำหนดแล้วรวม %s บาท%s\n\nจึงเรียนมาเพื่อโปรดตรวจสอบและดำเนินการชำระ หากได้ชำระเรียบร้อยแล้วต้องขออภัยมา ณ ที่นี้ และขอความกรุณาแจ้งหลักฐานการชำระเงินกลับมาด้วย\n\nขอแสดงความนับถือ\n%s",
			debtor.Name, shop, date, amount, age, shop)
	}
	return fmt.Sprintf("เรียน %s\n\n%s ขอแจ้งยอดค้างชำระที่เลยกำหนดแล้ว ณ วันที่ %s รวม %s บาท%s\n\nรบกวนตรวจสอบและแจ้งกำหนดชำระให้ด้วยนะครับ/ค่ะ หากโอนชำระแล้ว ขออภัยด้วย รบกวนส่งหลักฐานการโอนกลับมาให้ด้วยครับ/ค่ะ\n\nขอบคุณครับ/ค่ะ\n%s",
		debtor.Name, shop, date, amount, age, shop)
}
