import { mount } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import LineFlexPreview from './LineFlexPreview.vue';
import type { FlexPreview } from '@/api';

const preview: FlexPreview = {
  presentationVersion: 'ai-bcc-executive-report-v2',
  altText: 'สรุปผู้บริหาร ร้านตัวอย่าง: ข้อมูลวันที่ 2026-07-10 (1 รายงาน)',
  tenantName: 'ร้านตัวอย่าง',
  period: { preset: 'TODAY_TO_NOW', dateFrom: '2026-07-10', dateTo: '2026-07-10' },
  periodLabel: 'ข้อมูลวันที่ 2026-07-10',
  contextNote: 'วันนี้ยังไม่มีช่วงเวลาเปรียบเทียบที่เท่ากัน',
  generatedAt: '2026-07-11T01:30:00+07:00',
  actionUrl: 'https://dashboard.nextstep-soft.com/app',
  payloadBytes: 2048,
  exampleScheduledFor: '2026-07-11T01:00:00Z',
  mixedPeriods: false,
  message: {},
  reports: [
    {
      key: 'sales_goods_services',
      label: 'รายงานขายสินค้าและบริการ',
      categoryLabel: 'ขาย',
      primary: { label: 'ยอดขาย', value: '1,234,567.89', unit: 'บาท' },
      supporting: [{ label: 'บิลขาย', value: '128', unit: 'ใบ' }, { label: 'ยอดเฉลี่ยต่อบิล', value: '9,645.06', unit: 'บาท' }],
      comparison: { text: '↓ 7.82% เทียบ 9 ก.ค. 2569 (1,339,300.00 บาท)', direction: 'DOWN' },
      highlights: [{ label: 'สินค้าขายดี', value: 'สินค้าตัวอย่าง: 45,000.00 บาท' }],
      attention: { severity: 'WARNING', text: 'ตัวอย่างสถานะที่ต้องตรวจสอบ' },
      actionUrl: 'https://dashboard.nextstep-soft.com/app/tenant/t/report/sales_goods_services',
      periodLabel: 'ข้อมูล ณ 11 ก.ค. 2569',
      metrics: [{ label: 'เอกสาร', value: '128' }, { label: 'ยอดขาย', value: '1,234,567.89' }]
    }
  ]
};

describe('LineFlexPreview', () => {
  const mountPreview = (value: FlexPreview) => mount(LineFlexPreview, {
    props: { preview: value },
    global: { stubs: { Tag: { template: '<span><slot />{{ value }}</span>', props: ['value'] } } }
  });

  it('renders the executive card with units, dated comparison and highlights', () => {
    const wrapper = mountPreview(preview);
    const text = wrapper.text();

    expect(text).toContain('ตัวเลขสมมติเท่านั้น');
    expect(text).toContain('ไม่ดึงข้อมูลจาก SML');
    expect(text).toContain('วันนี้ยังไม่มีช่วงเวลาเปรียบเทียบที่เท่ากัน');
    expect(wrapper.find('.flex-preview-kicker').text()).toBe('ขาย · รายวัน');
    expect(wrapper.find('.flex-preview-header h3').text()).toBe('ขายสินค้าและบริการ');
    expect(wrapper.find('.flex-preview-primary-amount').text()).toContain('1,234,567.89');
    expect(wrapper.find('.flex-preview-primary-amount').text()).toContain('บาท');
    expect(text).toContain('128 ใบ');
    expect(text).toContain('9,645.06 บาท');
    expect(wrapper.find('.flex-preview-comparison').text()).toContain('เทียบ 9 ก.ค. 2569');
    expect(wrapper.find('.flex-preview-highlight').text()).toContain('สินค้าตัวอย่าง: 45,000.00 บาท');
    expect(wrapper.find('.flex-preview-status').text()).toBe('ควรตรวจสอบ');
    expect(text).toContain('ตัวอย่างสถานะที่ต้องตรวจสอบ');
    expect(text).not.toContain('กดปุ่มด้านล่างเพื่อดูรายละเอียดเพิ่มเติม');
    expect(text).not.toContain('฿');
    expect(text).toContain('เปิดรายละเอียด');
    expect(text).toContain('2.0 KB');
    expect(wrapper.find('[role="button"]').attributes('aria-disabled')).toBe('true');
    expect(wrapper.find('.flex-preview-carousel').attributes('data-presentation-version')).toBe('ai-bcc-executive-report-v2');
    expect(wrapper.find('.flex-preview-version-warning').exists()).toBe(false);
  });

  it('uses only the backend ZERO state and hides repeated zero metric rows', () => {
    const zeroPreview: FlexPreview = {
      ...preview,
      reports: [{
        ...preview.reports[0]!,
        dataState: 'ZERO',
        stateText: 'ไม่มีรายการขายในช่วงนี้',
        primary: { label: 'ยอดขาย', value: '0.00', unit: 'บาท' },
        supporting: [{ label: 'บิลขาย', value: '0', unit: 'ใบ' }, { label: 'ยอดเฉลี่ยต่อบิล', value: '0.00', unit: 'บาท' }],
        comparison: undefined,
        highlights: undefined,
        attention: undefined
      }]
    };
    const wrapper = mountPreview(zeroPreview);

    expect(wrapper.text()).toContain('ไม่มีรายการขายในช่วงนี้');
    expect(wrapper.find('.flex-preview-status').text()).toBe('ไม่มีรายการ');
    expect(wrapper.findAll('.flex-preview-metric')).toHaveLength(0);
    expect(wrapper.find('.flex-preview-primary-amount').text()).toContain('0.00');
  });

  it('warns instead of claiming an exact preview for unsupported versions', () => {
    const unsupportedPreview: FlexPreview = { ...preview, presentationVersion: 'executive-navy-v2' };
    const wrapper = mountPreview(unsupportedPreview);

    expect(wrapper.find('.flex-preview-version-warning').text()).toContain('ตัวอย่างอาจไม่ตรงกับข้อความจริง');
  });

  it('shows each backend-resolved period for mixed-period cards', () => {
    const wrapper = mountPreview({ ...preview, mixedPeriods: true, periodLabel: 'ช่วงข้อมูลแตกต่างตามรายงาน' });
    expect(wrapper.text()).toContain('ข้อมูล ณ 11 ก.ค. 2569');
  });
});
