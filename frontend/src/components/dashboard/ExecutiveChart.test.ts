import { mount } from '@vue/test-utils';
import { defineComponent, watch } from 'vue';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { DashboardVisualization } from '@/api';
import ExecutiveChart from './ExecutiveChart.vue';

function ranking(): DashboardVisualization {
  return {
    key: 'top_stock_value', title: 'สินค้าที่มีมูลค่าคงเหลือสูงสุด', intent: 'RANKING', unit: 'THB',
    categories: Array.from({ length: 10 }, (_, index) => `สินค้าชื่อยาวอันดับ ${index + 1}`),
    series: [{ key: 'value', label: 'มูลค่า', values: Array.from({ length: 10 }, (_, index) => String(1000 - index * 50)) }]
  };
}

function composition(): DashboardVisualization {
  return {
    key: 'cash_receipt_methods', title: 'ช่องทางการรับเงิน', intent: 'COMPOSITION', unit: 'THB',
    categories: ['เงินสด', 'เงินโอน'],
    series: [{ key: 'amount', label: 'จำนวนเงิน', values: ['25', '75'] }]
  };
}

function trend(): DashboardVisualization {
  return {
    key: 'sales_trend', title: 'แนวโน้มยอดขาย', intent: 'TREND', unit: 'THB', categories: ['1', '2'],
    series: [
      { key: 'current', label: 'งวดปัจจุบัน', values: ['100', '200'], pointLabels: ['2026-07-10', '2026-07-11'] },
      { key: 'previous', label: 'งวดก่อน', values: ['80', '160'], pointLabels: ['2026-07-03', '2026-07-04'] }
    ]
  };
}

function installResizeObserver(width: number) {
  const disconnect = vi.fn();
  class MockResizeObserver {
    constructor(private readonly callback: ResizeObserverCallback) {}
    observe(target: Element) { this.callback([{ target, contentRect: { width } } as ResizeObserverEntry], this as unknown as ResizeObserver); }
    unobserve() {}
    disconnect = disconnect;
  }
  vi.stubGlobal('ResizeObserver', MockResizeObserver);
  vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => { callback(0); return 1; });
  vi.stubGlobal('cancelAnimationFrame', vi.fn());
  return disconnect;
}

function installControlledResizeObserver(initialWidth: number) {
  let callback: ResizeObserverCallback | undefined;
  let observedTarget: Element | undefined;
  class MockResizeObserver {
    constructor(nextCallback: ResizeObserverCallback) { callback = nextCallback; }
    observe(target: Element) {
      observedTarget = target;
      callback?.([{ target, contentRect: { width: initialWidth } } as ResizeObserverEntry], this as unknown as ResizeObserver);
    }
    unobserve() {}
    disconnect() {}
  }
  vi.stubGlobal('ResizeObserver', MockResizeObserver);
  vi.stubGlobal('requestAnimationFrame', (nextCallback: FrameRequestCallback) => {
    queueMicrotask(() => nextCallback(0));
    return 1;
  });
  vi.stubGlobal('cancelAnimationFrame', vi.fn());
  return {
    resize(width: number) {
      if (!callback || !observedTarget) throw new Error('ResizeObserver was not connected');
      callback([{ target: observedTarget, contentRect: { width } } as ResizeObserverEntry], {} as ResizeObserver);
    }
  };
}

const chartStub = { name: 'Chart', props: ['type', 'data', 'options'], template: '<div data-testid="chart-canvas" />' };

afterEach(() => vi.unstubAllGlobals());

describe('ExecutiveChart responsive presentation', () => {
  it('shows five semantic ranking rows without mounting canvas in a narrow overview card', async () => {
    const disconnect = installResizeObserver(390);
    const wrapper = mount(ExecutiveChart, { props: { visualization: ranking(), compact: true }, global: { stubs: { Chart: chartStub } } });
    await wrapper.vm.$nextTick();

    expect(wrapper.findAll('[data-testid="ranking-item"]')).toHaveLength(5);
    expect(wrapper.find('[data-testid="chart-canvas"]').exists()).toBe(false);
    expect(wrapper.text()).toContain('ดูครบทั้ง 10 รายการในรูปแบบตาราง');
    expect(wrapper.text()).toContain('สินค้าชื่อยาวอันดับ 1');
    wrapper.unmount();
    expect(disconnect).toHaveBeenCalledOnce();
  });

  it('keeps the Chart.js canvas for a wide card', async () => {
    installResizeObserver(768);
    const wrapper = mount(ExecutiveChart, { props: { visualization: ranking() }, global: { stubs: { Chart: chartStub } } });
    await wrapper.vm.$nextTick();

    expect(wrapper.find('[data-testid="chart-canvas"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="mobile-ranking"]').exists()).toBe(false);
  });

  it('does not update Chart.js props while replacing the canvas with the mobile presentation', async () => {
    const observer = installControlledResizeObserver(768);
    const optionUpdates = vi.fn();
    const guardedChartStub = defineComponent({
      name: 'GuardedChartStub',
      props: ['type', 'data', 'options'],
      setup(chartProps) { watch(() => chartProps.options, optionUpdates); },
      template: '<div data-testid="chart-canvas" />'
    });
    const wrapper = mount(ExecutiveChart, { props: { visualization: ranking() }, global: { stubs: { Chart: guardedChartStub } } });
    await new Promise((resolve) => setTimeout(resolve, 0));
    const updatesBeforeResize = optionUpdates.mock.calls.length;

    observer.resize(390);
    await new Promise((resolve) => setTimeout(resolve, 0));
    await wrapper.vm.$nextTick();

    expect(wrapper.find('[data-testid="chart-canvas"]').exists()).toBe(false);
    expect(wrapper.find('[data-testid="mobile-ranking"]').exists()).toBe(true);
    expect(optionUpdates).toHaveBeenCalledTimes(updatesBeforeResize);
  });

  it('waits a render frame before mounting canvas again after a rapid mobile round trip', async () => {
    const observer = installControlledResizeObserver(768);
    const wrapper = mount(ExecutiveChart, { props: { visualization: ranking() }, global: { stubs: { Chart: chartStub } } });
    await new Promise((resolve) => setTimeout(resolve, 0));

    observer.resize(390);
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(wrapper.find('[data-testid="mobile-ranking"]').exists()).toBe(true);

    observer.resize(768);
    expect(wrapper.find('[data-testid="chart-canvas"]').exists()).toBe(false);
    await new Promise((resolve) => setTimeout(resolve, 0));
    await wrapper.vm.$nextTick();

    expect(wrapper.find('[data-testid="chart-canvas"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="mobile-ranking"]').exists()).toBe(false);
  });

  it('uses the category axis for tooltip hit testing on horizontal bars', async () => {
    installResizeObserver(768);
    const wrapper = mount(ExecutiveChart, { props: { visualization: ranking() }, global: { stubs: { Chart: chartStub } } });
    await wrapper.vm.$nextTick();

    const chart = wrapper.getComponent({ name: 'Chart' });
    const options = chart.props('options') as { interaction: { axis?: string; intersect: boolean; mode: string }; resizeDelay?: number };

    expect(options.interaction).toEqual({ mode: 'index', axis: 'y', intersect: false });
    expect(options.resizeDelay).toBeUndefined();
  });

  it('keeps trend tooltip hit testing on the horizontal time axis', async () => {
    installResizeObserver(768);
    const wrapper = mount(ExecutiveChart, { props: { visualization: trend() }, global: { stubs: { Chart: chartStub } } });
    await wrapper.vm.$nextTick();

    const chart = wrapper.getComponent({ name: 'Chart' });
    const options = chart.props('options') as { interaction: { axis?: string; intersect: boolean; mode: string } };

    expect(options.interaction).toEqual({ mode: 'index', axis: 'x', intersect: false });
  });

  it('shows exact composition percentages without mounting canvas in a narrow card', async () => {
    installResizeObserver(390);
    const wrapper = mount(ExecutiveChart, { props: { visualization: composition() }, global: { stubs: { Chart: chartStub } } });
    await wrapper.vm.$nextTick();

    expect(wrapper.find('[data-testid="mobile-composition"]').exists()).toBe(true);
    expect(wrapper.find('[data-testid="chart-canvas"]').exists()).toBe(false);
    expect(wrapper.text()).toContain('25%');
    expect(wrapper.text()).toContain('75%');
  });

  it('explains invalid chart data without mounting canvas', async () => {
    installResizeObserver(390);
    const invalid = ranking();
    invalid.series[0]!.values = ['100'];
    const wrapper = mount(ExecutiveChart, { props: { visualization: invalid }, global: { stubs: { Chart: chartStub } } });
    await wrapper.vm.$nextTick();

    expect(wrapper.get('[role="alert"]').text()).toContain('ไม่สามารถสร้างกราฟจากข้อมูลชุดนี้');
    expect(wrapper.find('[data-testid="chart-canvas"]').exists()).toBe(false);
  });

  it('uses current dates on the trend axis and the exact series date in its tooltip', async () => {
    installResizeObserver(390);
    const wrapper = mount(ExecutiveChart, { props: { visualization: trend() }, global: { stubs: { Chart: chartStub } } });
    await wrapper.vm.$nextTick();
    const chart = wrapper.getComponent({ name: 'Chart' });
    const data = chart.props('data') as { labels: string[]; datasets: Array<{ label: string; pointLabels: string[] }> };
    const options = chart.props('options') as { plugins: { tooltip: { callbacks: { label: (context: unknown) => string } } } };

    expect(data.labels).toEqual(['10 ก.ค. 69', '11 ก.ค. 69']);
    expect(options.plugins.tooltip.callbacks.label({ dataIndex: 1, dataset: data.datasets[1], raw: 160 })).toContain('4 ก.ค. 2569');
  });
});
