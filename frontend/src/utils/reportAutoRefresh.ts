import type { DashboardSnapshot, ReportRevalidation } from '@/api';

// A report page that opens on figures that are old (or on none at all) asks the server to read them again by itself and follows it until
// the new figures are there. The server decides whether a read is allowed and shares one read between everyone asking, so the page only
// has to follow what it is told.

export type AutoRefreshState = 'idle' | 'updating' | 'blocked' | 'failed';

export type AutoRefreshStep =
  | { kind: 'fresh'; snapshot: DashboardSnapshot }
  | { kind: 'wait'; waitMs: number }
  | { kind: 'blocked'; retryAfterSeconds: number }
  | { kind: 'unsupported' }
  | { kind: 'failed' };

const FINISHED_WITHOUT_RESULT = new Set(['FAILED', 'CANCELLED', 'EXPIRED']);

export const AUTO_REFRESH_POLL_MS = 4_000;
export const AUTO_REFRESH_POLL_HIDDEN_MS = 15_000;
// A read can take a while for a big shop (a stock report took about two minutes), so the page follows it for this long.
export const AUTO_REFRESH_LIMIT_MS = 4 * 60_000;

export function nextAutoRefreshStep(result: ReportRevalidation, hidden: boolean): AutoRefreshStep {
  const pollMs = hidden ? AUTO_REFRESH_POLL_HIDDEN_MS : AUTO_REFRESH_POLL_MS;
  // The server has this feature switched off: keep the page as it was (old figures with the manual button) and say nothing.
  if (result.legacyFallback) return { kind: 'unsupported' };
  switch (result.disposition) {
    case 'FRESH_CACHE':
      return result.snapshot ? { kind: 'fresh', snapshot: result.snapshot } : { kind: 'failed' };
    case 'CIRCUIT_OPEN':
    case 'DISABLED':
      return { kind: 'blocked', retryAfterSeconds: Math.max(0, result.retryAfter ?? 0) };
    case 'STALE_REFRESHING':
    case 'MISSING_REFRESHING':
    case 'JOINED': {
      const status = result.run?.status;
      if (status && FINISHED_WITHOUT_RESULT.has(status)) return { kind: 'failed' };
      // A read that has just finished is picked up by the next question as fresh figures.
      if (status === 'SUCCEEDED') return { kind: 'wait', waitMs: 1_500 };
      return { kind: 'wait', waitMs: pollMs };
    }
    default:
      return { kind: 'failed' };
  }
}

export function autoRefreshNotice(state: AutoRefreshState, hasFigures: boolean, shownAt: string): string {
  if (state === 'updating') return hasFigures ? `กำลังอัปเดตตัวเลขให้เป็นล่าสุด ตอนนี้แสดงข้อมูลเมื่อ ${shownAt}` : '';
  if (state === 'blocked' || state === 'failed') {
    return hasFigures
      ? `ตอนนี้อัปเดตตัวเลขไม่ได้ กำลังแสดงข้อมูลเมื่อ ${shownAt}`
      : 'ยังไม่มีข้อมูลของรายงานนี้ และตอนนี้ดึงข้อมูลใหม่ไม่ได้ ลองอีกครั้งในอีกสักครู่';
  }
  return '';
}
