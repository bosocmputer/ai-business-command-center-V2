import type { ReportModeItem, ReportModeMeasurement } from '@/api';

const sourceLabels: Record<ReportModeItem['source'], string> = {
  DEFAULT: 'ค่าเริ่มต้น',
  ENV_SEED: 'ตั้งไว้จากค่าเดิมของระบบ',
  MEASURED: 'วัดขนาดแล้วตั้งให้',
  AUTO_SWITCHED: 'ระบบสลับเอง',
  MANUAL: 'ตั้งด้วยมือ'
};

export function modeLabel(mode: ReportModeItem['mode']): string {
  return mode === 'CHUNKED' ? 'แบ่งชุด' : 'ดึงตรง';
}

export function modeSeverity(mode: ReportModeItem['mode']): 'warn' | 'success' {
  return mode === 'CHUNKED' ? 'warn' : 'success';
}

export function sourceLabel(source: ReportModeItem['source']): string {
  return sourceLabels[source] ?? source;
}

// 388000 ms reads as "6:28 นาที"; null means nothing has been recorded yet.
export function formatModeDuration(milliseconds: number | null): string {
  if (milliseconds === null || milliseconds < 0) return '—';
  if (milliseconds < 1000) return 'ไม่ถึง 1 วินาที';
  const totalSeconds = Math.round(milliseconds / 1000);
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = String(totalSeconds % 60).padStart(2, '0');
  return `${minutes}:${seconds} นาที`;
}

export function formatModeRows(rows: number | null): string {
  return rows === null ? '—' : rows.toLocaleString('th-TH');
}

const measureErrorLabels: Record<string, string> = {
  SML_TIMEOUT: 'ร้านตอบช้าเกินกำหนด',
  SML_UNREACHABLE: 'ต่อ SML ของร้านไม่ได้',
  SML_AUTH_FAILED: 'SML ปฏิเสธรหัสผ่าน'
};

// One line per measured report for the result banner.
export function measurementSummary(item: ReportModeMeasurement, label: string): string {
  if (item.units === null) {
    const reason = item.safeErrorCode ? (measureErrorLabels[item.safeErrorCode] ?? `รหัสข้อผิดพลาด ${item.safeErrorCode}`) : 'วัดไม่สำเร็จ';
    return `${label}: วัดไม่ได้ (${reason})`;
  }
  const size = `${item.units.toLocaleString('th-TH')} รายการ (เกณฑ์ ${item.threshold.toLocaleString('th-TH')})`;
  const advice = item.recommendedMode === 'CHUNKED' ? 'แนะนำแบ่งชุด' : 'ดึงตรงได้';
  return `${label}: ${size} · ${advice}${item.applied ? ' · ตั้งให้แล้ว' : ''}`;
}
