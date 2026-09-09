import { describe, expect, it } from 'vitest';
import { cleanViewerQuery, snapshotReplayInput, validSnapshotRunId, validViewerRunId } from './viewerSnapshot';

describe('viewer snapshot navigation', () => {
  it('accepts only UUID snapshot run identifiers', () => {
    expect(validSnapshotRunId('6bb2747b-65c7-4d5e-9296-40c53b69b09a')).toBe('6bb2747b-65c7-4d5e-9296-40c53b69b09a');
    expect(validSnapshotRunId('not-a-run')).toBeUndefined();
    expect(validSnapshotRunId(['6bb2747b-65c7-4d5e-9296-40c53b69b09a'])).toBeUndefined();
    expect(validViewerRunId('1d669ca3-a9cb-447e-b28a-1798bbaa6beb')).toBe('1d669ca3-a9cb-447e-b28a-1798bbaa6beb');
  });

  it('removes delivery and OAuth references while preserving the snapshot route state', () => {
    expect(cleanViewerQuery({
      snapshotRunId: '6bb2747b-65c7-4d5e-9296-40c53b69b09a',
      deliveryRef: 'secret-reference',
      code: 'oauth-code',
      state: 'oauth-state',
      keep: 'safe'
    })).toEqual({ snapshotRunId: '6bb2747b-65c7-4d5e-9296-40c53b69b09a', keep: 'safe' });
  });

  it('replays the delivered date range except for current-only reorder', () => {
    const run = { periodPreset: 'YESTERDAY', dateFrom: '2026-07-10', dateTo: '2026-07-10' } as const;
    expect(snapshotReplayInput('sales_goods_services', run)).toEqual({ periodPreset: 'CUSTOM', dateFrom: '2026-07-10', dateTo: '2026-07-10' });
    expect(snapshotReplayInput('stock_reorder', run)).toEqual({ periodPreset: 'AS_OF_RUN' });
  });
});
