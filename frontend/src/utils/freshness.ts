import type { FreshnessStatus, ReportRun } from '@/api';

export function freshnessPresentation(status?: FreshnessStatus) {
  const presentations = {
    FRESH: { label: 'ข้อมูลล่าสุด', severity: 'success' as const, icon: 'pi pi-check-circle' },
    STALE: { label: 'ข้อมูลเก่า', severity: 'warn' as const, icon: 'pi pi-clock' },
    EXPIRED: { label: 'ข้อมูลล่าสุดไม่พร้อมใช้งาน', severity: 'danger' as const, icon: 'pi pi-exclamation-circle' },
    MISSING: { label: 'ยังไม่มีข้อมูลช่วงนี้', severity: 'secondary' as const, icon: 'pi pi-minus-circle' },
    REFRESHING: { label: 'กำลังอัปเดต', severity: 'info' as const, icon: 'pi pi-sync' },
    REFRESH_FAILED: { label: 'อัปเดตไม่สำเร็จ', severity: 'danger' as const, icon: 'pi pi-times-circle' }
  };
  return presentations[status ?? 'MISSING'];
}

export function progressPresentation(phase?: NonNullable<ReportRun['progress']>['phase']) {
  const labels: Record<string, string> = {
    QUEUED: 'กำลังรอคิว',
    CONNECTING: 'กำลังเชื่อมต่อ SML',
    QUERYING_CURRENT: 'กำลังดึงข้อมูลจาก SML',
    QUERYING_COMPARISON: 'กำลังดึงข้อมูลช่วงเปรียบเทียบ',
    BUILDING_DASHBOARD: 'กำลังคำนวณภาพรวม',
    SAVING_RESULT: 'กำลังบันทึกผลลัพธ์',
    WAITING_RETRY: 'กำลังรอลองเชื่อมต่อใหม่',
    COMPLETED: 'ข้อมูลพร้อมใช้งาน'
  };
  return labels[phase ?? 'QUEUED'] ?? 'กำลังเตรียมรายงาน';
}

export function typicalDurationText(progress?: Pick<NonNullable<ReportRun['progress']>, 'expectedP50Ms' | 'expectedP90Ms' | 'sampleCount'>) {
  if (!progress || progress.sampleCount < 5 || !progress.expectedP50Ms || !progress.expectedP90Ms) return '';
  const lower = Math.max(1, Math.round(progress.expectedP50Ms / 1000));
  const upper = Math.max(lower, Math.round(progress.expectedP90Ms / 1000));
  return `โดยปกติประมาณ ${lower.toLocaleString('th-TH')}–${upper.toLocaleString('th-TH')} วินาที`;
}
