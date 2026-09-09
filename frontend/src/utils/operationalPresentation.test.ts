import { describe, expect, it } from 'vitest';
import type { OperationalIncidentDetail, OperationalIncidentEvent } from '@/api';
import {
  buildCodexIncidentText,
  causalChain,
  formatDurationMs,
  incidentLifecycleIcon,
  incidentLifecycleSeverity,
  lineImpactLabel,
  reportImpactLabel,
  transportPhaseLabel
} from './operationalPresentation';

const impact = {
  reportsTotal: 10,
  reportsSucceeded: 0,
  reportsFailed: 1,
  reportsCancelled: 9,
  notificationOutcome: 'NOT_CREATED_INCOMPLETE_REPORT_SET' as const
};

it('อธิบายระยะเวลาและผลกระทบเป็นภาษาไทย', () => {
  expect(formatDurationMs(3_200)).toBe('3.2 วินาที');
  expect(formatDurationMs(65_000)).toContain('1 นาที');
  expect(reportImpactLabel(impact)).toContain('ยกเลิก 9');
  expect(lineImpactLabel(impact.notificationOutcome)).toContain('ไม่ได้ส่ง LINE');
  expect(transportPhaseLabel('REQUEST_SENT_RESULT_UNKNOWN')).toContain('ยังไม่ทราบผล');
});

it('แสดงสถานะการเชื่อมต่อเป็นภาษาภาพที่ตรงกับหลักฐาน', () => {
  expect(incidentLifecycleSeverity('CONNECTION_FAILED')).toBe('danger');
  expect(incidentLifecycleIcon('CONNECTION_FAILED')).toContain('exclamation');
  expect(incidentLifecycleSeverity('CONNECTION_RESTORED')).toBe('success');
  expect(incidentLifecycleIcon('CONNECTION_RESTORED')).toContain('check');
  expect(incidentLifecycleSeverity('ACCEPTED_RISK')).toBe('warn');
});

describe('ข้อความสำหรับ Codex', () => {
  it('มี stage, transport, impact และเวลา แต่ไม่มีชื่อร้านหรือ endpoint', () => {
    const event: OperationalIncidentEvent = {
      id: '19b034bc-6fd6-449f-8097-a9994aff45c4',
      eventKind: 'OBSERVED',
      sourceKind: 'REPORT',
      tenantName: 'ร้านที่ห้ามคัดลอก',
      reportKey: 'stock_balance',
      observedAt: '2026-07-16T11:00:04Z',
      isDownstream: false,
      connectionChangedSinceFailure: false,
      impact,
      failureEvidence: {
        version: 1,
        level: 'CONFIRMED',
        category: 'JAVA_WS_CONNECTIVITY',
        stage: 'CONNECT_JAVA_WS',
        transportPhase: 'BEFORE_REQUEST_SENT',
        occurredAt: '2026-07-16T11:00:04Z',
        retryable: true,
        remoteStateUnknown: false,
        protocolEvidence: {
          requestRef: 'NXR-ABCDEFGHIJKLMNOP', requestCount: 1, retryCount: 0,
          responseSha256: 'a'.repeat(64), tenantConcurrentQueries: 1, hostConcurrentQueries: 1
        },
        safeErrorCode: 'SML_UNREACHABLE',
        presentation: { titleTh: 'ติดต่อ Java Web Service ของร้านไม่สำเร็จ', summaryTh: 'ทดสอบ', stageTh: 'เชื่อมต่อ', nextActionsTh: ['ตรวจสอบ'] }
      }
    };
    const incident = {
      id: '14e88612-bb55-4161-8e91-000468c91ed7', alertRef: 'NST-ABC123DEF456', incidentType: 'SCHEDULED_REPORT_FAILED',
      rootCause: 'SML_CONNECTIVITY', severity: 'P1', status: 'OPEN', occurrenceCount: 1, affectedCount: 1,
      activeAffectedCount: 1, observationMode: 'DISCRETE', subjectType: 'TENANT',
      firstSeenAt: '2026-07-16T11:00:04Z', lastSeenAt: '2026-07-16T11:00:04Z', version: 1,
      presentation: event.failureEvidence!.presentation,
      statusPresentation: {
        state: 'CONNECTION_FAILED', headlineTh: 'เชื่อมต่อ Java Web Service ไม่สำเร็จ',
        statusSummaryTh: 'ควรตรวจสอบ Java Web Service, Network และ Server ของลูกค้า', actionRequired: true
      },
      isDownstream: false, events: [event]
    } satisfies OperationalIncidentDetail;

    const value = buildCodexIncidentText(incident);

    expect(value).toContain('stage=CONNECT_JAVA_WS');
    expect(value).toContain('transport=BEFORE_REQUEST_SENT');
    expect(value).toContain('cancelled=9');
    expect(value).not.toContain(event.tenantName);
    expect(value).not.toContain('endpoint');
    expect(value).not.toContain('NXR-ABCDEFGHIJKLMNOP');
    expect(value).not.toContain('a'.repeat(64));
    expect(causalChain(event)).toHaveLength(4);
  });
});
