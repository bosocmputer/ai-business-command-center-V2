package viewer

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
)

// Every drill link is opened as an equality filter on stored rows, so both
// columns must exist as identifier columns of their reports. This keeps a link
// from being declared against a column the row filter would refuse.
func TestEveryDrillLinkPointsAtRealIdentifierColumns(t *testing.T) {
	approved := map[report.Key]bool{}
	for _, key := range report.Keys() {
		approved[key] = true
	}
	for _, source := range report.DrillSourceReports() {
		seen := map[report.DrillLink]bool{}
		for _, link := range report.DrillLinksFor(source) {
			if seen[link] {
				t.Errorf("%s declares %+v twice", source, link)
			}
			seen[link] = true
			if !approved[link.TargetReport] || link.TargetReport == source {
				t.Errorf("%s links to %s, which is not another approved report", source, link.TargetReport)
			}
			if reportRowFilterColumns[source][link.Column] != rowColumnIdentifier {
				t.Errorf("%s: %s is not an identifier column of the source report", source, link.Column)
			}
			if reportRowFilterColumns[link.TargetReport][link.TargetColumn] != rowColumnIdentifier {
				t.Errorf("%s -> %s: %s is not an identifier column of the target report", source, link.TargetReport, link.TargetColumn)
			}
			if link.LabelColumn != "" && reportRowFilterColumns[source][link.LabelColumn] != rowColumnText {
				t.Errorf("%s: label column %s is not a text column of the source report", source, link.LabelColumn)
			}
			if link.Kind != report.DrillDocument && link.LabelColumn == "" {
				t.Errorf("%s -> %s: a customer or item link needs the name column it is offered from", source, link.TargetReport)
			}
			switch link.Kind {
			case report.DrillCustomer, report.DrillItem, report.DrillDocument:
			default:
				t.Errorf("%s -> %s has unknown kind %q", source, link.TargetReport, link.Kind)
			}
		}
	}
}

// A drill link to a report that the viewer cannot open must not be offered, and
// the list must never carry a nil slice the client would have to special-case.
func TestListReportsOffersOnlyDrillLinksToPermittedReports(t *testing.T) {
	now := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	recipientID, tenantID := uuid.New(), uuid.New()
	store := &memoryViewerStore{reports: []ReportAccess{
		{Key: report.ARAging}, {Key: report.ARDebtReceipt}, {Key: report.CashBankReceipts},
	}}
	service := NewService(nil, &fakeRecipientResolver{}, store, nil, func() time.Time { return now })
	items, err := service.ListReports(context.Background(), recipientID, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[report.Key]ReportAccess{}
	for _, item := range items {
		byKey[item.Key] = item
	}
	aging := byKey[report.ARAging].DrillLinks
	if len(aging) == 0 {
		t.Fatal("aging should still offer its permitted receipt link")
	}
	for _, link := range aging {
		if link.TargetReport != report.ARDebtReceipt {
			t.Errorf("aging offers %s, but the viewer may only open the receipt report", link.TargetReport)
		}
	}
	if links := byKey[report.CashBankReceipts].DrillLinks; links == nil || len(links) != 0 {
		t.Errorf("a report with no links must give an empty list, got %#v", links)
	}
	// Receipts link to aging and movement; only aging is permitted here.
	if links := byKey[report.ARDebtReceipt].DrillLinks; len(links) != 1 || links[0].TargetReport != report.ARAging {
		t.Errorf("receipts should offer only the permitted aging link: %+v", links)
	}
}
