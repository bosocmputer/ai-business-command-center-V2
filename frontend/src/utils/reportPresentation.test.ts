import { describe, expect, it } from 'vitest';
import { reportDefinitions } from '@/api';
import { formatReportCell, presentationFor, reportPresentations, visibleReportColumns } from './reportPresentation';

describe('report presentation metadata', () => {
  it('defines a business-first presentation for all ten reports', () => {
    expect(Object.keys(reportPresentations).sort()).toEqual(reportDefinitions.map((item) => item.reportKey).sort());
    for (const definition of Object.values(reportPresentations)) {
      expect(definition.columns.filter((column) => column.defaultVisible).length).toBeGreaterThanOrEqual(3);
      expect(new Set(definition.columns.map((column) => column.key)).size).toBe(definition.columns.length);
      expect(definition.columns.filter((column) => column.technical && column.defaultVisible)).toEqual([]);
    }
  });

  it('shows configured business columns first and keeps unknown columns optional', () => {
    const available = ['last_status', 'doc_no', 'total_amount', 'future_provider_field', 'doc_date'];
    expect(visibleReportColumns('sales_goods_services', available).map((column) => column.key)).toEqual(['doc_date', 'doc_no', 'total_amount']);
    expect(presentationFor('sales_goods_services', available).find((column) => column.key === 'future_provider_field')).toEqual(expect.objectContaining({ technical: true, defaultVisible: false }));
  });
});

describe('typed report table presentation', () => {
  it('preserves identifiers including leading zeroes and long barcodes', () => {
    const identifier = { key: 'doc_no', label: 'เลขที่เอกสาร', defaultVisible: true, mobilePriority: 5, dataType: 'IDENTIFIER' as const };
    expect(formatReportCell('00123', identifier)).toBe('00123');
    expect(formatReportCell('885000000000012345', identifier)).toBe('885000000000012345');
  });

  it('formats only explicitly numeric columns with the intended precision', () => {
    const amount = { key: 'total_amount', label: 'ยอดรวม', defaultVisible: true, mobilePriority: 5, dataType: 'NUMBER' as const, numberFormat: 'AMOUNT' as const };
    const integer = { key: 'credit_day', label: 'เครดิต', defaultVisible: true, mobilePriority: 2, dataType: 'NUMBER' as const, numberFormat: 'INTEGER' as const };
    expect(formatReportCell('-1234.5', amount)).toContain('-1,234.50');
    expect(formatReportCell('0', amount)).toContain('0.00');
    expect(formatReportCell('15', integer)).toBe('15');
  });

  it('defines a semantic type for every known column in all ten reports', () => {
    expect(Object.keys(reportPresentations)).toHaveLength(10);
    for (const presentation of Object.values(reportPresentations)) {
      for (const column of presentation.columns) {
        expect(column.dataType, `${presentation.reportKey}.${column.key}`).toMatch(/^(TEXT|IDENTIFIER|DATE|TIME|NUMBER)$/);
        if (column.dataType === 'NUMBER') expect(column.numberFormat, `${presentation.reportKey}.${column.key}`).toBeTruthy();
      }
    }
  });
});
