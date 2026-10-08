package report

// PersonNameVisualizations lists the dashboard visualizations whose category labels are the names of
// customers or suppliers. A caller that may not see names (the assistant, by default) gets stable aliases in
// their place. Product names are not listed: they are not personal data.
var PersonNameVisualizations = map[string]struct{}{
	"top_debtors":               {},
	"overdue_debtors":           {},
	"overdue_debtor_days":       {},
	"stale_overdue_debtors":     {},
	"stale_overdue_debtor_days": {},
	"top_customers":             {},
	"customers_behind_rhythm":   {},
	"customer_net_movement":     {},
	"customer_debit_credit":     {},
	"top_suppliers":             {},
	"top_profit_customers":      {},
	"loss_customers":            {},
}
