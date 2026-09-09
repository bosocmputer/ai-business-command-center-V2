import { describe, expect, it } from 'vitest';
import type { DashboardVisualization } from '@/api';
import {
  buildChartPresentation,
  chartDateLabel,
  truncateChartLabel,
  visualizationTone
} from './chartPresentation';

function visualization(overrides: Partial<DashboardVisualization> = {}): DashboardVisualization {
  return {
    key: 'top_stock_value',
    title: 'สินค้าที่มีมูลค่าคงเหลือสูงสุด',
    intent: 'RANKING',
    unit: 'THB',
    categories: ['สินค้าชื่อยาวมากสำหรับทดสอบบนโทรศัพท์', 'สินค้ารอง'],
    series: [{ key: 'value', label: 'มูลค่า', values: ['10000000', '100000'] }],
    ...overrides
  };
}

describe('chart presentation model', () => {
  it('builds proportional ranking rows without inflating small values', () => {
    const model = buildChartPresentation(visualization());

    expect(model.status).toBe('READY');
    expect(model.rankingItems.map((item) => item.widthPercent)).toEqual([100, 1]);
    expect(model.rankingItems[0]?.formattedValue).toBe('฿10,000,000');
  });

  it('distinguishes zero, no data, partial data, and invalid contracts', () => {
    expect(buildChartPresentation(visualization({ categories: [], series: [] })).status).toBe('NO_DATA');
    expect(buildChartPresentation(visualization({ series: [{ key: 'value', label: 'มูลค่า', values: ['0', '0'] }] })).status).toBe('ALL_ZERO');
    expect(buildChartPresentation(visualization({ series: [{ key: 'value', label: 'มูลค่า', values: ['100', ''] }] })).status).toBe('PARTIAL_DATA');
    expect(buildChartPresentation(visualization({ series: [{ key: 'value', label: 'มูลค่า', values: ['100'] }] })).status).toBe('INVALID_DATA');
  });

  it('rejects mismatched trend point labels instead of assigning the wrong date', () => {
    const model = buildChartPresentation(visualization({
      key: 'sales_trend',
      intent: 'TREND',
      categories: ['1', '2'],
      series: [{ key: 'current', label: 'งวดปัจจุบัน', values: ['10', '20'], pointLabels: ['2026-07-01'] }]
    }));

    expect(model.status).toBe('INVALID_DATA');
  });

  it('calculates composition percentages only for non-negative totals', () => {
    const positive = buildChartPresentation(visualization({
      key: 'cash_receipt_methods',
      intent: 'COMPOSITION',
      categories: ['เงินสด', 'เงินโอน'],
      series: [{ key: 'amount', label: 'จำนวนเงิน', values: ['25', '75'] }]
    }));
    const negative = buildChartPresentation(visualization({
      key: 'cash_receipt_methods',
      intent: 'COMPOSITION',
      categories: ['เงินสด', 'ปรับปรุง'],
      series: [{ key: 'amount', label: 'จำนวนเงิน', values: ['25', '-5'] }]
    }));

    expect(positive.compositionRows.map((row) => row.percent)).toEqual([25, 75]);
    expect(negative.compositionRows.every((row) => row.percent === undefined)).toBe(true);
    expect(negative.warnings).toContain('ไม่คำนวณสัดส่วนเปอร์เซ็นต์ เนื่องจากมีค่าติดลบ');
  });

  it('handles Thai graphemes and exact Thai chart dates', () => {
    expect(truncateChartLabel('สินค้าที่มีชื่อยาวมากเป็นพิเศษ', 10)).toBe('สินค้าที่มีชื่อยา…');
    expect(chartDateLabel('2026-07-12', false)).toBe('12 ก.ค. 69');
    expect(chartDateLabel('2026-07-12', true)).toBe('12 ก.ค. 2569');
    expect(chartDateLabel('2026-02-31', true)).toBe('2026-02-31');
  });

  it('uses semantic tones only for verified loss and exception charts', () => {
    expect(visualizationTone(visualization({ key: 'loss_products' }))).toBe('danger');
    expect(visualizationTone(visualization({ key: 'reorder_shortage_ratio' }))).toBe('warning');
    expect(visualizationTone(visualization())).toBe('primary');
  });
});
