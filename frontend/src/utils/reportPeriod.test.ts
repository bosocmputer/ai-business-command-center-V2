import { beforeEach, describe, expect, it } from 'vitest';
import {
  clearStoredPeriodSelections,
  defaultPeriodSelection,
  loadPeriodSelection,
  periodModeForReport,
  savePeriodSelection,
  selectionForMode,
  selectionFromReportPeriod,
  selectionToRunInput,
  validatePeriodSelection,
  type ReportPeriodSelection
} from './reportPeriod';

const now = new Date('2026-07-12T02:00:00.000Z');

describe('report period model', () => {
  beforeEach(() => sessionStorage.clear());

  it('maps all report keys to their supported period mode', () => {
    expect(periodModeForReport('sales_goods_services')).toBe('DATE_RANGE');
    expect(periodModeForReport('stock_balance')).toBe('AS_OF_DATE');
    expect(periodModeForReport('stock_reorder')).toBe('CURRENT_ONLY');
    expect(periodModeForReport('ar_customer_movement')).toBe('AS_OF_DATE');
    expect(periodModeForReport('cash_bank_payments')).toBe('DATE_RANGE');
  });

  it('uses month-to-date as the first-use selection', () => {
    expect(defaultPeriodSelection()).toEqual({ periodPreset: 'MONTH_TO_DATE' });
  });

  it('validates custom dates against Bangkok today and the 366-day limit', () => {
    expect(validatePeriodSelection({ periodPreset: 'CUSTOM', dateFrom: '2026-07-01', dateTo: '2026-07-12' }, now).valid).toBe(true);
    expect(validatePeriodSelection({ periodPreset: 'CUSTOM', dateFrom: '2026-07-13', dateTo: '2026-07-13' }, now).message).toContain('อนาคต');
    expect(validatePeriodSelection({ periodPreset: 'CUSTOM', dateFrom: '2026-07-12', dateTo: '2026-07-01' }, now).message).toContain('สิ้นสุด');
    expect(validatePeriodSelection({ periodPreset: 'CUSTOM', dateFrom: '2025-07-11', dateTo: '2026-07-12' }, now).message).toContain('366');
  });

  it('maps one smart selection to date-range, as-of and current-only inputs', () => {
    const selection: ReportPeriodSelection = { periodPreset: 'CUSTOM', dateFrom: '2026-07-01', dateTo: '2026-07-10' };
    expect(selectionToRunInput('DATE_RANGE', selection, now)).toEqual(selection);
    expect(selectionToRunInput('AS_OF_DATE', selection, now)).toEqual({ periodPreset: 'CUSTOM', dateFrom: '2026-07-10', dateTo: '2026-07-10' });
    expect(selectionToRunInput('CURRENT_ONLY', selection, now)).toEqual({ periodPreset: 'AS_OF_RUN' });
  });

  it('normalizes a shared range to its end date for as-of reports', () => {
    expect(selectionForMode('AS_OF_DATE', { periodPreset: 'MONTH_TO_DATE' }, now)).toEqual({ periodPreset: 'TODAY_TO_NOW' });
    expect(selectionForMode('AS_OF_DATE', {
      periodPreset: 'CUSTOM',
      dateFrom: '2026-06-01',
      dateTo: '2026-06-18'
    }, now)).toEqual({ periodPreset: 'CUSTOM', dateFrom: '2026-06-18', dateTo: '2026-06-18' });
  });

  it('reconstructs a shared selection from an exact dashboard period', () => {
    expect(selectionFromReportPeriod('DATE_RANGE', { preset: 'CUSTOM', dateFrom: '2026-07-01', dateTo: '2026-07-10' }, now)).toEqual({ periodPreset: 'CUSTOM', dateFrom: '2026-07-01', dateTo: '2026-07-10' });
    expect(selectionFromReportPeriod('AS_OF_DATE', { preset: 'CUSTOM', dateFrom: '2026-07-10', dateTo: '2026-07-10' }, now)).toEqual({ periodPreset: 'CUSTOM', dateFrom: '2026-07-10', dateTo: '2026-07-10' });
    expect(selectionFromReportPeriod('AS_OF_DATE', { preset: 'AS_OF_RUN', dateFrom: '2026-07-12', dateTo: '2026-07-12' }, now)).toEqual({ periodPreset: 'TODAY_TO_NOW' });
    expect(selectionFromReportPeriod('DATE_RANGE', { preset: 'YESTERDAY', dateFrom: '2026-07-10', dateTo: '2026-07-10' }, now)).toEqual({ periodPreset: 'CUSTOM', dateFrom: '2026-07-10', dateTo: '2026-07-10' });
  });

  it('stores a versioned selection per tenant and discards malformed or future values', () => {
    savePeriodSelection('tenant-a', { periodPreset: 'YESTERDAY' }, now);
    expect(loadPeriodSelection('tenant-a', now)).toEqual({ periodPreset: 'YESTERDAY' });
    sessionStorage.setItem('nextstep.viewer.period.v1:tenant-b', JSON.stringify({ version: 1, selection: { periodPreset: 'CUSTOM', dateFrom: '2026-07-13', dateTo: '2026-07-13' } }));
    expect(loadPeriodSelection('tenant-b', now)).toEqual(defaultPeriodSelection());
    clearStoredPeriodSelections();
    expect(sessionStorage.getItem('nextstep.viewer.period.v1:tenant-a')).toBeNull();
  });
});
