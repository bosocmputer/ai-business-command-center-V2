import type { CreateReportRunInput, ReportKey, ReportRun } from '@/api';

const uuidPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const transientQueryKeys = new Set(['deliveryRef', 'ref', 'code', 'state', 'liffClientId', 'liffRedirectUri']);

export function validSnapshotRunId(value: unknown): string | undefined {
  return typeof value === 'string' && uuidPattern.test(value) ? value : undefined;
}

export function validViewerRunId(value: unknown): string | undefined {
  return typeof value === 'string' && uuidPattern.test(value) ? value : undefined;
}

export function cleanViewerQuery(query: Record<string, unknown>): Record<string, string | string[]> {
  const cleaned: Record<string, string | string[]> = {};
  for (const [key, value] of Object.entries(query)) {
    if (transientQueryKeys.has(key)) continue;
    if (typeof value === 'string' || Array.isArray(value) && value.every((item) => typeof item === 'string')) cleaned[key] = value as string | string[];
  }
  return cleaned;
}

export function snapshotReplayInput(reportKey: ReportKey, run: Pick<ReportRun, 'periodPreset' | 'dateFrom' | 'dateTo'>): CreateReportRunInput {
  if (reportKey === 'stock_reorder') return { periodPreset: 'AS_OF_RUN' };
  if (!run.dateFrom || !run.dateTo) return { periodPreset: run.periodPreset as CreateReportRunInput['periodPreset'] };
  return { periodPreset: 'CUSTOM', dateFrom: run.dateFrom, dateTo: run.dateTo };
}
