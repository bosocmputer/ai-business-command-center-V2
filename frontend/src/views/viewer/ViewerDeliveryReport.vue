<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import AppPageHeader from '@/components/AppPageHeader.vue';
import ExecutiveChart from '@/components/dashboard/ExecutiveChart.vue';
import { ApiError, viewerApi, type DeliveryContext, type DeliveryContextReport, type ReportKey } from '@/api';
import { useViewerSession } from '@/stores/viewer';
import { comparisonPeriodText, formatDashboardValue, formatPeriodRange } from '@/utils/dashboard';
import { formatDateTime, formatSourceCollection } from '@/utils/format';
import { deliveryContextRoute } from '@/utils/viewerRouting';

const route = useRoute();
const router = useRouter();
const { state } = useViewerSession();
const item = ref<DeliveryContextReport>();
const scheduledFor = ref('');
const loading = ref(true);
const error = ref('');
let controller: AbortController | undefined;
let generation = 0;

const tenantId = computed(() => String(route.params.tenantId));
const deliveryId = computed(() => String(route.params.deliveryId));
const reportKey = computed(() => String(route.params.reportKey) as ReportKey);
const dashboard = computed(() => item.value?.summary?.dashboard);

async function load() {
  const current = ++generation;
  controller?.abort('delivery-report-changed');
  controller = new AbortController();
  loading.value = true;
  error.value = '';
  try {
    const cached = state.deliveryContexts[deliveryId.value] as DeliveryContext | undefined;
    const cachedReport = cached?.tenantId === tenantId.value ? cached.reports.find((entry) => entry.reportKey === reportKey.value) : undefined;
    if (cachedReport) {
      item.value = cachedReport;
      scheduledFor.value = cached!.scheduledFor;
    } else {
      const result = await viewerApi.deliveryReport(tenantId.value, deliveryId.value, reportKey.value, controller.signal);
      if (current !== generation || controller.signal.aborted || result.tenantId !== tenantId.value || result.report.reportKey !== reportKey.value) return;
      item.value = result.report;
      scheduledFor.value = result.scheduledFor;
    }
  } catch (cause) {
    if (!(cause instanceof ApiError && cause.code === 'CANCELLED') && current === generation) error.value = 'ไม่สามารถเปิดรายงานจาก LINE หรือไม่มีสิทธิ์ใช้งาน';
  } finally {
    if (current === generation) loading.value = false;
  }
}

onMounted(load);
onBeforeUnmount(() => { generation++; controller?.abort('delivery-report-unmounted'); });
watch([tenantId, deliveryId, reportKey], load);
</script>

<template>
  <AppPageHeader :title="item?.label ?? 'รายงานจาก LINE'" desktop-mode="viewerCompact">
    <template #back><Button label="รายงานใน LINE รอบนี้" icon="pi pi-arrow-left" text class="delivery-back" @click="router.push(deliveryContextRoute(tenantId, deliveryId))" /></template>
    <template #actions><Tag severity="info" value="ข้อมูลจาก LINE" /></template>
  </AppPageHeader>

  <Message v-if="error" severity="error" :closable="false" class="mb-4">{{ error }}</Message>
  <section v-if="loading" class="card text-center"><ProgressSpinner style="width: 2.5rem; height: 2.5rem" stroke-width="6" /><p class="text-muted-color mb-0">กำลังเปิด Snapshot ที่ใช้ส่ง LINE โดยไม่ดึง SQL ใหม่</p></section>

  <template v-else-if="item && dashboard">
    <section class="card snapshot-context">
      <div><span>ข้อมูล</span><strong>{{ formatPeriodRange(dashboard.period) }}</strong></div>
      <div><span>ส่งตามรอบ</span><strong>{{ formatDateTime(scheduledFor) }} เวลาไทย</strong></div>
      <div><span>แหล่งข้อมูล</span><strong>{{ formatSourceCollection(item.summary?.sourceStartedAt, item.summary?.sourceFinishedAt ?? dashboard.generatedAt, item.summary?.sourceConsistency) }}</strong></div>
    </section>

    <Message v-if="item.snapshotStatus === 'DETAIL_EXPIRED'" severity="info" :closable="false" class="mb-4">ภาพรวมยังพร้อมใช้งาน แต่ข้อมูลแถวรายละเอียดหมดอายุแล้ว ระบบจะไม่ดึงข้อมูลจาก SML ใหม่อัตโนมัติ</Message>
    <div class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4 mb-5">
      <article v-for="metric in dashboard.kpis" :key="metric.key" class="card delivery-kpi">
        <span class="text-sm text-muted-color">{{ metric.label }}</span>
        <strong>{{ formatDashboardValue(metric.value, metric.unit) }}</strong>
        <span v-if="metric.comparison.availability === 'AVAILABLE'" class="text-xs text-muted-color">{{ formatDashboardValue(metric.comparison.delta, metric.unit) }} · {{ comparisonPeriodText(dashboard.comparisonPeriod) }}</span>
      </article>
    </div>
    <div class="grid grid-cols-1 2xl:grid-cols-2 gap-5">
      <article v-for="visualization in dashboard.visualizations" :key="visualization.key" class="card delivery-chart"><h2>{{ visualization.title }}</h2><ExecutiveChart :visualization="visualization" /></article>
    </div>
  </template>
  <Message v-else-if="item" severity="warn" :closable="false">Snapshot สรุปของรายงานนี้หมดอายุแล้ว ระบบเก็บประวัติว่ารายงานนี้เคยถูกส่ง แต่จะไม่ดึง SQL ใหม่โดยอัตโนมัติ</Message>
</template>

<style scoped>
.snapshot-context { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 1rem; margin-bottom: 1rem; }
.snapshot-context > div { display: grid; gap: .25rem; }
.snapshot-context span { color: var(--text-color-secondary); font-size: .75rem; }
.snapshot-context strong { overflow-wrap: anywhere; }
.delivery-kpi { display: grid; gap: .55rem; min-height: 7rem; margin: 0; }
.delivery-kpi > strong { font-size: 1.5rem; font-variant-numeric: tabular-nums; overflow-wrap: anywhere; }
.delivery-chart { margin: 0; }
.delivery-chart h2 { margin: 0 0 .75rem; font-size: 1.05rem; }
@media (max-width: 767px) { .delivery-back { display: none; } .snapshot-context { grid-template-columns: 1fr; padding: 1.25rem; } }
</style>
