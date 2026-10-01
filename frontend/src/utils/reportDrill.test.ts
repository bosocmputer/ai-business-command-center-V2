import { describe, expect, it } from 'vitest';
import type { ReportDrillLink } from '@/api';
import { drillAnchors, drillQuery, drillRequestFromQuery, drillValueOf, withoutDrillQuery } from './reportDrill';

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
});
