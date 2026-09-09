import { describe, expect, it } from 'vitest';
import { freshnessPresentation, progressPresentation, typicalDurationText } from './freshness';

describe('freshness presentation', () => {
  it('uses explicit copy instead of claiming realtime data', () => {
    expect(freshnessPresentation('FRESH').label).toBe('ข้อมูลล่าสุด');
    expect(freshnessPresentation('STALE').label).toBe('ข้อมูลเก่า');
    expect(freshnessPresentation('EXPIRED').label).toContain('ไม่พร้อม');
    expect(freshnessPresentation('REFRESHING').label).toBe('กำลังอัปเดต');
  });

  it('maps backend progress phases to honest Thai status', () => {
    expect(progressPresentation('QUERYING_CURRENT')).toContain('SML');
    expect(progressPresentation('QUERYING_COMPARISON')).toContain('เปรียบเทียบ');
    expect(progressPresentation('SAVING_RESULT')).toContain('บันทึก');
  });

  it('shows a historical range only with enough samples', () => {
    expect(typicalDurationText({ expectedP50Ms: 45_000, expectedP90Ms: 47_000, sampleCount: 11 })).toBe('โดยปกติประมาณ 45–47 วินาที');
    expect(typicalDurationText({ expectedP50Ms: 45_000, expectedP90Ms: 47_000, sampleCount: 4 })).toBe('');
  });
});
