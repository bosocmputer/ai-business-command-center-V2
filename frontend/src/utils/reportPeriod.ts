import type { CreateReportRunInput, ReportDefinition, ReportKey } from '@/api';

export type ReportPeriodMode = NonNullable<ReportDefinition['periodMode']>;
export type SmartPeriodPreset = 'YESTERDAY' | 'TODAY_TO_NOW' | 'MONTH_TO_DATE' | 'CUSTOM';
export type ReportPeriodSelection = {
  periodPreset: SmartPeriodPreset;
  dateFrom?: string;
  dateTo?: string;
};

export type PeriodValidation = {
  valid: boolean;
  message: string;
  warning: string;
  dateFrom?: string;
  dateTo?: string;
};

const storagePrefix = 'nextstep.viewer.period.v1:';
const reportPeriodModes: Record<ReportKey, ReportPeriodMode> = {
  sales_goods_services: 'DATE_RANGE',
  purchase_goods_payables: 'DATE_RANGE',
  gross_profit_by_product: 'DATE_RANGE',
  gross_profit_by_ar_customer: 'DATE_RANGE',
  stock_balance: 'AS_OF_DATE',
  stock_reorder: 'CURRENT_ONLY',
  ar_customer_movement: 'AS_OF_DATE',
  ar_debt_receipt: 'DATE_RANGE',
  cash_bank_receipts: 'DATE_RANGE',
  cash_bank_payments: 'DATE_RANGE'
};

export function periodModeForReport(reportKey: ReportKey): ReportPeriodMode {
  return reportPeriodModes[reportKey];
}

export function defaultPeriodSelection(): ReportPeriodSelection {
  return { periodPreset: 'MONTH_TO_DATE' };
}

export function bangkokToday(now = new Date()): string {
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone: 'Asia/Bangkok', year: 'numeric', month: '2-digit', day: '2-digit'
  }).formatToParts(now);
  const value = Object.fromEntries(parts.map((part) => [part.type, part.value]));
  return `${value.year}-${value.month}-${value.day}`;
}

export function validatePeriodSelection(selection: ReportPeriodSelection, now = new Date()): PeriodValidation {
  const resolved = resolveSelection(selection, now);
  if (!resolved) return { valid: false, message: 'กรุณาเลือกวันที่ให้ครบถ้วน', warning: '' };
  const { dateFrom, dateTo } = resolved;
  if (dateTo < dateFrom) return { valid: false, message: 'วันที่สิ้นสุดต้องไม่น้อยกว่าวันที่เริ่มต้น', warning: '', dateFrom, dateTo };
  if (dateFrom > bangkokToday(now) || dateTo > bangkokToday(now)) return { valid: false, message: 'ไม่สามารถเลือกวันที่ในอนาคตได้', warning: '', dateFrom, dateTo };
  const days = inclusiveDays(dateFrom, dateTo);
  if (!Number.isFinite(days) || days < 1) return { valid: false, message: 'รูปแบบวันที่ไม่ถูกต้อง', warning: '', dateFrom, dateTo };
  if (days > 366) return { valid: false, message: 'เลือกช่วงข้อมูลได้สูงสุด 366 วัน', warning: '', dateFrom, dateTo };
  return {
    valid: true,
    message: '',
    warning: days > 90 ? 'ช่วงข้อมูลเกิน 90 วัน อาจใช้เวลาประมวลผลนานกว่าปกติ' : '',
    dateFrom,
    dateTo
  };
}

export function selectionToRunInput(mode: ReportPeriodMode, selection: ReportPeriodSelection, now = new Date()): CreateReportRunInput {
  if (mode === 'CURRENT_ONLY') return { periodPreset: 'AS_OF_RUN' };
  const validation = validatePeriodSelection(selection, now);
  if (!validation.valid || !validation.dateFrom || !validation.dateTo) throw new Error(validation.message || 'ช่วงข้อมูลไม่ถูกต้อง');
  if (mode === 'DATE_RANGE') {
    if (selection.periodPreset === 'CUSTOM') return { periodPreset: 'CUSTOM', dateFrom: validation.dateFrom, dateTo: validation.dateTo };
    return { periodPreset: selection.periodPreset };
  }
  if (selection.periodPreset === 'TODAY_TO_NOW' || selection.periodPreset === 'MONTH_TO_DATE') return { periodPreset: 'AS_OF_RUN' };
  return { periodPreset: 'CUSTOM', dateFrom: validation.dateTo, dateTo: validation.dateTo };
}

export function selectionForMode(mode: ReportPeriodMode, selection: ReportPeriodSelection, now = new Date()): ReportPeriodSelection {
  if (mode !== 'AS_OF_DATE') return { ...selection };
  const validation = validatePeriodSelection(selection, now);
  if (!validation.valid || !validation.dateTo) return { periodPreset: 'TODAY_TO_NOW' };
  const today = bangkokToday(now);
  if (validation.dateTo === today) return { periodPreset: 'TODAY_TO_NOW' };
  if (validation.dateTo === shiftDate(today, -1)) return { periodPreset: 'YESTERDAY' };
  return { periodPreset: 'CUSTOM', dateFrom: validation.dateTo, dateTo: validation.dateTo };
}

export function selectionFromReportPeriod(
  mode: ReportPeriodMode,
  period: { preset: string; dateFrom: string; dateTo: string },
  now = new Date()
): ReportPeriodSelection {
  if (mode === 'DATE_RANGE') {
    if (period.preset === 'YESTERDAY' || period.preset === 'TODAY_TO_NOW' || period.preset === 'MONTH_TO_DATE') {
      const relative = { periodPreset: period.preset } as ReportPeriodSelection;
      const resolved = validatePeriodSelection(relative, now);
      if (resolved.dateFrom === period.dateFrom && resolved.dateTo === period.dateTo) return relative;
    }
    return { periodPreset: 'CUSTOM', dateFrom: period.dateFrom, dateTo: period.dateTo };
  }
  if (mode === 'AS_OF_DATE') {
    if (period.dateTo === bangkokToday(now)) return { periodPreset: 'TODAY_TO_NOW' };
    if (period.dateTo === shiftDate(bangkokToday(now), -1)) return { periodPreset: 'YESTERDAY' };
    return { periodPreset: 'CUSTOM', dateFrom: period.dateTo, dateTo: period.dateTo };
  }
  return defaultPeriodSelection();
}

export function selectionLabel(selection: ReportPeriodSelection, mode: ReportPeriodMode | 'SMART_OVERVIEW', now = new Date()): string {
  if (mode === 'CURRENT_ONLY') return 'สถานะปัจจุบัน';
  const validation = validatePeriodSelection(selection, now);
  if (!validation.valid || !validation.dateFrom || !validation.dateTo) return 'ยังไม่ได้เลือกช่วงข้อมูล';
  if (mode === 'AS_OF_DATE') return `ณ วันที่ ${formatThaiDateOnly(validation.dateTo)}`;
  if (validation.dateFrom === validation.dateTo) return formatThaiDateOnly(validation.dateTo);
  return `${formatThaiDateOnly(validation.dateFrom)}–${formatThaiDateOnly(validation.dateTo)}`;
}

export function loadPeriodSelection(tenantId: string, now = new Date()): ReportPeriodSelection {
  if (!tenantId) return defaultPeriodSelection();
  try {
    const raw = sessionStorage.getItem(`${storagePrefix}${tenantId}`);
    if (!raw) return defaultPeriodSelection();
    const stored = JSON.parse(raw) as { version?: unknown; selection?: unknown };
    if (stored.version !== 1 || !isSelection(stored.selection) || !validatePeriodSelection(stored.selection, now).valid) throw new Error('invalid stored period');
    return { ...stored.selection };
  } catch {
    try { sessionStorage.removeItem(`${storagePrefix}${tenantId}`); } catch { /* storage unavailable */ }
    return defaultPeriodSelection();
  }
}

export function savePeriodSelection(tenantId: string, selection: ReportPeriodSelection, now = new Date()): void {
  if (!tenantId || !validatePeriodSelection(selection, now).valid) return;
  try { sessionStorage.setItem(`${storagePrefix}${tenantId}`, JSON.stringify({ version: 1, selection })); } catch { /* storage unavailable */ }
}

export function clearStoredPeriodSelections(allowedTenantIds?: string[]): void {
  try {
    const allowed = allowedTenantIds ? new Set(allowedTenantIds) : undefined;
    for (let index = sessionStorage.length - 1; index >= 0; index--) {
      const key = sessionStorage.key(index);
      if (key?.startsWith(storagePrefix) && (!allowed || !allowed.has(key.slice(storagePrefix.length)))) sessionStorage.removeItem(key);
    }
  } catch { /* storage unavailable */ }
}

function resolveSelection(selection: ReportPeriodSelection, now: Date): { dateFrom: string; dateTo: string } | undefined {
  const today = bangkokToday(now);
  switch (selection.periodPreset) {
    case 'TODAY_TO_NOW': return { dateFrom: today, dateTo: today };
    case 'YESTERDAY': {
      const yesterday = shiftDate(today, -1);
      return { dateFrom: yesterday, dateTo: yesterday };
    }
    case 'MONTH_TO_DATE': return { dateFrom: `${today.slice(0, 8)}01`, dateTo: today };
    case 'CUSTOM':
      if (!isDateOnly(selection.dateFrom) || !isDateOnly(selection.dateTo)) return undefined;
      return { dateFrom: selection.dateFrom, dateTo: selection.dateTo };
  }
}

function isSelection(value: unknown): value is ReportPeriodSelection {
  if (!value || typeof value !== 'object') return false;
  const candidate = value as Partial<ReportPeriodSelection>;
  return ['YESTERDAY', 'TODAY_TO_NOW', 'MONTH_TO_DATE', 'CUSTOM'].includes(String(candidate.periodPreset))
    && (candidate.dateFrom === undefined || typeof candidate.dateFrom === 'string')
    && (candidate.dateTo === undefined || typeof candidate.dateTo === 'string');
}

function isDateOnly(value: unknown): value is string {
  if (typeof value !== 'string' || !/^\d{4}-\d{2}-\d{2}$/.test(value)) return false;
  const date = new Date(`${value}T00:00:00Z`);
  return !Number.isNaN(date.getTime()) && date.toISOString().slice(0, 10) === value;
}

function inclusiveDays(dateFrom: string, dateTo: string): number {
  const from = Date.parse(`${dateFrom}T00:00:00Z`);
  const to = Date.parse(`${dateTo}T00:00:00Z`);
  return Math.floor((to - from) / 86_400_000) + 1;
}

function shiftDate(value: string, days: number): string {
  const date = new Date(`${value}T00:00:00Z`);
  date.setUTCDate(date.getUTCDate() + days);
  return date.toISOString().slice(0, 10);
}

function formatThaiDateOnly(value: string): string {
  return new Intl.DateTimeFormat('th-TH', { day: 'numeric', month: 'short', year: 'numeric', timeZone: 'UTC' }).format(new Date(`${value}T00:00:00Z`));
}
