<script setup lang="ts">
import { computed, ref } from 'vue';
import type { DataTableFilterEvent } from 'primevue/datatable';
import { adminApi, type OperationalIncident, type OperationalIncidentSeverity, type OperationalIncidentStatus } from '@/api';
import SakaiTableHeader from '@/components/table/SakaiTableHeader.vue';
import { useServerTable } from '@/composables/useServerTable';
import { useSakaiFilterMenu } from '@/composables/useSakaiFilterMenu';
import { formatDateTime } from '@/utils/format';

const statusOptions = [
  { label: 'ยังไม่รับทราบ', value: 'OPEN' },
  { label: 'รับทราบแล้ว', value: 'ACKNOWLEDGED' },
  { label: 'ระบบยืนยันว่าหายแล้ว', value: 'RESOLVED' },
  { label: 'ยอมรับความเสี่ยง', value: 'CLOSED_ACCEPTED' }
];
const severityOptions = [
  { label: 'P1 · แจ้ง Telegram', value: 'P1' },
  { label: 'P2 · ติดตามในระบบ', value: 'P2' }
];
const rootCauseOptions = [{ label: 'Java Web Service / SML', value: 'SML_CONNECTIVITY' }, { label: 'ข้อมูลรายงาน', value: 'REPORT_DATA' }, { label: 'การส่ง LINE', value: 'LINE_DELIVERY' }, { label: 'ระบบ Nextstep', value: 'PLATFORM' }, { label: 'ทรัพยากร Server', value: 'CAPACITY' }];
type RootCause = OperationalIncident['rootCause'];
type IncidentFilters = { statuses: OperationalIncidentStatus[]; severities: OperationalIncidentSeverity[]; rootCauses: RootCause[]; activeOnly: boolean };
const primeFilters = ref({ severity: { value: null as OperationalIncidentSeverity[] | null, matchMode: 'in' }, rootCause: { value: null as RootCause[] | null, matchMode: 'in' }, status: { value: null as OperationalIncidentStatus[] | null, matchMode: 'in' } });
useSakaiFilterMenu(primeFilters);
const table = useServerTable<OperationalIncident, IncidentFilters>({ initialFilters: { statuses: [], severities: [], rootCauses: [], activeOnly: true }, query: (input, signal) => adminApi.queryIncidents(input, signal) });
const rows = table.rows; const loading = table.loading; const error = table.error;
const showHistory = ref(false);
const hasFilters = computed(() => Boolean(table.appliedGlobalSearch.value || table.appliedFilters.value.statuses.length || table.appliedFilters.value.severities.length || table.appliedFilters.value.rootCauses.length || !table.appliedFilters.value.activeOnly));
function filterValue<T>(event: DataTableFilterEvent, key: string): T | undefined { return (event.filters[key] as { value?: T } | undefined)?.value; }
function applyPrimeFilters(event: DataTableFilterEvent) { table.draftFilters.value = { statuses: filterValue<OperationalIncidentStatus[] | null>(event, 'status') ?? [], severities: filterValue<OperationalIncidentSeverity[] | null>(event, 'severity') ?? [], rootCauses: filterValue<RootCause[] | null>(event, 'rootCause') ?? [], activeOnly: !showHistory.value }; void table.applyFilters(); }
function toggleHistory() { table.draftFilters.value.activeOnly = !showHistory.value; void table.applyFilters(); }
function clearTableFilters() { Object.values(primeFilters.value).forEach((filter) => { filter.value = null; }); showHistory.value = false; void table.clearFilters(); }

function statusLabel(value: OperationalIncidentStatus) {
  return ({ OPEN: 'ยังไม่รับทราบ', ACKNOWLEDGED: 'รับทราบแล้ว', RESOLVED: 'ระบบยืนยันว่าหายแล้ว', CLOSED_ACCEPTED: 'ยอมรับความเสี่ยง' } as const)[value];
}
function statusSeverity(value: OperationalIncidentStatus) {
  return value === 'OPEN' ? 'danger' : value === 'ACKNOWLEDGED' ? 'warn' : value === 'RESOLVED' ? 'success' : 'secondary';
}
function subjectLabel(item: OperationalIncident) {
  return ({ TENANT: 'ร้านที่ได้รับผล', HOST_RESOURCE: 'ทรัพยากร Server ที่ต้องตรวจสอบ', BACKUP_POLICY: 'นโยบายสำรองข้อมูล', DATABASE: 'ฐานข้อมูลที่ได้รับผล', CONTAINER: 'บริการระบบที่ได้รับผล', LINE_PROVIDER: 'ผู้ให้บริการ LINE ที่ได้รับผล' } as const)[item.subjectType] ?? 'ส่วนที่ได้รับผล';
}
function impactLabel(item: OperationalIncident) { return `${subjectLabel(item)}: ${item.activeAffectedCount.toLocaleString('th-TH')}${item.subjectType === 'TENANT' ? ' ร้าน' : ' รายการ'}`; }
function checkArea(item: OperationalIncident) {
  if (item.rootCause === 'SML_CONNECTIVITY') return 'Server และ Java Web Service ของลูกค้า';
  if (item.rootCause === 'LINE_DELIVERY') return 'ผู้ให้บริการ LINE';
  if (item.subjectType === 'DATABASE') return 'ฐานข้อมูล Dashboard';
  return 'ระบบ Nextstep Dashboard';
}
function measurementLabel(item: OperationalIncident) {
  if (!item.measurement) return '';
  const suffix = item.measurement.unit === 'PERCENT' ? '%' : item.measurement.unit === 'SECONDS' ? ' วินาที' : '';
  return `ค่าที่พบ ${item.measurement.value.toLocaleString('th-TH', { maximumFractionDigits: 1 })}${suffix} · เริ่มเตือนที่ ${item.measurement.threshold.toLocaleString('th-TH', { maximumFractionDigits: 1 })}${suffix}`;
}
function tenantExamplesLabel(item: OperationalIncident) {
  const examples = (item.tenantExamples ?? []).slice(0, 2);
  if (examples.length === 0) return '';
  const remaining = Math.max(0, item.activeAffectedCount - examples.length);
  const unit = item.subjectType === 'TENANT' ? 'ร้าน' : 'รายการ';
  return remaining > 0
    ? `${examples.join(' · ')} และอีก ${remaining.toLocaleString('th-TH')} ${unit}`
    : examples.join(' · ');
}

</script>

<template>
  <AppPageHeader title="เหตุสำคัญ" subtitle="แสดงเฉพาะปัญหาที่ต้องติดตาม · การรับทราบไม่ถือว่าปัญหาหายแล้ว" />
  <Message severity="info" :closable="false" class="mb-4">
    P1 แจ้ง Telegram เมื่อเปิดโหมดส่ง ส่วน P2 เก็บไว้ตรวจสอบในหน้านี้ ระบบจะเปลี่ยนเป็น “หายแล้ว” เมื่อมีหลักฐานจากระบบเท่านั้น
  </Message>
  <div class="card table-card">
    <Message v-if="error" severity="error" :closable="false" class="mb-4">โหลดข้อมูลใหม่ไม่สำเร็จ ข้อมูลเดิมยังแสดงอยู่ · {{ error }}</Message>
    <DataTable v-model:filters="primeFilters" :value="rows" :loading="loading" data-key="id" lazy paginator :first="table.page.value * table.pageSize.value" :rows="table.pageSize.value" :total-records="table.total.value" :rows-per-page-options="[25, 50, 100]" filter-display="menu" row-hover show-gridlines scrollable current-page-report-template="หน้า {currentPage} จาก {totalPages} · ทั้งหมด {totalRecords} รายการ" paginator-template="RowsPerPageDropdown FirstPageLink PrevPageLink PageLinks NextPageLink LastPageLink CurrentPageReport" @page="table.changePage" @filter="applyPrimeFilters">
      <template #header><SakaiTableHeader v-model:global-search="table.globalSearch.value" :loading="loading" :has-filters="hasFilters" @clear="clearTableFilters"><template #start><Button label="รีเฟรช" icon="pi pi-refresh" outlined :loading="loading" @click="table.refresh()" /><div class="flex items-center gap-2"><Checkbox v-model="showHistory" binary input-id="incident-history" @change="toggleHistory" /><label for="incident-history">รวมประวัติที่จบแล้ว</label></div></template></SakaiTableHeader></template>
      <Column field="severity" header="ระดับ" :show-filter-match-modes="false"><template #body="{ data }"><Tag :severity="data.severity === 'P1' ? 'danger' : 'warn'" :value="data.severity" /></template><template #filter="{ filterModel }"><MultiSelect v-model="filterModel.value" :options="severityOptions" option-label="label" option-value="value" placeholder="ทุกระดับ" /></template></Column>
      <Column field="presentation.titleTh" header="เกิดอะไรขึ้น"><template #body="{ data }"><div class="max-w-96"><div class="font-semibold">{{ data.presentation.titleTh }}</div><small class="text-muted-color">อ้างอิง {{ data.alertRef }}</small></div></template></Column>
      <Column field="rootCause" header="ส่วนที่ควรตรวจสอบ" :show-filter-match-modes="false"><template #body="{ data }"><div class="max-w-72"><span>{{ checkArea(data) }}</span><small v-if="measurementLabel(data)" class="block text-muted-color">{{ measurementLabel(data) }}</small></div></template><template #filter="{ filterModel }"><MultiSelect v-model="filterModel.value" :options="rootCauseOptions" option-label="label" option-value="value" placeholder="ทุกส่วน" /></template></Column>
      <Column field="activeAffectedCount" header="ผลกระทบที่ยังไม่หาย">
        <template #body="{ data }">
          <div>{{ impactLabel(data) }}</div>
          <small v-if="tenantExamplesLabel(data)" class="block max-w-80 truncate text-muted-color" :title="tenantExamplesLabel(data)">{{ tenantExamplesLabel(data) }}</small>
          <small class="text-muted-color">{{ data.observationMode === 'CONTINUOUS' ? 'ตรวจพบต่อเนื่อง' : `พบ ${data.occurrenceCount.toLocaleString('th-TH')} เหตุการณ์` }}</small>
        </template>
      </Column>
      <Column field="lastSeenAt" header="พบล่าสุด"><template #body="{ data }">{{ formatDateTime(data.lastSeenAt) }}</template></Column>
      <Column field="status" header="สถานะ" :show-filter-match-modes="false"><template #body="{ data }"><Tag :severity="statusSeverity(data.status)" :value="statusLabel(data.status)" /></template><template #filter="{ filterModel }"><MultiSelect v-model="filterModel.value" :options="statusOptions" option-label="label" option-value="value" placeholder="ทุกสถานะ" /></template></Column>
      <Column header="" header-class="table-action-column" body-class="table-action-column"><template #body="{ data }"><Button as="router-link" :to="`/admin/operational-incidents/${data.id}`" icon="pi pi-chevron-right" text rounded aria-label="เปิดรายละเอียดเหตุสำคัญ" /></template></Column>
      <template #empty><div class="py-8 text-center text-muted-color">ไม่พบเหตุสำคัญตามตัวกรองนี้ <Button v-if="hasFilters" label="ล้างตัวกรอง" text size="small" @click="clearTableFilters" /></div></template>
    </DataTable>
  </div>
</template>
