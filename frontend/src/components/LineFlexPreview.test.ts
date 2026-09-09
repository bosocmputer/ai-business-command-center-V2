import { mount } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import LineFlexPreview from './LineFlexPreview.vue';
import type { FlexPreview } from '@/api';

const preview: FlexPreview = {
  presentationVersion: 'executive-navy-v1',
  altText: 'รายงาน ร้านตัวอย่าง — ข้อมูลวันที่ 2026-07-10',
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
      primary: { label: 'ยอดขาย', value: '1,234,567.89' },
      supporting: [{ label: 'จำนวนเอกสาร', value: '128' }, { label: 'ยอดเฉลี่ยต่อเอกสาร', value: '9,645.06' }],
      comparison: { text: '↓ 7.82% จากช่วงก่อน', direction: 'DOWN' },
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

  it('renders the versioned executive navy hierarchy with backend values', () => {
    const wrapper = mountPreview(preview);

    expect(wrapper.text()).toContain('ตัวเลขสมมติเท่านั้น');
    expect(wrapper.text()).toContain('ไม่ดึงข้อมูลจาก SML');
    expect(wrapper.text()).toContain('แบบอักษรอาจต่างเล็กน้อยตาม iOS/Android');
    expect(wrapper.text()).toContain('วันนี้ยังไม่มีช่วงเวลาเปรียบเทียบที่เท่ากัน');
    expect(wrapper.text()).toContain('รายงานขายสินค้าและบริการ');
    expect(wrapper.text()).toContain('1,234,567.89');
    expect(wrapper.text()).not.toContain('฿');
    expect(wrapper.text()).toContain('↓ 7.82% จากช่วงก่อน');
    expect(wrapper.text()).toContain('ตัวอย่างสถานะที่ต้องตรวจสอบ');
    expect(wrapper.text()).toContain('ดูภาพรวมร้าน');
    expect(wrapper.text()).toContain('2.0 KB');
    expect(wrapper.find('[role="button"]').attributes('aria-disabled')).toBe('true');
    expect(wrapper.find('.flex-preview-card').classes()).toContain('is-executive-navy');
    expect(wrapper.find('.flex-preview-card').attributes('data-presentation-version')).toBe('executive-navy-v1');
  });

  it('uses only the backend ZERO state and hides repeated zero metric rows', () => {
    const zeroPreview: FlexPreview = {
      ...preview,
      reports: [{
        ...preview.reports[0]!,
        dataState: 'ZERO',
        stateText: 'ไม่มีรายการขายในช่วงนี้',
        primary: { label: 'ยอดขาย', value: '0.00' },
        supporting: [{ label: 'จำนวนเอกสาร', value: '0' }, { label: 'ยอดเฉลี่ยต่อเอกสาร', value: '0.00' }],
        comparison: undefined,
        attention: undefined
      }]
    };
    const wrapper = mountPreview(zeroPreview);

    expect(wrapper.text()).toContain('ไม่มีรายการขายในช่วงนี้');
    expect(wrapper.findAll('.flex-preview-metric')).toHaveLength(0);
    expect(wrapper.find('.flex-preview-state').exists()).toBe(true);
  });

  it('falls back to the legacy preview when presentationVersion is absent', () => {
    const legacyPreview: FlexPreview = { ...preview, presentationVersion: undefined };
    const wrapper = mountPreview(legacyPreview);

    expect(wrapper.find('.flex-preview-card').classes()).toContain('is-legacy');
    expect(wrapper.find('.flex-preview-version-warning').exists()).toBe(false);
    expect(wrapper.text()).toContain('1,234,567.89');
  });

  it('warns instead of claiming an exact preview for unsupported versions', () => {
    const unsupportedPreview: FlexPreview = { ...preview, presentationVersion: 'executive-navy-v3' };
    const wrapper = mountPreview(unsupportedPreview);

    expect(wrapper.find('.flex-preview-version-warning').text()).toContain('ตัวอย่างอาจไม่ตรงกับข้อความจริง');
    expect(wrapper.find('.flex-preview-card').classes()).toContain('is-legacy');
  });

  it('shows each backend-resolved period for mixed-period cards', () => {
    const wrapper = mountPreview({ ...preview, presentationVersion: 'executive-navy-v2', mixedPeriods: true, periodLabel: 'ช่วงข้อมูลแตกต่างตามรายงาน' });
    expect(wrapper.text()).toContain('ข้อมูล ณ 11 ก.ค. 2569');
    expect(wrapper.find('.flex-preview-report-period').exists()).toBe(true);
  });
});
