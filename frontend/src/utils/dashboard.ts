import type { DashboardMetric, DashboardSnapshot, DashboardVisualization, ReportDashboard, ReportKey } from '@/api';

type DashboardPeriod = ReportDashboard['period'];

export type ExecutiveKpi = {
  key: string;
  label: string;
  icon: string;
  reportKey: ReportKey;
  metricKey: string;
  metric?: DashboardMetric;
  period?: DashboardPeriod;
  comparisonPeriod?: DashboardPeriod;
};

const executiveKpiDefinitions: Omit<ExecutiveKpi, 'metric' | 'period' | 'comparisonPeriod'>[] = [
  { key: 'sales', label: 'ยอดขาย', icon: 'pi pi-shopping-cart', reportKey: 'sales_goods_services', metricKey: 'total_amount' },
  { key: 'profit', label: 'กำไรขั้นต้น', icon: 'pi pi-chart-line', reportKey: 'gross_profit_by_product', metricKey: 'gross_profit_amount' },
  { key: 'inventory', label: 'มูลค่าสต็อก', icon: 'pi pi-box', reportKey: 'stock_balance', metricKey: 'balance_amount' },
  { key: 'receivable', label: 'ลูกหนี้เคลื่อนไหวสุทธิ', icon: 'pi pi-users', reportKey: 'ar_customer_movement', metricKey: 'net_movement_amount' }
];

export const executiveFeaturedVisualizationKeys = {
  sales_goods_services: 'sales_trend',
  gross_profit_by_product: 'top_profit_products',
  stock_balance: 'top_stock_value',
  cash_bank_receipts: 'cash_receipt_trend'
} satisfies Partial<Record<ReportKey, string>>;

export function numberForChart(value: string): number | null {
  const number = Number(value.replaceAll(',', '').trim());
  return Number.isFinite(number) ? number : null;
}

export function chartTypeFor(visualization: DashboardVisualization): 'line' | 'bar' {
  return visualization.intent === 'TREND' && visualization.categories.length >= 8 ? 'line' : 'bar';
}

export function visualizationCategoryLabels(visualization: DashboardVisualization): string[] {
  const currentLabels = visualization.series.find((series) => series.key === 'current')?.pointLabels;
  if (currentLabels?.length === visualization.categories.length && currentLabels.every(Boolean)) return currentLabels;
  return visualization.categories;
}

export function snapshotForReport(items: DashboardSnapshot[], reportKey: ReportKey): DashboardSnapshot | undefined {
  return items
    .filter((item) => item.dashboard.reportKey === reportKey)
    .sort((left, right) => Date.parse(right.dashboard.generatedAt) - Date.parse(left.dashboard.generatedAt))[0];
}

export function metricForReport(items: DashboardSnapshot[], reportKey: ReportKey, metricKey: string): DashboardMetric | undefined {
  return snapshotForReport(items, reportKey)?.dashboard.kpis.find((metric) => metric.key === metricKey);
}

export function buildExecutiveKpis(items: DashboardSnapshot[]): ExecutiveKpi[] {
  return executiveKpiDefinitions.map((definition) => {
    const snapshot = snapshotForReport(items, definition.reportKey);
    return {
      ...definition,
      metric: snapshot?.dashboard.kpis.find((metric) => metric.key === definition.metricKey),
      period: snapshot?.dashboard.period,
      comparisonPeriod: snapshot?.dashboard.comparisonPeriod
    };
  });
}

export function dashboardMetricValue(metric?: DashboardMetric): string | undefined {
  return metric?.value;
}

export function formatDashboardValue(value: string | undefined, unit: DashboardMetric['unit']): string {
  if (value === undefined || value === '') return 'ยังไม่มีข้อมูล';
  const number = numberForChart(value);
  if (number === null) return value;
  const maximumFractionDigits = unit === 'COUNT' ? 0 : 2;
  const formatted = new Intl.NumberFormat('th-TH', { minimumFractionDigits: 0, maximumFractionDigits }).format(number);
  if (unit === 'THB') return `฿${formatted}`;
  if (unit === 'PERCENT') return `${formatted}%`;
  return formatted;
}

export function periodLabel(preset: string): string {
  const labels: Record<string, string> = {
    YESTERDAY: 'เมื่อวาน',
    TODAY_TO_NOW: 'วันนี้ถึงปัจจุบัน',
    MONTH_TO_DATE: 'เดือนนี้ถึงปัจจุบัน',
    AS_OF_RUN: 'ณ เวลาที่อัปเดต',
    CUSTOM: 'ช่วงวันที่ที่เลือก'
  };
  return labels[preset] ?? preset;
}

function formatIsoDateOnly(value: string): string {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value);
  if (!match) return value;
  const year = Number(match[1]);
  const month = Number(match[2]) - 1;
  const day = Number(match[3]);
  const date = new Date(Date.UTC(year, month, day, 12));
  if (Number.isNaN(date.getTime()) || date.getUTCFullYear() !== year || date.getUTCMonth() !== month || date.getUTCDate() !== day) return value;
  return new Intl.DateTimeFormat('th-TH', {
    day: 'numeric',
    month: 'short',
    year: 'numeric',
    timeZone: 'Asia/Bangkok'
  }).format(date);
}

export function formatPeriodRange(period?: DashboardPeriod): string {
  if (!period) return 'ยังไม่ระบุช่วงข้อมูล';
  const from = formatIsoDateOnly(period.dateFrom);
  const to = formatIsoDateOnly(period.dateTo);
  return from === to ? from : `${from} – ${to}`;
}

export function comparisonPeriodText(period?: DashboardPeriod): string {
  return period ? `เทียบกับ ${formatPeriodRange(period)}` : 'ไม่มีข้อมูลเปรียบเทียบที่เทียบช่วงเวลาเดียวกันได้';
}

export function reportIcon(reportKey: ReportKey): string {
  if (reportKey.startsWith('sales_')) return 'pi pi-shopping-cart';
  if (reportKey.startsWith('purchase_')) return 'pi pi-truck';
  if (reportKey.startsWith('gross_profit_')) return 'pi pi-chart-line';
  if (reportKey.startsWith('stock_')) return 'pi pi-box';
  if (reportKey.startsWith('ar_')) return 'pi pi-users';
  return reportKey === 'cash_bank_receipts' ? 'pi pi-wallet' : 'pi pi-credit-card';
}
