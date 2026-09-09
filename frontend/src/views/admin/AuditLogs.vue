<script setup lang="ts">
import { computed, ref } from 'vue';
import type { DataTableFilterEvent } from 'primevue/datatable';
import { adminApi, type AuditPage } from '@/api';
import TenantFilterSelect from '@/components/admin/TenantFilterSelect.vue';
import SakaiTableHeader from '@/components/table/SakaiTableHeader.vue';
import { useServerTable } from '@/composables/useServerTable';
import { useSakaiFilterMenu } from '@/composables/useSakaiFilterMenu';
import { normalizedAuditAction, toDateFilter } from '@/utils/adminTableFilters';
import { formatDateTime } from '@/utils/format';
import { auditActionLabel, statusLabel } from '@/utils/status';

type Audit = AuditPage['data'][number];
const actorTypes = ['ADMIN', 'VIEWER', 'WORKER', 'SYSTEM'].map((value) => ({ value, label: statusLabel(value) }));
const results = ['SUCCESS', 'DENIED', 'FAILED'].map((value) => ({ value, label: statusLabel(value) }));
const selected = ref<Audit>();
type AuditFilters = { tenantId?: string; actorTypes: Array<'ADMIN' | 'VIEWER' | 'WORKER' | 'SYSTEM'>; actions: string[]; results: Array<'SUCCESS' | 'DENIED' | 'FAILED'>; dateFrom?: string; dateTo?: string };
const primeFilters = ref({ tenantName: { value: null as string | null, matchMode: 'equals' }, createdAt: { value: null as Date[] | null, matchMode: 'between' }, actorType: { value: null as string[] | null, matchMode: 'in' }, action: { value: null as string | null, matchMode: 'equals' }, result: { value: null as string[] | null, matchMode: 'in' } });
useSakaiFilterMenu(primeFilters);
const table = useServerTable<Audit, AuditFilters>({ initialFilters: { actorTypes: [], actions: [], results: [] }, query: (input, signal) => adminApi.queryAudit(input, signal) });
const rows = table.rows; const loading = table.loading; const error = table.error;
const hasFilters = computed(() => Boolean(table.appliedGlobalSearch.value || table.appliedFilters.value.tenantId || table.appliedFilters.value.actorTypes.length || table.appliedFilters.value.actions.length || table.appliedFilters.value.results.length || table.appliedFilters.value.dateFrom || table.appliedFilters.value.dateTo));
function filterValue<T>(event: DataTableFilterEvent, key: string): T | undefined { return (event.filters[key] as { value?: T } | undefined)?.value; }
function applyPrimeFilters(event: DataTableFilterEvent) { const range = filterValue<Date[] | null>(event, 'createdAt') ?? []; const action = normalizedAuditAction(filterValue<string | null>(event, 'action') ?? ''); table.draftFilters.value = { tenantId: filterValue<string | null>(event, 'tenantName') || undefined, actorTypes: (filterValue<string[] | null>(event, 'actorType') ?? []) as AuditFilters['actorTypes'], actions: action ? [action] : [], results: (filterValue<string[] | null>(event, 'result') ?? []) as AuditFilters['results'], dateFrom: toDateFilter(range[0]), dateTo: toDateFilter(range[1]) }; void table.applyFilters(); }
function clearTableFilters() { Object.values(primeFilters.value).forEach((filter) => { filter.value = null; }); void table.clearFilters(); }
</script>

<template>
  <AppPageHeader title="ประวัติการใช้งาน" subtitle="ใครทำอะไร เมื่อใด และได้ผลอย่างไร · เก็บ 365 วัน" />
  <div class="card table-card">
    <Message v-if="error" severity="error" :closable="false" class="mb-4">โหลดข้อมูลใหม่ไม่สำเร็จ ข้อมูลเดิมยังแสดงอยู่ · {{ error }}</Message>
    <DataTable v-model:filters="primeFilters" :value="rows" :loading="loading" data-key="id" lazy paginator :first="table.page.value * table.pageSize.value" :rows="table.pageSize.value" :total-records="table.total.value" :rows-per-page-options="[25, 50, 100]" filter-display="menu" row-hover show-gridlines scrollable current-page-report-template="หน้า {currentPage} จาก {totalPages} · ทั้งหมด {totalRecords} รายการ" paginator-template="RowsPerPageDropdown FirstPageLink PrevPageLink PageLinks NextPageLink LastPageLink CurrentPageReport" @page="table.changePage" @filter="applyPrimeFilters">
      <template #header><SakaiTableHeader v-model:global-search="table.globalSearch.value" :loading="loading" :has-filters="hasFilters" @clear="clearTableFilters"><template #start><Button label="รีเฟรช" icon="pi pi-refresh" outlined :loading="loading" @click="table.refresh()" /></template></SakaiTableHeader></template>
      <Column field="tenantName" header="ร้านค้า" frozen :show-filter-match-modes="false"><template #body="{ data }"><span class="font-semibold">{{ data.tenantName || 'ระบบส่วนกลาง' }}</span></template><template #filter="{ filterModel }"><TenantFilterSelect v-model="filterModel.value" /></template></Column>
      <Column field="createdAt" header="เวลา" :show-filter-match-modes="false"><template #body="{ data }">{{ formatDateTime(data.createdAt) }}</template><template #filter="{ filterModel }"><DatePicker v-model="filterModel.value" selection-mode="range" date-format="dd/mm/yy" placeholder="เลือกช่วงวันที่" /></template></Column>
      <Column field="actorType" header="ผู้ดำเนินการ" :show-filter-match-modes="false"><template #body="{ data }"><Tag severity="secondary" :value="statusLabel(data.actorType)" /></template><template #filter="{ filterModel }"><MultiSelect v-model="filterModel.value" :options="actorTypes" option-label="label" option-value="value" placeholder="ทั้งหมด" /></template></Column>
      <Column field="action" header="การทำงาน" :show-filter-match-modes="false"><template #body="{ data }"><span class="font-medium">{{ auditActionLabel(data.action) }}</span></template><template #filter="{ filterModel }"><InputText v-model="filterModel.value" maxlength="100" placeholder="เช่น TENANT_UPDATED" /></template></Column>
      <Column field="result" header="ผลลัพธ์" :show-filter-match-modes="false"><template #body="{ data }"><Tag :severity="data.result === 'SUCCESS' ? 'success' : data.result === 'DENIED' ? 'warn' : 'danger'" :value="statusLabel(data.result)" /></template><template #filter="{ filterModel }"><MultiSelect v-model="filterModel.value" :options="results" option-label="label" option-value="value" placeholder="ทั้งหมด" /></template></Column>
      <Column header="" header-class="table-action-column" body-class="table-action-column"><template #body="{ data }"><Button icon="pi pi-info-circle" text rounded class="touch-action" aria-label="ดูรายละเอียดทางเทคนิค" v-tooltip.top="'รายละเอียดทางเทคนิค'" @click="selected = data" /></template></Column>
      <template #empty><div class="py-8 text-center text-muted-color">ไม่พบข้อมูลตามเงื่อนไข <Button v-if="hasFilters" label="ล้างตัวกรอง" text size="small" @click="clearTableFilters" /></div></template>
    </DataTable>
  </div>
  <Dialog :visible="!!selected" modal header="รายละเอียดเหตุการณ์" class="responsive-dialog" :style="{ width: '34rem' }" @update:visible="selected = undefined"><dl v-if="selected" class="grid grid-cols-[9rem_1fr] gap-3 m-0"><dt>รหัสเหตุการณ์</dt><dd class="technical-detail m-0">{{ selected.id }}</dd><dt>รหัสร้าน</dt><dd class="technical-detail m-0">{{ selected.tenantId || '—' }}</dd><dt>รหัสการทำงาน</dt><dd class="technical-detail m-0">{{ selected.action }}</dd><dt>ประเภทข้อมูล</dt><dd class="technical-detail m-0">{{ selected.resourceType }}</dd><dt>รหัสข้อมูล</dt><dd class="technical-detail m-0">{{ selected.resourceId || '—' }}</dd><dt>รหัสข้อผิดพลาด</dt><dd class="technical-detail m-0">{{ selected.safeErrorCode || '—' }}</dd></dl></Dialog>
</template>
