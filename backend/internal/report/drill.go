package report

// DrillKind says what a drill-down key identifies, so that every report that
// carries the same kind of key can link to the others without a bespoke rule.
type DrillKind string

const (
	DrillCustomer DrillKind = "CUSTOMER"
	DrillItem     DrillKind = "ITEM"
	DrillDocument DrillKind = "DOCUMENT"
)

// DrillLink says that a row of one report can open another report filtered to the
// same customer, item or document. Column is the identifier column in the
// source report's rows and TargetColumn the identifier column the target report
// is filtered on. The target is opened as an ordinary row filter on the target
// report's own stored rows, so it needs the target report's permission like any
// other way into that report, and it never reaches SML by itself.
type DrillLink struct {
	Column       string    `json:"column"`
	Kind         DrillKind `json:"kind"`
	TargetReport Key       `json:"targetReport"`
	TargetColumn string    `json:"targetColumn"`
	// LabelColumn is the human-readable column that shows the same customer or
	// item in the source report (the name beside the code). Reports usually show
	// the name and keep the code hidden, so the link is offered from whichever of
	// the two is on screen.
	LabelColumn string `json:"labelColumn,omitempty"`
}

func customerLink(column, label string, target Key) DrillLink {
	return DrillLink{Column: column, LabelColumn: label, Kind: DrillCustomer, TargetReport: target, TargetColumn: "cust_code"}
}

func itemLink(column, label string, target Key, targetColumn string) DrillLink {
	return DrillLink{Column: column, LabelColumn: label, Kind: DrillItem, TargetReport: target, TargetColumn: targetColumn}
}

// drillLinks is the single list of how reports open each other. A link is added
// when the two reports answer one question together: who is this customer and
// what do they owe, buy and pay; what is this item and where did it go.
var drillLinks = map[Key][]DrillLink{
	SalesGoodsServices: {
		customerLink("cust_code", "cust_name", ARAging), customerLink("cust_code", "cust_name", ARCustomerMovement),
		customerLink("cust_code", "cust_name", CustomerRFM), customerLink("cust_code", "cust_name", PurchaseFrequency),
		itemLink("item_code", "item_name", StockBalance, "ic_code"), itemLink("item_code", "item_name", GrossProfitByProduct, "code"),
	},
	CustomerRFM: {
		customerLink("cust_code", "cust_name", SalesGoodsServices), customerLink("cust_code", "cust_name", PurchaseFrequency), customerLink("cust_code", "cust_name", ARAging),
	},
	PurchaseFrequency: {
		customerLink("cust_code", "cust_name", SalesGoodsServices), customerLink("cust_code", "cust_name", CustomerRFM), customerLink("cust_code", "cust_name", ARAging),
	},
	ARAging: {
		customerLink("cust_code", "cust_name", ARCustomerMovement), customerLink("cust_code", "cust_name", ARDebtReceipt),
		customerLink("cust_code", "cust_name", SalesGoodsServices), customerLink("cust_code", "cust_name", CustomerRFM),
		{Column: "doc_no", Kind: DrillDocument, TargetReport: ARCustomerMovement, TargetColumn: "doc_no"},
		{Column: "doc_no", Kind: DrillDocument, TargetReport: SalesGoodsServices, TargetColumn: "doc_no"},
	},
	ARCustomerMovement: {
		customerLink("cust_code", "cust_name", ARAging), customerLink("cust_code", "cust_name", ARDebtReceipt), customerLink("cust_code", "cust_name", SalesGoodsServices),
		{Column: "doc_no", Kind: DrillDocument, TargetReport: SalesGoodsServices, TargetColumn: "doc_no"},
	},
	ARDebtReceipt: {
		customerLink("cust_code", "cust_name", ARAging), customerLink("cust_code", "cust_name", ARCustomerMovement),
	},
	GrossProfitByARCustomer: {
		customerLink("ar_code", "ar_detail", SalesGoodsServices), customerLink("ar_code", "ar_detail", ARAging), customerLink("ar_code", "ar_detail", CustomerRFM),
	},
	GrossProfitByProduct: {
		itemLink("code", "name_1", SalesGoodsServices, "item_code"), itemLink("code", "name_1", StockBalance, "ic_code"),
	},
	StockBalance: {
		itemLink("ic_code", "ic_name", SalesGoodsServices, "item_code"), itemLink("ic_code", "ic_name", StockReorder, "ic_code"), itemLink("ic_code", "ic_name", GrossProfitByProduct, "code"),
	},
	StockReorder: {
		itemLink("ic_code", "ic_name", StockBalance, "ic_code"), itemLink("ic_code", "ic_name", SalesGoodsServices, "item_code"),
	},
}

// DrillLinksFor returns a copy of the links a report declares, in a stable order.
func DrillLinksFor(key Key) []DrillLink {
	return append([]DrillLink(nil), drillLinks[key]...)
}

// DrillSourceReports lists the reports that declare at least one drill link.
func DrillSourceReports() []Key {
	keys := make([]Key, 0, len(drillLinks))
	for _, key := range Keys() {
		if len(drillLinks[key]) > 0 {
			keys = append(keys, key)
		}
	}
	return keys
}
