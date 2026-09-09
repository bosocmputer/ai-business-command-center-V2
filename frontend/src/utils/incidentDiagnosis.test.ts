import { describe, expect, it } from 'vitest';
import type { IncidentDiagnosis } from '@/api';
import { isLegacyDiagnosis, javaWSStatusLabel, matchingSuccessLabel, protocolRequestLabel } from './incidentDiagnosis';

function diagnosis(overrides: Partial<IncidentDiagnosis> = {}): IncidentDiagnosis {
  return {
    assessment: {
      problemArea: 'CUSTOMER_JAVA_WS', investigationOwner: 'CUSTOMER_IT', loadSignal: 'NO_NEXTSTEP_LOAD_SIGNAL',
      summaryTh: 'เชื่อมต่อ Java Web Service สำเร็จ แต่ข้อมูลตอบกลับไม่ใช่ ZIP', problemAreaTh: 'คำตอบจาก Java Web Service',
      ownerTh: 'ผู้ดูแล Java Web Service ของลูกค้า', loadSignalTh: 'ไม่พบสัญญาณว่า Nextstep สร้างภาระผิดปกติ', customerActionTh: 'ตรวจ Java Web Service'
    },
    protocolEvidence: {
      requestRef: 'NXR-ABCDEFGHIJKLMNOP', requestCount: 1, retryCount: 0, httpStatus: 200,
      soapValid: true, base64Valid: true, zipSignatureValid: false,
      tenantConcurrentQueries: 1, hostConcurrentQueries: 1
    },
    baseline: { p50Ms: 80, p90Ms: 96, sampleCount: 8 },
    ...overrides
  };
}

describe('Incident diagnosis presentation', () => {
  it('explains confirmed invalid ZIP without blaming an unspecified subsystem', () => {
    const value = diagnosis();
    expect(javaWSStatusLabel(value)).toBe('เชื่อมต่อสำเร็จและได้รับคำตอบ แต่ข้อมูลไม่ใช่ ZIP');
    expect(protocolRequestLabel(value)).toBe('1 ครั้ง · ไม่มี Retry');
    expect(isLegacyDiagnosis(value)).toBe(false);
  });

  it('states when the same exact report succeeded before and after', () => {
    const value = diagnosis({
      priorMatchingSuccess: { finishedAt: '2026-07-21T00:00:00Z', durationMs: 80 },
      subsequentMatchingSuccess: { finishedAt: '2026-07-21T02:00:00Z', durationMs: 75 }
    });
    expect(matchingSuccessLabel(value)).toContain('ทั้งก่อนและหลัง');
  });

  it('does not invent JavaWS protocol facts for legacy evidence', () => {
    const value = diagnosis({ protocolEvidence: undefined, priorMatchingSuccess: undefined, subsequentMatchingSuccess: undefined });
    expect(isLegacyDiagnosis(value)).toBe(true);
    expect(javaWSStatusLabel(value)).toContain('หลักฐานจากระบบรุ่นเดิมไม่เพียงพอ');
    expect(protocolRequestLabel(value)).toContain('หลักฐานรุ่นเดิม');
  });
});
