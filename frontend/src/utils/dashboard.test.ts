import { describe, expect, it } from 'vitest';
import type { DashboardMetric, DashboardSnapshot, DashboardVisualization } from '@/api';
import {
  buildExecutiveKpis,
  chartTypeFor,
  comparisonPeriodText,
  dashboardMetricValue,
  formatDashboardValue,
  formatPeriodRange,
  numberForChart,
  snapshotForReport,
  visualizationCategoryLabels,
  executiveFeaturedVisualizationKeys
} from './dashboard';

function metric(key: string, value: string): DashboardMetric {
  return { key, label: key, value, unit: 'THB', comparison: { availability: 'UNAVAILABLE' } };
}

function snapshot(reportKey: DashboardSnapshot['dashboard']['reportKey'], generatedAt: string, kpis: DashboardMetric[]): DashboardSnapshot {
  return {
    runId: crypto.randomUUID(),
    dashboard: {
      reportKey,
      version: '1.0.0',
      period: { preset: 'YESTERDAY', dateFrom: '2026-07-09', dateTo: '2026-07-09' },
      comparisonPeriod: { preset: 'CUSTOM', dateFrom: '2026-07-08', dateTo: '2026-07-08' },
      timezone: 'Asia/Bangkok',
      generatedAt,
      kpis,
      visualizations: [],
      quality: { status: 'OK', warnings: [] }
    }
  };
}

describe('dashboard helpers', () => {
  it('converts valid decimals for Chart.js without emitting non-finite values', () => {
    expect(numberForChart('1,234.50')).toBe(1234.5);
    expect(numberForChart('-8.25')).toBe(-8.25);
    expect(numberForChart('not-a-number')).toBeNull();
    expect(numberForChart('1e999')).toBeNull();
  });

  it('formats metric units for Thai executives', () => {
    expect(formatDashboardValue('604058.00', 'THB')).toBe('฿604,058');
    expect(formatDashboardValue('8.19', 'PERCENT')).toBe('8.19%');
    expect(formatDashboardValue(undefined, 'THB')).toBe('ยังไม่มีข้อมูล');
  });

  it('selects the newest snapshot for a report defensively', () => {
    const oldItem = snapshot('sales_goods_services', '2026-07-09T10:00:00Z', []);
    const newest = snapshot('sales_goods_services', '2026-07-10T10:00:00Z', []);
    expect(snapshotForReport([newest, oldItem], 'sales_goods_services')).toBe(newest);
  });

  it('maps visualization intent to an executive-readable chart type', () => {
    const base: DashboardVisualization = { key: 'x', title: 'x', intent: 'TREND', unit: 'THB', categories: [], series: [] };
    expect(chartTypeFor({ ...base, categories: Array.from({ length: 8 }, (_, index) => String(index)) })).toBe('line');
    expect(chartTypeFor({ ...base, categories: ['A', 'B'] })).toBe('bar');
    expect(chartTypeFor({ ...base, intent: 'RANKING' })).toBe('bar');
    expect(chartTypeFor({ ...base, intent: 'COMPOSITION' })).toBe('bar');
  });

  it('uses backend point labels for trend dates and falls back to categories', () => {
    const trend: DashboardVisualization = { key: 'trend', title: 'trend', intent: 'TREND', unit: 'THB', categories: ['1', '2'], series: [{ key: 'current', label: 'ปัจจุบัน', values: ['1', '2'], pointLabels: ['2026-07-01', '2026-07-02'] }] };
    expect(visualizationCategoryLabels(trend)).toEqual(['2026-07-01', '2026-07-02']);
    expect(visualizationCategoryLabels({ ...trend, series: [] })).toEqual(['1', '2']);
  });

  it('describes report and comparison periods with exact Thai dates', () => {
    expect(formatPeriodRange({ preset: 'YESTERDAY', dateFrom: '2026-07-09', dateTo: '2026-07-09' })).toBe('9 ก.ค. 2569');
    expect(formatPeriodRange({ preset: 'CUSTOM', dateFrom: '2026-06-28', dateTo: '2026-07-02' })).toBe('28 มิ.ย. 2569 – 2 ก.ค. 2569');
    expect(comparisonPeriodText({ preset: 'CUSTOM', dateFrom: '2026-07-08', dateTo: '2026-07-08' })).toBe('เทียบกับ 8 ก.ค. 2569');
  });

  it('builds the four owner KPIs without substituting missing reports with zero', () => {
    const sales = snapshot('sales_goods_services', '2026-07-10T10:00:00Z', [metric('total_amount', '604058.00')]);
    const profit = snapshot('gross_profit_by_product', '2026-07-10T10:00:00Z', [metric('gross_profit_amount', '46308.80')]);
    const result = buildExecutiveKpis([sales, profit]);
    expect(dashboardMetricValue(result[0]?.metric)).toBe('604058.00');
    expect(formatPeriodRange(result[0]?.period)).toBe('9 ก.ค. 2569');
    expect(comparisonPeriodText(result[0]?.comparisonPeriod)).toBe('เทียบกับ 8 ก.ค. 2569');
    expect(dashboardMetricValue(result[1]?.metric)).toBe('46308.80');
    expect(result[2]?.metric).toBeUndefined();
    expect(result[3]?.metric).toBeUndefined();
  });

  it('selects overview charts by the backend visualization keys', () => {
    expect(executiveFeaturedVisualizationKeys.gross_profit_by_product).toBe('top_profit_products');
    expect(executiveFeaturedVisualizationKeys.stock_balance).toBe('top_stock_value');
  });
});
