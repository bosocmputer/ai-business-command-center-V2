import { describe, expect, it } from 'vitest';
import { auditActionLabel, reportRunErrorLabel, statusLabel } from './status';

describe('statusLabel', () => {
  it('translates operational states without obscuring unknown codes', () => {
    expect(statusLabel('SUCCEEDED')).toBe('สำเร็จ');
    expect(statusLabel('FAILED_PERMANENT')).toBe('ส่งไม่สำเร็จ');
    expect(statusLabel('FUTURE_STATE')).toBe('FUTURE_STATE');
    expect(statusLabel()).toBe('—');
  });
});

describe('auditActionLabel', () => {
  it('uses business language for known actions and a readable fallback', () => {
    expect(auditActionLabel('SCHEDULE_TEST_SEND_ENQUEUED')).toBe('เริ่มทดสอบส่งรายงานจริง');
    expect(auditActionLabel('TENANT_UPDATED')).toBe('แก้ไขข้อมูลร้านค้า');
    expect(auditActionLabel('FUTURE_ACTION')).toBe('FUTURE ACTION');
  });
});

describe('reportRunErrorLabel', () => {
  it('explains bounded JavaWS ZIP failures without exposing technical payloads', () => {
    expect(reportRunErrorLabel('SML_ZIP_FORMAT_INVALID')).toContain('รูปแบบ ZIP');
    expect(reportRunErrorLabel('SML_ZIP_EMPTY')).toContain('ไม่มีข้อมูล');
    expect(reportRunErrorLabel('SML_ZIP_TOO_LARGE')).toContain('ขนาดใหญ่');
    expect(reportRunErrorLabel('SML_ZIP_READ_FAILED')).toContain('อ่านผลลัพธ์');
    expect(reportRunErrorLabel('SML_ZIP_INVALID')).toContain('ไม่สมบูรณ์');
    expect(reportRunErrorLabel('FUTURE_ERROR')).toBe('');
  });
});
