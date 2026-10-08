package lookup

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/agent"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/sml"
	"github.com/google/uuid"
)

type fakeConnections struct{ err error }

func (fake fakeConnections) Open(context.Context, uuid.UUID) (sml.Connection, error) {
	return sml.Connection{}, fake.err
}

type fakeClient struct {
	mu         sync.Mutex
	statements []string
	answer     func(statement string) ([]map[string]string, error)
	block      chan struct{}
}

func (fake *fakeClient) Query(_ context.Context, _ sml.Connection, statement string) ([]map[string]string, error) {
	fake.mu.Lock()
	fake.statements = append(fake.statements, statement)
	fake.mu.Unlock()
	if fake.block != nil {
		<-fake.block
	}
	return fake.answer(statement)
}

func balanceAnswer(statement string) ([]map[string]string, error) {
	if strings.Contains(statement, "overdue_recent") {
		return []map[string]string{{"total_balance": "1500.5", "overdue_recent": "500", "overdue_old": "250", "not_due": "0", "no_due_date": "750.5", "credit": "0", "documents": "4"}}, nil
	}
	return []map[string]string{
		{"doc_no": "INV-1", "doc_date": "2025-01-10 00:00:00", "due_date": "2025-02-10", "balance": "250", "days_past_due": "600", "bucket": "OVERDUE_120_PLUS"},
		{"doc_no": "INV-2", "doc_date": "2026-09-01", "due_date": "", "balance": "750.5", "days_past_due": "", "bucket": "NO_DUE_DATE"},
	}, nil
}

var tenant = uuid.New()

func runner(client *fakeClient, at *time.Time) *Runner {
	value := NewRunner(fakeConnections{}, client, func() time.Time { return *at })
	value.QueryTimeout = time.Second
	return value
}

func TestCustomerBalanceReadsOneCustomerWithTheReportsOwnBase(t *testing.T) {
	now := time.Date(2026, 10, 8, 5, 0, 0, 0, time.UTC)
	client := &fakeClient{answer: balanceAnswer}
	result, err := runner(client, &now).Lookup(context.Background(), tenant, agent.LookupCustomerBalance, "C'001", "2026-10-08")
	if err != nil || !result.Found || result.Cached || !result.AsOf.Equal(now) {
		t.Fatalf("result = %+v %v", result, err)
	}
	figures := map[string]string{}
	for _, figure := range result.Figures {
		figures[figure.Key] = figure.Value
	}
	if figures["total_balance"] != "1500.50" || figures["overdue_recent"] != "500.00" || figures["overdue_old"] != "250.00" || figures["no_due_date"] != "750.50" || figures["documents"] != "4" {
		t.Errorf("figures = %v", figures)
	}
	if len(result.Tables) != 1 || len(result.Tables[0].Rows) != 2 || result.Tables[0].Rows[0][1] != "2025-01-10" || result.Tables[0].Rows[1][2] != "ไม่ระบุ" {
		t.Errorf("tables = %+v", result.Tables)
	}
	if len(client.statements) != 2 {
		t.Fatalf("at most two statements per question, got %d", len(client.statements))
	}
	for _, statement := range client.statements {
		lower := strings.ToLower(statement)
		if !strings.HasPrefix(strings.TrimSpace(lower), "with") || strings.Contains(statement, "$") || !strings.Contains(statement, `'C''001'`) || !strings.Contains(statement, "'2026-10-08'") {
			t.Errorf("the code must be a quoted literal and nothing else from outside:\n%s", statement)
		}
		for _, forbidden := range []string{"insert ", "update ", "delete ", "drop ", "alter ", "truncate ", ";"} {
			if strings.Contains(lower, forbidden) {
				t.Errorf("statement contains %q", forbidden)
			}
		}
	}
}

func TestEveryKindIsAFixedReadOnlySelectWithNoMoneyColumnsCopiedOut(t *testing.T) {
	for _, kind := range []agent.LookupKind{agent.LookupCustomerBalance, agent.LookupCustomerSales, agent.LookupItemStock} {
		statements, _, err := plan(kind, "X1", "2026-10-08")
		if err != nil || len(statements) == 0 {
			t.Fatalf("%s: %v", kind, err)
		}
		for _, statement := range statements {
			lower := strings.ToLower(strings.TrimSpace(statement))
			if !(strings.HasPrefix(lower, "select ") || strings.HasPrefix(lower, "with")) || strings.Contains(lower, ";") || strings.Contains(statement, "$") {
				t.Errorf("%s: %s", kind, statement)
			}
		}
	}
	if _, _, err := plan("nothing", "X", "2026-10-08"); err == nil {
		t.Error("an unknown kind must be refused")
	}
}

func TestAnAnswerIsKeptForAFewMinutesAndCostsNothingAgainInThatTime(t *testing.T) {
	now := time.Date(2026, 10, 8, 5, 0, 0, 0, time.UTC)
	client := &fakeClient{answer: balanceAnswer}
	value := runner(client, &now)
	value.PerTenantPerHour = 1
	if _, err := value.Lookup(context.Background(), tenant, agent.LookupCustomerBalance, "C1", "2026-10-08"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(4 * time.Minute)
	again, err := value.Lookup(context.Background(), tenant, agent.LookupCustomerBalance, "C1", "2026-10-08")
	if err != nil || !again.Cached || len(client.statements) != 2 {
		t.Fatalf("a repeat within the time must come from memory: %+v %v (%d statements)", again, err, len(client.statements))
	}
	now = now.Add(2 * time.Minute) // 6 minutes after the first read: expired, and the shop's one question this hour is used
	if _, err := value.Lookup(context.Background(), tenant, agent.LookupCustomerBalance, "C1", "2026-10-08"); !errors.Is(err, agent.ErrLookupBusy) {
		t.Fatalf("a shop that used its hourly allowance must be told it is busy: %v", err)
	}
	now = now.Add(time.Hour)
	if _, err := value.Lookup(context.Background(), tenant, agent.LookupCustomerBalance, "C1", "2026-10-08"); err != nil {
		t.Fatalf("an hour later the allowance is back: %v", err)
	}
	other := uuid.New()
	value.PerTenantPerHour = 1
	if _, err := value.Lookup(context.Background(), other, agent.LookupCustomerBalance, "C1", "2026-10-08"); err != nil {
		t.Fatalf("another shop has its own allowance: %v", err)
	}
}

func TestTooManyAtOnceIsBusyNotQueuedForever(t *testing.T) {
	now := time.Date(2026, 10, 8, 5, 0, 0, 0, time.UTC)
	block := make(chan struct{})
	client := &fakeClient{answer: balanceAnswer, block: block}
	value := runner(client, &now)
	value.Concurrency = 1
	value.PerTenantPerHour = 100
	done := make(chan error, 1)
	go func() {
		_, err := value.Lookup(context.Background(), tenant, agent.LookupCustomerBalance, "C1", "2026-10-08")
		done <- err
	}()
	for deadline := time.Now().Add(2 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		client.mu.Lock()
		started := len(client.statements)
		client.mu.Unlock()
		if started > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the first lookup never started")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, err := value.Lookup(ctx, tenant, agent.LookupCustomerBalance, "C2", "2026-10-08"); err == nil {
		t.Fatal("a second question while the only slot is taken must not run")
	}
	close(block)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestItemStockAndSalesAndAMissingItem(t *testing.T) {
	now := time.Date(2026, 10, 8, 5, 0, 0, 0, time.UTC)
	stock := &fakeClient{answer: func(string) ([]map[string]string, error) {
		return []map[string]string{{"unit": "ถุง", "on_hand": "1250.0000", "to_receive": "300", "to_deliver": "12.5", "reserved": "0", "reorder_point": "0"}}, nil
	}}
	result, err := runner(stock, &now).Lookup(context.Background(), tenant, agent.LookupItemStock, "A01", "2026-10-08")
	if err != nil || !result.Found || result.Figures[0].Value != "1250" || result.Figures[0].Unit != "ถุง" || result.Figures[2].Value != "12.5" {
		t.Fatalf("stock = %+v %v", result, err)
	}
	none := &fakeClient{answer: func(string) ([]map[string]string, error) { return nil, nil }}
	missing, err := runner(none, &now).Lookup(context.Background(), tenant, agent.LookupItemStock, "ZZZ", "2026-10-08")
	if err != nil || missing.Found {
		t.Fatalf("an item the system does not have is not found: %+v %v", missing, err)
	}
	sales := &fakeClient{answer: func(statement string) ([]map[string]string, error) {
		if strings.Contains(statement, "last_date") {
			return []map[string]string{{"documents": "12", "total": "98765.4", "last_date": "2026-10-01"}}, nil
		}
		return []map[string]string{{"doc_no": "IV-9", "doc_date": "2026-10-01", "total_amount": "1000"}}, nil
	}}
	got, err := runner(sales, &now).Lookup(context.Background(), tenant, agent.LookupCustomerSales, "C1", "2026-10-08")
	if err != nil || got.Figures[1].Value != "98765.40" || got.Figures[0].Value != "12" || len(got.Tables) != 1 || got.Tables[0].Rows[0][0] != "IV-9" {
		t.Fatalf("sales = %+v %v", got, err)
	}
}

func TestAFailureOrAJunkFigureNeverBecomesAnAnswer(t *testing.T) {
	now := time.Date(2026, 10, 8, 5, 0, 0, 0, time.UTC)
	broken := &fakeClient{answer: func(string) ([]map[string]string, error) { return nil, errors.New("timeout") }}
	if _, err := runner(broken, &now).Lookup(context.Background(), tenant, agent.LookupItemStock, "A01", "2026-10-08"); err == nil {
		t.Fatal("a failed read must be an error")
	}
	junk := &fakeClient{answer: func(string) ([]map[string]string, error) {
		return []map[string]string{{"unit": "x", "on_hand": "abc", "to_receive": "1", "to_deliver": "1", "reserved": "1", "reorder_point": "1"}}, nil
	}}
	if _, err := runner(junk, &now).Lookup(context.Background(), tenant, agent.LookupItemStock, "A01", "2026-10-08"); err == nil {
		t.Fatal("a figure that is not a number must be an error")
	}
	value := NewRunner(fakeConnections{err: errors.New("no connection")}, junk, func() time.Time { return now })
	if _, err := value.Lookup(context.Background(), tenant, agent.LookupItemStock, "A01", "2026-10-08"); err == nil {
		t.Fatal("no connection must be an error")
	}
}
