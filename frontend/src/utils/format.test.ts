import { describe, expect, it } from 'vitest';
import { errorMessage, formatDateOnly, formatDateTimeWithMilliseconds, formatMetric, formatSourceCollection, formatTime, metricLabel } from './format';

describe('format utilities', () => {
  it('preserves the local calendar date instead of converting through UTC', () => {
    const selected = new Date(2026, 6, 10, 0, 0, 0);
    expect(formatDateOnly(selected)).toBe('2026-07-10');
  });

  it('formats numeric metrics and safe empty values', () => {
    expect(formatMetric('1234.50')).toContain('1,234.5');
    expect(formatMetric(null)).toBe('—');
  });

  it('uses Thai KPI labels with a readable fallback', () => {
    expect(metricLabel('gross_profit_amount')).toBe('กำไรขั้นต้น');
    expect(metricLabel('doc_no')).toBe('เลขที่เอกสาร');
    expect(metricLabel('custom_metric')).toBe('custom metric');
  });

  it('turns known API error codes into actionable Thai messages', () => {
    expect(errorMessage({ code: 'REPORT_CONCURRENCY_LIMIT', message: 'English provider text' })).toContain('กำลังสร้างรายงาน');
    expect(errorMessage({ code: 'FUTURE_ERROR', message: 'ข้อความสำรอง' })).toBe('ข้อความสำรอง');
  });

  it('explains guarded JavaWS tests without encouraging an unsafe retry', () => {
    expect(errorMessage({ code: 'SML_TEST_BUSY', message: 'English' })).toContain('กำลังติดต่อ Server ลูกค้า');
    expect(errorMessage({ code: 'SML_TIMEOUT', message: 'English' })).toContain('อย่าเริ่มทดสอบซ้ำทันที');
  });
});

describe('formatTime', () => {
  it('formats UTC timestamps as Thailand time', () => {
    expect(formatTime('2026-07-12T13:24:00Z')).toBe('20:24');
  });

  it('returns a safe placeholder for invalid values', () => {
    expect(formatTime('not-a-date')).toBe('—');
    expect(formatTime()).toBe('—');
  });
});

describe('formatDateTimeWithMilliseconds', () => {
  it('preserves milliseconds while formatting the timestamp in Thailand time', () => {
    const formatted = formatDateTimeWithMilliseconds('2026-07-12T13:24:00.123Z');

    expect(formatted).toMatch(/20:24:00/);
    expect(formatted).toMatch(/[.,]123/);
  });

  it('returns a safe placeholder for an invalid or missing timestamp', () => {
    expect(formatDateTimeWithMilliseconds('not-a-date')).toBe('—');
    expect(formatDateTimeWithMilliseconds()).toBe('—');
  });
});

describe('formatSourceCollection', () => {
  it('describes chunked results as a collection window instead of a point-in-time snapshot', () => {
    expect(formatSourceCollection('2026-07-12T01:30:00Z', '2026-07-12T01:36:00Z', 'CHUNK_WINDOW')).toContain('08:30–08:36');
  });

  it('uses a completed-at label for a direct statement result', () => {
    const label = formatSourceCollection('2026-07-12T01:30:00Z', '2026-07-12T01:30:20Z', 'STATEMENT');
    expect(label).toContain('ดึงจาก SML สำเร็จเมื่อ');
    expect(label).not.toContain('ระหว่าง');
  });
});
