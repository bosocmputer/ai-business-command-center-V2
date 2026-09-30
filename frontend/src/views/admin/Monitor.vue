<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { adminApi, type MonitorHistoryPoint, type MonitorSample } from '@/api';
import { ApiError } from '@/api/client';
import { useLayout } from '@/layout/composables/layout';
import {
  containerMemoryPercent, containerSeries, formatBytes, formatUptime, shortContainerName, timeLabel, usagePercent, usageSeverity
} from '@/utils/monitorPresentation';

const currentIntervalMs = 5000;
const historyIntervalMs = 30000;
const { layoutConfig, isDarkTheme } = useLayout();

const sample = ref<MonitorSample>();
const failures = ref(0);
const warmingUp = ref(false);
const history = ref<MonitorHistoryPoint[]>([]);
const historyFailed = ref(false);
const rangeMinutes = ref(60);
const ranges = [{ label: '1 ชั่วโมง', value: 60 }, { label: '6 ชั่วโมง', value: 360 }, { label: '24 ชั่วโมง', value: 1440 }];

let currentTimer: number | undefined;
let historyTimer: number | undefined;
let currentAbort: AbortController | undefined;
let historyAbort: AbortController | undefined;

async function loadCurrent() {
  if (document.hidden) return;
  currentAbort?.abort();
  currentAbort = new AbortController();
  try {
    sample.value = await adminApi.monitorCurrent(currentAbort.signal);
    failures.value = 0;
    warmingUp.value = false;
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') return;
    if (error instanceof ApiError && error.status === 503) { warmingUp.value = true; return; }
    failures.value += 1;
  }
}

async function loadHistory() {
  if (document.hidden) return;
  historyAbort?.abort();
  historyAbort = new AbortController();
  try {
    history.value = (await adminApi.monitorHistory(rangeMinutes.value, historyAbort.signal)).data;
    historyFailed.value = false;
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') return;
    historyFailed.value = true;
  }
}

function onVisibility() {
  if (!document.hidden) { void loadCurrent(); void loadHistory(); }
}

onMounted(() => {
  void loadCurrent();
  void loadHistory();
  currentTimer = window.setInterval(() => void loadCurrent(), currentIntervalMs);
  historyTimer = window.setInterval(() => void loadHistory(), historyIntervalMs);
  document.addEventListener('visibilitychange', onVisibility);
});
onBeforeUnmount(() => {
  window.clearInterval(currentTimer);
  window.clearInterval(historyTimer);
  currentAbort?.abort();
  historyAbort?.abort();
  document.removeEventListener('visibilitychange', onVisibility);
});
watch(rangeMinutes, () => { void loadHistory(); });

const host = computed(() => sample.value?.host);
const memoryUsed = computed(() => host.value ? host.value.memoryTotalBytes - host.value.memoryAvailableBytes : 0);
const memoryPercent = computed(() => host.value ? usagePercent(memoryUsed.value, host.value.memoryTotalBytes) : 0);
const diskPercent = computed(() => host.value ? usagePercent(host.value.diskUsedBytes, host.value.diskTotalBytes) : 0);
const loadPercent = computed(() => host.value ? usagePercent(host.value.load1, host.value.cores) : 0);
const stale = computed(() => failures.value >= 2);
const tiles = computed(() => {
  const h = host.value;
  if (!h) return [];
  return [
    { key: 'cpu', label: 'CPU', value: `${h.cpuPercent.toFixed(1)}%`, percent: h.cpuPercent, detail: `${h.cores} แกน` },
    { key: 'memory', label: 'หน่วยความจำ', value: `${memoryPercent.value.toFixed(1)}%`, percent: memoryPercent.value, detail: `${formatBytes(memoryUsed.value)} จาก ${formatBytes(h.memoryTotalBytes)}` },
    { key: 'disk', label: 'ดิสก์', value: `${diskPercent.value.toFixed(1)}%`, percent: diskPercent.value, detail: `${formatBytes(h.diskUsedBytes)} จาก ${formatBytes(h.diskTotalBytes)}` },
    { key: 'load', label: 'Load (1 นาที)', value: h.load1.toFixed(2), percent: loadPercent.value, detail: `5 นาที ${h.load5.toFixed(2)} · 15 นาที ${h.load15.toFixed(2)}` }
  ];
});
const containers = computed(() => [...(sample.value?.containers ?? [])].sort((left, right) => right.memoryUsedBytes - left.memoryUsedBytes));
const updatedAt = computed(() => sample.value ? new Date(sample.value.at).toLocaleTimeString('th-TH', { timeZone: 'Asia/Bangkok' }) : '');

const palette = ['--p-primary-500', '--p-sky-500', '--p-orange-500', '--p-violet-500', '--p-teal-500', '--p-pink-500'];
function chartOptions(unit: string, max?: number) {
  void isDarkTheme.value; void layoutConfig.primary; void layoutConfig.surface;
  const style = getComputedStyle(document.documentElement);
  const text = style.getPropertyValue('--text-color').trim();
  const muted = style.getPropertyValue('--text-color-secondary').trim();
  const border = style.getPropertyValue('--surface-border').trim();
  return {
    maintainAspectRatio: false,
    animation: false,
    interaction: { mode: 'index', intersect: false },
    plugins: { legend: { position: 'bottom', labels: { color: text, usePointStyle: true, boxWidth: 8 } }, tooltip: { callbacks: { label: (item: { dataset: { label?: string }; parsed: { y: number | null } }) => `${item.dataset.label}: ${item.parsed.y ?? '-'} ${unit}` } } },
    scales: {
      x: { ticks: { color: muted, maxRotation: 0, autoSkip: true, maxTicksLimit: 6 }, grid: { display: false } },
      y: { beginAtZero: true, max, ticks: { color: muted, callback: (value: string | number) => `${value}` }, grid: { color: border } }
    }
  };
}
function color(index: number) {
  return getComputedStyle(document.documentElement).getPropertyValue(palette[index % palette.length] ?? '--p-primary-500').trim();
}
function dataset(label: string, data: Array<number | null>, index: number) {
  const line = color(index);
  return { label, data, borderColor: line, backgroundColor: line, borderWidth: 2, pointRadius: 0, tension: 0.25, spanGaps: false };
}
const labels = computed(() => history.value.map((point) => timeLabel(point.at, rangeMinutes.value)));
const hostChart = computed(() => { void isDarkTheme.value; return { labels: labels.value, datasets: [
  dataset('CPU %', history.value.map((point) => point.cpuPercent), 0),
  dataset('หน่วยความจำ %', history.value.map((point) => point.memoryUsedPercent), 2)
] }; });
const cpuChart = computed(() => { void isDarkTheme.value; return { labels: labels.value, datasets: containerSeries(history.value, 'cpu').map((series, index) => dataset(series.label, series.data, index)) }; });
const memoryChart = computed(() => { void isDarkTheme.value; return { labels: labels.value, datasets: containerSeries(history.value, 'memory').map((series, index) => dataset(series.label, series.data, index)) }; });
const hostOptions = computed(() => chartOptions('%', 100));
const cpuOptions = computed(() => chartOptions('%'));
const memoryOptions = computed(() => chartOptions('MB'));
const hasContainerHistory = computed(() => history.value.some((point) => point.containers.length > 0));
</script>

<template>
  <AppPageHeader title="สถานะเครื่อง" subtitle="ทรัพยากรของเครื่อง server และแต่ละ container แบบเรียลไทม์">
    <template #actions>
      <Tag v-if="stale" severity="danger" value="เชื่อมต่อไม่ได้ กำลังลองใหม่" />
      <Tag v-else-if="sample" severity="success" :value="`อัปเดต ${updatedAt} · ทุก 5 วินาที`" />
      <Tag v-else severity="secondary" :value="warmingUp ? 'กำลังเริ่มเก็บข้อมูล' : 'กำลังโหลด'" />
    </template>
  </AppPageHeader>

  <Message v-if="stale" severity="error" :closable="false" class="mb-4">ตัวเลขด้านล่างเป็นค่าล่าสุดที่ได้รับ ไม่ใช่ค่าปัจจุบัน</Message>

  <section aria-labelledby="monitor-host-title">
    <h2 id="monitor-host-title" class="sr-only">เครื่อง server</h2>
    <div v-if="tiles.length" class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4 mb-4">
      <div v-for="tile in tiles" :key="tile.key" class="card tile" :data-testid="`tile-${tile.key}`">
        <div class="flex items-center justify-between gap-2"><span class="text-muted-color">{{ tile.label }}</span><Tag :severity="usageSeverity(tile.percent)" :value="tile.value" /></div>
        <ProgressBar :value="tile.percent" :show-value="false" class="mt-3" style="height: .5rem" />
        <p class="m-0 mt-3 text-sm text-muted-color">{{ tile.detail }}</p>
      </div>
    </div>
    <p v-if="host" class="text-sm text-muted-color mt-0 mb-4">เปิดเครื่องมาแล้ว {{ formatUptime(host.uptimeSeconds) }}</p>
  </section>

  <section class="card" aria-labelledby="monitor-containers-title">
    <div class="mb-4"><h2 id="monitor-containers-title" class="text-xl font-semibold m-0">แต่ละ container</h2><p class="text-muted-color mt-1 mb-0">CPU เป็นเปอร์เซ็นต์ของ 1 แกน (เกิน 100% ได้ถ้าใช้หลายแกน) · โมเดล AI รันที่ผู้ให้บริการ ตัวเลขนี้คือภาระบนเครื่องเราเท่านั้น</p></div>
    <Message v-if="sample && !sample.containersAvailable" severity="warn" :closable="false" class="mb-4">ยังไม่มีข้อมูลราย container: ตัวเก็บข้อมูลบนเครื่อง (aibcc-v2-host-monitor) ไม่ทำงานหรือไม่ได้อัปเดตเกิน 30 วินาที ข้อมูลของเครื่องด้านบนยังใช้ได้ปกติ</Message>
    <DataTable v-else :value="containers" data-key="name" size="small" striped-rows responsive-layout="scroll">
      <Column header="Container"><template #body="{ data }"><span class="font-medium">{{ shortContainerName(data.name) }}</span><span v-if="shortContainerName(data.name) !== data.name" class="text-muted-color text-sm ml-2">{{ data.name }}</span></template></Column>
      <Column header="CPU"><template #body="{ data }"><span class="tabular">{{ data.cpuPercent.toFixed(1) }}%</span></template></Column>
      <Column header="หน่วยความจำ"><template #body="{ data }">
        <div class="flex items-center gap-3">
          <span class="tabular">{{ formatBytes(data.memoryUsedBytes) }}</span>
          <template v-if="host && containerMemoryPercent(data, host.memoryTotalBytes) !== null">
            <ProgressBar :value="containerMemoryPercent(data, host.memoryTotalBytes) ?? 0" :show-value="false" style="height: .4rem; width: 6rem" />
            <span class="text-sm text-muted-color tabular">{{ containerMemoryPercent(data, host.memoryTotalBytes) }}% ของ {{ formatBytes(data.memoryLimitBytes) }}</span>
          </template>
          <span v-else class="text-sm text-muted-color">ไม่มีเพดาน</span>
        </div>
      </template></Column>
      <template #empty><div class="py-6 text-center text-muted-color">ไม่พบ container</div></template>
    </DataTable>
  </section>

  <section class="card" aria-labelledby="monitor-history-title">
    <div class="flex flex-wrap items-center justify-between gap-3 mb-4">
      <div><h2 id="monitor-history-title" class="text-xl font-semibold m-0">ย้อนหลัง</h2><p class="text-muted-color mt-1 mb-0">เก็บทุก 30 วินาที นาน 48 ชั่วโมง · ใช้ดูช่วงที่มีคนคุยกับผู้ช่วย AI ว่าภาระขึ้นเท่าไร</p></div>
      <SelectButton v-model="rangeMinutes" :options="ranges" option-label="label" option-value="value" :allow-empty="false" aria-label="ช่วงเวลาย้อนหลัง" />
    </div>
    <Message v-if="historyFailed" severity="warn" :closable="false" class="mb-4">โหลดข้อมูลย้อนหลังไม่สำเร็จ กำลังลองใหม่</Message>
    <div v-if="history.length < 2" class="py-8 text-center text-muted-color">ยังมีข้อมูลไม่พอสำหรับกราฟ (ระบบเพิ่งเริ่มเก็บ) รอสักครู่</div>
    <div v-else class="grid grid-cols-1 xl:grid-cols-2 gap-6">
      <div><h3 class="chart-title">ทั้งเครื่อง: CPU และหน่วยความจำ (%)</h3><Chart type="line" :data="hostChart" :options="hostOptions" class="chart" /></div>
      <div v-if="hasContainerHistory"><h3 class="chart-title">CPU แต่ละ container (% ของ 1 แกน)</h3><Chart type="line" :data="cpuChart" :options="cpuOptions" class="chart" /></div>
      <div v-if="hasContainerHistory"><h3 class="chart-title">หน่วยความจำแต่ละ container (MB)</h3><Chart type="line" :data="memoryChart" :options="memoryOptions" class="chart" /></div>
    </div>
  </section>
</template>

<style scoped>
.tile { margin-bottom: 0; }
.tabular { font-variant-numeric: tabular-nums; }
.chart { height: 16rem; }
.chart-title { margin: 0 0 .75rem; font-size: 1rem; font-weight: 600; }
</style>
