import type { DashboardMetric, DashboardVisualization } from '@/api';
import { formatDashboardValue } from './dashboard';

export type ChartPresentationStatus = 'READY' | 'ALL_ZERO' | 'NO_DATA' | 'PARTIAL_DATA' | 'INVALID_DATA';
export type VisualizationTone = 'primary' | 'warning' | 'danger';

export type ChartSeriesModel = {
  key: string;
  label: string;
  rawValues: string[];
  values: Array<number | null>;
  pointLabels?: string[];
};

export type RankingItem = {
  index: number;
  label: string;
  value: number | null;
  formattedValue: string;
  widthPercent: number;
};

export type CompositionValue = {
  key: string;
  label: string;
  value: number | null;
  formattedValue: string;
  widthPercent: number;
};

export type CompositionRow = {
  index: number;
  label: string;
  values: CompositionValue[];
  percent?: number;
};

export type ChartPresentationModel = {
  status: ChartPresentationStatus;
  warnings: string[];
  labels: string[];
  series: ChartSeriesModel[];
  rankingItems: RankingItem[];
  compositionRows: CompositionRow[];
  tone: VisualizationTone;
};

const supportedUnits = new Set<DashboardMetric['unit']>(['THB', 'COUNT', 'PERCENT', 'QUANTITY', 'RATIO']);

function parseChartValue(value: string): number | null {
  if (!value.trim()) return null;
  const number = Number(value.replaceAll(',', '').trim());
  return Number.isFinite(number) ? number : null;
}

export function visualizationTone(visualization: DashboardVisualization): VisualizationTone {
  if (visualization.key === 'loss_products' || visualization.key === 'loss_customers') return 'danger';
  if (visualization.intent === 'EXCEPTION' || visualization.key === 'reorder_shortage_ratio') return 'warning';
  return 'primary';
}

function baseModel(visualization: DashboardVisualization, status: ChartPresentationStatus, warnings: string[] = []): ChartPresentationModel {
  return { status, warnings, labels: [...visualization.categories], series: [], rankingItems: [], compositionRows: [], tone: visualizationTone(visualization) };
}

export function buildChartPresentation(visualization: DashboardVisualization): ChartPresentationModel {
  if (!supportedUnits.has(visualization.unit)) return baseModel(visualization, 'INVALID_DATA', ['หน่วยของกราฟไม่อยู่ในรายการที่รองรับ']);
  if (!visualization.categories.length) return baseModel(visualization, 'NO_DATA');
  if (!visualization.series.length) return baseModel(visualization, 'INVALID_DATA', ['กราฟไม่มีชุดข้อมูล']);

  for (const series of visualization.series) {
    if (series.values.length !== visualization.categories.length) {
      return baseModel(visualization, 'INVALID_DATA', ['จำนวนค่าในกราฟไม่ตรงกับจำนวนหมวด']);
    }
    if (series.pointLabels && series.pointLabels.length !== visualization.categories.length) {
      return baseModel(visualization, 'INVALID_DATA', ['จำนวนวันที่ของกราฟไม่ตรงกับจำนวนจุดข้อมูล']);
    }
  }

  const series: ChartSeriesModel[] = visualization.series.map((item) => ({
    key: item.key,
    label: item.key === 'previous' ? 'ช่วงเปรียบเทียบ' : item.label,
    rawValues: [...item.values],
    values: item.values.map(parseChartValue),
    pointLabels: item.pointLabels ? [...item.pointLabels] : undefined
  }));
  const values = series.flatMap((item) => item.values);
  const validValues = values.filter((value): value is number => value !== null);
  if (!validValues.length) {
    const hasNonEmptyInput = visualization.series.some((item) => item.values.some((value) => value.trim() !== ''));
    return { ...baseModel(visualization, hasNonEmptyInput ? 'INVALID_DATA' : 'NO_DATA', hasNonEmptyInput ? ['ค่าของกราฟไม่ใช่ตัวเลขที่รองรับ'] : []), series };
  }

  const hasMissing = validValues.length !== values.length;
  const status: ChartPresentationStatus = hasMissing ? 'PARTIAL_DATA' : validValues.every((value) => value === 0) ? 'ALL_ZERO' : 'READY';
  const warnings = hasMissing ? ['ข้อมูลบางจุดไม่สมบูรณ์'] : [];
  const maximumAbsolute = Math.max(...validValues.map(Math.abs), 0);
  const firstSeries = series[0];
  const rankingItems: RankingItem[] = firstSeries && (visualization.intent === 'RANKING' || visualization.intent === 'EXCEPTION')
    ? visualization.categories.map((label, index) => {
        const value = firstSeries.values[index] ?? null;
        return {
          index,
          label,
          value,
          formattedValue: formatDashboardValue(value === null ? undefined : firstSeries.rawValues[index], visualization.unit),
          widthPercent: value === null || maximumAbsolute === 0 ? 0 : Math.abs(value) / maximumAbsolute * 100
        };
      })
    : [];

  let compositionWarning: string | undefined;
  let percentageTotal: number | undefined;
  if (visualization.intent === 'COMPOSITION' && series.length === 1) {
    if (validValues.some((value) => value < 0)) compositionWarning = 'ไม่คำนวณสัดส่วนเปอร์เซ็นต์ เนื่องจากมีค่าติดลบ';
    else {
      const total = validValues.reduce((sum, value) => sum + value, 0);
      if (total > 0 && !hasMissing) percentageTotal = total;
    }
  }
  if (compositionWarning) warnings.push(compositionWarning);
  const compositionRows: CompositionRow[] = visualization.intent === 'COMPOSITION'
    ? visualization.categories.map((label, index) => ({
        index,
        label,
        values: series.map((item) => {
          const value = item.values[index] ?? null;
          return {
            key: item.key,
            label: item.label,
            value,
            formattedValue: formatDashboardValue(value === null ? undefined : item.rawValues[index], visualization.unit),
            widthPercent: value === null || maximumAbsolute === 0 ? 0 : Math.abs(value) / maximumAbsolute * 100
          };
        }),
        percent: percentageTotal === undefined ? undefined : (firstSeries?.values[index] ?? 0) / percentageTotal * 100
      }))
    : [];

  return { status, warnings, labels: [...visualization.categories], series, rankingItems, compositionRows, tone: visualizationTone(visualization) };
}

export function truncateChartLabel(value: string, maximumGraphemes: number): string {
  if (maximumGraphemes < 1) return '';
  const segmenter = new Intl.Segmenter('th', { granularity: 'grapheme' });
  const graphemes = Array.from(segmenter.segment(value), (item) => item.segment);
  return graphemes.length <= maximumGraphemes ? value : `${graphemes.slice(0, maximumGraphemes).join('')}…`;
}

export function chartDateLabel(value: string, includeFullYear: boolean): string {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value);
  if (!match) return value;
  const year = Number(match[1]);
  const month = Number(match[2]) - 1;
  const day = Number(match[3]);
  const date = new Date(Date.UTC(year, month, day, 12));
  if (Number.isNaN(date.getTime()) || date.getUTCFullYear() !== year || date.getUTCMonth() !== month || date.getUTCDate() !== day) return value;
  return new Intl.DateTimeFormat('th-TH', {
    day: 'numeric', month: 'short', year: includeFullYear ? 'numeric' : '2-digit', timeZone: 'Asia/Bangkok'
  }).format(date);
}
