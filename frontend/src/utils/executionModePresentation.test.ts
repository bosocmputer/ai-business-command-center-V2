import { describe, expect, it } from 'vitest';
import { formatModeDuration, formatModeRows, measurementSummary, modeLabel, modeSeverity, sourceLabel } from './executionModePresentation';

describe('execution mode presentation', () => {
  it('labels modes and sources for the admin page', () => {
    expect(modeLabel('CHUNKED')).toBe('แบ่งชุด');
    expect(modeLabel('DIRECT')).toBe('ดึงตรง');
    expect(modeSeverity('CHUNKED')).toBe('warn');
    expect(modeSeverity('DIRECT')).toBe('success');
    expect(sourceLabel('AUTO_SWITCHED')).toBe('ระบบสลับเอง');
    expect(sourceLabel('MANUAL')).toBe('ตั้งด้วยมือ');
  });

  it('formats duration as minutes and seconds', () => {
    expect(formatModeDuration(388000)).toBe('6:28 นาที');
    expect(formatModeDuration(41000)).toBe('0:41 นาที');
    expect(formatModeDuration(400)).toBe('ไม่ถึง 1 วินาที');
    expect(formatModeDuration(null)).toBe('—');
    expect(formatModeRows(8080)).toBe('8,080');
    expect(formatModeRows(null)).toBe('—');
  });

  it('summarises a measurement, including a failed one', () => {
    expect(measurementSummary({ reportKey: 'stock_balance', units: 8080, recommendedMode: 'CHUNKED', applied: true, threshold: 5000 }, 'สต็อกคงเหลือ'))
      .toBe('สต็อกคงเหลือ: 8,080 รายการ (เกณฑ์ 5,000) · แนะนำแบ่งชุด · ตั้งให้แล้ว');
    expect(measurementSummary({ reportKey: 'ar_customer_movement', units: 900, recommendedMode: 'DIRECT', applied: false, threshold: 8000 }, 'ลูกหนี้'))
      .toBe('ลูกหนี้: 900 รายการ (เกณฑ์ 8,000) · ดึงตรงได้');
    expect(measurementSummary({ reportKey: 'stock_balance', units: null, recommendedMode: '', applied: false, threshold: 5000, safeErrorCode: 'SML_TIMEOUT' }, 'สต็อกคงเหลือ'))
      .toBe('สต็อกคงเหลือ: วัดไม่ได้ (ร้านตอบช้าเกินกำหนด)');
    expect(measurementSummary({ reportKey: 'stock_balance', units: null, recommendedMode: '', applied: false, threshold: 5000, safeErrorCode: 'ODD' }, 'x'))
      .toContain('รหัสข้อผิดพลาด ODD');
  });
});
