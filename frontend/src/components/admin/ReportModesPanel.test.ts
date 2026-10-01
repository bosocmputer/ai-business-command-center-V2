import { flushPromises, mount } from '@vue/test-utils';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const listReportModes = vi.fn();
const setReportMode = vi.fn();
const measureReportModes = vi.fn();
const toastAdd = vi.fn();
vi.mock('@/api', async () => {
  const client = await import('@/api/client');
  return { ApiError: client.ApiError, adminApi: { listReportModes: (...a: unknown[]) => listReportModes(...a), setReportMode: (...a: unknown[]) => setReportMode(...a), measureReportModes: (...a: unknown[]) => measureReportModes(...a) } };
});
vi.mock('primevue/usetoast', () => ({ useToast: () => ({ add: toastAdd }) }));

import { ApiError } from '@/api/client';
import ReportModesPanel from './ReportModesPanel.vue';

const item = (overrides: Record<string, unknown>) => ({
  reportKey: 'stock_balance', label: 'สต็อกคงเหลือ', chunkable: true, mode: 'DIRECT', source: 'DEFAULT', reason: '',
  lastRows: null, lastDurationMs: null, changedAt: null, chunkThreshold: 5000, ...overrides
});
const problem = (status: number, code: string) => new ApiError(status, { code, message: code, requestId: 'r', retryable: false });

const ButtonStub = { props: ['label', 'loading', 'disabled'], emits: ['click'], template: '<button type="button" :disabled="disabled" @click="$emit(\'click\')">{{ label }}</button>' };
const slotStub = { template: '<div><slot /></div>' };
const DataTableStub = {
  props: ['value'],
  template: '<table><tr v-for="row in value" :key="row.reportKey" :data-key="row.reportKey"><td>{{ row.label }}</td><td class="mode">{{ row.mode }}</td><td class="src">{{ row.source }}</td><td class="act"><slot name="rowaction" :data="row" /></td></tr></table>'
};

// The real DataTable renders Column templates; this stub renders just enough to
// exercise the panel's own logic (load, change, measure) without PrimeVue.
function mountPanel() {
  return mount(ReportModesPanel, {
    props: { tenantId: 'tenant-1' },
    global: { stubs: { Button: ButtonStub, Message: slotStub, Tag: true, DataTable: DataTableStub, Column: true } }
  });
}

describe('ReportModesPanel', () => {
  beforeEach(() => { vi.clearAllMocks(); });

  it('loads the tenant reports on mount', async () => {
    listReportModes.mockResolvedValue({ data: [item({}), item({ reportKey: 'sales_goods_services', label: 'ขาย', chunkable: false })] });
    const wrapper = mountPanel();
    await flushPromises();
    expect(listReportModes).toHaveBeenCalledWith('tenant-1');
    expect(wrapper.findAll('tr')).toHaveLength(2);
  });

  it('shows a retryable error when the list cannot be loaded', async () => {
    listReportModes.mockRejectedValueOnce(problem(500, 'INTERNAL_ERROR')).mockResolvedValueOnce({ data: [item({})] });
    const wrapper = mountPanel();
    await flushPromises();
    expect(wrapper.text()).toContain('ลองใหม่');
    const retry = wrapper.findAll('button').find((button) => button.text() === 'ลองใหม่');
    await retry?.trigger('click');
    await flushPromises();
    expect(listReportModes).toHaveBeenCalledTimes(2);
    expect(wrapper.findAll('tr')).toHaveLength(1);
  });

  it('reports each measurement and reloads the list afterwards', async () => {
    listReportModes.mockResolvedValue({ data: [item({})] });
    measureReportModes.mockResolvedValue({ data: [{ reportKey: 'stock_balance', units: 8080, recommendedMode: 'CHUNKED', applied: true, threshold: 5000 }] });
    const wrapper = mountPanel();
    await flushPromises();
    await wrapper.findAll('button').find((button) => button.text() === 'วัดขนาดร้านนี้')?.trigger('click');
    await flushPromises();
    expect(measureReportModes).toHaveBeenCalledWith('tenant-1');
    expect(wrapper.get('[data-testid="measure-result"]').text()).toBe('สต็อกคงเหลือ: 8,080 รายการ (เกณฑ์ 5,000) · แนะนำแบ่งชุด · ตั้งให้แล้ว');
    expect(listReportModes).toHaveBeenCalledTimes(2);
  });

  it('explains a busy shop and a missing SML connection instead of a raw error', async () => {
    listReportModes.mockResolvedValue({ data: [item({})] });
    measureReportModes.mockRejectedValueOnce(problem(409, 'TENANT_BUSY')).mockRejectedValueOnce(problem(409, 'SML_NOT_CONFIGURED'));
    const wrapper = mountPanel();
    await flushPromises();
    const measure = () => wrapper.findAll('button').find((button) => button.text() === 'วัดขนาดร้านนี้');
    await measure()?.trigger('click');
    await flushPromises();
    expect(wrapper.text()).toContain('กำลังสร้างรายงานอยู่');
    await measure()?.trigger('click');
    await flushPromises();
    expect(wrapper.text()).toContain('ยังไม่ได้ตั้งค่าการเชื่อมต่อ SML');
    expect(wrapper.text()).not.toContain('กำลังสร้างรายงานอยู่');
  });

  it('flips the mode of one report and keeps the row list in step with the server', async () => {
    listReportModes.mockResolvedValue({ data: [item({ mode: 'CHUNKED', source: 'AUTO_SWITCHED' }), item({ reportKey: 'ar_customer_movement', label: 'ลูกหนี้' })] });
    setReportMode.mockResolvedValue(item({ mode: 'DIRECT', source: 'MANUAL' }));
    const wrapper = mountPanel();
    await flushPromises();
    const vm = wrapper.vm as unknown as { change: (row: ReturnType<typeof item>) => Promise<void> };
    await vm.change(item({ mode: 'CHUNKED' }));
    expect(setReportMode).toHaveBeenCalledWith('tenant-1', 'stock_balance', 'DIRECT');
    expect(wrapper.findAll('.mode').map((cell) => cell.text())).toEqual(['DIRECT', 'DIRECT']);
    expect(wrapper.findAll('.src')[0]?.text()).toBe('MANUAL');
    expect(toastAdd).toHaveBeenCalledWith(expect.objectContaining({ severity: 'success' }));
  });

  it('leaves the row untouched and says so when the change is rejected', async () => {
    listReportModes.mockResolvedValue({ data: [item({ mode: 'CHUNKED', source: 'AUTO_SWITCHED' })] });
    setReportMode.mockRejectedValue(problem(422, 'REPORT_MODE_UNSUPPORTED'));
    const wrapper = mountPanel();
    await flushPromises();
    const vm = wrapper.vm as unknown as { change: (row: ReturnType<typeof item>) => Promise<void> };
    await vm.change(item({ mode: 'CHUNKED' }));
    expect(wrapper.findAll('.mode').map((cell) => cell.text())).toEqual(['CHUNKED']);
    expect(toastAdd).toHaveBeenCalledWith(expect.objectContaining({ severity: 'error' }));
  });
});
