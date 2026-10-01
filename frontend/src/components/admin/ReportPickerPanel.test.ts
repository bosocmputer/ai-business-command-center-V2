import { mount } from '@vue/test-utils';
import PrimeVue from 'primevue/config';
import { afterEach, describe, expect, it } from 'vitest';
import type { AdminReportDefinition, ReportKey } from '@/api';
import ReportPickerPanel from './ReportPickerPanel.vue';

const definition = (reportKey: ReportKey, label: string, category: string, categoryLabel: string, status: 'ACTIVE' | 'DEPRECATED' = 'ACTIVE'): AdminReportDefinition =>
  ({ reportKey, version: '1.0.0', label, category, categoryLabel, status, periodMode: 'DATE_RANGE' });

// Listed out of category order on purpose: the picker must still keep each category together.
const definitions = [
  definition('sales_goods_services', 'ขาย', 'SALES', 'ขาย'),
  definition('ar_aging', 'อายุหนี้', 'AR', 'ลูกหนี้'),
  definition('customer_rfm', 'RFM', 'CUSTOMER', 'ลูกค้า/CRM'),
  definition('ar_customer_movement', 'ความเคลื่อนไหวลูกหนี้', 'AR', 'ลูกหนี้'),
  definition('purchase_frequency', 'ความถี่การซื้อ', 'CUSTOMER', 'ลูกค้า/CRM'),
  definition('ar_debt_receipt', 'รับชำระหนี้', 'AR', 'ลูกหนี้', 'DEPRECATED')
];

const mounted: ReturnType<typeof mount>[] = [];
function mountPicker(props: Record<string, unknown> = {}) {
  const wrapper = mount(ReportPickerPanel, { props: { definitions, modelValue: [] as ReportKey[], ...props }, global: { plugins: [PrimeVue], stubs: { SakaiTableHeader: true } } });
  mounted.push(wrapper);
  return wrapper;
}
// The filter menu watches the document for removed overlays; unmounting stops it.
afterEach(() => { mounted.splice(0).forEach((wrapper) => wrapper.unmount()); });
const lastSelection = (wrapper: ReturnType<typeof mountPicker>) => wrapper.emitted('update:modelValue')?.at(-1)?.[0] as ReportKey[] | undefined;
const buttonByText = (wrapper: ReturnType<typeof mountPicker>, text: string) => wrapper.findAll('button').find((button) => button.text().includes(text));

describe('ReportPickerPanel grouped by category', () => {
  it('shows one header per category, with the new customer category, and a count of what is chosen', () => {
    const wrapper = mountPicker({ modelValue: ['customer_rfm'] });
    const headers = wrapper.findAll('tr.p-datatable-row-group-header, tr[data-p-rowgroup-header]').map((row) => row.text());
    expect(headers).toHaveLength(3);
    expect(headers.join('|')).toContain('ลูกค้า/CRM');
    expect(headers.find((text) => text.includes('ลูกค้า/CRM'))).toContain('เลือก 1/2');
    expect(headers.find((text) => text.includes('ลูกหนี้'))).toContain('เลือก 0/3');
  });

  it('keeps reports of a category together even when the catalog interleaves them', () => {
    const wrapper = mountPicker();
    const rows = wrapper.findAll('tbody tr').map((row) => row.text());
    const order = ['ขาย', 'ลูกหนี้', 'ลูกค้า/CRM'];
    const positions = order.map((label) => rows.findIndex((text) => text.includes(label) && text.includes('เลือก')));
    expect(positions).toEqual([...positions].sort((a, b) => a - b));
    const agingIndex = rows.findIndex((text) => text.includes('อายุหนี้'));
    const movementIndex = rows.findIndex((text) => text.includes('ความเคลื่อนไหวลูกหนี้'));
    expect(Math.abs(agingIndex - movementIndex)).toBeLessThanOrEqual(2);
  });

  it('selects every active report of a category and leaves the retired one alone', async () => {
    const wrapper = mountPicker();
    await buttonByText(wrapper, 'เลือกทั้งหมวด')!.trigger('click');
    expect(wrapper.emitted('update:modelValue')).toBeTruthy();
    const buttons = wrapper.findAll('button').filter((button) => button.text().includes('เลือกทั้งหมวด'));
    expect(buttons).toHaveLength(3);
    // Third category in order is CUSTOMER; the AR one holds a deprecated report.
    await buttons[1]!.trigger('click');
    expect(lastSelection(wrapper)?.sort()).toEqual(['ar_aging', 'ar_customer_movement']);
  });

  it('stops at the limit and says so', async () => {
    const wrapper = mountPicker({ maxSelected: 1 });
    await wrapper.findAll('button').filter((button) => button.text().includes('เลือกทั้งหมวด'))[1]!.trigger('click');
    expect(lastSelection(wrapper)).toEqual(['ar_aging']);
    expect(wrapper.text()).toContain('เลือกได้สูงสุด 1 รายงาน');
  });

  it('clears a whole category but keeps a report that a schedule still needs', async () => {
    const wrapper = mountPicker({ modelValue: ['customer_rfm', 'purchase_frequency', 'ar_aging'], lockedKeys: ['purchase_frequency'] });
    await buttonByText(wrapper, 'ล้างทั้งหมวด')!.trigger('click');
    expect(lastSelection(wrapper)).toEqual(['purchase_frequency', 'ar_aging']);
  });
});
