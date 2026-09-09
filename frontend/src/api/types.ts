import type { components } from './schema';

export type ProblemEnvelope = components['schemas']['ProblemEnvelope'];
export type AdminSession = components['schemas']['AdminSession'];
export type Tenant = components['schemas']['Tenant'];
export type TenantInput = components['schemas']['TenantInput'];
export type TenantPatch = components['schemas']['TenantPatch'];
export type TenantPage = components['schemas']['TenantPage'];
export type SMLConnectionInput = components['schemas']['SMLConnectionInput'];
export type SMLConnectionStatus = components['schemas']['SMLConnectionStatus'];
export type SMLConnectionTestResult = components['schemas']['SMLConnectionTestResult'];
export type Recipient = components['schemas']['Recipient'];
export type RecipientPage = components['schemas']['RecipientPage'];
export type RecipientQueryInput = components['schemas']['RecipientQueryInput'];
export type RecipientQueryResult = components['schemas']['RecipientQueryResult'];
export type PermissionDependencies = components['schemas']['PermissionDependencies'];
export type ScheduleRecipientOptionsInput = components['schemas']['ScheduleRecipientOptionsInput'];
export type ScheduleRecipientOption = components['schemas']['ScheduleRecipientOption'];
export type ScheduleRecipientOptions = components['schemas']['ScheduleRecipientOptions'];
export type ReportKey = components['schemas']['ReportKey'];
export type ReportDefinition = components['schemas']['ReportDefinition'];
export type AdminReportDefinition = components['schemas']['AdminReportDefinition'];
export type AdminReportCatalog = components['schemas']['AdminReportCatalog'];
export type ScheduleInput = components['schemas']['ScheduleInput'];
export type SchedulePatch = components['schemas']['SchedulePatch'];
export type Schedule = components['schemas']['Schedule'];
export type SchedulePage = components['schemas']['SchedulePage'];
export type FlexPreviewInput = components['schemas']['FlexPreviewInput'];
export type FlexPreview = components['schemas']['FlexPreview'];
export type NotificationExecution = components['schemas']['NotificationExecution'];
export type ReportRun = components['schemas']['ReportRun'];
export type ReportRunDetail = components['schemas']['ReportRunDetail'];
export type ReportRunPage = components['schemas']['ReportRunPage'];
export type FailureEvidence = components['schemas']['FailureEvidence'];
export type FailureImpact = components['schemas']['FailureImpact'];
export type FailurePresentation = components['schemas']['FailurePresentation'];
export type OperationalIncidentEvent = components['schemas']['OperationalIncidentEvent'];
export type ReportRowPage = components['schemas']['ReportRowPage'];
export type ReportRowQueryInput = components['schemas']['ReportRowQueryInput'];
export type ReportRowQueryPage = components['schemas']['ReportRowQueryPage'];
export type ViewerMe = components['schemas']['ViewerMe'];
export type ViewerTenant = components['schemas']['ViewerTenant'];
export type DeliveryContext = components['schemas']['DeliveryContext'];
export type DeliveryReportContext = components['schemas']['DeliveryReportContext'];
export type DeliveryContextReport = components['schemas']['DeliveryContextReport'];
export type DeliveryPage = components['schemas']['DeliveryPage'];
export type LineQuotaStatus = components['schemas']['LineQuotaStatus'];
export type AuditPage = components['schemas']['AuditPage'];
export type OperationalIncident = components['schemas']['OperationalIncident'];
export type OperationalIncidentDetail = components['schemas']['OperationalIncidentDetail'];
export type OperationalIncidentPage = components['schemas']['OperationalIncidentPage'];
export type OperationalIncidentStatus = components['schemas']['OperationalIncidentStatus'];
export type OperationalIncidentSeverity = components['schemas']['OperationalIncidentSeverity'];
export type OperationalIncidentOccurrence = components['schemas']['OperationalIncidentOccurrence'];
export type OperationalIncidentOccurrencePage = components['schemas']['OperationalIncidentOccurrencePage'];
export type OperationalIncidentCauseBreakdown = components['schemas']['OperationalIncidentCauseBreakdown'];
export type IncidentDiagnosis = components['schemas']['IncidentDiagnosis'];
export type AdminFailureAssessment = components['schemas']['AdminFailureAssessment'];
export type JavaWSProtocolEvidence = components['schemas']['JavaWSProtocolEvidence'];
export type TenantsTableQueryInput = components['schemas']['TenantsTableQueryInput'];
export type TenantsTableQueryResult = components['schemas']['TenantsTableQueryResult'];
export type SchedulesTableQueryInput = components['schemas']['SchedulesTableQueryInput'];
export type SchedulesTableQueryResult = components['schemas']['SchedulesTableQueryResult'];
export type ReportRunsTableQueryInput = components['schemas']['ReportRunsTableQueryInput'];
export type ReportRunsTableQueryResult = components['schemas']['ReportRunsTableQueryResult'];
export type DeliveriesTableQueryInput = components['schemas']['DeliveriesTableQueryInput'];
export type DeliveriesTableQueryResult = components['schemas']['DeliveriesTableQueryResult'];
export type AuditTableQueryInput = components['schemas']['AuditTableQueryInput'];
export type AuditTableQueryResult = components['schemas']['AuditTableQueryResult'];
export type IncidentsTableQueryInput = components['schemas']['IncidentsTableQueryInput'];
export type IncidentsTableQueryResult = components['schemas']['IncidentsTableQueryResult'];
export type OccurrencesTableQueryInput = components['schemas']['OccurrencesTableQueryInput'];
export type OccurrencesTableQueryResult = components['schemas']['OccurrencesTableQueryResult'];
export type SMLConnectionReference = components['schemas']['SMLConnectionReference'];
export type WatchdogStatus = components['schemas']['WatchdogStatus'];
export type CreateReportRunInput = components['schemas']['CreateReportRunInput'];
export type MetricUnit = components['schemas']['MetricUnit'];
export type MetricComparison = components['schemas']['MetricComparison'];
export type DashboardMetric = components['schemas']['DashboardMetric'];
export type VisualizationSeries = components['schemas']['VisualizationSeries'];
export type DashboardVisualization = components['schemas']['DashboardVisualization'];
export type DashboardQuality = components['schemas']['DashboardQuality'];
export type ReportDashboard = components['schemas']['ReportDashboard'];
export type DashboardSnapshot = components['schemas']['DashboardSnapshot'];
export type ExecutiveOverview = components['schemas']['ExecutiveOverview'];
export type DashboardRefreshRun = components['schemas']['DashboardRefreshRun'];
export type DashboardRefresh = components['schemas']['DashboardRefresh'];
export type DashboardRefreshInput = components['schemas']['DashboardRefreshInput'];
export type DashboardRefreshFailure = components['schemas']['DashboardRefreshFailure'];
export type DashboardRefreshResult = components['schemas']['DashboardRefreshResult'];
export type DashboardRefreshPolicy = components['schemas']['DashboardRefreshPolicy'];
export type DashboardRefreshPolicyInput = components['schemas']['DashboardRefreshPolicyInput'];
export type ReportRevalidation = components['schemas']['ReportRevalidation'];
export type OverviewRevalidation = components['schemas']['OverviewRevalidation'];
export type FreshnessStatus = NonNullable<DashboardSnapshot['freshnessStatus']>;

export interface DataPage<T> {
  data: T[];
  page: components['schemas']['PageInfo'];
}

export const reportDefinitions: ReportDefinition[] = [
  { reportKey: 'sales_goods_services', version: '1.0.0', label: 'รายงานขายสินค้าและบริการ', category: 'SALES', isSensitive: false, periodMode: 'DATE_RANGE' },
  { reportKey: 'purchase_goods_payables', version: '1.0.0', label: 'รายงานซื้อสินค้าและตั้งหนี้', category: 'PURCHASE', isSensitive: true, periodMode: 'DATE_RANGE' },
  { reportKey: 'gross_profit_by_product', version: '1.0.0', label: 'กำไรขั้นต้นตามสินค้า', category: 'GROSS_PROFIT', isSensitive: true, periodMode: 'DATE_RANGE' },
  { reportKey: 'gross_profit_by_ar_customer', version: '1.0.0', label: 'กำไรขั้นต้นตามลูกหนี้', category: 'GROSS_PROFIT', isSensitive: true, periodMode: 'DATE_RANGE' },
  { reportKey: 'stock_balance', version: '1.0.0', label: 'รายงานสต็อกคงเหลือ', category: 'INVENTORY', isSensitive: true, periodMode: 'AS_OF_DATE' },
  { reportKey: 'stock_reorder', version: '1.0.0', label: 'รายงานสินค้าถึงจุดสั่งซื้อ', category: 'INVENTORY', isSensitive: false, periodMode: 'CURRENT_ONLY' },
  { reportKey: 'ar_customer_movement', version: '1.0.0', label: 'รายงานความเคลื่อนไหวลูกหนี้', category: 'AR', isSensitive: true, periodMode: 'AS_OF_DATE' },
  { reportKey: 'ar_debt_receipt', version: '1.0.0', label: 'รายงานรับชำระหนี้', category: 'AR', isSensitive: true, periodMode: 'DATE_RANGE' },
  { reportKey: 'cash_bank_receipts', version: '1.0.0', label: 'รายงานรับเงิน', category: 'CASH_BANK', isSensitive: true, periodMode: 'DATE_RANGE' },
  { reportKey: 'cash_bank_payments', version: '1.0.0', label: 'รายงานจ่ายเงิน', category: 'CASH_BANK', isSensitive: true, periodMode: 'DATE_RANGE' }
];

export const reportDefinitionByKey = new Map(reportDefinitions.map((item) => [item.reportKey, item]));
