import { flushPromises, mount } from '@vue/test-utils';
import PrimeVue from 'primevue/config';
import { defineComponent, h } from 'vue';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
  replace: vi.fn(async () => undefined),
  push: vi.fn(async () => undefined),
  exactSnapshot: vi.fn(),
  createRun: vi.fn(),
  revalidateReport: vi.fn(),
  run: vi.fn(),
  queryRows: vi.fn(),
  route: { params: { tenantId: 'tenant-1', reportKey: 'ar_aging' }, query: {} as Record<string, string>, path: '/app/tenant/tenant-1/report/ar_aging', hash: '' }
}));

vi.mock('vue-router', () => ({ useRoute: () => mocks.route, useRouter: () => ({ replace: mocks.replace, push: mocks.push }) }));
vi.mock('primevue/useconfirm', () => ({ useConfirm: () => ({ require: vi.fn(), close: vi.fn() }) }));
vi.mock('primevue/usetoast', () => ({ useToast: () => ({ add: vi.fn() }) }));
vi.mock('@/api', async () => {
  const actual = await vi.importActual<typeof import('@/api')>('@/api');
  return { ...actual, viewerApi: { exactSnapshot: mocks.exactSnapshot, run: mocks.run, queryRows: mocks.queryRows, createRun: mocks.createRun, revalidateReport: mocks.revalidateReport } };
});
vi.mock('@/stores/viewer', async () => {
  const period = await vi.importActual<typeof import('@/utils/reportPeriod')>('@/utils/reportPeriod');
  const reports = [
    { reportKey: 'ar_aging', version: '1.0.0', label: 'รายงานอายุหนี้ลูกหนี้', category: 'AR', isSensitive: true, periodMode: 'AS_OF_DATE', drillLinks: [{ column: 'cust_code', labelColumn: 'cust_name', kind: 'CUSTOMER', targetReport: 'customer_rfm', targetColumn: 'cust_code' }] },
    { reportKey: 'customer_rfm', version: '1.0.0', label: 'รายงานลูกค้า RFM', category: 'CUSTOMER', isSensitive: true, periodMode: 'DATE_RANGE', drillLinks: [{ column: 'cust_code', labelColumn: 'cust_name', kind: 'CUSTOMER', targetReport: 'ar_aging', targetColumn: 'cust_code' }] }
  ];
  return {
    useViewerSession: () => ({
      state: { tenants: [{ id: 'tenant-1', name: 'ร้านทดสอบ' }], reportsByTenant: { 'tenant-1': reports } },
      selectTenant: vi.fn(), ensureReports: async () => reports, periodSelection: () => period.defaultPeriodSelection(), setPeriodSelection: vi.fn()
    })
  };
});

import { ApiError } from '@/api/client';
import ViewerReport from './ViewerReport.vue';

const dashboard = { period: { preset: 'AS_OF_RUN', dateFrom: '2026-10-01', dateTo: '2026-10-01' }, kpis: [], visualizations: [], quality: { status: 'OK', warnings: [] }, generatedAt: '2026-10-01T04:00:00Z' };
const snapshot = { runId: 'run-1', dashboard, freshnessStatus: 'FRESH', detailsAvailable: true, sourceFinishedAt: '2026-10-01T04:00:00Z' };
const succeededRun = { id: 'run-1', status: 'SUCCEEDED', queuedAt: '2026-10-01T04:00:00Z', finishedAt: '2026-10-01T04:00:05Z', periodPreset: 'AS_OF_RUN', dateFrom: '2026-10-01', dateTo: '2026-10-01', resultKind: 'DETAIL' };
const rowPage = (data: Record<string, unknown>[]) => ({
  runId: 'run-1', columns: ['cust_code', 'cust_name', 'balance'], data, rowOrdinals: data.map((_, index) => index), page: 0, pageSize: 25, total: data.length,
  filterCapabilities: [{ columnKey: 'cust_code', dataType: 'IDENTIFIER', operators: ['CONTAINS', 'EQUALS'], globalSearchable: true }]
});

const passthrough = { template: '<div><slot /></div>' };
const ButtonStub = { props: ['label'], emits: ['click'], template: '<button type="button" @click="$emit(\'click\', $event)">{{ label }}</button>' };
// A Menu stand-in that records the items it is given and the toggle, so the test
// can follow what a viewer would choose without PrimeVue's popup and teleport.
const toggled = vi.fn();
const MenuStub = defineComponent({
  props: { model: { type: Array, default: () => [] } },
  setup(props, { expose }) { expose({ toggle: (event: Event) => toggled(() => props.model, event) }); return () => h('div'); }
});

function mountReportWithTable() {
  return mount(ViewerReport, {
    global: {
      plugins: [PrimeVue],
      stubs: {
        AppPageHeader: passthrough, ReportPeriodToolbar: true, ExecutiveChart: true, SakaiTableHeader: true, Dialog: true, Menu: MenuStub,
        Tabs: passthrough, TabList: passthrough, Tab: passthrough, TabPanels: passthrough, TabPanel: passthrough,
        Message: passthrough, Tag: true, ProgressSpinner: true, MultiSelect: true, Paginator: true, DatePicker: true, Select: true, InputText: true
      }
    }
  });
}

function mountReport() {
  return mount(ViewerReport, {
    global: {
      stubs: {
        AppPageHeader: passthrough, ReportPeriodToolbar: true, ExecutiveChart: true, SakaiTableHeader: true, DataTable: true, Column: true, Dialog: true, Menu: true,
        Tabs: passthrough, TabList: passthrough, Tab: passthrough, TabPanels: passthrough, TabPanel: passthrough,
        Message: passthrough, Button: ButtonStub, Tag: true, ProgressSpinner: true, MultiSelect: true, Paginator: true, DatePicker: true, Select: true, InputText: true
      }
    }
  });
}

describe('ViewerReport drill-down', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.route.query = {};
    mocks.exactSnapshot.mockResolvedValue(snapshot);
    mocks.run.mockResolvedValue(succeededRun);
    mocks.queryRows.mockResolvedValue(rowPage([{ cust_code: 'C001', cust_name: 'ลูกค้า 1', balance: '1000.00' }]));
  });

  it('opens the detail rows filtered to the drilled customer and shows how to clear it', async () => {
    mocks.route.query = { drillColumn: 'cust_code', drillValue: 'C001', drillFrom: 'customer_rfm' };
    const wrapper = mountReport();
    await flushPromises();
    expect(mocks.queryRows).toHaveBeenCalledTimes(1);
    expect(mocks.queryRows.mock.calls[0]![3]).toMatchObject({ filters: [{ columnKey: 'cust_code', operator: 'EQUALS', value: 'C001' }], page: 0 });
    expect(wrapper.text()).toContain('กำลังดูเฉพาะ');
    expect(wrapper.text()).toContain('C001');
    expect(wrapper.text()).toContain('รายงานลูกค้าตามความถี่และมูลค่าการซื้อ (RFM)');
    // The one-time request is removed from the address so a reload does not re-apply it.
    expect(mocks.replace).toHaveBeenCalledWith(expect.objectContaining({ query: {} }));
  });

  it('clears the drill filter and reloads every row', async () => {
    mocks.route.query = { drillColumn: 'cust_code', drillValue: 'C001', drillFrom: 'customer_rfm' };
    const wrapper = mountReport();
    await flushPromises();
    await wrapper.findAll('button').find((button) => button.text() === 'ดูทั้งหมด')!.trigger('click');
    await flushPromises();
    expect(mocks.queryRows).toHaveBeenCalledTimes(2);
    expect(mocks.queryRows.mock.calls[1]![3]).toMatchObject({ filters: [] });
    expect(wrapper.text()).not.toContain('กำลังดูเฉพาะ');
  });

  it('ignores a drill request for a column no report links to', async () => {
    mocks.route.query = { drillColumn: 'doc_no', drillValue: 'A1' };
    const wrapper = mountReport();
    await flushPromises();
    expect(mocks.queryRows).not.toHaveBeenCalled();
    expect(wrapper.text()).not.toContain('กำลังดูเฉพาะ');
  });

  it('offers the permitted target reports from the customer name and carries the hidden code and the source report', async () => {
    mocks.route.params.reportKey = 'customer_rfm';
    mocks.route.query = { drillColumn: 'cust_code', drillValue: 'C001', drillFrom: 'ar_aging' };
    mocks.exactSnapshot.mockResolvedValue({ ...snapshot, dashboard: { ...dashboard, period: { preset: 'CUSTOM', dateFrom: '2026-04-03', dateTo: '2026-09-30' } } });
    mocks.run.mockResolvedValue({ ...succeededRun, periodPreset: 'CUSTOM', dateFrom: '2026-04-03', dateTo: '2026-09-30' });
    mocks.queryRows.mockResolvedValue({ ...rowPage([{ cust_code: 'C001', cust_name: 'ลูกค้า 1', balance: '1000.00' }]), columns: ['cust_code', 'cust_name', 'monetary'] });
    const wrapper = mountReportWithTable();
    await flushPromises();
    const drillButtons = wrapper.findAll('button.drill-cell');
    expect(drillButtons).toHaveLength(1);
    // The code is hidden by default for this report, so the link sits on the name.
    expect(drillButtons[0]!.attributes('aria-label')).toContain('ลูกค้า 1');
    await drillButtons[0]!.trigger('click');
    await flushPromises();
    const model = (toggled.mock.calls[0]![0] as () => { label: string; items: { label: string; command: () => void }[] }[])();
    expect(model[0]!.label).toBe('C001');
    expect(model[0]!.items.map((item) => item.label)).toEqual(['รายงานอายุหนี้ลูกหนี้']);
    model[0]!.items[0]!.command();
    expect(mocks.push).toHaveBeenCalledWith({
      name: 'viewer-report', params: { tenantId: 'tenant-1', reportKey: 'ar_aging' },
      // RFM is a date-range report and aging is as-of, so the target opens as of the range's last day.
      query: { drillColumn: 'cust_code', drillValue: 'C001', drillFrom: 'customer_rfm', drillDateFrom: '2026-09-30', drillDateTo: '2026-09-30' }
    });
    mocks.route.params.reportKey = 'ar_aging';
  });

  const noSnapshot = () => new ApiError(404, { code: 'NOT_FOUND', message: 'no snapshot', requestId: 'r', retryable: false });

  it('fetches the report at once when a drill finds no snapshot for the period it asked for', async () => {
    mocks.route.params.reportKey = 'customer_rfm';
    mocks.route.query = { drillColumn: 'cust_code', drillValue: 'C001', drillFrom: 'ar_aging', drillDateFrom: '2026-07-04', drillDateTo: '2026-10-01' };
    mocks.exactSnapshot.mockRejectedValueOnce(noSnapshot());
    mocks.createRun.mockResolvedValue({ ...succeededRun, id: 'run-2', status: 'QUEUED', periodPreset: 'CUSTOM', dateFrom: '2026-07-04', dateTo: '2026-10-01' });
    mountReport();
    await flushPromises();
    expect(mocks.exactSnapshot.mock.calls[0]![2]).toEqual({ periodPreset: 'CUSTOM', dateFrom: '2026-07-04', dateTo: '2026-10-01' });
    expect(mocks.createRun).toHaveBeenCalledTimes(1);
    expect(mocks.createRun.mock.calls[0]![2]).toEqual({ periodPreset: 'CUSTOM', dateFrom: '2026-07-04', dateTo: '2026-10-01' });
    mocks.route.params.reportKey = 'ar_aging';
  });

  it('does not start a full run by itself for an ordinary visit with no snapshot, it reads the figures again and says so plainly', async () => {
    mocks.route.query = {};
    mocks.exactSnapshot.mockRejectedValueOnce(noSnapshot());
    mocks.revalidateReport.mockResolvedValue({ disposition: 'MISSING_REFRESHING', run: { ...succeededRun, id: 'run-9', status: 'RUNNING' }, retryAfter: 0, legacyFallback: false });
    const wrapper = mountReport();
    await flushPromises();
    expect(mocks.createRun).not.toHaveBeenCalled();
    expect(mocks.revalidateReport).toHaveBeenCalledTimes(1);
    expect(wrapper.text()).toContain('กำลังดึงข้อมูลล่าสุด');
    expect(wrapper.text()).not.toMatch(/Snapshot|SML/);
  });

  it('fetches a run of its own for a drill when the snapshot found is a summary with no rows', async () => {
    mocks.route.query = { drillColumn: 'cust_code', drillValue: 'C001', drillFrom: 'customer_rfm' };
    mocks.exactSnapshot.mockResolvedValueOnce({ ...snapshot, detailsAvailable: false });
    mocks.run.mockResolvedValueOnce({ ...succeededRun, resultKind: 'SUMMARY' });
    mocks.createRun.mockResolvedValue({ ...succeededRun, id: 'run-3', status: 'QUEUED' });
    const wrapper = mountReport();
    await flushPromises();
    expect(mocks.createRun).toHaveBeenCalledTimes(1);
    // The rows are not asked for until there is a run that has them.
    expect(mocks.queryRows).not.toHaveBeenCalled();
    expect(wrapper.text()).not.toContain('กำลังดูเฉพาะ');
  });
});

describe('ViewerReport reads old figures again by itself', () => {
  const kpi = (value: string) => [{ key: 'total_balance', label: 'ยอดค้างรวม', value, unit: 'THB', comparison: { availability: 'UNAVAILABLE' } }];
  const withKpi = (value: string) => ({ ...dashboard, kpis: kpi(value) });
  const staleSnapshot = { runId: 'run-old', dashboard: withKpi('100.00'), freshnessStatus: 'STALE', detailsAvailable: true, sourceFinishedAt: '2026-10-01T01:17:00Z' };
  const newSnapshot = { runId: 'run-new', dashboard: withKpi('200.00'), freshnessStatus: 'FRESH', detailsAvailable: true, sourceFinishedAt: '2026-10-01T08:30:00Z' };

  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers();
    mocks.route.query = {};
    mocks.route.params.reportKey = 'ar_aging';
    mocks.run.mockResolvedValue(succeededRun);
  });
  afterEach(() => { vi.useRealTimers(); });

  it('shows the old figures at once, reads again, and swaps in the new figures without a button', async () => {
    mocks.exactSnapshot.mockResolvedValue(staleSnapshot);
    mocks.revalidateReport
      .mockResolvedValueOnce({ disposition: 'STALE_REFRESHING', snapshot: staleSnapshot, run: { ...succeededRun, id: 'run-new', status: 'RUNNING' }, retryAfter: 0, legacyFallback: false })
      .mockResolvedValue({ disposition: 'FRESH_CACHE', snapshot: newSnapshot, retryAfter: 0, legacyFallback: false });
    const wrapper = mountReport();
    await flushPromises();
    expect(wrapper.text()).toContain('100');
    expect(wrapper.text()).toContain('กำลังอัปเดตตัวเลขให้เป็นล่าสุด');
    expect(wrapper.text()).not.toMatch(/Snapshot|SML/);
    await vi.advanceTimersByTimeAsync(4_000);
    await flushPromises();
    expect(mocks.revalidateReport).toHaveBeenCalledTimes(2);
    expect(wrapper.text()).toContain('200');
    expect(wrapper.text()).not.toContain('กำลังอัปเดตตัวเลขให้เป็นล่าสุด');
    expect(mocks.createRun).not.toHaveBeenCalled();
  });

  it('does not ask the server to read again when the figures are fresh', async () => {
    mocks.exactSnapshot.mockResolvedValue({ ...newSnapshot });
    mountReport();
    await flushPromises();
    expect(mocks.revalidateReport).not.toHaveBeenCalled();
  });

  it('says plainly that it cannot update now, keeps the old figures, and lets the viewer try again', async () => {
    mocks.exactSnapshot.mockResolvedValue(staleSnapshot);
    mocks.revalidateReport.mockResolvedValue({ disposition: 'CIRCUIT_OPEN', snapshot: staleSnapshot, retryAfter: 120, legacyFallback: false });
    const wrapper = mountReport();
    await flushPromises();
    expect(wrapper.text()).toContain('ตอนนี้อัปเดตตัวเลขไม่ได้');
    expect(wrapper.text()).toContain('100');
    mocks.revalidateReport.mockResolvedValue({ disposition: 'FRESH_CACHE', snapshot: newSnapshot, retryAfter: 0, legacyFallback: false });
    await wrapper.findAll('button').find((button) => button.text() === 'ลองอีกครั้ง')!.trigger('click');
    await flushPromises();
    expect(mocks.revalidateReport).toHaveBeenCalledTimes(2);
    expect(wrapper.text()).toContain('200');
  });

  it('stops following after the time limit and offers a retry instead of waiting forever', async () => {
    mocks.exactSnapshot.mockResolvedValue(staleSnapshot);
    mocks.revalidateReport.mockResolvedValue({ disposition: 'STALE_REFRESHING', snapshot: staleSnapshot, run: { ...succeededRun, id: 'run-slow', status: 'RUNNING' }, retryAfter: 0, legacyFallback: false });
    const wrapper = mountReport();
    await flushPromises();
    await vi.advanceTimersByTimeAsync(5 * 60_000);
    await flushPromises();
    expect(wrapper.text()).toContain('ตอนนี้อัปเดตตัวเลขไม่ได้');
    const calls = mocks.revalidateReport.mock.calls.length;
    await vi.advanceTimersByTimeAsync(60_000);
    expect(mocks.revalidateReport.mock.calls.length).toBe(calls);
  });
});
