import { flushPromises, mount } from '@vue/test-utils';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ref } from 'vue';

const monitorCurrent = vi.fn();
const monitorHistory = vi.fn();
vi.mock('@/api', () => ({ adminApi: { monitorCurrent: (...args: unknown[]) => monitorCurrent(...args), monitorHistory: (...args: unknown[]) => monitorHistory(...args) } }));
vi.mock('@/layout/composables/layout', () => ({ useLayout: () => ({ layoutConfig: { primary: 'emerald', surface: null }, isDarkTheme: ref(false) }) }));

import { ApiError } from '@/api/client';
import Monitor from './Monitor.vue';

const GB = 1024 ** 3;
const sample = (overrides: Record<string, unknown> = {}) => ({
  at: '2026-09-30T05:00:00Z',
  host: { cores: 4, cpuPercent: 12.5, load1: 0.5, load5: 0.6, load15: 0.7, memoryTotalBytes: 16 * GB, memoryAvailableBytes: 4 * GB, diskTotalBytes: 100 * GB, diskUsedBytes: 92 * GB, uptimeSeconds: 7200 },
  containersAvailable: true,
  containers: [{ name: 'ai-bcc-v2-api-1', cpuPercent: 1.5, memoryUsedBytes: 100 * 1048576, memoryLimitBytes: 512 * 1048576 }],
  ...overrides
});

const slotStub = { template: '<div><slot /><slot name="actions" /></div>' };
const stubs = {
  AppPageHeader: slotStub, Message: slotStub, DataTable: { props: ['value'], template: '<ul class="containers"><li v-for="row in value" :key="row.name">{{ row.name }}</li></ul>' },
  Column: true, Chart: true, ProgressBar: true, SelectButton: true,
  Tag: { props: ['value', 'severity'], template: '<span class="tag" :data-severity="severity">{{ value }}</span>' }
};

describe('Monitor page', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    monitorHistory.mockResolvedValue({ minutes: 60, data: [] });
  });
  afterEach(() => { vi.useRealTimers(); vi.clearAllMocks(); });

  it('shows host tiles with severity and lists containers', async () => {
    monitorCurrent.mockResolvedValue(sample());
    const wrapper = mount(Monitor, { global: { stubs } });
    await flushPromises();

    const tile = (key: string) => wrapper.get(`[data-testid="tile-${key}"] .tag`);
    expect(tile('cpu').text()).toBe('12.5%');
    expect(tile('cpu').attributes('data-severity')).toBe('success');
    expect(tile('memory').text()).toBe('75.0%');
    expect(tile('memory').attributes('data-severity')).toBe('warn');
    expect(tile('disk').attributes('data-severity')).toBe('danger');
    expect(wrapper.get('.containers').text()).toContain('ai-bcc-v2-api-1');
    expect(wrapper.text()).toContain('เปิดเครื่องมาแล้ว 2 ชม. 0 นาที');
    wrapper.unmount();
  });

  it('says so when the host helper is not providing container figures', async () => {
    monitorCurrent.mockResolvedValue(sample({ containersAvailable: false, containers: [] }));
    const wrapper = mount(Monitor, { global: { stubs } });
    await flushPromises();
    expect(wrapper.text()).toContain('ยังไม่มีข้อมูลราย container');
    expect(wrapper.find('.containers').exists()).toBe(false);
    expect(wrapper.find('[data-testid="tile-cpu"]').exists()).toBe(true);
    wrapper.unmount();
  });

  it('treats a 503 as warming up and only flags an outage after repeated failures', async () => {
    monitorCurrent.mockRejectedValue(new ApiError(503, { code: 'MONITOR_WARMING_UP', message: 'warming', requestId: 'r', retryable: true }));
    const wrapper = mount(Monitor, { global: { stubs } });
    await flushPromises();
    expect(wrapper.text()).toContain('กำลังเริ่มเก็บข้อมูล');
    expect(wrapper.text()).not.toContain('เชื่อมต่อไม่ได้');

    monitorCurrent.mockRejectedValue(new Error('network'));
    await vi.advanceTimersByTimeAsync(5000);
    expect(wrapper.text()).not.toContain('เชื่อมต่อไม่ได้');
    await vi.advanceTimersByTimeAsync(5000);
    expect(wrapper.text()).toContain('เชื่อมต่อไม่ได้');
    wrapper.unmount();
  });

  it('polls every five seconds and stops after unmount', async () => {
    monitorCurrent.mockResolvedValue(sample());
    const wrapper = mount(Monitor, { global: { stubs } });
    await flushPromises();
    expect(monitorCurrent).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(10000);
    expect(monitorCurrent).toHaveBeenCalledTimes(3);
    wrapper.unmount();
    await vi.advanceTimersByTimeAsync(20000);
    expect(monitorCurrent).toHaveBeenCalledTimes(3);
  });
});
