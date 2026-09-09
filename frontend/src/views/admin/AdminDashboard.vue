<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { apiRequest } from '@/api/client';
import { adminApi, type AdminReportDefinition, type LineQuotaStatus } from '@/api';
import { loadAdminReportCatalog } from '@/stores/reportCatalog';
import SakaiTableHeader from '@/components/table/SakaiTableHeader.vue';
import { useSakaiFilterMenu } from '@/composables/useSakaiFilterMenu';

const ready = ref<'checking' | 'ready' | 'unavailable'>('checking');
const lineQuota = ref<LineQuotaStatus>();
const reportDefinitions = ref<AdminReportDefinition[]>([]);
const reportFilters = ref({
  global: { value: null as string | null, matchMode: 'contains' },
  categoryLabel: { value: null as string[] | null, matchMode: 'in' }
});
useSakaiFilterMenu(reportFilters);
onMounted(async () => {
  const [healthResult, quotaResult, reportResult] = await Promise.allSettled([
    apiRequest('/api/v1/health/ready', { timeoutMs: 3000 }), adminApi.lineQuota(), loadAdminReportCatalog()
  ]);
  ready.value = healthResult.status === 'fulfilled' ? 'ready' : 'unavailable';
  if (quotaResult.status === 'fulfilled') lineQuota.value = quotaResult.value;
  if (reportResult.status === 'fulfilled') reportDefinitions.value = reportResult.value.data;
});

const quotaPercent = computed(() => {
  const status = lineQuota.value;
  if (!status?.providerLimit || status.providerConsumed === null) return 0;
  return Math.min(100, Math.round(status.providerConsumed / status.providerLimit * 100));
});
const quotaRemaining = computed(() => {
  const status = lineQuota.value;
  if (status?.providerLimit === null || status?.providerConsumed === null || status?.providerLimit === undefined) return undefined;
  return Math.max(0, status.providerLimit - status.providerConsumed);
});
const quotaSeverity = computed(() => {
  const status = lineQuota.value;
  if (!status || status.state === 'UNSYNCED' || status.state === 'STALE') return 'warn';
  if (status.providerLimit && status.providerConsumed !== null) {
    const usable = status.providerLimit * (100 - status.operationalReservePercent) / 100;
    if (status.providerConsumed >= usable) return 'danger';
  }
  return 'success';
});
const reportSearch = computed({ get: () => reportFilters.value.global.value ?? '', set: (value: string) => { reportFilters.value.global.value = value || null; } });
const reportCategories = computed(() => [...new Set(reportDefinitions.value.map((item) => item.categoryLabel))].map((value) => ({ label: value, value })));
const hasReportFilters = computed(() => Boolean(reportSearch.value || reportFilters.value.categoryLabel.value?.length));
function clearReportFilters() {
  reportFilters.value = { global: { value: null, matchMode: 'contains' }, categoryLabel: { value: null, matchMode: 'in' } };
}

const shortcuts = [
  { label: 'จัดการร้านค้า', detail: 'SML ผู้รับ และตารางส่งรายงาน', icon: 'pi-building', to: '/admin/tenants' },
  { label: 'ตรวจการสร้างรายงาน', detail: 'ดูคิว งานสำเร็จ และข้อผิดพลาด', icon: 'pi-database', to: '/admin/report-runs' },
  { label: 'ตรวจการส่ง LINE', detail: 'ดูสถานะส่งซ้ำและผลการส่ง', icon: 'pi-send', to: '/admin/deliveries' }
];
</script>

<template>
  <AppPageHeader title="ภาพรวมระบบ" subtitle="ศูนย์ควบคุมรายงาน SML และการส่ง LINE"><template #actions><Tag :severity="ready === 'ready' ? 'success' : ready === 'checking' ? 'secondary' : 'danger'" :value="ready === 'ready' ? 'ระบบพร้อมใช้งาน' : ready === 'checking' ? 'กำลังตรวจสอบ' : 'ระบบไม่พร้อม'" /></template></AppPageHeader>
  <section class="card" aria-labelledby="line-quota-title">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div><h2 id="line-quota-title" class="text-lg font-semibold m-0">LINE OA กลาง</h2><p class="m-0 mt-1 text-sm text-muted-color">ยอดประมาณจาก LINE รวม Nexflow และ OA Manager · Nextstep accepted {{ (lineQuota?.locallyAccepted ?? 0).toLocaleString('th-TH') }} ข้อความ</p></div>
      <Tag :severity="quotaSeverity" :value="lineQuota?.state ?? 'กำลังโหลด'" />
    </div>
    <template v-if="lineQuota?.state === 'READY' && lineQuota.providerLimit !== null && lineQuota.providerConsumed !== null">
      <div class="flex flex-wrap justify-between gap-2 mt-4 mb-2 text-sm"><span>ใช้แล้ว {{ lineQuota.providerConsumed.toLocaleString('th-TH') }} / {{ lineQuota.providerLimit.toLocaleString('th-TH') }}</span><span>เหลือประมาณ {{ quotaRemaining?.toLocaleString('th-TH') }} · สำรอง {{ lineQuota.operationalReservePercent }}%</span></div>
      <ProgressBar :value="quotaPercent" :show-value="false" style="height: .6rem" />
    </template>
    <Message v-else-if="lineQuota?.state === 'STALE' || lineQuota?.state === 'UNSYNCED' || !lineQuota" severity="warn" :closable="false" class="mt-4">ข้อมูล quota ยังไม่สด ระบบส่งยังใช้ retry และจะปฏิบัติต่อ LINE 429 เป็นเหตุขัดข้องชั่วคราว</Message>
    <Message v-else-if="lineQuota.state === 'UNLIMITED'" severity="success" :closable="false" class="mt-4">OA นี้ไม่มี target limit ที่ LINE รายงาน</Message>
  </section>
  <div class="grid grid-cols-1 md:grid-cols-3 gap-4 mb-6">
    <RouterLink v-for="item in shortcuts" :key="item.to" :to="item.to" class="card shortcut-card no-underline text-color">
      <div class="flex items-start justify-between gap-4"><div><h2 class="text-lg font-semibold mt-0 mb-2">{{ item.label }}</h2><p class="m-0 text-muted-color">{{ item.detail }}</p></div><span class="grid place-items-center w-11 h-11 rounded-md bg-primary-50 dark:bg-primary-950 text-primary"><i :class="['pi', item.icon]" /></span></div>
    </RouterLink>
  </div>
  <div class="card table-card">
    <div class="mb-4"><h2 class="text-xl font-semibold m-0">รายงานที่รองรับ</h2><p class="text-muted-color mt-1 mb-0">ใช้เฉพาะคำสั่ง SQL ที่ระบบตรวจสอบไว้ล่วงหน้า</p></div>
    <DataTable v-model:filters="reportFilters" :value="reportDefinitions" data-key="reportKey" :global-filter-fields="['label', 'categoryLabel']" filter-display="menu" row-hover show-gridlines paginator :rows="25" :rows-per-page-options="[25, 50, 100]" paginator-template="RowsPerPageDropdown FirstPageLink PrevPageLink PageLinks NextPageLink LastPageLink CurrentPageReport" current-page-report-template="หน้า {currentPage} จาก {totalPages} · ทั้งหมด {totalRecords} รายการ" striped-rows responsive-layout="scroll">
      <template #header><SakaiTableHeader v-model:global-search="reportSearch" :has-filters="hasReportFilters" @clear="clearReportFilters"><template #start><Badge :value="reportDefinitions.length" /></template></SakaiTableHeader></template>
      <Column field="label" header="ชื่อรายงาน"><template #body="{ data }"><span class="font-medium">{{ data.label }}</span></template></Column>
      <Column field="categoryLabel" header="หมวด" :show-filter-match-modes="false"><template #body="{ data }"><Tag severity="secondary" :value="data.categoryLabel" /></template><template #filter="{ filterModel }"><MultiSelect v-model="filterModel.value" :options="reportCategories" option-label="label" option-value="value" placeholder="ทุกหมวด" /></template></Column>
      <template #empty><div class="py-8 text-center text-muted-color">ไม่พบรายงานที่ตรงกับตัวกรอง <Button v-if="hasReportFilters" label="ล้างตัวกรอง" text size="small" @click="clearReportFilters" /></div></template>
    </DataTable>
  </div>
</template>

<style scoped>
.shortcut-card { margin-bottom: 0; transition: transform .2s; }
.shortcut-card:hover { transform: translateY(-2px); }
</style>
