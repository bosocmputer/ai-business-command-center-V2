import { describe, expect, it } from 'vitest';
import type { ReportRevalidation } from '@/api';
import { autoRefreshNotice, AUTO_REFRESH_POLL_HIDDEN_MS, AUTO_REFRESH_POLL_MS, nextAutoRefreshStep } from './reportAutoRefresh';

const snapshot = { runId: 'run-1', freshnessStatus: 'FRESH' } as unknown as NonNullable<ReportRevalidation['snapshot']>;
const result = (partial: Partial<ReportRevalidation>): ReportRevalidation => ({ disposition: 'FRESH_CACHE', retryAfter: 0, legacyFallback: false, ...partial });
const run = (status: string) => ({ id: 'run-2', status }) as unknown as NonNullable<ReportRevalidation['run']>;

describe('nextAutoRefreshStep', () => {
  it('shows fresh figures at once', () => {
    expect(nextAutoRefreshStep(result({ snapshot }), false)).toEqual({ kind: 'fresh', snapshot });
  });

  it('treats fresh cache without figures as a failure instead of waiting forever', () => {
    expect(nextAutoRefreshStep(result({}), false)).toEqual({ kind: 'failed' });
  });

  it('keeps following a read that is running, more slowly in a hidden tab', () => {
    for (const disposition of ['STALE_REFRESHING', 'MISSING_REFRESHING', 'JOINED'] as const) {
      expect(nextAutoRefreshStep(result({ disposition, run: run('RUNNING') }), false)).toEqual({ kind: 'wait', waitMs: AUTO_REFRESH_POLL_MS });
      expect(nextAutoRefreshStep(result({ disposition, run: run('QUEUED') }), true)).toEqual({ kind: 'wait', waitMs: AUTO_REFRESH_POLL_HIDDEN_MS });
    }
  });

  it('asks again soon after a read has finished so the new figures are picked up', () => {
    expect(nextAutoRefreshStep(result({ disposition: 'JOINED', run: run('SUCCEEDED') }), false)).toEqual({ kind: 'wait', waitMs: 1500 });
  });

  it('gives up when the read ended without a result', () => {
    for (const status of ['FAILED', 'CANCELLED', 'EXPIRED']) {
      expect(nextAutoRefreshStep(result({ disposition: 'STALE_REFRESHING', run: run(status) }), false)).toEqual({ kind: 'failed' });
    }
  });

  it('says it is blocked when the server will not read now, with the wait it gave', () => {
    expect(nextAutoRefreshStep(result({ disposition: 'CIRCUIT_OPEN', retryAfter: 90 }), false)).toEqual({ kind: 'blocked', retryAfterSeconds: 90 });
    expect(nextAutoRefreshStep(result({ disposition: 'DISABLED', retryAfter: 0 }), false)).toEqual({ kind: 'blocked', retryAfterSeconds: 0 });
  });
});

describe('autoRefreshNotice', () => {
  it('uses plain words, never Snapshot or SML', () => {
    const texts = [autoRefreshNotice('updating', true, '08:17 น.'), autoRefreshNotice('blocked', true, '08:17 น.'), autoRefreshNotice('failed', false, '')];
    for (const text of texts) expect(text).not.toMatch(/snapshot|sml|cache/i);
    expect(texts[0]).toContain('08:17 น.');
    expect(texts[2]).toContain('ลองอีกครั้ง');
  });

  it('is silent when nothing is happening or when there is nothing to say yet', () => {
    expect(autoRefreshNotice('idle', true, 'x')).toBe('');
    expect(autoRefreshNotice('updating', false, '')).toBe('');
  });
});
