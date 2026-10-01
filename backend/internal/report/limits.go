package report

// MaxCatalogReports is the most reports the system is built to hold. It bounds
// the database constraints and the OpenAPI schemas that count reports across
// the whole catalog, such as a dashboard refresh. The catalog may grow up to
// this size without a schema change.
const MaxCatalogReports = 64

// MaxReportsPerCard is how many reports one LINE card carries. A Flex message has
// a size limit, so a schedule's report set is capped here even when the catalog
// is larger. Raising it needs a Flex size check, not just a constant change.
const MaxReportsPerCard = 10
