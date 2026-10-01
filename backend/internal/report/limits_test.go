package report

import "testing"

func TestCatalogFitsItsLimitsAndACardFitsInTheCatalog(t *testing.T) {
	if got := len(Keys()); got < 1 || got > MaxCatalogReports {
		t.Fatalf("catalog has %d reports, limit is %d: raise MaxCatalogReports together with the migration and OpenAPI bounds", got, MaxCatalogReports)
	}
	if MaxReportsPerCard < 1 || MaxReportsPerCard > MaxCatalogReports {
		t.Fatalf("a card holds %d reports but the catalog limit is %d", MaxReportsPerCard, MaxCatalogReports)
	}
	if len(Definitions()) != len(Keys()) {
		t.Fatalf("definitions (%d) and keys (%d) disagree", len(Definitions()), len(Keys()))
	}
}
