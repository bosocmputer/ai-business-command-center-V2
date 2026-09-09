<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import type { DataTableFilterEvent } from 'primevue/datatable';
import { ApiError, adminApi, type AdminReportDefinition, type ReportKey, type ReportRun, type ReportRunDetail } from '@/api';
import TenantFilterSelect from '@/components/admin/TenantFilterSelect.vue';
import SakaiTableHeader from '@/components/table/SakaiTableHeader.vue';
import { useServerTable } from '@/composables/useServerTable';
import { useSakaiFilterMenu } from '@/composables/useSakaiFilterMenu';
import { loadAdminReportCatalog } from '@/stores/reportCatalog';
import { errorMessage, formatDateTime, formatDateTimeWithMilliseconds } from '@/utils/format';
import { statusLabel } from '@/utils/status';
import { evidenceLevelLabel, formatDurationMs, lineImpactLabel, reportImpactLabel, transportPhaseLabel, triggerKindLabel } from '@/utils/operationalPresentation';
import { toDateFilter } from '@/utils/adminTableFilters';
const statuses = ['QUEUED', 'CLAIMED', 'RUNNING', 'SUCCEEDED', 'FAILED', 'CANCELLED', 'EXPIRED'].map((value) => ({ value, label: statusLabel(value) }));
const sources = [
  { value: 'SCHEDULE', label: 'ตารางส่ง LINE' },
  { value: 'DASHBOARD', label: 'ผู้ใช้ Dashboard' },
  { value: 'BACKGROUND', label: 'อัปเดตเบื้องหลัง' }
];
const selected = ref<ReportRun>();
const detail = ref<ReportRunDetail>();
const detailVisible = ref(false);
const detailLoading = ref(false);
const detailError = ref('');
const reportDefinitions = ref<AdminReportDefinition[]>([]);
const reportDefinitionByKey = computed(() => new Map(reportDefinitions.value.map((item) => [item.reportKey, item])));
let detailController: AbortController | undefined;
let detailGeneration = 0;

type RunFilters = { tenantId?: string; statuses: ReportRun['status'][]; reportKeys: ReportKey[]; sources: Array<'DASHBOARD' | 'SCHEDULE' | 'BACKGROUND'>; dateFrom?: string; dateTo?: string };
const primeFilters = ref({
  tenantName: { value: null as string | null, matchMode: 'equals' },
  reportKey: { value: null as ReportKey[] | null, matchMode: 'in' },
  status: { value: null as ReportRun['status'][] | null, matchMode: 'in' },
  source: { value: null as RunFilters['sources'] | null, matchMode: 'in' },
  queuedAt: { value: null as Date[] | null, matchMode: 'between' }
});
useSakaiFilterMenu(primeFilters);
const table = useServerTable<ReportRun, RunFilters>({
  initialFilters: { statuses: [], reportKeys: [], sources: [] },
  query: (input, signal) => adminApi.queryReportRuns(input, signal)
});
const rows = table.rows;
const loading = table.loading;
const error = table.error;
const hasFilters = computed(() => Boolean(table.appliedGlobalSearch.value || table.appliedFilters.value.tenantId || table.appliedFilters.value.statuses.length || table.appliedFilters.value.reportKeys.length || table.appliedFilters.value.sources.length || table.appliedFilters.value.dateFrom || table.appliedFilters.value.dateTo));

function filterValue<T>(event: DataTableFilterEvent, key: string): T | undefined { return (event.filters[key] as { value?: T } | undefined)?.value; }
function applyPrimeFilters(event: DataTableFilterEvent) {
  const range = filterValue<Date[] | null>(event, 'queuedAt') ?? [];
  table.draftFilters.value = {
    tenantId: filterValue<string | null>(event, 'tenantName') || undefined,
    reportKeys: filterValue<ReportKey[] | null>(event, 'reportKey') ?? [],
    statuses: filterValue<ReportRun['status'][] | null>(event, 'status') ?? [],
    sources: filterValue<RunFilters['sources'] | null>(event, 'source') ?? [],
    dateFrom: toDateFilter(range[0]), dateTo: toDateFilter(range[1])
  };
  void table.applyFilters();
}
function clearTableFilters() {
  Object.values(primeFilters.value).forEach((filter) => { filter.value = null; });
  void table.clearFilters();
}
function severity(value: string) { return value === 'SUCCEEDED' ? 'success' : value === 'FAILED' ? 'danger' : value === 'RUNNING' || value === 'CLAIMED' ? 'info' : value === 'QUEUED' ? 'warn' : 'secondary'; }
function runStatusLabel(run: ReportRun) { return run.runtimeStatus === 'STALLED' ? 'งานหยุดค้าง' : statusLabel(run.status); }
function runSeverity(run: ReportRun) { return run.runtimeStatus === 'STALLED' ? 'danger' : severity(run.status); }
function waitReasonLabel(value?: string) {
  return ({
    TENANT_BUSY: 'รอรายงานอื่นของร้าน', HOST_BUSY: 'รอ Server ปลายทาง',
    TENANT_COOLDOWN: 'พักการเชื่อมต่อของร้านชั่วคราว', HOST_COOLDOWN: 'Server ปลายทางอยู่ช่วงพัก',
    SCHEDULE_RESERVED: 'รอรอบส่ง LINE ก่อน'
  } as Record<string, string>)[value ?? ''] ?? '';
}
function failureTitle(run: ReportRun) { return run.failureSummary?.presentation.titleTh || (run.status === 'FAILED' ? 'ไม่พบรายละเอียดสาเหตุจากระบบรุ่นเดิม' : '—'); }
async function openDetail(run: ReportRun) {
  selected.value = run;
  detail.value = undefined;
  detailError.value = '';
  detailVisible.value = true;
  detailLoading.value = true;
  detailGeneration++;
  const requestGeneration = detailGeneration;
  detailController?.abort('superseded');
  detailController = new AbortController();
  try {
    const result = await adminApi.reportRun(run.id, detailController.signal);
    if (requestGeneration === detailGeneration) detail.value = result;
  } catch (cause) {
    if (requestGeneration === detailGeneration && !(cause instanceof ApiError && cause.code === 'CANCELLED')) detailError.value = errorMessage(cause);
  } finally {
    if (requestGeneration === detailGeneration) detailLoading.value = false;
  }
}
function closeDetail() {
  detailVisible.value = false;
  detailGeneration++;
  detailController?.abort('dialog-closed');
  detailController = undefined;
  detail.value = undefined;
  selected.value = undefined;
}
onMounted(() => {
  void loadAdminReportCatalog().then((catalog) => { reportDefinitions.value = catalog.data; }).catch(() => undefined);
});
onBeforeUnmount(() => { detailController?.abort('unmounted'); });
</script>

<template>
  <AppPageHeader title="การสร้างรายงาน" subtitle="ตรวจคิวและผลการทำงาน · เวลาไทย" />
  <div class="card table-card">
    <Message v-if="error" severity="error" :closable="false" class="mb-4">โหลดข้อมูลใหม่ไม่สำเร็จ ข้อมูลเดิมยังแสดงอยู่ · {{ error }}</Message>
    <DataTable v-model:filters="primeFilters" :value="rows" :loading="loading" data-key="id" lazy paginator :first="table.page.value * table.pageSize.value" :rows="table.pageSize.value" :total-records="table.total.value" :rows-per-page-options="[25, 50, 100]" filter-display="menu" row-hover show-gridlines scrollable current-page-report-template="หน้า {currentPage} จาก {totalPages} · ทั้งหมด {totalRecords} รายการ" paginator-template="RowsPerPageDropdown FirstPageLink PrevPageLink PageLinks NextPageLink LastPageLink CurrentPageReport" @page="table.changePage" @filter="applyPrimeFilters">
      <template #header><SakaiTableHeader v-model:global-search="table.globalSearch.value" :loading="loading" :has-filters="hasFilters" @clear="clearTableFilters"><template #start><Button label="รีเฟรช" icon="pi pi-refresh" outlined :loading="loading" @click="table.refresh()" /></template></SakaiTableHeader></template>
      <Column field="tenantName" header="ร้านค้า" frozen :show-filter-match-modes="false"><template #body="{ data }"><span class="font-semibold">{{ data.tenantName || '—' }}</span></template><template #filter="{ filterModel }"><TenantFilterSelect v-model="filterModel.value" /></template></Column>
      <Column field="reportKey" header="รายงาน" :show-filter-match-modes="false"><template #body="{ data }"><div class="font-medium">{{ reportDefinitionByKey.get(data.reportKey)?.label ?? data.reportKey }}</div></template><template #filter="{ filterModel }"><MultiSelect v-model="filterModel.value" :options="reportDefinitions" option-label="label" option-value="reportKey" placeholder="ทุกรายงาน" /></template></Column>
      <Column field="status" header="สถานะ" :show-filter-match-modes="false"><template #body="{ data }"><div class="grid gap-1"><Tag class="w-fit" :severity="runSeverity(data)" :value="runStatusLabel(data)" /><small v-if="data.waitReason" class="text-muted-color">{{ waitReasonLabel(data.waitReason) }}</small><small v-if="data.retryAvailableAt" class="text-muted-color">ลองใหม่ได้หลัง {{ formatDateTime(data.retryAvailableAt) }}</small></div></template><template #filter="{ filterModel }"><MultiSelect v-model="filterModel.value" :options="statuses" option-label="label" option-value="value" placeholder="ทุกสถานะ" /></template></Column>
      <Column field="source" header="รายงานถูกสร้างจาก" :show-filter-match-modes="false"><template #body="{ data }">{{ sources.find((item) => item.value === data.source)?.label ?? data.source ?? '—' }}</template><template #filter="{ filterModel }"><MultiSelect v-model="filterModel.value" :options="sources" option-label="label" option-value="value" placeholder="ทุกแหล่งที่สร้างรายงาน" /></template></Column>
      <Column header="สาเหตุ"><template #body="{ data }"><div class="max-w-80"><span :class="data.status === 'FAILED' ? 'font-semibold text-red-600 dark:text-red-400' : 'text-muted-color'">{{ failureTitle(data) }}</span><small v-if="data.failureSummary?.level === 'LEGACY_PARTIAL'" class="block text-muted-color mt-1">หลักฐานจากระบบรุ่นเดิมมีรายละเอียดจำกัด</small></div></template></Column>
      <Column header="ช่วงข้อมูล"><template #body="{ data }">{{ data.dateFrom || '—' }}<span v-if="data.dateTo && data.dateTo !== data.dateFrom"> → {{ data.dateTo }}</span></template></Column>
      <Column field="rowCount" header="จำนวนแถว" header-class="table-number-column" body-class="table-number-column"><template #body="{ data }"><span class="metric-value">{{ data.rowCount.toLocaleString('th-TH') }}</span></template></Column>
      <Column field="queuedAt" header="เข้าคิวเมื่อ" :show-filter-match-modes="false"><template #body="{ data }"><span class="whitespace-nowrap">{{ formatDateTimeWithMilliseconds(data.queuedAt) }}</span></template><template #filter="{ filterModel }"><DatePicker v-model="filterModel.value" selection-mode="range" date-format="dd/mm/yy" placeholder="เลือกช่วงวันที่" show-icon /></template></Column>
      <Column field="finishedAt" header="เสร็จเมื่อ"><template #body="{ data }"><span class="whitespace-nowrap">{{ formatDateTimeWithMilliseconds(data.finishedAt) }}</span></template></Column>
      <Column header="" header-class="table-action-column" body-class="table-action-column"><template #body="{ data }"><Button icon="pi pi-info-circle" text rounded class="touch-action" aria-label="ดูสาเหตุและหลักฐาน" v-tooltip.top="'ดูสาเหตุและหลักฐาน'" @click="openDetail(data)" /></template></Column>
      <template #empty><div class="py-8 text-center text-muted-color">ไม่พบข้อมูลตามเงื่อนไข <Button v-if="hasFilters" label="ล้างตัวกรอง" text size="small" @click="clearTableFilters" /></div></template>
    </DataTable>
  </div>
  <Dialog :visible="detailVisible" modal header="สาเหตุและหลักฐานการสร้างรายงาน" class="responsive-dialog failure-dialog" :style="{ width: '46rem' }" @update:visible="(value) => { if (!value) closeDetail(); }">
    <div v-if="detailLoading" class="grid gap-3"><Skeleton height="5rem" /><Skeleton height="10rem" /></div>
    <Message v-else-if="detailError" severity="error" :closable="false">{{ detailError }}</Message>
    <template v-else-if="detail">
      <Message v-if="detail.failureSummary" severity="error" :closable="false" class="mb-5">
        <div class="font-bold text-lg">{{ detail.failureSummary.presentation.titleTh }}</div>
        <div class="mt-1">{{ detail.failureSummary.presentation.summaryTh }}</div>
      </Message>
      <Message v-if="detail.connectionChangedSinceFailure" severity="warn" :closable="false" class="mb-5">การตั้งค่าการเชื่อมต่อ SML ถูกแก้ไขหลังเกิดเหตุ หลักฐานนี้อ้างอิงค่าที่ระบบใช้ในเวลานั้น</Message>

      <section class="failure-section">
        <h3>เกิดอะไรขึ้นและล้มที่ขั้นตอนไหน</h3>
        <dl class="failure-facts">
          <dt>รายงาน</dt><dd>{{ reportDefinitionByKey.get(detail.reportKey)?.label ?? detail.reportKey }}</dd>
          <dt>รายงานถูกสร้างจาก</dt><dd>{{ triggerKindLabel(detail.triggerKind) }}</dd>
          <dt>ขั้นตอนที่ล้ม</dt><dd>{{ detail.failureSummary?.presentation.stageTh || 'ระบบรุ่นเดิมไม่ได้บันทึกขั้นตอน' }}</dd>
          <dt>ระดับหลักฐาน</dt><dd>{{ evidenceLevelLabel(detail.failureSummary?.level) }}</dd>
          <dt>การรับส่งข้อมูล</dt><dd>{{ transportPhaseLabel(detail.failureSummary?.transportPhase) }}</dd>
        </dl>
      </section>

      <section class="failure-section">
        <h3>เวลาและระยะเวลา</h3>
        <dl class="failure-facts">
          <dt>เกิดเมื่อ</dt><dd>{{ formatDateTime(detail.failureSummary?.occurredAt || detail.finishedAt || detail.queuedAt) }} เวลาไทย</dd>
          <dt>ใช้เวลา</dt><dd>{{ formatDurationMs(detail.failureSummary?.durationMs) }}</dd>
          <dt>จำนวนครั้งที่ลอง</dt><dd>{{ (detail.failureSummary?.attempt ?? detail.progress?.attempt ?? 0).toLocaleString('th-TH') }}</dd>
        </dl>
      </section>

      <section class="failure-section">
        <h3>ผลกระทบต่อรายงานและ LINE</h3>
        <p>{{ reportImpactLabel(detail.impact) }}</p>
        <p class="font-semibold mb-0">{{ lineImpactLabel(detail.impact.notificationOutcome) }}</p>
      </section>

      <section v-if="detail.failureSummary" class="failure-section">
        <h3>หลักฐานจากระบบและสิ่งที่ควรตรวจสอบ</h3>
        <p v-if="detail.failureSummary.presentation.evidenceNoteTh" class="text-muted-color">{{ detail.failureSummary.presentation.evidenceNoteTh }}</p>
        <ul class="pl-5 mb-4"><li v-for="action in detail.failureSummary.presentation.nextActionsTh" :key="action" class="mb-2">{{ action }}</li></ul>
        <Button as="router-link" :to="`/admin/tenants/${detail.tenantId}?tab=sml`" label="เปิดการเชื่อมต่อ SML ของร้าน" icon="pi pi-external-link" outlined />
      </section>

      <Accordion>
        <AccordionPanel value="technical">
          <AccordionHeader>ข้อมูลสำหรับทีมเทคนิค</AccordionHeader>
          <AccordionContent>
            <dl class="failure-facts technical-detail">
              <dt>รหัสงาน</dt><dd>{{ detail.id }}</dd>
              <dt>รหัสข้อผิดพลาด</dt><dd>{{ detail.failureSummary?.safeErrorCode || detail.safeErrorCode || 'UNKNOWN' }}</dd>
              <dt>Stage</dt><dd>{{ detail.failureSummary?.stage || 'UNKNOWN' }}</dd>
              <dt>Transport phase</dt><dd>{{ detail.failureSummary?.transportPhase || 'UNKNOWN' }}</dd>
              <dt>Remote state</dt><dd>{{ detail.failureSummary?.remoteStateUnknown ? 'UNKNOWN' : 'CONFIRMED TERMINAL' }}</dd>
            </dl>
          </AccordionContent>
        </AccordionPanel>
      </Accordion>
    </template>
  </Dialog>
</template>

<style scoped>
.failure-section { padding-block: .25rem 1rem; border-bottom: 1px solid var(--surface-border); margin-bottom: 1rem; }
.failure-section h3 { font-size: 1rem; margin: 0 0 .75rem; color: var(--text-color); }
.failure-facts { display: grid; grid-template-columns: minmax(9rem, auto) 1fr; gap: .65rem 1.25rem; margin: 0; }
.failure-facts dt { color: var(--text-color-secondary); }
.failure-facts dd { margin: 0; overflow-wrap: anywhere; }
@media (max-width: 640px) { .failure-facts { grid-template-columns: 1fr; gap: .2rem; } .failure-facts dd { margin-bottom: .65rem; } }
</style>
