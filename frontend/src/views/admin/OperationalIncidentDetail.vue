<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import type { DataTableFilterEvent } from 'primevue/datatable';
import { useRoute, useRouter } from 'vue-router';
import { useToast } from 'primevue/usetoast';
import { useConfirm } from 'primevue/useconfirm';
import { ApiError, adminApi, type AdminReportDefinition, type IncidentDiagnosis, type OperationalIncidentDetail, type OperationalIncidentEvent, type OperationalIncidentOccurrence, type ReportKey, type SMLConnectionTestResult } from '@/api';
import { errorMessage, formatDateTime } from '@/utils/format';
import { loadAdminReportCatalog } from '@/stores/reportCatalog';
import SakaiTableHeader from '@/components/table/SakaiTableHeader.vue';
import IncidentDiagnosisPanel from '@/components/operations/IncidentDiagnosisPanel.vue';
import { useServerTable } from '@/composables/useServerTable';
import { useSakaiFilterMenu } from '@/composables/useSakaiFilterMenu';
import {
  buildCodexIncidentText, causalChain, evidenceLevelLabel, eventKindLabel, formatDurationMs,
  incidentLifecycleIcon, incidentLifecycleSeverity, lineImpactLabel, reportImpactLabel,
  sourceKindLabel, triggerKindLabel
} from '@/utils/operationalPresentation';

const route = useRoute();
const router = useRouter();
const toast = useToast();
const confirm = useConfirm();
const incident = ref<OperationalIncidentDetail>();
const loading = ref(false);
const mutating = ref(false);
const error = ref('');
const acceptDialog = ref(false);
const acceptReason = ref('');
const reportDefinitions = ref<AdminReportDefinition[]>([]);
const eventFilters = ref({
  global: { value: null as string | null, matchMode: 'contains' },
  eventKind: { value: null as string[] | null, matchMode: 'in' },
  sourceKind: { value: null as string[] | null, matchMode: 'in' }
});
const testingTenantId = ref('');
const testResults = ref<Record<string, SMLConnectionTestResult>>({});
const diagnosisCache = ref<Record<string, IncidentDiagnosis>>({});
const diagnosisErrors = ref<Record<string, string>>({});
const diagnosisLoadingId = ref('');
const diagnosisDialog = ref(false);
const selectedDiagnosisOccurrence = ref<OperationalIncidentOccurrence>();
let controller: AbortController | undefined;
let diagnosisController: AbortController | undefined;
let diagnosisGeneration = 0;

const incidentId = computed(() => String(route.params.incidentId || ''));
type OccurrenceSource = OperationalIncidentOccurrence['sourceKind'];
type OccurrenceFilters = { reportKeys: ReportKey[]; sourceKinds: OccurrenceSource[]; safeErrorCodes: string[]; dateFrom?: string; dateTo?: string };
const occurrencePrimeFilters = ref({
  reportKey: { value: null as string[] | null, matchMode: 'in' },
  sourceKind: { value: null as OccurrenceSource[] | null, matchMode: 'in' },
  safeErrorCode: { value: null as string[] | null, matchMode: 'in' },
  observedAt: { value: null as Date[] | null, matchMode: 'between' }
});
useSakaiFilterMenu(eventFilters);
useSakaiFilterMenu(occurrencePrimeFilters);
const occurrenceTable = useServerTable<OperationalIncidentOccurrence, OccurrenceFilters>({
  immediate: false,
  initialFilters: { reportKeys: [], sourceKinds: [], safeErrorCodes: [] },
  query: (input, signal) => adminApi.queryIncidentOccurrences(incidentId.value, input, signal)
});
const occurrences = occurrenceTable.rows;
const occurrenceLoading = occurrenceTable.loading;
const canAcknowledge = computed(() => incident.value?.status === 'OPEN');
const canAcceptRisk = computed(() => incident.value?.status === 'OPEN' || incident.value?.status === 'ACKNOWLEDGED');
const unsafeReason = computed(() => /(https?:\/\/|\b[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}\b|\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}\b|\b[0-9]{8,}\b|\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b|\b[0-9]{5,16}:[A-Za-z0-9_-]{24,}\b|(?:deliveryRef|[?&]ref)=)/i.test(acceptReason.value));
const validReason = computed(() => [...acceptReason.value.trim()].length >= 12 && [...acceptReason.value.trim()].length <= 500 && !unsafeReason.value);
const reportDefinitionByKey = computed(() => new Map(reportDefinitions.value.map((item) => [item.reportKey, item])));
const primaryEvent = computed(() => incident.value?.events.find((event) => event.failureEvidence && !event.isDownstream) ?? incident.value?.events.find((event) => event.failureEvidence));
const chain = computed(() => causalChain(primaryEvent.value));
const affectedLabel = computed(() => incident.value?.causeBreakdown?.[0]?.affectedLabelTh ?? 'ส่วนที่ได้รับผล');
const singleOccurrence = computed(() => occurrenceTable.total.value === 1 && occurrences.value.length === 1 ? occurrences.value[0] : undefined);
const hasMultipleOccurrences = computed(() => occurrenceTable.total.value > 1);
const singleDiagnosis = computed(() => singleOccurrence.value ? diagnosisCache.value[singleOccurrence.value.id] : undefined);
const singleDiagnosisError = computed(() => singleOccurrence.value ? diagnosisErrors.value[singleOccurrence.value.id] : '');
const selectedDiagnosis = computed(() => selectedDiagnosisOccurrence.value ? diagnosisCache.value[selectedDiagnosisOccurrence.value.id] : undefined);
const selectedDiagnosisError = computed(() => selectedDiagnosisOccurrence.value ? diagnosisErrors.value[selectedDiagnosisOccurrence.value.id] : '');
const isSMLIncident = computed(() => incident.value?.rootCause === 'SML_CONNECTIVITY');
const isResolvedLifecycle = computed(() => {
  const state = incident.value?.statusPresentation.state;
  return state === 'CONNECTION_RESTORED' || state === 'RESOLVED';
});
const lifecycleVerifiedAt = computed(() => incident.value?.statusPresentation.verifiedAt || incident.value?.lastSeenAt);
const eventGlobalSearch = computed({
  get: () => eventFilters.value.global.value ?? '',
  set: (value: string) => { eventFilters.value.global.value = value || null; }
});
const eventHasFilters = computed(() => Boolean(eventGlobalSearch.value || eventFilters.value.eventKind.value?.length || eventFilters.value.sourceKind.value?.length));
const occurrenceHasFilters = computed(() => Boolean(
  occurrenceTable.appliedGlobalSearch.value || occurrenceTable.appliedFilters.value.reportKeys.length ||
  occurrenceTable.appliedFilters.value.sourceKinds.length || occurrenceTable.appliedFilters.value.safeErrorCodes.length ||
  occurrenceTable.appliedFilters.value.dateFrom || occurrenceTable.appliedFilters.value.dateTo
));
const eventKindOptions = computed(() => [...new Set(incident.value?.events.map((item) => item.eventKind) ?? [])].map((value) => ({ label: eventKindLabel(value), value })));
const sourceKindOptions = computed(() => [...new Set([
  ...(incident.value?.events.map((item) => item.sourceKind) ?? []),
  ...occurrences.value.map((item) => item.sourceKind)
])].map((value) => ({ label: sourceKindLabel(value), value })));
const reportOptions = computed(() => reportDefinitions.value.map((item) => ({ label: item.label, value: item.reportKey })));
const safeErrorOptions = computed(() => {
  const labels = new Map<string, string>();
  for (const item of occurrences.value) {
    if (!item.safeErrorCode || labels.has(item.safeErrorCode)) continue;
    labels.set(item.safeErrorCode, item.failureEvidence?.presentation.titleTh ?? 'ปัญหาที่ระบบบันทึกไว้');
  }
  return [...labels].map(([value, label]) => ({ label, value }));
});

function lifecycleStatusLabel() {
  if (!incident.value) return '';
  return ({ OPEN: 'ต้องตรวจสอบ', ACKNOWLEDGED: 'รับทราบแล้ว', RESOLVED: 'ปิดเหตุแล้ว', CLOSED_ACCEPTED: 'ปิดโดยยอมรับความเสี่ยง' } as const)[incident.value.status];
}
function reportLabel(event: OperationalIncidentEvent) {
  if (!event.reportKey) return 'ไม่ระบุรายงาน';
  return reportDefinitionByKey.value.get(event.reportKey)?.label ?? 'รายงานที่เกี่ยวข้อง';
}
function eventTitle(event: OperationalIncidentEvent) {
  return event.failureEvidence?.presentation.titleTh ?? eventKindLabel(event.eventKind);
}

async function load() {
  controller?.abort('superseded');
  diagnosisController?.abort('superseded');
  diagnosisGeneration += 1;
  diagnosisCache.value = {};
  diagnosisErrors.value = {};
  diagnosisLoadingId.value = '';
  selectedDiagnosisOccurrence.value = undefined;
  diagnosisDialog.value = false;
  controller = new AbortController();
  loading.value = true;
  error.value = '';
  try {
    incident.value = await adminApi.incident(incidentId.value, controller.signal);
    occurrenceTable.page.value = 0;
    void occurrenceTable.refresh();
  } catch (cause) {
    if (!(cause instanceof ApiError && cause.code === 'CANCELLED')) error.value = errorMessage(cause);
  } finally {
    loading.value = false;
  }
}

async function loadDiagnosis(item: OperationalIncidentOccurrence, openDialog = false) {
  if (openDialog) {
    selectedDiagnosisOccurrence.value = item;
    diagnosisDialog.value = true;
  }
  if (diagnosisCache.value[item.id]) return;
  diagnosisController?.abort('superseded');
  diagnosisController = new AbortController();
  const generation = ++diagnosisGeneration;
  diagnosisLoadingId.value = item.id;
  diagnosisErrors.value = { ...diagnosisErrors.value, [item.id]: '' };
  try {
    const value = await adminApi.incidentDiagnosis(incidentId.value, item.id, diagnosisController.signal);
    if (generation !== diagnosisGeneration) return;
    diagnosisCache.value = { ...diagnosisCache.value, [item.id]: value };
  } catch (cause) {
    if (generation !== diagnosisGeneration || (cause instanceof ApiError && cause.code === 'CANCELLED')) return;
    diagnosisErrors.value = { ...diagnosisErrors.value, [item.id]: errorMessage(cause) };
  } finally {
    if (generation === diagnosisGeneration) diagnosisLoadingId.value = '';
  }
}

function dateFilterValue(value: unknown): string | undefined {
  if (!(value instanceof Date) || Number.isNaN(value.getTime())) return undefined;
  const year = value.getFullYear();
  const month = String(value.getMonth() + 1).padStart(2, '0');
  const day = String(value.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}
function occurrenceFilterValue<T>(event: DataTableFilterEvent, key: string): T | undefined {
  return (event.filters[key] as { value?: T } | undefined)?.value;
}
function applyOccurrenceFilters(event: DataTableFilterEvent) {
  const dates = occurrenceFilterValue<Date[] | null>(event, 'observedAt') ?? [];
  occurrenceTable.draftFilters.value = {
    reportKeys: occurrenceFilterValue<ReportKey[] | null>(event, 'reportKey') ?? [],
    sourceKinds: occurrenceFilterValue<OccurrenceSource[] | null>(event, 'sourceKind') ?? [],
    safeErrorCodes: occurrenceFilterValue<string[] | null>(event, 'safeErrorCode') ?? [],
    dateFrom: dateFilterValue(dates[0]), dateTo: dateFilterValue(dates[1] ?? dates[0])
  };
  void occurrenceTable.applyFilters();
}
function clearOccurrenceFilters() {
  Object.values(occurrencePrimeFilters.value).forEach((filter) => { filter.value = null; });
  void occurrenceTable.clearFilters();
}
function clearEventFilters() {
  eventFilters.value = {
    global: { value: null, matchMode: 'contains' },
    eventKind: { value: null, matchMode: 'in' },
    sourceKind: { value: null, matchMode: 'in' }
  };
}

function displayEndpoint(item: OperationalIncidentOccurrence) {
  return item.smlConnectionReference?.currentEndpointUrl || item.smlConnectionReference?.endpointUrlAtFailure || '';
}
function currentTestEndpoint(item: OperationalIncidentOccurrence) { return item.smlConnectionReference?.currentEndpointUrl || ''; }
function testResult(item: OperationalIncidentOccurrence) { return item.tenantId ? testResults.value[item.tenantId] : undefined; }

function confirmConnectionTest(item: OperationalIncidentOccurrence) {
  const endpoint = currentTestEndpoint(item);
  if (!item.tenantId || !endpoint || testingTenantId.value) return;
  confirm.require({
    header: 'ทดสอบจาก Server Dashboard',
    message: `ระบบจะติดต่อ Java Web Service ของ ${item.tenantName || 'ร้านนี้'} ที่ URL ปัจจุบันด้วยคำสั่ง read-only ขนาดเล็ก โดยไม่ดึงรายงานหรือ KPI\n\n${endpoint}`,
    icon: 'pi pi-exclamation-triangle',
    rejectLabel: 'ยกเลิก', acceptLabel: 'เริ่มทดสอบ',
    accept: () => void testConnection(item)
  });
}

async function testConnection(item: OperationalIncidentOccurrence) {
  if (!item.tenantId || testingTenantId.value) return;
  testingTenantId.value = item.tenantId;
  try {
    const result = await adminApi.testSML(item.tenantId);
    testResults.value = { ...testResults.value, [item.tenantId]: result };
    toast.add({ severity: 'success', summary: 'ทดสอบสำเร็จ', detail: `Server Dashboard ติดต่อ Java Web Service ได้ · ใช้เวลา ${result.latencyMs.toLocaleString('th-TH')} มิลลิวินาที`, life: 6000 });
  } catch (cause) {
    const wait = cause instanceof ApiError && cause.retryAfterMs ? ` ลองใหม่ได้อีกประมาณ ${Math.ceil(cause.retryAfterMs / 1000)} วินาที` : '';
    toast.add({ severity: 'warn', summary: 'ยังทดสอบไม่ได้', detail: `${errorMessage(cause)}${wait}`, life: 7000 });
  } finally { testingTenantId.value = ''; }
}

async function acknowledge() {
  if (!incident.value || !canAcknowledge.value || mutating.value) return;
  mutating.value = true;
  try {
    await adminApi.acknowledgeIncident(incident.value);
    await load();
    toast.add({ severity: 'success', summary: 'รับทราบเหตุสำคัญแล้ว', detail: 'ระบบหยุดข้อความเตือนซ้ำ แต่ยังติดตามหลักฐานการฟื้นตัวต่อ', life: 5000 });
  } catch (cause) { error.value = errorMessage(cause); }
  finally { mutating.value = false; }
}

async function acceptRisk() {
  if (!incident.value || !canAcceptRisk.value || !validReason.value || mutating.value) return;
  mutating.value = true;
  try {
    await adminApi.acceptIncidentRisk(incident.value, acceptReason.value.trim());
    acceptDialog.value = false;
    acceptReason.value = '';
    await load();
    toast.add({ severity: 'warn', summary: 'ปิดโดยยอมรับความเสี่ยงแล้ว', detail: 'รายการนี้ไม่ได้ถูกระบุว่าระบบหายแล้ว', life: 5000 });
  } catch (cause) { error.value = errorMessage(cause); }
  finally { mutating.value = false; }
}

async function copyForCodex() {
  if (!incident.value) return;
  const value = buildCodexIncidentText(incident.value);
  try {
    await navigator.clipboard.writeText(value);
    toast.add({ severity: 'success', summary: 'คัดลอกข้อมูลปลอดภัยแล้ว', detail: 'ข้อความไม่มี KPI, SQL, token หรือข้อมูลลูกค้า', life: 4000 });
  } catch {
    toast.add({ severity: 'error', summary: 'คัดลอกไม่สำเร็จ', detail: 'กรุณาอนุญาตการใช้ Clipboard แล้วลองใหม่', life: 4000 });
  }
}

async function copyForCustomer(diagnosis: IncidentDiagnosis) {
  if (!diagnosis.customerMessageTh) return;
  try {
    await navigator.clipboard.writeText(diagnosis.customerMessageTh);
    toast.add({ severity: 'success', summary: 'คัดลอกคำตอบให้ลูกค้าแล้ว', detail: 'ข้อความนี้มีชื่อร้าน, Base URL และ Request Ref ที่ผ่านการล้างข้อมูลแล้ว', life: 5000 });
  } catch {
    toast.add({ severity: 'error', summary: 'คัดลอกไม่สำเร็จ', detail: 'กรุณาอนุญาตการใช้ Clipboard แล้วลองใหม่', life: 4000 });
  }
}

watch(singleOccurrence, (item) => {
  if (item) void loadDiagnosis(item);
}, { flush: 'post' });

onMounted(() => {
  void loadAdminReportCatalog().then((catalog) => { reportDefinitions.value = catalog.data; }).catch(() => undefined);
  void load();
});
onBeforeUnmount(() => {
  controller?.abort('unmounted');
  diagnosisController?.abort('unmounted');
});
</script>

<template>
  <AppPageHeader title="รายละเอียดเหตุสำคัญ" :subtitle="incident ? incident.alertRef : 'กำลังโหลดข้อมูล'" mobile-mode="entity">
    <template #back><Button icon="pi pi-arrow-left" text rounded aria-label="กลับไปเหตุสำคัญ" @click="router.push('/admin/operational-incidents')" /></template>
    <template #actions>
      <Button v-if="canAcknowledge" label="รับทราบ" icon="pi pi-check" :loading="mutating" @click="acknowledge" />
      <Button v-if="canAcceptRisk" label="ยอมรับความเสี่ยง" icon="pi pi-times-circle" severity="danger" outlined :disabled="mutating" @click="acceptDialog = true" />
    </template>
  </AppPageHeader>
  <Message v-if="error" severity="error" :closable="false" class="mb-4">{{ error }}</Message>
  <div v-if="loading && !incident" class="card"><Skeleton height="12rem" /></div>
  <template v-else-if="incident">
    <section class="card incident-status-card" :class="`is-${incidentLifecycleSeverity(incident.statusPresentation.state)}`">
      <div class="status-icon" aria-hidden="true"><i :class="incidentLifecycleIcon(incident.statusPresentation.state)" /></div>
      <div class="status-content">
        <div class="flex flex-wrap items-center gap-2 mb-3">
          <Tag :severity="incident.severity === 'P1' ? 'danger' : 'warn'" :value="incident.severity" />
          <Tag :severity="incidentLifecycleSeverity(incident.statusPresentation.state)" :value="lifecycleStatusLabel()" />
        </div>
        <h2 class="incident-title">{{ incident.statusPresentation.headlineTh }}</h2>
        <p class="status-summary">{{ incident.statusPresentation.statusSummaryTh }}</p>
        <dl class="incident-summary status-facts m-0">
          <dt>ตรวจสอบล่าสุด</dt><dd>{{ formatDateTime(lifecycleVerifiedAt) }} เวลาไทย</dd>
          <dt>ผลต่อ LINE</dt><dd>{{ lineImpactLabel(primaryEvent?.impact?.notificationOutcome) }}</dd>
          <dt>{{ affectedLabel }}</dt><dd>{{ (isResolvedLifecycle ? incident.affectedCount : incident.activeAffectedCount).toLocaleString('th-TH') }} รายการ</dd>
          <template v-if="incident.measurement"><dt>ค่าที่ตรวจพบล่าสุด</dt><dd>{{ incident.measurement.value.toLocaleString('th-TH', { maximumFractionDigits: 1 }) }}{{ incident.measurement.unit === 'PERCENT' ? '%' : incident.measurement.unit === 'SECONDS' ? ' วินาที' : '' }} · เริ่มเตือนที่ {{ incident.measurement.threshold.toLocaleString('th-TH', { maximumFractionDigits: 1 }) }}{{ incident.measurement.unit === 'PERCENT' ? '%' : incident.measurement.unit === 'SECONDS' ? ' วินาที' : '' }}</dd></template>
          <template v-if="incident.acceptedReason"><dt>เหตุผลที่ปิดการติดตาม</dt><dd>{{ incident.acceptedReason }}</dd></template>
        </dl>
      </div>
    </section>

    <section v-if="occurrenceLoading && !occurrences.length" class="card"><Skeleton height="9rem" /></section>
    <IncidentDiagnosisPanel
      v-else-if="singleOccurrence"
      :diagnosis="singleDiagnosis"
      :occurrence="singleOccurrence"
      :report-label="singleOccurrence.reportKey ? (reportDefinitionByKey.get(singleOccurrence.reportKey)?.label ?? 'รายงานที่เกี่ยวข้อง') : sourceKindLabel(singleOccurrence.sourceKind)"
      :loading="diagnosisLoadingId === singleOccurrence.id"
      :error="singleDiagnosisError"
      @retry="loadDiagnosis(singleOccurrence)"
      @copy-customer="copyForCustomer"
    >
      <template v-if="isSMLIncident" #connection>
        <div class="connection-panel diagnosis-connection">
          <h3 class="m-0">URL และการตรวจสอบ Java Web Service</h3>
          <div v-if="singleOccurrence.smlConnectionReference && displayEndpoint(singleOccurrence)" class="endpoint-cell">
            <small>{{ singleOccurrence.smlConnectionReference.endpointUrlAtFailure ? 'Java Web Service URL ที่ใช้ตอนเกิดเหตุ' : 'Java Web Service URL ปัจจุบัน' }}</small>
            <a :href="singleOccurrence.smlConnectionReference.endpointUrlAtFailure || displayEndpoint(singleOccurrence)" target="_blank" rel="noopener noreferrer" class="endpoint-link">{{ singleOccurrence.smlConnectionReference.endpointUrlAtFailure || displayEndpoint(singleOccurrence) }}</a>
            <template v-if="singleOccurrence.smlConnectionReference.status === 'CHANGED_SINCE_FAILURE'">
              <small class="text-orange-600">การตั้งค่าเปลี่ยนหลังเกิดเหตุ · URL ปัจจุบัน</small>
              <a :href="singleOccurrence.smlConnectionReference.currentEndpointUrl" target="_blank" rel="noopener noreferrer" class="endpoint-link">{{ singleOccurrence.smlConnectionReference.currentEndpointUrl }}</a>
            </template>
            <small v-else-if="singleOccurrence.smlConnectionReference.status === 'CURRENT_ONLY'" class="text-orange-600">ไม่มี URL รุ่นเดิม จึงแสดง URL ปัจจุบันโดยไม่อ้างว่าเป็นค่าตอนเกิดเหตุ</small>
            <small v-if="singleOccurrence.smlConnectionReference.schemeSecurity === 'HTTP'" class="text-orange-600">การเชื่อมต่อนี้ไม่ได้เข้ารหัส</small>
          </div>
          <span v-else class="text-muted-color">ไม่มี URL ที่ยืนยันได้จากหลักฐาน</span>
          <div class="connection-actions">
            <a v-if="singleOccurrence.smlConnectionReference && displayEndpoint(singleOccurrence)" :href="singleOccurrence.smlConnectionReference.endpointUrlAtFailure || displayEndpoint(singleOccurrence)" target="_blank" rel="noopener noreferrer" class="p-button p-component p-button-outlined endpoint-button"><i class="pi pi-external-link" aria-hidden="true" /><span>เปิด URL ใน Browser</span></a>
            <Button label="ทดสอบจาก Server Dashboard" icon="pi pi-bolt" outlined :loading="testingTenantId === singleOccurrence.tenantId" :disabled="!singleOccurrence.tenantId || !currentTestEndpoint(singleOccurrence) || Boolean(testingTenantId)" @click="confirmConnectionTest(singleOccurrence)" />
          </div>
          <small class="text-muted-color">การเปิด URL ยืนยันได้เฉพาะเครื่องของคุณ ส่วนการทดสอบจาก Server Dashboard จะติดต่อ Java Web Service จริงด้วยคำสั่ง read-only และไม่เปลี่ยนสถานะเหตุนี้</small>
          <Message v-if="testResult(singleOccurrence)" severity="success" :closable="false" class="mt-2 mb-0">ทดสอบสำเร็จเมื่อ {{ formatDateTime(testResult(singleOccurrence)!.testedAt) }} · {{ testResult(singleOccurrence)!.latencyMs.toLocaleString('th-TH') }} มิลลิวินาที</Message>
        </div>
      </template>
    </IncidentDiagnosisPanel>

    <section v-else-if="hasMultipleOccurrences" class="card table-card">
      <h2 class="text-lg mt-0 mb-1">ร้านและรายงานที่ได้รับผล</h2>
      <p class="text-muted-color mt-0 mb-4">มีหลายรายการ จึงแสดงเป็นตารางเพื่อค้นหาและตรวจสอบแยกร้าน</p>
      <Message v-if="occurrenceTable.error.value" severity="error" :closable="false" class="mb-4">โหลดข้อมูลใหม่ไม่สำเร็จ ข้อมูลเดิมยังแสดงอยู่ · {{ occurrenceTable.error.value }}</Message>
      <DataTable v-model:filters="occurrencePrimeFilters" :value="occurrences" :loading="occurrenceLoading" data-key="id" lazy paginator :first="occurrenceTable.page.value * occurrenceTable.pageSize.value" :rows="occurrenceTable.pageSize.value" :total-records="occurrenceTable.total.value" :rows-per-page-options="[25, 50, 100]" filter-display="menu" row-hover show-gridlines striped-rows scrollable current-page-report-template="หน้า {currentPage} จาก {totalPages} · ทั้งหมด {totalRecords} รายการ" paginator-template="RowsPerPageDropdown FirstPageLink PrevPageLink PageLinks NextPageLink LastPageLink CurrentPageReport" @page="occurrenceTable.changePage" @filter="applyOccurrenceFilters">
        <template #header><SakaiTableHeader v-model:global-search="occurrenceTable.globalSearch.value" :loading="occurrenceLoading" :has-filters="occurrenceHasFilters" @clear="clearOccurrenceFilters"><template #start><Button label="รีเฟรช" icon="pi pi-refresh" outlined :loading="occurrenceLoading" @click="occurrenceTable.refresh()" /></template></SakaiTableHeader></template>
        <Column field="tenantName" header="ร้าน/รายงาน"><template #body="{ data }"><div class="event-description"><span>{{ data.tenantName || 'ระบบส่วนกลาง' }}</span><small>{{ data.reportKey ? (reportDefinitionByKey.get(data.reportKey)?.label ?? 'รายงานที่เกี่ยวข้อง') : sourceKindLabel(data.sourceKind) }}</small></div></template></Column>
        <Column field="reportKey" header="รายงาน" :show-filter-match-modes="false"><template #body="{ data }">{{ data.reportKey ? (reportDefinitionByKey.get(data.reportKey)?.label ?? 'รายงานที่เกี่ยวข้อง') : '—' }}</template><template #filter="{ filterModel }"><MultiSelect v-model="filterModel.value" :options="reportOptions" option-label="label" option-value="value" filter placeholder="ทุกรายงาน" /></template></Column>
        <Column field="sourceKind" header="ส่วนของระบบ" :show-filter-match-modes="false"><template #body="{ data }">{{ sourceKindLabel(data.sourceKind) }}</template><template #filter="{ filterModel }"><MultiSelect v-model="filterModel.value" :options="sourceKindOptions" option-label="label" option-value="value" placeholder="ทุกส่วนของระบบ" /></template></Column>
        <Column field="safeErrorCode" header="เกิดอะไรขึ้น" :show-filter-match-modes="false"><template #body="{ data }">{{ data.failureEvidence?.presentation.titleTh || 'ปัญหาที่ระบบบันทึกไว้' }}</template><template #filter="{ filterModel }"><MultiSelect v-model="filterModel.value" :options="safeErrorOptions" option-label="label" option-value="value" filter placeholder="ทุกสาเหตุ" /></template></Column>
        <Column field="observedAt" header="เวลา" :show-filter-match-modes="false"><template #body="{ data }"><span class="whitespace-nowrap">{{ formatDateTime(data.observedAt) }}</span></template><template #filter="{ filterModel }"><DatePicker v-model="filterModel.value" selection-mode="range" date-format="dd/mm/yy" placeholder="เลือกช่วงวันที่" /></template></Column>
        <Column v-if="isSMLIncident" header="Java Web Service URL"><template #body="{ data }"><div v-if="data.smlConnectionReference && displayEndpoint(data)" class="endpoint-cell"><a :href="data.smlConnectionReference.endpointUrlAtFailure || displayEndpoint(data)" target="_blank" rel="noopener noreferrer" class="endpoint-link">{{ data.smlConnectionReference.endpointUrlAtFailure || displayEndpoint(data) }}</a><small v-if="data.smlConnectionReference.status === 'CURRENT_ONLY'" class="text-orange-600">URL ปัจจุบัน</small></div><span v-else class="text-muted-color">ไม่มี URL ที่ยืนยันได้</span></template></Column>
        <Column v-if="isSMLIncident" header="ตรวจจาก Server" header-class="table-action-column" body-class="table-action-column"><template #body="{ data }"><Button label="ทดสอบ" icon="pi pi-bolt" outlined size="small" :loading="testingTenantId === data.tenantId" :disabled="!data.tenantId || !currentTestEndpoint(data) || Boolean(testingTenantId)" @click="confirmConnectionTest(data)" /></template></Column>
        <Column header="สาเหตุ" header-class="table-action-column" body-class="table-action-column"><template #body="{ data }"><Button label="ดูสาเหตุ" icon="pi pi-search" size="small" @click="loadDiagnosis(data, true)" /></template></Column>
        <template #empty><div class="py-6 text-center text-muted-color">ไม่พบเหตุที่ตรงกับตัวกรอง <Button v-if="occurrenceHasFilters" label="ล้างตัวกรอง" text size="small" @click="clearOccurrenceFilters" /></div></template>
      </DataTable>
    </section>
    <Message v-else-if="!occurrenceLoading" severity="warn" :closable="false" class="mb-4">ไม่พบรายการร้านหรือทรัพยากรที่เชื่อมโยงกับเหตุนี้</Message>

    <section v-if="primaryEvent?.failureEvidence && !singleOccurrence" class="card action-card">
      <template v-if="incident.statusPresentation.actionRequired">
        <h2 class="text-lg mt-0">Admin ควรทำอะไรต่อ</h2>
        <p v-if="primaryEvent.failureEvidence.presentation.evidenceNoteTh" class="text-muted-color">{{ primaryEvent.failureEvidence.presentation.evidenceNoteTh }}</p>
        <ul class="pl-5 mb-0"><li v-for="action in primaryEvent.failureEvidence.presentation.nextActionsTh" :key="action" class="mb-2">{{ action }}</li></ul>
        <Message v-if="incident.subjectType === 'TENANT'" severity="info" :closable="false" class="mt-4 mb-0">ระบบจะตรวจยืนยันอีกครั้งจากรอบส่งตามตารางที่สำเร็จ หากตารางถูกพักหรือลบ จะไม่มีรอบถัดไปสำหรับยืนยันอัตโนมัติ</Message>
      </template>
      <Message v-else severity="success" :closable="false" class="m-0"><strong>ไม่ต้องดำเนินการ</strong><br>ระบบมีหลักฐานยืนยันสถานะปัจจุบันแล้ว คุณยังเปิดรายละเอียดด้านล่างเพื่อตรวจสอบย้อนหลังได้</Message>
      <Message v-if="primaryEvent.connectionChangedSinceFailure" severity="warn" :closable="false" class="mt-4 mb-0">การตั้งค่าการเชื่อมต่อ SML ถูกแก้ไขหลังเกิดเหตุ หลักฐานนี้อ้างอิงค่าที่ระบบใช้ในเวลานั้น</Message>
    </section>

    <Accordion class="mb-4 evidence-accordion">
      <AccordionPanel value="evidence">
        <AccordionHeader>หลักฐานและรายละเอียดเพิ่มเติม</AccordionHeader>
        <AccordionContent>
          <div v-if="incident.causeBreakdown?.length" class="evidence-section">
            <h3>สาเหตุที่ระบบบันทึก</h3>
            <p class="text-muted-color">ระบบไม่สรุปว่า Server ปิดหรือ Firewall บล็อก หากยังไม่มีหลักฐานยืนยัน</p>
            <div class="grid grid-cols-1 lg:grid-cols-2 gap-3">
              <div v-for="cause in incident.causeBreakdown" :key="`${cause.category}-${cause.stage}-${cause.transportPhase}`" class="cause-card"><strong>{{ cause.presentation.titleTh }}</strong><span>{{ cause.presentation.stageTh }}</span><small>{{ cause.affectedLabelTh }}ที่ยังไม่หาย {{ cause.activeAffectedCount.toLocaleString('th-TH') }} รายการ</small></div>
            </div>
          </div>
          <div v-if="chain.length" class="evidence-section">
            <h3>ลำดับสาเหตุและผลกระทบ</h3>
            <ol class="causal-chain m-0 p-0"><li v-for="(item, index) in chain" :key="`${index}-${item}`"><span class="causal-index" aria-hidden="true">{{ index + 1 }}</span><span>{{ item }}</span></li></ol>
          </div>
          <div class="evidence-section table-card">
            <h3>หลักฐานตามลำดับเวลา</h3>
            <DataTable v-model:filters="eventFilters" :value="incident.events" data-key="id" filter-display="menu" row-hover show-gridlines striped-rows scrollable paginator :rows="25" :rows-per-page-options="[25, 50, 100]" :global-filter-fields="['eventKind', 'tenantName', 'reportKey', 'sourceKind']" current-page-report-template="หน้า {currentPage} จาก {totalPages} · ทั้งหมด {totalRecords} รายการ" paginator-template="RowsPerPageDropdown FirstPageLink PrevPageLink PageLinks NextPageLink LastPageLink CurrentPageReport">
              <template #header><SakaiTableHeader v-model:global-search="eventGlobalSearch" :has-filters="eventHasFilters" @clear="clearEventFilters" /></template>
              <Column field="observedAt" header="เวลา"><template #body="{ data }"><span class="whitespace-nowrap">{{ formatDateTime(data.observedAt) }}</span></template></Column>
              <Column field="eventKind" header="เกิดอะไรขึ้น" :show-filter-match-modes="false"><template #body="{ data }"><div class="event-description"><span class="font-semibold">{{ eventTitle(data) }}</span><small>{{ eventKindLabel(data.eventKind) }} · {{ evidenceLevelLabel(data.failureEvidence?.level) }}</small></div></template><template #filter="{ filterModel }"><MultiSelect v-model="filterModel.value" :options="eventKindOptions" option-label="label" option-value="value" placeholder="ทุกเหตุการณ์" /></template></Column>
              <Column field="tenantName" header="ร้าน/รายงาน"><template #body="{ data }"><div class="event-description"><span>{{ data.tenantName || 'ระบบส่วนกลาง' }}</span><small>{{ reportLabel(data) }}</small></div></template></Column>
              <Column field="sourceKind" header="ส่วนของระบบ" :show-filter-match-modes="false"><template #body="{ data }"><div class="event-description"><span>{{ sourceKindLabel(data.sourceKind) }}</span><small>{{ triggerKindLabel(data.triggerKind) }}</small></div></template><template #filter="{ filterModel }"><MultiSelect v-model="filterModel.value" :options="sourceKindOptions" option-label="label" option-value="value" placeholder="ทุกส่วนของระบบ" /></template></Column>
              <Column header="ขั้นตอนและระยะเวลา"><template #body="{ data }"><div class="event-description"><span>{{ data.failureEvidence?.presentation.stageTh || 'ไม่มีรายละเอียดขั้นตอนเพิ่มเติม' }}</span><small>{{ formatDurationMs(data.failureEvidence?.durationMs) }}</small></div></template></Column>
              <Column header="ผลกระทบ"><template #body="{ data }"><div class="event-description"><span>{{ reportImpactLabel(data.impact) }}</span><small>{{ lineImpactLabel(data.impact?.notificationOutcome) }}</small></div></template></Column>
            </DataTable>
          </div>
          <div class="evidence-section technical-detail">
            <h3>ข้อมูลสำหรับทีมเทคนิค</h3>
            <dl class="incident-summary m-0">
              <dt>Alert reference</dt><dd>{{ incident.alertRef }}</dd>
              <dt>Incident type</dt><dd>{{ incident.incidentType }}</dd>
              <dt>Root cause</dt><dd>{{ incident.rootCause }}</dd>
              <dt>Safe error code</dt><dd>{{ incident.safeErrorCode || 'UNKNOWN' }}</dd>
              <template v-if="primaryEvent?.failureEvidence"><dt>Stage</dt><dd>{{ primaryEvent.failureEvidence.stage }}</dd><dt>Transport phase</dt><dd>{{ primaryEvent.failureEvidence.transportPhase || 'UNKNOWN' }}</dd><dt>Remote state</dt><dd>{{ primaryEvent.failureEvidence.remoteStateUnknown ? 'UNKNOWN' : 'CONFIRMED TERMINAL' }}</dd></template>
            </dl>
            <Button label="คัดลอกข้อมูลปลอดภัยสำหรับ Codex" icon="pi pi-copy" outlined class="mt-4" @click="copyForCodex" />
          </div>
        </AccordionContent>
      </AccordionPanel>
    </Accordion>
  </template>
  <Dialog v-model:visible="diagnosisDialog" modal header="สาเหตุและหลักฐาน" class="responsive-dialog diagnosis-dialog" :style="{ width: 'min(58rem, 96vw)' }">
    <IncidentDiagnosisPanel
      v-if="selectedDiagnosisOccurrence"
      :diagnosis="selectedDiagnosis"
      :occurrence="selectedDiagnosisOccurrence"
      :report-label="selectedDiagnosisOccurrence.reportKey ? (reportDefinitionByKey.get(selectedDiagnosisOccurrence.reportKey)?.label ?? 'รายงานที่เกี่ยวข้อง') : sourceKindLabel(selectedDiagnosisOccurrence.sourceKind)"
      :loading="diagnosisLoadingId === selectedDiagnosisOccurrence.id"
      :error="selectedDiagnosisError"
      embedded
      @retry="loadDiagnosis(selectedDiagnosisOccurrence)"
      @copy-customer="copyForCustomer"
    />
  </Dialog>
  <Dialog v-model:visible="acceptDialog" modal header="ยอมรับความเสี่ยง" class="responsive-dialog" :style="{ width: '34rem' }">
    <Message severity="warn" :closable="false" class="mb-4">การทำรายการนี้จะปิด Incident โดยไม่อ้างว่าระบบฟื้นตัว กรุณาระบุเหตุผลที่ตรวจสอบย้อนหลังได้</Message>
    <label for="accept-risk-reason" class="block font-semibold mb-2">เหตุผล (12–500 ตัวอักษร)</label>
    <small class="block text-muted-color mb-2">ระบุเฉพาะเหตุผลเชิงปฏิบัติการ ห้ามใส่ชื่อร้าน รหัสลูกค้า URL เบอร์โทร token KPI หรือข้อมูลดิบ</small>
    <Textarea id="accept-risk-reason" v-model="acceptReason" rows="5" class="w-full" maxlength="500" />
    <small v-if="unsafeReason" class="block text-red-500 mt-2">ลบข้อมูลอ้างอิงหรือข้อมูลลูกค้าออกก่อนยืนยัน</small>
    <template #footer><Button label="ยกเลิก" text @click="acceptDialog = false" /><Button label="ยืนยันการยอมรับความเสี่ยง" severity="danger" :disabled="!validReason" :loading="mutating" @click="acceptRisk" /></template>
  </Dialog>
</template>

<style scoped>
.incident-summary { display: grid; grid-template-columns: minmax(9rem, auto) 1fr; gap: .75rem 1.5rem; }
.incident-summary dt { color: var(--text-color-secondary); }
.incident-summary dd { margin: 0; overflow-wrap: anywhere; }
.incident-title { font-size: clamp(1.25rem, 2vw, 1.65rem); line-height: 1.35; margin: 0 0 .35rem; }
.incident-status-card { display: grid; grid-template-columns: 3.25rem minmax(0, 1fr); gap: 1rem; align-items: start; border-inline-start: .3rem solid var(--surface-border); }
.incident-status-card.is-danger { border-inline-start-color: var(--p-red-500); }
.incident-status-card.is-success { border-inline-start-color: var(--p-green-500); }
.incident-status-card.is-warn { border-inline-start-color: var(--p-orange-500); }
.status-icon { width: 3.25rem; height: 3.25rem; display: grid; place-items: center; border-radius: 999px; background: var(--surface-100); color: var(--text-color-secondary); font-size: 1.35rem; }
.is-danger .status-icon { color: var(--p-red-600); background: color-mix(in srgb, var(--p-red-500) 12%, transparent); }
.is-success .status-icon { color: var(--p-green-600); background: color-mix(in srgb, var(--p-green-500) 12%, transparent); }
.is-warn .status-icon { color: var(--p-orange-600); background: color-mix(in srgb, var(--p-orange-500) 12%, transparent); }
.status-summary { color: var(--text-color-secondary); margin: .25rem 0 1.25rem; line-height: 1.55; }
.status-facts { max-width: 52rem; }
.connection-panel { display: grid; gap: .85rem; padding-top: 1rem; border-top: 1px solid var(--surface-border); }
.connection-actions { display: flex; flex-wrap: wrap; gap: .75rem; }
.endpoint-button { display: inline-flex; align-items: center; gap: .5rem; text-decoration: none; }
.action-card { border-inline-start: .25rem solid var(--p-primary-color); }
.evidence-accordion { width: 100%; min-width: 0; max-width: 100%; }
.evidence-accordion :deep(.p-accordioncontent-wrapper), .evidence-accordion :deep(.p-accordioncontent-content) { min-width: 0; max-width: 100%; }
.evidence-section { min-width: 0; max-width: 100%; padding-block: .25rem 1.25rem; }
.evidence-section.table-card { overflow-x: auto; }
.evidence-section + .evidence-section { padding-top: 1.25rem; border-top: 1px solid var(--surface-border); }
.evidence-section h3 { margin: 0 0 .75rem; font-size: 1rem; }
.causal-chain { list-style: none; display: grid; gap: .75rem; }
.causal-chain li { display: grid; grid-template-columns: 2rem minmax(0, 1fr); align-items: center; gap: .75rem; position: relative; }
.causal-chain li:not(:last-child)::after { content: ''; position: absolute; inset-block-start: 2rem; inset-inline-start: .95rem; height: .75rem; border-inline-start: 2px solid var(--surface-border); }
.causal-index { width: 2rem; height: 2rem; border-radius: 999px; display: inline-grid; place-items: center; color: var(--p-primary-color); background: color-mix(in srgb, var(--p-primary-color) 12%, transparent); font-weight: 700; }
.event-description { min-width: 11rem; display: grid; gap: .25rem; }
.event-description small { color: var(--text-color-secondary); line-height: 1.4; }
.cause-card { display: grid; gap: .35rem; padding: 1rem; border: 1px solid var(--surface-border); border-radius: var(--content-border-radius); }
.cause-card span, .cause-card small { color: var(--text-color-secondary); line-height: 1.45; }
.endpoint-cell { min-width: 18rem; max-width: 32rem; display: grid; gap: .35rem; }
.endpoint-cell small { line-height: 1.35; }
.endpoint-link { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.endpoint-open-action { display: inline-flex; align-items: center; gap: .4rem; width: max-content; max-width: 100%; font-weight: 600; }
.technical-detail { font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; font-size: .875rem; }
@media (max-width: 640px) {
  .incident-status-card { grid-template-columns: 1fr; }
  .status-icon { width: 2.75rem; height: 2.75rem; }
  .incident-summary { grid-template-columns: 1fr; gap: .25rem; }
  .incident-summary dd { margin-bottom: .75rem; }
  .connection-actions > * { width: 100%; justify-content: center; }
  .endpoint-cell { min-width: 0; width: 100%; }
}
</style>
