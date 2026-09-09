<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import AppPageHeader from '@/components/AppPageHeader.vue';
import { ApiError, viewerApi, type DeliveryContext } from '@/api';
import { useViewerSession } from '@/stores/viewer';
import { formatDashboardValue, formatPeriodRange, reportIcon } from '@/utils/dashboard';
import { formatDateTime, formatSourceCollection } from '@/utils/format';
import { deliveryContextRoute } from '@/utils/viewerRouting';

const route = useRoute();
const router = useRouter();
const { state, setDeliveryContext } = useViewerSession();
const context = ref<DeliveryContext>();
const loading = ref(true);
const error = ref('');
let controller: AbortController | undefined;
let generation = 0;

const tenantId = computed(() => String(route.params.tenantId));
const deliveryId = computed(() => String(route.params.deliveryId));
const tenantName = computed(() => state.tenants.find((item) => item.id === tenantId.value)?.name ?? 'ร้านค้า');

async function load() {
  const current = ++generation;
  controller?.abort('delivery-context-changed');
  controller = new AbortController();
  loading.value = true;
  error.value = '';
  try {
    const cached = state.deliveryContexts[deliveryId.value] as DeliveryContext | undefined;
    const result = cached?.tenantId === tenantId.value
      ? cached
      : await viewerApi.deliveryContext(tenantId.value, deliveryId.value, controller.signal);
    if (current !== generation || controller.signal.aborted || result.tenantId !== tenantId.value) return;
    context.value = result;
    setDeliveryContext(result);
  } catch (cause) {
    if (!(cause instanceof ApiError && cause.code === 'CANCELLED') && current === generation) {
      error.value = 'ไม่สามารถเปิดรายงานจาก LINE หรือไม่มีสิทธิ์ใช้งาน';
    }
  } finally {
    if (current === generation) loading.value = false;
  }
}

function openReport(reportKey: DeliveryContext['reports'][number]['reportKey']) {
  if (!context.value) return;
  void router.push(deliveryContextRoute(context.value.tenantId, context.value.deliveryId, reportKey));
}

function reportSource(item: DeliveryContext['reports'][number]) {
  return item.summary
    ? formatSourceCollection(item.summary.sourceStartedAt, item.summary.sourceFinishedAt ?? item.summary.dashboard.generatedAt, item.summary.sourceConsistency)
    : 'Snapshot สรุปไม่พร้อมใช้งานแล้ว';
}

onMounted(load);
onBeforeUnmount(() => { generation++; controller?.abort('delivery-unmounted'); });
watch([tenantId, deliveryId], load);
</script>

<template>
  <AppPageHeader title="รายงานจาก LINE" :subtitle="tenantName" desktop-mode="viewerCompact" />

  <Message v-if="error" severity="error" :closable="false" class="mb-4">{{ error }} <Button label="กลับภาพรวมร้าน" text size="small" @click="router.push(`/app/tenant/${tenantId}`)" /></Message>
  <div v-if="loading" class="grid gap-4"><Skeleton v-for="index in 2" :key="index" height="9rem" /></div>

  <template v-else-if="context">
    <section class="card delivery-summary">
      <div>
        <span class="text-sm text-muted-color">ส่งตามรอบเมื่อ</span>
        <strong>{{ formatDateTime(context.scheduledFor) }} เวลาไทย</strong>
      </div>
      <div class="delivery-actions"><Tag severity="info" :value="`${context.reports.length.toLocaleString('th-TH')} รายงานใน LINE รอบนี้`" /><Button label="ไป Dashboard ร้าน" icon="pi pi-arrow-right" icon-pos="right" outlined size="small" @click="router.push(`/app/tenant/${context.tenantId}`)" /></div>
    </section>

    <Message v-if="context.orderStatus === 'LEGACY'" severity="secondary" :closable="false" class="mb-4">รายงานรอบเก่าแสดงตามลำดับที่ระบบกู้คืนได้ และอาจไม่ใช่ลำดับเดิมในตารางส่งรายงาน</Message>
    <Message v-if="context.dataStatus !== 'AVAILABLE'" severity="warn" :closable="false" class="mb-4">Snapshot บางรายงานหมดอายุแล้ว ระบบจะแสดงเฉพาะข้อมูลที่ยังพร้อมใช้งานและจะไม่ดึงข้อมูลจาก SML ใหม่อัตโนมัติ</Message>

    <div class="grid grid-cols-1 xl:grid-cols-2 gap-4">
      <button v-for="item in context.reports" :key="item.reportRunId" type="button" class="card delivery-report-card" @click="openReport(item.reportKey)">
        <span class="report-icon"><i :class="reportIcon(item.reportKey)" /></span>
        <span class="report-copy">
          <strong class="report-title">{{ item.label }}</strong>
          <span v-if="item.summary" class="text-sm text-muted-color">ข้อมูล {{ formatPeriodRange(item.summary.dashboard.period) }}</span>
          <span class="text-xs text-muted-color">{{ reportSource(item) }}</span>
          <span v-if="item.summary" class="metric-list">
            <span v-for="metric in item.summary.dashboard.kpis.slice(0, 3)" :key="metric.key"><span>{{ metric.label }}</span><strong>{{ formatDashboardValue(metric.value, metric.unit) }}</strong></span>
          </span>
          <Tag v-else severity="warn" value="Snapshot สรุปหมดอายุ" class="justify-self-start" />
        </span>
        <i class="pi pi-chevron-right text-muted-color" aria-hidden="true" />
      </button>
    </div>
  </template>
</template>

<style scoped>
.delivery-summary { display: flex; align-items: center; justify-content: space-between; gap: 1rem; margin-bottom: 1rem; }
.delivery-summary > div { display: grid; gap: .25rem; }
.delivery-actions { display: flex !important; align-items: center; gap: .75rem !important; }
.delivery-report-card { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: start; gap: 1rem; margin: 0; border: 0; color: inherit; text-align: left; cursor: pointer; }
.report-icon { display: grid; place-items: center; width: 2.75rem; height: 2.75rem; border-radius: var(--content-border-radius); color: var(--primary-color); background: var(--p-primary-50); }
.report-copy { display: grid; gap: .35rem; min-width: 0; }
.report-title { font-size: 1.05rem; overflow-wrap: anywhere; }
.metric-list { display: grid; gap: .3rem; margin-top: .4rem; }
.metric-list > span { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: .75rem; color: var(--text-color-secondary); font-size: .8rem; }
.metric-list strong { color: var(--text-color); font-variant-numeric: tabular-nums; white-space: nowrap; }
@media (max-width: 575px) { .delivery-summary { align-items: flex-start; flex-direction: column; } .delivery-actions { width: 100%; justify-content: space-between; flex-wrap: wrap; } .delivery-report-card { padding: 1.25rem; } }
</style>
