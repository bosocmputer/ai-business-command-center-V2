<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue';
import type { DataTableFilterEvent } from 'primevue/datatable';
import { ApiError, adminApi, reportDefinitionByKey, reportDefinitions, type DeliveryPage, type Recipient, type ReportKey } from '@/api';
import TenantFilterSelect from '@/components/admin/TenantFilterSelect.vue';
import SakaiTableHeader from '@/components/table/SakaiTableHeader.vue';
import { useServerTable } from '@/composables/useServerTable';
import { useSakaiFilterMenu } from '@/composables/useSakaiFilterMenu';
import { toDateFilter } from '@/utils/adminTableFilters';
import { formatDateTime } from '@/utils/format';
import { statusLabel } from '@/utils/status';

type Delivery = DeliveryPage['data'][number];
const statuses = ['PENDING', 'SENDING', 'ACCEPTED', 'RETRY_WAIT', 'UNCERTAIN', 'FAILED_PERMANENT'].map((value) => ({ value, label: statusLabel(value) }));
const selected = ref<Delivery>();
const recipientOptions = ref<Recipient[]>([]);
const loadingRecipients = ref(false);
const recipientOptionsError = ref(false);
let recipientController: AbortController | undefined;
let recipientGeneration = 0;
type DeliveryStatus = Delivery['status'];
type DeliveryFilters = { tenantId?: string; recipientId?: string; statuses: DeliveryStatus[]; reportKeys: ReportKey[]; dateFrom?: string; dateTo?: string };
const primeFilters = ref({ tenantName: { value: null as string | null, matchMode: 'equals' }, recipientDisplayName: { value: null as string | null, matchMode: 'equals' }, reportCount: { value: null as ReportKey[] | null, matchMode: 'in' }, status: { value: null as DeliveryStatus[] | null, matchMode: 'in' }, createdAt: { value: null as Date[] | null, matchMode: 'between' } });
useSakaiFilterMenu(primeFilters);
const table = useServerTable<Delivery, DeliveryFilters>({ initialFilters: { statuses: [], reportKeys: [] }, query: (input, signal) => adminApi.queryDeliveries(input, signal) });
const rows = table.rows; const loading = table.loading; const error = table.error;
const hasFilters = computed(() => Boolean(table.appliedGlobalSearch.value || table.appliedFilters.value.tenantId || table.appliedFilters.value.recipientId || table.appliedFilters.value.statuses.length || table.appliedFilters.value.reportKeys.length || table.appliedFilters.value.dateFrom || table.appliedFilters.value.dateTo));
function filterValue<T>(event: DataTableFilterEvent, key: string): T | undefined { return (event.filters[key] as { value?: T } | undefined)?.value; }
async function loadRecipientOptions(tenantId?: string) {
  recipientGeneration += 1;
  const generation = recipientGeneration;
  recipientController?.abort('recipient-options-replaced');
  recipientController = undefined;
  recipientOptions.value = [];
  recipientOptionsError.value = false;
  if (!tenantId) return;
  recipientController = new AbortController();
  loadingRecipients.value = true;
  try {
    const first = await adminApi.queryRecipients(tenantId, { search: '', page: 0, pageSize: 100 }, recipientController.signal);
    const remainingPages = Array.from({ length: Math.max(0, Math.min(first.totalPages, 5) - 1) }, (_, index) => index + 1);
    const remaining = await Promise.all(remainingPages.map((page) => adminApi.queryRecipients(tenantId, { search: '', page, pageSize: 100 }, recipientController!.signal)));
    if (generation === recipientGeneration) recipientOptions.value = [first, ...remaining].flatMap((page) => page.data);
  } catch (cause) {
    if (generation === recipientGeneration && !(cause instanceof ApiError && cause.code === 'CANCELLED')) recipientOptionsError.value = true;
  } finally {
    if (generation === recipientGeneration) loadingRecipients.value = false;
  }
}
function applyPrimeFilters(event: DataTableFilterEvent) {
  const range = filterValue<Date[] | null>(event, 'createdAt') ?? [];
  const tenantId = filterValue<string | null>(event, 'tenantName') || undefined;
  const previousTenantId = table.appliedFilters.value.tenantId;
  const tenantChanged = tenantId !== previousTenantId;
  if (tenantChanged) primeFilters.value.recipientDisplayName.value = null;
  table.draftFilters.value = { tenantId, recipientId: tenantChanged ? undefined : filterValue<string | null>(event, 'recipientDisplayName') || undefined, reportKeys: filterValue<ReportKey[] | null>(event, 'reportCount') ?? [], statuses: filterValue<DeliveryStatus[] | null>(event, 'status') ?? [], dateFrom: toDateFilter(range[0]), dateTo: toDateFilter(range[1]) };
  void table.applyFilters();
  if (tenantChanged) void loadRecipientOptions(tenantId);
}
function clearTableFilters() { Object.values(primeFilters.value).forEach((filter) => { filter.value = null; }); void loadRecipientOptions(); void table.clearFilters(); }
function severity(value: string) { return value === 'ACCEPTED' ? 'success' : value === 'FAILED_PERMANENT' ? 'danger' : value === 'UNCERTAIN' ? 'warn' : value === 'SENDING' ? 'info' : 'secondary'; }
function reportLabel(reportKey: ReportKey) { return reportDefinitionByKey.get(reportKey)?.label ?? reportKey; }
function reportCount(delivery: Delivery) { return delivery.reportCount ?? delivery.reportKeys?.length ?? 0; }
onBeforeUnmount(() => {
  recipientGeneration += 1;
  recipientController?.abort('deliveries-unmounted');
});
</script>

<template>
  <AppPageHeader title="การส่ง LINE" subtitle="ประวัติการส่งเก็บ 365 วัน โดยไม่แสดง LINE user ID หรือเนื้อหาข้อความ" />
  <div class="card table-card">
    <Message v-if="error" severity="error" :closable="false" class="mb-4">โหลดข้อมูลใหม่ไม่สำเร็จ ข้อมูลเดิมยังแสดงอยู่ · {{ error }}</Message>
    <DataTable v-model:filters="primeFilters" :value="rows" :loading="loading" data-key="id" lazy paginator :first="table.page.value * table.pageSize.value" :rows="table.pageSize.value" :total-records="table.total.value" :rows-per-page-options="[25, 50, 100]" filter-display="menu" row-hover show-gridlines scrollable current-page-report-template="หน้า {currentPage} จาก {totalPages} · ทั้งหมด {totalRecords} รายการ" paginator-template="RowsPerPageDropdown FirstPageLink PrevPageLink PageLinks NextPageLink LastPageLink CurrentPageReport" @page="table.changePage" @filter="applyPrimeFilters">
      <template #header><SakaiTableHeader v-model:global-search="table.globalSearch.value" :loading="loading" :has-filters="hasFilters" @clear="clearTableFilters"><template #start><Button label="รีเฟรช" icon="pi pi-refresh" outlined :loading="loading" @click="table.refresh()" /></template></SakaiTableHeader></template>
      <Column field="tenantName" header="ร้านค้า" frozen :show-filter-match-modes="false"><template #body="{ data }"><span class="font-semibold">{{ data.tenantName }}</span></template><template #filter="{ filterModel }"><TenantFilterSelect v-model="filterModel.value" /></template></Column>
      <Column field="recipientDisplayName" header="ผู้รับ" :show-filter-match-modes="false"><template #body="{ data }"><span class="font-medium">{{ data.recipientDisplayName }}</span></template><template #filter="{ filterModel }"><Select v-model="filterModel.value" :options="recipientOptions" option-label="displayName" option-value="id" filter show-clear :loading="loadingRecipients" :disabled="!table.appliedFilters.value.tenantId || recipientOptionsError" :placeholder="recipientOptionsError ? 'โหลดผู้รับไม่สำเร็จ' : table.appliedFilters.value.tenantId ? 'เลือกผู้รับ' : 'เลือกร้านและใช้ตัวกรองก่อน'" /></template></Column>
      <Column field="reportCount" header="รายงานที่ส่งจริง" :show-filter-match-modes="false" header-class="table-number-column" body-class="table-number-column"><template #body="{ data }"><Button :label="`${reportCount(data).toLocaleString('th-TH')} รายงาน`" size="small" text @click="selected = data" /></template><template #filter="{ filterModel }"><MultiSelect v-model="filterModel.value" :options="reportDefinitions" option-label="label" option-value="reportKey" placeholder="ทุกรายงาน" /></template></Column>
      <Column field="status" header="สถานะ" :show-filter-match-modes="false"><template #body="{ data }"><Tag :severity="severity(data.status)" :value="statusLabel(data.status)" /></template><template #filter="{ filterModel }"><MultiSelect v-model="filterModel.value" :options="statuses" option-label="label" option-value="value" placeholder="ทุกสถานะ" /></template></Column>
      <Column field="attempt" header="ครั้งที่ส่ง" header-class="table-number-column" body-class="table-number-column" />
      <Column field="createdAt" header="เริ่มส่งเมื่อ" :show-filter-match-modes="false"><template #body="{ data }">{{ formatDateTime(data.createdAt) }}</template><template #filter="{ filterModel }"><DatePicker v-model="filterModel.value" selection-mode="range" date-format="dd/mm/yy" placeholder="เลือกช่วงวันที่" /></template></Column>
      <Column field="acceptedAt" header="LINE รับเมื่อ"><template #body="{ data }">{{ formatDateTime(data.acceptedAt) }}</template></Column>
      <Column header="" header-class="table-action-column" body-class="table-action-column"><template #body="{ data }"><Button icon="pi pi-info-circle" text rounded class="touch-action" aria-label="ดูรายละเอียดทางเทคนิค" v-tooltip.top="'รายละเอียดทางเทคนิค'" @click="selected = data" /></template></Column>
      <template #empty><div class="py-8 text-center text-muted-color">ไม่พบข้อมูลตามเงื่อนไข <Button v-if="hasFilters" label="ล้างตัวกรอง" text size="small" @click="clearTableFilters" /></div></template>
    </DataTable>
  </div>
  <Dialog :visible="!!selected" modal header="รายละเอียดการส่ง LINE" class="responsive-dialog" :style="{ width: '36rem' }" @update:visible="selected = undefined"><template v-if="selected"><section class="mb-5"><h3 class="text-base mt-0 mb-2">รายงานที่ส่งจริงในรอบนี้</h3><ol class="m-0 pl-5 grid gap-2"><li v-for="reportKey in selected.reportKeys" :key="reportKey">{{ reportLabel(reportKey) }}</li></ol></section><dl class="grid grid-cols-[8rem_1fr] gap-3 m-0"><dt>รหัสการส่ง</dt><dd class="technical-detail m-0">{{ selected.id }}</dd><dt>รหัสร้าน</dt><dd class="technical-detail m-0">{{ selected.tenantId }}</dd><dt>รหัสข้อผิดพลาด</dt><dd class="technical-detail m-0">{{ selected.safeErrorCode || '—' }}</dd><dt>หมดอายุ</dt><dd class="m-0">{{ formatDateTime(selected.expiresAt) }}</dd></dl></template></Dialog>
</template>
