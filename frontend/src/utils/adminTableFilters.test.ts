import { describe, expect, it } from 'vitest';
import { normalizeScheduleTableRow, normalizedAuditAction, toDateFilter, toRecipientQueryInput } from './adminTableFilters';

describe('admin table filters', () => {
  it('serializes the selected local calendar date without a UTC shift', () => {
    expect(toDateFilter(new Date(2026, 6, 19, 23, 30))).toBe('2026-07-19');
  });

  it('normalizes an audit action into the backend-safe contract', () => {
    expect(normalizedAuditAction(' tenant updated ')).toBe('TENANT_UPDATED');
    expect(normalizedAuditAction('')).toBeUndefined();
  });

  it('adapts the shared server-table envelope to the flat recipient query contract', () => {
    expect(toRecipientQueryInput({
      page: 0,
      pageSize: 25,
      globalSearch: 'ผู้บริหาร',
      filters: { statuses: ['ACTIVE'], permissionStates: ['WITH_REPORTS'] }
    })).toEqual({
      page: 0,
      pageSize: 25,
      globalSearch: 'ผู้บริหาร',
      statuses: ['ACTIVE'],
      permissionStates: ['WITH_REPORTS']
    });
  });

  it('normalizes nullable schedule collections before the table renders them', () => {
    const schedule = normalizeScheduleTableRow({
      id: 'schedule-1',
      readinessBlockers: null,
      nextOccurrences: null
    });

    expect(schedule.readinessBlockers).toEqual([]);
    expect(schedule.nextOccurrences).toEqual([]);
  });
});
