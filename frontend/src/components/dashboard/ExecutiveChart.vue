<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useLayout } from '@/layout/composables/layout';
import MobileExecutiveChart from './MobileExecutiveChart.vue';
import type { DashboardVisualization } from '@/api';
import { chartTypeFor, formatDashboardValue } from '@/utils/dashboard';
import { buildChartPresentation, chartDateLabel, truncateChartLabel } from '@/utils/chartPresentation';

const props = defineProps<{ visualization: DashboardVisualization; compact?: boolean }>();
const { layoutConfig, isDarkTheme } = useLayout();
const root = ref<HTMLElement>();
const chartData = ref();
const chartOptions = ref();
const isNarrow = ref(false);
const sizeReady = ref(false);
const reducedMotion = ref(false);
const canvasReady = ref(false);
let resizeObserver: ResizeObserver | undefined;
let resizeFrame = 0;
let canvasFrame = 0;
let motionQuery: MediaQueryList | undefined;
let disposed = false;

const model = computed(() => buildChartPresentation(props.visualization));
const type = computed(() => chartTypeFor(props.visualization));
const canRender = computed(() => model.value.status === 'READY' || model.value.status === 'PARTIAL_DATA');
const mobileSemanticMode = computed(() => isNarrow.value && ['RANKING', 'EXCEPTION', 'COMPOSITION'].includes(props.visualization.intent));
const tableAvailable = computed(() => model.value.status !== 'INVALID_DATA' && model.value.labels.length > 0 && model.value.series.length > 0);
const tableLabel = computed(() => props.compact && mobileSemanticMode.value && model.value.labels.length > 5
  ? `ดูครบทั้ง ${model.value.labels.length} รายการในรูปแบบตาราง`
  : 'ดูข้อมูลในรูปแบบตาราง');
const multiSeriesComposition = computed(() => props.visualization.intent === 'COMPOSITION' && model.value.series.length > 1);
const horizontalCanvas = computed(() => props.visualization.intent === 'RANKING' || props.visualization.intent === 'EXCEPTION'
  || multiSeriesComposition.value && (model.value.labels.length > 6 || model.value.labels.some((label) => label.length > 16)));
const chartHeight = computed(() => {
  if (!horizontalCanvas.value) return props.compact ? '17rem' : '21rem';
  const minimum = props.compact ? 272 : 336;
  return `${Math.max(minimum, model.value.labels.length * 32 + 72)}px`;
});

const paletteProperties = ['--p-primary-500', '--p-sky-500', '--p-orange-500', '--p-violet-500', '--p-teal-500'];

function toneColor(documentStyle: CSSStyleDeclaration): string {
  if (model.value.tone === 'danger') return documentStyle.getPropertyValue('--p-red-500').trim();
  if (model.value.tone === 'warning') return documentStyle.getPropertyValue('--p-orange-500').trim();
  return documentStyle.getPropertyValue('--p-primary-500').trim();
}

function numericTick(value: string | number): string {
  const number = Number(value);
  return Number.isFinite(number) ? new Intl.NumberFormat('th-TH', { maximumFractionDigits: 0 }).format(number) : String(value);
}

function setChartOptions() {
  const documentStyle = getComputedStyle(document.documentElement);
  const colors = paletteProperties.map((property) => documentStyle.getPropertyValue(property).trim());
  const semanticColor = toneColor(documentStyle);
  const textColor = documentStyle.getPropertyValue('--text-color').trim();
  const textColorSecondary = documentStyle.getPropertyValue('--text-color-secondary').trim();
  const surfaceBorder = documentStyle.getPropertyValue('--surface-border').trim();
  const horizontal = horizontalCanvas.value;
  const stacked = props.visualization.intent === 'COMPOSITION';
  const currentPointLabels = model.value.series.find((series) => series.key === 'current')?.pointLabels;
  const trendLabels = currentPointLabels?.length === model.value.labels.length ? currentPointLabels : model.value.labels;
  const labels = props.visualization.intent === 'TREND'
    ? trendLabels.map((label) => chartDateLabel(label, false))
    : model.value.labels;

  chartData.value = {
    labels,
    datasets: model.value.series.map((series, index) => ({
      label: series.label,
      data: series.values,
      pointLabels: series.pointLabels,
      backgroundColor: model.value.series.length === 1 ? semanticColor : colors[index % colors.length],
      borderColor: model.value.series.length === 1 ? semanticColor : colors[index % colors.length],
      borderWidth: type.value === 'line' ? 2 : 0,
      borderRadius: type.value === 'bar' ? 5 : 0,
      fill: false,
      tension: 0.3,
      pointRadius: type.value === 'line' && !isNarrow.value && model.value.labels.length <= 12 ? 2.5 : 0,
      pointHoverRadius: type.value === 'line' ? 5 : 0
    }))
  };
  chartOptions.value = {
    responsive: true,
    maintainAspectRatio: false,
    indexAxis: horizontal ? 'y' : 'x',
    animation: isNarrow.value || reducedMotion.value ? false : { duration: 200 },
    interaction: { mode: 'index', axis: horizontal ? 'y' : 'x', intersect: false },
    plugins: {
      legend: { display: model.value.series.length > 1, position: 'bottom', labels: { color: textColor, usePointStyle: true, boxWidth: 8 } },
      tooltip: {
        callbacks: {
          label: (context: { dataIndex: number; dataset: { label?: string; pointLabels?: string[] }; raw: number | null }) => {
            const date = context.dataset.pointLabels?.[context.dataIndex];
            const dateText = date ? ` · ${chartDateLabel(date, true)}` : '';
            return `${context.dataset.label ?? ''}${dateText}: ${formatDashboardValue(context.raw === null ? undefined : String(context.raw), props.visualization.unit)}`;
          }
        }
      }
    },
    scales: horizontal ? {
      x: { stacked, beginAtZero: true, ticks: { color: textColorSecondary, callback: numericTick }, grid: { color: surfaceBorder } },
      y: { stacked, ticks: { color: textColorSecondary, autoSkip: false, callback: (_value: string | number, index: number) => truncateChartLabel(model.value.labels[index] ?? '', 24) }, grid: { display: false } }
    } : {
      x: { stacked, ticks: { color: textColorSecondary, maxRotation: 0, autoSkip: true, maxTicksLimit: isNarrow.value ? 4 : 8 }, grid: { display: false } },
      y: { stacked, beginAtZero: true, ticks: { color: textColorSecondary, callback: numericTick }, grid: { color: surfaceBorder } }
    }
  };
}

function applyWidth(width: number) {
  if (width <= 0) return;
  isNarrow.value = width < 480;
  sizeReady.value = true;
}

function updateWidth(width: number) {
  if (disposed || width <= 0) return;
  if (resizeFrame) cancelAnimationFrame(resizeFrame);
  resizeFrame = requestAnimationFrame(() => {
    if (!disposed) applyWidth(width);
    resizeFrame = 0;
  });
}

function updateMotionPreference(event?: MediaQueryListEvent) {
  reducedMotion.value = event?.matches ?? motionQuery?.matches ?? false;
}

function queueCanvasMount() {
  if (canvasFrame) cancelAnimationFrame(canvasFrame);
  canvasFrame = requestAnimationFrame(() => {
    if (!disposed && !mobileSemanticMode.value) canvasReady.value = true;
    canvasFrame = 0;
  });
}

onMounted(() => {
  if (root.value) {
    applyWidth(root.value.getBoundingClientRect().width);
    if (typeof ResizeObserver !== 'undefined') {
      resizeObserver = new ResizeObserver((entries) => updateWidth(entries[0]?.contentRect.width ?? 0));
      resizeObserver.observe(root.value);
    }
  }
  motionQuery = window.matchMedia('(prefers-reduced-motion: reduce)');
  updateMotionPreference();
  motionQuery.addEventListener('change', updateMotionPreference);
  if (!mobileSemanticMode.value) {
    setChartOptions();
    canvasReady.value = true;
  }
});
onBeforeUnmount(() => {
  disposed = true;
  resizeObserver?.disconnect();
  if (resizeFrame) cancelAnimationFrame(resizeFrame);
  if (canvasFrame) cancelAnimationFrame(canvasFrame);
  motionQuery?.removeEventListener('change', updateMotionPreference);
});
watch(mobileSemanticMode, (semanticMode) => {
  if (disposed) return;
  if (semanticMode) {
    canvasReady.value = false;
    return;
  }
  setChartOptions();
  queueCanvasMount();
});
watch([model, () => layoutConfig.primary, () => layoutConfig.surface, isDarkTheme, isNarrow, reducedMotion, horizontalCanvas], () => {
  // Do not update Chart.js props in the same render cycle that removes its
  // canvas. PrimeVue otherwise schedules an update after the canvas is gone.
  if (!disposed && !mobileSemanticMode.value && canvasReady.value) setChartOptions();
}, { deep: true });
</script>

<template>
  <section ref="root" class="executive-chart" :aria-label="visualization.title">
    <div v-if="model.status === 'PARTIAL_DATA'" class="chart-warning" role="status"><i class="pi pi-exclamation-triangle" /><span>ข้อมูลบางจุดไม่สมบูรณ์ กราฟแสดงเฉพาะค่าที่อ่านได้</span></div>

    <div v-if="!sizeReady" class="chart-pending" aria-hidden="true" />
    <MobileExecutiveChart v-else-if="canRender && mobileSemanticMode" :model="model" :compact="compact" />
    <Chart v-else-if="canRender && chartData && canvasReady" :type="type" :data="chartData" :options="chartOptions" :style="{ height: chartHeight }" />
    <div v-else-if="canRender" class="chart-pending" :style="{ height: chartHeight }" aria-hidden="true" />
    <div v-else-if="model.status === 'ALL_ZERO'" class="chart-empty" role="status"><i class="pi pi-minus-circle" /><strong>ไม่มีความเคลื่อนไหวในช่วงนี้</strong><span>ค่าที่ได้รับเป็นศูนย์ทั้งหมด</span></div>
    <div v-else-if="model.status === 'INVALID_DATA'" class="chart-empty" role="alert"><i class="pi pi-exclamation-triangle" /><strong>ไม่สามารถสร้างกราฟจากข้อมูลชุดนี้</strong><span>{{ model.warnings[0] ?? 'รูปแบบข้อมูลกราฟไม่ถูกต้อง' }}</span></div>
    <div v-else class="chart-empty" role="status"><i class="pi pi-chart-bar" /><strong>ยังไม่มีข้อมูลสำหรับช่วงนี้</strong></div>

    <p v-if="visualization.note" class="text-xs text-muted-color mt-3 mb-0">{{ visualization.note }}</p>
    <details v-if="tableAvailable" class="chart-table mt-3">
      <summary class="chart-table-summary">{{ tableLabel }}</summary>
      <div class="overflow-x-auto mt-3">
        <table>
          <thead><tr><th>รายการ</th><th v-for="series in model.series" :key="series.key" class="table-number-column">{{ series.label }}</th></tr></thead>
          <tbody><tr v-for="(category, categoryIndex) in model.labels" :key="`${category}-${categoryIndex}`"><td>{{ category }}</td><td v-for="series in model.series" :key="series.key" class="metric-value table-number-column">{{ formatDashboardValue(series.rawValues[categoryIndex], visualization.unit) }}</td></tr></tbody>
        </table>
      </div>
    </details>
  </section>
</template>

<style scoped>
.executive-chart { container-type: inline-size; min-width: 0; }
.chart-pending { min-height: 12rem; }
.chart-warning { display: flex; align-items: flex-start; gap: .6rem; margin: .25rem 0 .85rem; padding: .7rem .8rem; border-radius: var(--content-border-radius); background: var(--p-orange-50); color: var(--p-orange-800); font-size: .8rem; line-height: 1.45; }
.chart-warning i { margin-top: .15rem; }
.chart-empty { min-height: 17rem; display: grid; place-items: center; align-content: center; gap: .55rem; color: var(--text-color-secondary); text-align: center; }
.chart-empty i { color: var(--primary-color); font-size: 2rem; }
.chart-empty strong { color: var(--text-color); }
.chart-empty span { font-size: .8rem; }
.chart-table-summary { width: fit-content; min-height: 2.75rem; display: inline-flex; align-items: center; color: var(--primary-color); cursor: pointer; font-size: .875rem; font-weight: 600; }
.chart-table table { width: 100%; border-collapse: collapse; font-size: .875rem; }
.chart-table th, .chart-table td { padding: .65rem .75rem; text-align: left; border-bottom: 1px solid var(--surface-border); white-space: nowrap; }
.chart-table th { color: var(--text-color-secondary); font-weight: 600; }
:global(.app-dark) .chart-warning { background: color-mix(in srgb, var(--p-orange-500) 16%, transparent); color: var(--p-orange-200); }
</style>
