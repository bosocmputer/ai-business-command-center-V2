import { mount } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import ReportPeriodToolbar from './ReportPeriodToolbar.vue';

const global = {
  stubs: {
    Toolbar: { template: '<div class="p-toolbar"><div class="p-toolbar-start"><slot name="start" /></div><div class="p-toolbar-center"><slot name="center" /></div><div class="p-toolbar-end"><slot name="end" /></div></div>' },
    Select: { props: ['modelValue', 'options', 'inputId'], emits: ['update:modelValue'], template: '<select :id="inputId" aria-label="ช่วงข้อมูล"><option v-for="item in options" :key="item.value">{{ item.label }}</option></select>' },
    DatePicker: { template: '<input />' },
    Button: { props: ['label', 'disabled'], emits: ['click'], template: '<button :disabled="disabled" @click="$emit(\'click\')">{{ label }}</button>' },
    Message: { template: '<div><slot /></div>' }
  }
};

describe('ReportPeriodToolbar', () => {
  it('keeps the desktop toolbar to one value-first row with accessible hidden labels', () => {
    const wrapper = mount(ReportPeriodToolbar, {
      props: {
        mode: 'SMART_OVERVIEW',
        selection: { periodPreset: 'MONTH_TO_DATE' },
        displayedLabel: 'ข้อมูลล่าสุดแต่ละรายงาน',
        actionLabel: 'อัปเดตภาพรวม'
      },
      global
    });

    expect(wrapper.text()).toContain('ข้อมูลล่าสุดแต่ละรายงาน');
    expect(wrapper.text()).toContain('อัปเดตภาพรวม');
    expect(wrapper.text()).not.toContain('กำลังแสดง');
    expect(wrapper.text()).not.toContain('ดึงสำเร็จ');
    expect(wrapper.get('.period-field label').classes()).toContain('sr-only');
  });

  it('keeps current-only reports honest while still allowing a manual refresh', async () => {
    const wrapper = mount(ReportPeriodToolbar, {
      props: {
        mode: 'CURRENT_ONLY',
        selection: { periodPreset: 'MONTH_TO_DATE' },
        displayedLabel: 'สถานะล่าสุด',
        actionLabel: 'ดูสถานะปัจจุบัน'
      },
      global
    });

    expect(wrapper.text()).toContain('สถานะปัจจุบัน');
    expect(wrapper.text()).toContain('ไม่รองรับข้อมูลย้อนหลัง');
    expect(wrapper.findAll('input')).toHaveLength(0);
    await wrapper.findAll('button').at(-1)!.trigger('click');
    expect(wrapper.emitted('apply')?.[0]).toEqual([{ periodPreset: 'MONTH_TO_DATE' }]);
  });

  it('separates cache lookup from an explicit SML force refresh', async () => {
    const wrapper = mount(ReportPeriodToolbar, {
      props: {
        mode: 'DATE_RANGE',
        selection: { periodPreset: 'MONTH_TO_DATE' },
        actionLabel: 'ดูข้อมูลช่วงนี้',
        forceActionLabel: 'ดึงใหม่จาก SML'
      },
      global
    });

    const buttons = wrapper.findAll('button');
    expect(wrapper.text()).toContain('ดูข้อมูลช่วงนี้');
    expect(wrapper.text()).toContain('ดึงใหม่จาก SML');
    await buttons.find((button) => button.text() === 'ดึงใหม่จาก SML')!.trigger('click');
    expect(wrapper.emitted('force')?.[0]).toEqual([{ periodPreset: 'MONTH_TO_DATE' }]);
    expect(wrapper.emitted('apply')).toBeUndefined();
  });

  it('renders compact source context and generates unique field ids per instance', () => {
    const wrapper = mount({
      components: { ReportPeriodToolbar },
      template: `
        <ReportPeriodToolbar mode="AS_OF_DATE" :selection="{ periodPreset: 'TODAY_TO_NOW' }" displayed-label="ณ วันที่ 12 ก.ค. 2569" source-label="SML 20:24 น." desktop-mode="compact" />
        <ReportPeriodToolbar mode="AS_OF_DATE" :selection="{ periodPreset: 'TODAY_TO_NOW' }" displayed-label="ณ วันที่ 12 ก.ค. 2569" desktop-mode="compact" />
      `
    }, { global });
    const toolbars = wrapper.findAllComponents(ReportPeriodToolbar);
    const labels = wrapper.findAll('label');

    expect(toolbars[0]!.get('.period-toolbar-shell').classes()).toContain('period-toolbar-compact');
    expect(toolbars[0]!.text()).toContain('SML 20:24 น.');
    expect(labels[0]!.attributes('for')).not.toBe(labels[1]!.attributes('for'));
  });

  it('combines the displayed period and source time without redundant headings', () => {
    const wrapper = mount(ReportPeriodToolbar, {
      props: {
        mode: 'AS_OF_DATE',
        selection: { periodPreset: 'TODAY_TO_NOW' },
        displayedLabel: 'ณ เวลาที่อัปเดต · 12 ก.ค. 2569',
        sourceLabel: 'SML 21:33 น.',
        desktopMode: 'compact'
      },
      global
    });

    expect(wrapper.get('.period-context-value').text()).toContain('ณ เวลาที่อัปเดต · 12 ก.ค. 2569');
    expect(wrapper.get('.period-context-value').text()).toContain('SML 21:33 น.');
    expect(wrapper.text()).not.toContain('ดึงสำเร็จ');
  });
});
