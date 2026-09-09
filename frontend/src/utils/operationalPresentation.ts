import type {
  FailureEvidence,
  FailureImpact,
  OperationalIncidentDetail,
  OperationalIncidentEvent
} from '@/api';

type IncidentLifecycleState = OperationalIncidentDetail['statusPresentation']['state'];

export function incidentLifecycleSeverity(value: IncidentLifecycleState): 'danger' | 'success' | 'warn' | 'secondary' {
  if (value === 'CONNECTION_FAILED' || value === 'ACTIVE_PROBLEM') return 'danger';
  if (value === 'CONNECTION_RESTORED' || value === 'RESOLVED') return 'success';
  if (value === 'ACCEPTED_RISK') return 'warn';
  return 'secondary';
}

export function incidentLifecycleIcon(value: IncidentLifecycleState): string {
  if (value === 'CONNECTION_FAILED' || value === 'ACTIVE_PROBLEM') return 'pi pi-exclamation-triangle';
  if (value === 'CONNECTION_RESTORED' || value === 'RESOLVED') return 'pi pi-check-circle';
  return 'pi pi-info-circle';
}

export function formatDurationMs(value?: number): string {
  if (value === undefined || value < 0) return 'ไม่ทราบ';
  if (value < 1_000) return `${value.toLocaleString('th-TH')} มิลลิวินาที`;
  const seconds = value / 1_000;
  if (seconds < 60) return `${seconds.toLocaleString('th-TH', { maximumFractionDigits: seconds < 10 ? 1 : 0 })} วินาที`;
  const minutes = Math.floor(seconds / 60);
  const remainder = Math.round(seconds % 60);
  return remainder > 0 ? `${minutes.toLocaleString('th-TH')} นาที ${remainder.toLocaleString('th-TH')} วินาที` : `${minutes.toLocaleString('th-TH')} นาที`;
}

export function triggerKindLabel(value?: string): string {
  return ({
    SCHEDULED: 'รอบส่ง LINE ตามตาราง',
    TEST: 'การทดสอบส่งโดย Admin',
    DASHBOARD: 'ผู้ใช้ดึงจาก Dashboard',
    VIEWER: 'ผู้ใช้ดึงจาก Dashboard',
    BACKGROUND: 'การอัปเดตข้อมูลเบื้องหลัง',
    UNKNOWN: 'งานจากระบบรุ่นเดิม'
  } as Record<string, string>)[value ?? ''] ?? 'ไม่ทราบแหล่งงาน';
}

export function sourceKindLabel(value?: string): string {
  return ({
    NOTIFICATION: 'การเตรียมข้อความ LINE', DELIVERY: 'การส่งข้อความ LINE', REPORT: 'การสร้างรายงาน',
    WORKER: 'ระบบประมวลผลงาน', SML_CIRCUIT: 'ระบบป้องกันการเชื่อมต่อ SML', HOST: 'Server Dashboard',
    BACKUP: 'ระบบสำรองข้อมูล', DATABASE: 'ฐานข้อมูล Dashboard'
  } as Record<string, string>)[value ?? ''] ?? 'ระบบส่วนกลาง';
}

export function eventKindLabel(value: string): string {
  return ({
    OBSERVED: 'ตรวจพบต้นเหตุ', CONDITION_UPDATED: 'ค่าที่เฝ้าระวังเปลี่ยนแปลง', DOWNSTREAM_IMPACT: 'ตรวจพบผลกระทบต่อเนื่อง', SUBJECT_RECOVERED: 'ส่วนที่ได้รับผลฟื้นตัวแล้ว', ACKNOWLEDGED: 'Admin รับทราบ',
    POLICY_CHANGED: 'เปลี่ยนนโยบายการติดตาม',
    EVIDENCE_RESOLVED: 'ระบบยืนยันการฟื้นตัว', RISK_ACCEPTED: 'Admin ยอมรับความเสี่ยง',
    ALERT_SENT: 'ส่ง Telegram แล้ว', ALERT_FAILED: 'ส่ง Telegram ไม่สำเร็จ'
  } as Record<string, string>)[value] ?? 'เหตุการณ์จากระบบ';
}

export function evidenceLevelLabel(value?: string): string {
  return value === 'CONFIRMED' ? 'หลักฐานยืนยันจากขั้นตอนที่ล้ม' : 'หลักฐานบางส่วนจากระบบรุ่นเดิม';
}

export function transportPhaseLabel(value?: string): string {
  return ({
    BEFORE_REQUEST_SENT: 'ยังไม่เริ่มส่งคำขอไปยัง Server ลูกค้า',
    REQUEST_SENT_RESULT_UNKNOWN: 'ส่งคำขอแล้ว แต่ยังไม่ทราบผลจาก Server ลูกค้า',
    RESPONSE_STARTED: 'เริ่มรับคำตอบแล้ว แต่รับข้อมูลไม่ครบ'
  } as Record<string, string>)[value ?? ''] ?? 'ไม่มีรายละเอียดการรับส่งข้อมูลเพิ่มเติม';
}

export function reportImpactLabel(impact?: FailureImpact): string {
  if (!impact) return 'ยังไม่มีข้อมูลผลกระทบที่ยืนยันได้';
  const parts = [
    `ทั้งหมด ${impact.reportsTotal.toLocaleString('th-TH')} รายงาน`,
    `สำเร็จ ${impact.reportsSucceeded.toLocaleString('th-TH')}`,
    `ล้มเหลว ${impact.reportsFailed.toLocaleString('th-TH')}`,
    `ยกเลิก ${impact.reportsCancelled.toLocaleString('th-TH')}`
  ];
  return parts.join(' · ');
}

export function lineImpactLabel(outcome?: FailureImpact['notificationOutcome']): string {
  return ({
    NOT_APPLICABLE: 'งานนี้ไม่ใช่รอบส่ง LINE ตามตาราง',
    NOT_CREATED_INCOMPLETE_REPORT_SET: 'ไม่ได้ส่ง LINE เพราะสร้างรายงานในรอบนี้ไม่ครบ',
    CREATED: 'ระบบสร้างรายการส่ง LINE แล้ว',
    UNKNOWN: 'ระบบรุ่นเดิมไม่มีหลักฐานยืนยันผลการส่ง LINE'
  } as Record<string, string>)[outcome ?? ''] ?? 'ยังไม่มีหลักฐานยืนยันผลต่อ LINE';
}

export function causalChain(event?: OperationalIncidentEvent): string[] {
  if (!event) return [];
  const result: string[] = [];
  const evidence = event.failureEvidence;
  if (evidence) result.push(evidence.presentation.titleTh);
  if (event.reportKey) result.push('รายงานที่เกี่ยวข้องสร้างไม่สำเร็จ');
  if (event.impact?.reportsCancelled) result.push(`ยกเลิกรายงานที่ยังไม่เริ่ม ${event.impact.reportsCancelled.toLocaleString('th-TH')} รายงาน`);
  if (event.impact) result.push(lineImpactLabel(event.impact.notificationOutcome));
  return [...new Set(result)];
}

export function buildCodexIncidentText(incident: OperationalIncidentDetail): string {
  const lines = [
    'Nextstep Sentinel incident',
    `alert_ref: ${incident.alertRef}`,
    `severity: ${incident.severity}`,
    `status: ${incident.status}`,
    `safe_error_code: ${incident.safeErrorCode || 'UNKNOWN'}`,
    `observation_mode: ${incident.observationMode}`,
    `subject_type: ${incident.subjectType}`,
    `active_affected_count: ${incident.activeAffectedCount}`,
    `first_seen_at: ${incident.firstSeenAt}`,
    `last_seen_at: ${incident.lastSeenAt}`
  ];
  incident.events.slice(0, 20).forEach((event, index) => {
    const evidence: FailureEvidence | undefined = event.failureEvidence;
    lines.push(
      `event_${index + 1}: kind=${event.eventKind} source=${event.sourceKind || 'UNKNOWN'} report=${event.reportKey || 'NONE'} stage=${evidence?.stage || 'UNKNOWN'} transport=${evidence?.transportPhase || 'UNKNOWN'} observed_at=${event.observedAt}`
    );
    if (event.impact) {
      lines.push(`event_${index + 1}_impact: total=${event.impact.reportsTotal} succeeded=${event.impact.reportsSucceeded} failed=${event.impact.reportsFailed} cancelled=${event.impact.reportsCancelled} line=${event.impact.notificationOutcome}`);
    }
  });
  return lines.join('\n');
}
