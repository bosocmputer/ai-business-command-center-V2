import { describe, expect, it } from 'vitest';
import type { ReportDrillLink } from '@/api';
import { drillAnchors, drillPeriodFor, drillQuery, drillRequestFromQuery, drillValueOf, withoutDrillQuery } from './reportDrill';

const link = (column: string, targetReport: ReportDrillLink['targetReport']): ReportDrillLink => ({ column, labelColumn: 'cust_name', kind: 'CUSTOMER', targetReport, targetColumn: 'cust_code' });

describe('report drill-down', () => {
  const links = [link('cust_code', 'ar_aging'), link('cust_code', 'customer_rfm'), { column: 'doc_no', kind: 'DOCUMENT', targetReport: 'sales_goods_services', targetColumn: 'doc_no' } as ReportDrillLink];

  it('offers links on the identifier column when it is on screen', () => {
    const anchors = drillAnchors(links, ['cust_code', 'cust_name', 'doc_no', 'balance']);
    expect(anchors.get('cust_code')?.links.map((item) => item.targetReport)).toEqual(['ar_aging', 'customer_rfm']);
    expect(anchors.get('cust_code')?.valueColumn).toBe('cust_code');
    expect(anchors.has('cust_name')).toBe(false);
    expect(anchors.get('doc_no')?.links).toHaveLength(1);
  });

  it('offers customer links on the name column when the code is hidden, reading the code from the row', () => {
    const anchors = drillAnchors(links, ['cust_name', 'balance']);
    expect(anchors.get('cust_name')?.valueColumn).toBe('cust_code');
    expect(anchors.get('cust_name')?.links).toHaveLength(2);
    expect(anchors.has('doc_no')).toBe(false);
  });

  it('offers nothing when neither the key nor its name is shown, or there are no links', () => {
    expect(drillAnchors(links, ['balance']).size).toBe(0);
    expect(drillAnchors(undefined, ['cust_code']).size).toBe(0);
  });

  it('carries only a usable cell value', () => {
    expect(drillValueOf(' C001 ')).toBe('C001');
    expect(drillValueOf(1500)).toBe('1500');
    expect(drillValueOf('')).toBeUndefined();
    expect(drillValueOf('   ')).toBeUndefined();
    expect(drillValueOf(null)).toBeUndefined();
    expect(drillValueOf({})).toBeUndefined();
    expect(drillValueOf('x'.repeat(161))).toBeUndefined();
  });

  it('round-trips a drill request through the address', () => {
    const query = drillQuery(link('cust_code', 'ar_aging'), 'C001', 'customer_rfm');
    expect(query).toEqual({ drillColumn: 'cust_code', drillValue: 'C001', drillFrom: 'customer_rfm' });
    expect(drillRequestFromQuery(query, new Set(['cust_code', 'doc_no']))).toEqual({ column: 'cust_code', value: 'C001', from: 'customer_rfm' });
  });

  it('ignores an address the target report could not filter on', () => {
    const columns = new Set(['cust_code']);
    expect(drillRequestFromQuery({ drillColumn: 'doc_no', drillValue: 'A1' }, columns)).toBeUndefined();
    expect(drillRequestFromQuery({ drillColumn: 'Cust Code; drop', drillValue: 'A1' }, new Set(['Cust Code; drop']))).toBeUndefined();
    expect(drillRequestFromQuery({ drillColumn: 'cust_code', drillValue: '   ' }, columns)).toBeUndefined();
    expect(drillRequestFromQuery({ drillColumn: 'cust_code', drillValue: 'x'.repeat(161) }, columns)).toBeUndefined();
    expect(drillRequestFromQuery({ drillColumn: ['cust_code'], drillValue: 'C1' }, columns)).toBeUndefined();
    expect(drillRequestFromQuery({}, columns)).toBeUndefined();
  });

  it('removes only the drill keys from the address', () => {
    expect(withoutDrillQuery({ drillColumn: 'cust_code', drillValue: 'C1', drillFrom: 'ar_aging', runId: 'abc' })).toEqual({ runId: 'abc' });
  });

  describe('the period a drill opens the target for', () => {
    const now = new Date('2026-10-01T05:00:00Z');
    const customer = link('cust_code', 'sales_goods_services');
    const document: ReportDrillLink = { column: 'doc_no', dateColumn: 'doc_date', kind: 'DOCUMENT', targetReport: 'sales_goods_services', targetColumn: 'doc_no' };

    it('opens a date-range target for the document\'s own day', () => {
      expect(drillPeriodFor(document, 'DATE_RANGE', 'AS_OF_DATE', { dateFrom: '2026-10-01', dateTo: '2026-10-01' }, { doc_date: '2022-03-11' }, now)).toEqual({ dateFrom: '2022-03-11', dateTo: '2022-03-11' });
    });

    it('leaves an as-of target alone for a document, and ignores a missing or future document date', () => {
      expect(drillPeriodFor({ ...document, targetReport: 'ar_customer_movement' }, 'AS_OF_DATE', 'AS_OF_DATE', undefined, { doc_date: '2022-03-11' }, now)).toBeUndefined();
      expect(drillPeriodFor(document, 'DATE_RANGE', 'AS_OF_DATE', undefined, {}, now)).toBeUndefined();
      expect(drillPeriodFor(document, 'DATE_RANGE', 'AS_OF_DATE', undefined, { doc_date: '2027-01-01' }, now)).toBeUndefined();
    });

    it('hands a date-range source\'s range to a date-range target and its last day to an as-of target', () => {
      const source = { dateFrom: '2026-04-03', dateTo: '2026-09-30' };
      expect(drillPeriodFor(customer, 'DATE_RANGE', 'DATE_RANGE', source, {}, now)).toEqual(source);
      expect(drillPeriodFor(link('cust_code', 'ar_aging'), 'AS_OF_DATE', 'DATE_RANGE', source, {}, now)).toEqual({ dateFrom: '2026-09-30', dateTo: '2026-09-30' });
    });

    it('opens a date-range target for the 90 days up to an as-of source\'s date', () => {
      expect(drillPeriodFor(customer, 'DATE_RANGE', 'AS_OF_DATE', { dateFrom: '2026-10-01', dateTo: '2026-10-01' }, {}, now)).toEqual({ dateFrom: '2026-07-04', dateTo: '2026-10-01' });
    });

    it('leaves the selection alone when the target has no period or the source has none to give', () => {
      expect(drillPeriodFor(customer, 'CURRENT_ONLY', 'DATE_RANGE', { dateFrom: '2026-04-03', dateTo: '2026-09-30' }, {}, now)).toBeUndefined();
      expect(drillPeriodFor(customer, undefined, 'DATE_RANGE', { dateFrom: '2026-04-03', dateTo: '2026-09-30' }, {}, now)).toBeUndefined();
      expect(drillPeriodFor(customer, 'DATE_RANGE', 'DATE_RANGE', undefined, {}, now)).toBeUndefined();
      expect(drillPeriodFor(customer, 'AS_OF_DATE', 'AS_OF_DATE', { dateFrom: '2026-10-01', dateTo: '2026-10-01' }, {}, now)).toBeUndefined();
    });

    it('carries the period through the address and drops one that is malformed', () => {
      const period = { dateFrom: '2026-04-03', dateTo: '2026-09-30' };
      const query = drillQuery(customer, 'C001', 'customer_rfm', period);
      expect(drillRequestFromQuery(query, new Set(['cust_code']))).toEqual({ column: 'cust_code', value: 'C001', from: 'customer_rfm', ...period });
      const columns = new Set(['cust_code']);
      expect(drillRequestFromQuery({ ...query, drillDateTo: '2026-02-30' }, columns)?.dateFrom).toBeUndefined();
      expect(drillRequestFromQuery({ ...query, drillDateFrom: '2026-10-01', drillDateTo: '2026-09-30' }, columns)?.dateFrom).toBeUndefined();
      expect(drillRequestFromQuery({ ...query, drillDateFrom: '2024-01-01' }, columns)?.dateFrom).toBeUndefined();
      expect(drillRequestFromQuery({ drillColumn: 'cust_code', drillValue: 'C1', drillDateFrom: '2026-04-03' }, columns)?.dateFrom).toBeUndefined();
      expect(Object.keys(withoutDrillQuery({ ...query, runId: 'r' }))).toEqual(['runId']);
    });
  });
});
