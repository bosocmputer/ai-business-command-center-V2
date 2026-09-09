<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import type { DataTableFilterEvent } from 'primevue/datatable';
import { onBeforeRouteLeave, useRoute, useRouter } from 'vue-router';
import { useToast } from 'primevue/usetoast';
import { adminApi, ApiError, type AdminReportCatalog, type FlexPreview, type Schedule, type ScheduleInput, type ScheduleRecipientOption, type Tenant } from '@/api';
import { newIdempotencyKey } from '@/api/client';
import LineFlexPreview from '@/components/LineFlexPreview.vue';
import ReportPickerPanel from '@/components/admin/ReportPickerPanel.vue';
import SakaiTableHeader from '@/components/table/SakaiTableHeader.vue';
import { useSakaiFilterMenu } from '@/composables/useSakaiFilterMenu';
import { beginAdminTenantContext, setAdminTenantContext } from '@/stores/adminTenantContext';
import { loadAdminReportCatalog } from '@/stores/reportCatalog';
import { errorMessage, formatDateTime } from '@/utils/format';
import { schedulePeriodExample, schedulePeriodGroupDetail, schedulePeriodOptions, type SchedulePeriodMode } from '@/utils/schedulePeriodPresentation';

const route = useRoute();
const router = useRouter();
const toast = useToast();
const tenantId = String(route.params.tenantId);
beginAdminTenantContext(tenantId);
const scheduleId = route.params.scheduleId ? String(route.params.scheduleId) : '';
const editing = computed(() => !!scheduleId);
const tenant = ref<Tenant>();
const schedule = ref<Schedule>();
const recipients = ref<ScheduleRecipientOption[]>([]);
const selectedRecipientOptions = ref<ScheduleRecipientOption[]>([]);
const recipientSearch = ref('');
const recipientPage = ref(0);
const recipientPageSize = ref<25 | 50 | 100>(25);
const recipientTotal = ref(0);
const loadingRecipients = ref(false);
type RecipientStatus = 'PENDING' | 'ACTIVE';
type EligibilityState = 'ELIGIBLE' | 'NOT_ACTIVE' | 'MISSING_PERMISSIONS';
const recipientPrimeFilters = ref({
  status: { value: null as RecipientStatus[] | null, matchMode: 'in' },
  eligibility: { value: null as EligibilityState[] | null, matchMode: 'in' }
});
useSakaiFilterMenu(recipientPrimeFilters);
const appliedRecipientFilters = ref<{ statuses: RecipientStatus[]; eligibilityStates: EligibilityState[] }>({ statuses: [], eligibilityStates: [] });
const catalog = ref<AdminReportCatalog>();
const preview = ref<FlexPreview>();
const loading = ref(true);
const saving = ref(false);
const previewing = ref(false);
const error = ref('');
const baseline = ref('');
const controller = new AbortController();
let actionKey = '';
let previewController: AbortController | undefined;
let recipientController: AbortController | undefined;
let recipientGeneration = 0;
let recipientSearchTimer: ReturnType<typeof setTimeout> | undefined;
const serverConflict = ref<Schedule>();
const conflictAcknowledged = ref(false);

const form = reactive<ScheduleInput>({ name: '', daysOfWeek: [1, 2, 3, 4, 5], localTime: '09:00', timezone: 'Asia/Bangkok', periodPreset: 'YESTERDAY', reportKeys: [], recipientIds: [] });
const days = [{ label: 'อาทิตย์', value: 0 }, { label: 'จันทร์', value: 1 }, { label: 'อังคาร', value: 2 }, { label: 'พุธ', value: 3 }, { label: 'พฤหัสบดี', value: 4 }, { label: 'ศุกร์', value: 5 }, { label: 'เสาร์', value: 6 }];
const presets = schedulePeriodOptions;
const fingerprint = computed(() => JSON.stringify({ ...form, daysOfWeek: [...form.daysOfWeek].sort(), reportKeys: [...form.reportKeys], recipientIds: [...form.recipientIds].sort() }));
const dirty = computed(() => !!baseline.value && fingerprint.value !== baseline.value);
const selectedRecipientSet = computed(() => new Set(form.recipientIds));
const recipientOptionsById = computed(() => new Map([...selectedRecipientOptions.value, ...recipients.value].map((item) => [item.id, item])));
const maxReports = computed(() => catalog.value?.limits.maxScheduleReports ?? 10);
const invalidSelectedRecipients = computed(() => form.recipientIds.filter((id) => recipientOptionsById.value.get(id)?.eligible !== true));
const invalidSelectedOptions = computed(() => invalidSelectedRecipients.value.map((id) => recipientOptionsById.value.get(id)).filter((item): item is ScheduleRecipientOption => Boolean(item)));
const recipientHasFilters = computed(() => Boolean(recipientSearch.value.trim() || appliedRecipientFilters.value.statuses.length || appliedRecipientFilters.value.eligibilityStates.length));
const scheduleTimingValid = computed(() => form.daysOfWeek.length > 0 && /^([01][0-9]|2[0-3]):[0-5][0-9]$/.test(form.localTime));
const definitionsByKey = computed(() => new Map(catalog.value?.data.map((item) => [item.reportKey, item]) ?? []));
const periodModesReady = computed(() => form.reportKeys.every((key) => ['DATE_RANGE', 'AS_OF_DATE', 'CURRENT_ONLY'].includes(definitionsByKey.value.get(key)?.periodMode ?? '')));
const previewReady = computed(() => scheduleTimingValid.value && form.reportKeys.length > 0 && form.reportKeys.length <= maxReports.value && periodModesReady.value);
const previewDisabledReason = computed(() => {
  if (!form.daysOfWeek.length) return 'เลือกวันส่งอย่างน้อย 1 วัน';
  if (!/^([01][0-9]|2[0-3]):[0-5][0-9]$/.test(form.localTime)) return 'กรอกเวลาส่งให้ถูกต้อง เช่น 08:00';
  if (!form.reportKeys.length) return 'เลือกรายงานอย่างน้อย 1 รายงาน';
  if (!periodModesReady.value) return 'ข้อมูลความสามารถของรายงานไม่ครบ กรุณาโหลดหน้าใหม่ก่อน Preview หรือบันทึก';
  return '';
});
const valid = computed(() => form.name.trim().length > 0 && previewReady.value && form.recipientIds.length > 0 && invalidSelectedRecipients.value.length === 0 && (!serverConflict.value || conflictAcknowledged.value));
const readOnly = computed(() => schedule.value?.status === 'ACTIVE');
const periodExample = computed(() => schedulePeriodExample(form.periodPreset, form.localTime, form.daysOfWeek));
const periodGroupCopy = computed(() => {
  const groups: { mode: SchedulePeriodMode; title: string }[] = [
    { mode: 'DATE_RANGE', title: 'รายงานที่สรุปเป็นช่วงเวลา' },
    { mode: 'AS_OF_DATE', title: 'รายงานที่แสดงข้อมูล ณ วันที่' },
    { mode: 'CURRENT_ONLY', title: 'รายงานสถานะปัจจุบัน' }
  ];
  return groups.map((group) => {
    const reports = form.reportKeys.map((key) => definitionsByKey.value.get(key)).filter((item) => item?.periodMode === group.mode);
    return {
      ...group,
      detail: schedulePeriodGroupDetail(group.mode, form.periodPreset, form.localTime),
      reports,
      reportLabels: reports.map((item) => item?.label ?? '').filter(Boolean).join(', ')
    };
  });
});
const activePeriodGroups = computed(() => periodGroupCopy.value.filter((group) => group.reports.length > 0));
const conflictDiff = computed(() => {
  const latest = serverConflict.value;
  if (!latest) return [];
  const items: string[] = [];
  if (latest.name !== form.name) items.push(`ชื่อบนระบบ: ${latest.name}`);
  if (JSON.stringify(latest.reportKeys) !== JSON.stringify(form.reportKeys)) items.push(`รายงานบนระบบ: ${latest.reportKeys.length} รายงาน`);
  if (JSON.stringify([...latest.recipientIds].sort()) !== JSON.stringify([...form.recipientIds].sort())) items.push(`ผู้รับบนระบบ: ${latest.recipientIds.length} คน`);
  if (JSON.stringify(latest.daysOfWeek) !== JSON.stringify([...form.daysOfWeek].sort()) || latest.localTime !== form.localTime) items.push(`รอบส่งบนระบบ: ${latest.localTime} น.`);
  if (latest.periodPreset !== form.periodPreset) items.push('วันที่ของข้อมูลบนระบบถูกเปลี่ยน');
  return items;
});

async function load() {
  loading.value = true; error.value = '';
  try {
    const [tenantResult, catalogResult, scheduleResult] = await Promise.all([
      adminApi.getTenant(tenantId), loadAdminReportCatalog(controller.signal),
      scheduleId ? adminApi.getSchedule(tenantId, scheduleId, controller.signal) : Promise.resolve(undefined)
    ]);
    tenant.value = tenantResult; catalog.value = catalogResult; schedule.value = scheduleResult;
    setAdminTenantContext(tenantId, tenantResult.name);
    if (scheduleResult) Object.assign(form, { name: scheduleResult.name, daysOfWeek: [...scheduleResult.daysOfWeek], localTime: scheduleResult.localTime, timezone: scheduleResult.timezone, periodPreset: scheduleResult.periodPreset, reportKeys: [...scheduleResult.reportKeys], recipientIds: [...scheduleResult.recipientIds] });
    baseline.value = fingerprint.value;
  } catch (cause) { error.value = errorMessage(cause); }
  finally { loading.value = false; }
}

async function loadRecipientOptions(resetPage = false) {
  if (!form.reportKeys.length) { recipients.value = []; selectedRecipientOptions.value = []; recipientTotal.value = 0; return; }
  if (resetPage) recipientPage.value = 0;
  recipientController?.abort('recipient-options-replaced');
  const requestController = new AbortController();
  recipientController = requestController;
  const generation = ++recipientGeneration;
  loadingRecipients.value = true;
  try {
    const normalizedSearch = recipientSearch.value.trim();
    const result = await adminApi.scheduleRecipientOptions(tenantId, {
      reportKeys: [...form.reportKeys], selectedRecipientIds: [...form.recipientIds],
      globalSearch: normalizedSearch.length >= 2 ? normalizedSearch : undefined,
      statuses: appliedRecipientFilters.value.statuses,
      eligibilityStates: appliedRecipientFilters.value.eligibilityStates,
      page: recipientPage.value, pageSize: recipientPageSize.value
    }, requestController.signal);
    if (generation !== recipientGeneration) return;
    recipients.value = result.data;
    selectedRecipientOptions.value = result.selected;
    recipientTotal.value = result.total;
  } catch (cause) {
    if (!(cause instanceof ApiError && cause.code === 'CANCELLED')) toast.add({ severity: 'error', summary: 'โหลดความพร้อมผู้รับไม่สำเร็จ', detail: errorMessage(cause), life: 5000 });
  } finally { if (generation === recipientGeneration) loadingRecipients.value = false; }
}

function recipientFilterValue<T>(event: DataTableFilterEvent, key: string): T | undefined { return (event.filters[key] as { value?: T } | undefined)?.value; }
function applyRecipientFilters(event: DataTableFilterEvent) {
  appliedRecipientFilters.value = {
    statuses: recipientFilterValue<RecipientStatus[] | null>(event, 'status') ?? [],
    eligibilityStates: recipientFilterValue<EligibilityState[] | null>(event, 'eligibility') ?? []
  };
  void loadRecipientOptions(true);
}
function clearRecipientFilters() {
  recipientPrimeFilters.value.status.value = null;
  recipientPrimeFilters.value.eligibility.value = null;
  appliedRecipientFilters.value = { statuses: [], eligibilityStates: [] };
  recipientSearch.value = '';
  void loadRecipientOptions(true);
}

function changeRecipientPage(event: { page: number; rows: number }) {
  recipientPage.value = event.rows === recipientPageSize.value ? event.page : 0;
  recipientPageSize.value = event.rows as 25 | 50 | 100;
  void loadRecipientOptions();
}

function toggleRecipient(item: ScheduleRecipientOption, checked: boolean) {
  if (checked) {
    if (!item.eligible) return;
    if (!selectedRecipientSet.value.has(item.id)) form.recipientIds = [...form.recipientIds, item.id];
  } else form.recipientIds = form.recipientIds.filter((id) => id !== item.id);
}

function removeInvalidRecipients() { form.recipientIds = form.recipientIds.filter((id) => !invalidSelectedRecipients.value.includes(id)); }

async function createPreview() {
	if (!previewReady.value) return;
	previewController?.abort('preview-replaced');
	const requestController = new AbortController();
	previewController = requestController;
  previewing.value = true;
  try {
    const result = await adminApi.previewSchedule(tenantId, { periodPreset: form.periodPreset, reportKeys: [...form.reportKeys], daysOfWeek: [...form.daysOfWeek], localTime: form.localTime, timezone: form.timezone }, requestController.signal);
    if (previewController === requestController) preview.value = result;
  }
  catch (cause) { if (!(cause instanceof ApiError && cause.code === 'CANCELLED')) toast.add({ severity: 'error', summary: 'สร้างตัวอย่างไม่สำเร็จ', detail: errorMessage(cause), life: 5000 }); }
  finally { if (previewController === requestController) previewing.value = false; }
}

async function save() {
  if (!valid.value || saving.value || readOnly.value) return;
  saving.value = true;
  try {
    let updated: Schedule;
    if (schedule.value) updated = await adminApi.updateSchedule(tenantId, schedule.value.id, { ...form, version: schedule.value.version });
    else {
      actionKey ||= newIdempotencyKey('schedule');
      updated = await adminApi.createSchedule(tenantId, { ...form }, actionKey);
      actionKey = '';
    }
    schedule.value = updated; baseline.value = fingerprint.value;
    toast.add({ severity: 'success', summary: 'บันทึกตารางส่งรายงานแล้ว', life: 3000 });
    back();
  } catch (cause) {
    if (!(cause instanceof ApiError) || !cause.retryable) actionKey = '';
    if (cause instanceof ApiError && cause.code === 'VERSION_CONFLICT' && scheduleId) {
      try {
        serverConflict.value = await adminApi.getSchedule(tenantId, scheduleId);
        schedule.value = serverConflict.value;
        conflictAcknowledged.value = false;
      } catch (refreshCause) { toast.add({ severity: 'error', summary: 'โหลดค่าล่าสุดไม่สำเร็จ', detail: errorMessage(refreshCause), life: 6000 }); }
    } else toast.add({ severity: 'error', summary: 'บันทึกไม่สำเร็จ', detail: errorMessage(cause), life: 6000 });
  } finally { saving.value = false; }
}

function back() { void router.push({ name: 'admin-tenant-detail', params: { tenantId }, query: { tab: 'schedules' } }); }
function beforeUnload(event: BeforeUnloadEvent) { if (dirty.value) { event.preventDefault(); event.returnValue = ''; } }
function onVisibilityChange() { if (!document.hidden && form.reportKeys.length) void loadRecipientOptions(); }
watch(() => [form.periodPreset, form.localTime, ...form.daysOfWeek, ...form.reportKeys], () => { previewController?.abort('preview-input-changed'); preview.value = undefined; });
watch(() => [...form.reportKeys], () => { serverConflict.value = undefined; conflictAcknowledged.value = false; void loadRecipientOptions(true); });
watch(recipientSearch, (value) => {
  clearTimeout(recipientSearchTimer);
  if (value.trim().length === 1) return;
  recipientSearchTimer = setTimeout(() => { void loadRecipientOptions(true); }, 400);
});
watch(form, () => { if (!saving.value) actionKey = ''; }, { deep: true });
onBeforeRouteLeave(() => !dirty.value || window.confirm('มีตารางส่งรายงานที่ยังไม่ได้บันทึก ต้องการออกจากหน้านี้หรือไม่'));
onMounted(() => { window.addEventListener('beforeunload', beforeUnload); window.addEventListener('focus', onVisibilityChange); document.addEventListener('visibilitychange', onVisibilityChange); void load(); });
onBeforeUnmount(() => { controller.abort('unmount'); previewController?.abort('unmount'); recipientController?.abort('unmount'); clearTimeout(recipientSearchTimer); window.removeEventListener('beforeunload', beforeUnload); window.removeEventListener('focus', onVisibilityChange); document.removeEventListener('visibilitychange', onVisibilityChange); });
</script>

<template>
  <div v-if="loading" class="card"><Skeleton height="2rem" width="18rem" class="mb-4" /><Skeleton height="30rem" /></div>
  <Message v-else-if="error" severity="error" :closable="false">{{ error }} <Button label="ลองใหม่" text @click="load" /></Message>
  <template v-else-if="tenant && catalog">
    <AppPageHeader :title="editing ? 'แก้ไขตารางส่งรายงาน' : 'เพิ่มตารางส่งรายงาน'" :subtitle="`ร้าน ${tenant.name} · เวลาไทย`" mobile-mode="entity"><template #back><Button label="ตารางส่งรายงาน" icon="pi pi-arrow-left" text class="entity-back-action -ml-3 mb-1 touch-action" @click="back" /></template><template v-if="schedule" #actions><Tag :value="schedule.status === 'ACTIVE' ? 'กำลังใช้งาน' : 'ฉบับร่าง/พักไว้'" :severity="schedule.status === 'ACTIVE' ? 'success' : 'secondary'" /></template></AppPageHeader>
    <Message v-if="readOnly" severity="warn" :closable="false" class="mb-4">ตารางนี้กำลังใช้งาน กรุณากลับไปพักตารางก่อนแก้ไข</Message>
    <Message v-if="serverConflict" severity="warn" :closable="false" class="mb-4"><strong>ตารางนี้ถูกแก้จากอีกหน้าจอ</strong><div class="mt-2">ระบบเก็บค่าที่คุณกำลังแก้ไว้และโหลดค่าล่าสุดมาเปรียบเทียบ โดยยังไม่รวม/เขียนทับอัตโนมัติ</div><ul class="my-2 pl-5"><li v-for="item in conflictDiff" :key="item">{{ item }}</li></ul><Button label="ตรวจสอบแล้ว ใช้ค่าที่กำลังแก้" icon="pi pi-check" outlined @click="conflictAcknowledged = true" /></Message>
    <form class="flex flex-col gap-5" @submit.prevent="save">
      <div class="card">
        <div class="grid gap-2 max-w-3xl mb-5"><label for="schedule-name">ชื่อตารางส่ง LINE</label><InputText id="schedule-name" v-model="form.name" :disabled="readOnly" placeholder="เช่น สรุปผู้บริหารทุกเช้า" fluid /></div>
        <div class="flex flex-wrap items-center justify-between gap-3 mb-5"><div><h2 class="text-lg font-semibold m-0">1. รายงานใน LINE รอบนี้: {{ form.reportKeys.length.toLocaleString('th-TH') }} รายงาน</h2><p class="text-muted-color mt-1 mb-0">การเลือกนี้กำหนดสิ่งที่ส่งจริง ไม่ใช่สิทธิ์ทั้งหมดของผู้รับ · สูงสุด {{ maxReports }} รายงาน</p></div><Tag :value="`1 LINE Card · ${form.reportKeys.length}/${maxReports} รายงาน${preview ? ` · ${(preview.payloadBytes / 1024).toFixed(1)} KB / ${(catalog.limits.maxFlexPayloadBytes / 1024).toFixed(0)} KB` : ''}`" severity="info" /></div>
        <ReportPickerPanel v-model="form.reportKeys" :definitions="catalog.data" :max-selected="maxReports" :disabled="readOnly" ordered />
      </div>

      <div class="card">
        <div class="flex flex-wrap items-center justify-between gap-3 mb-4"><div><h2 class="text-lg font-semibold m-0">2. ผู้รับ LINE</h2><p class="text-muted-color mt-1 mb-0">ผู้รับต้องยืนยัน LINE และมีสิทธิ์เปิดดู Dashboard ครบทุกรายงานที่เลือก</p></div><Button v-if="invalidSelectedRecipients.length" label="นำผู้รับที่ไม่พร้อมออก" icon="pi pi-user-minus" severity="warn" outlined class="touch-action" @click="removeInvalidRecipients" /></div>
        <Message v-if="invalidSelectedRecipients.length" severity="error" :closable="false" class="mb-4"><strong>ผู้รับที่เลือกไว้ {{ invalidSelectedRecipients.length }} คนไม่พร้อม</strong><div class="mt-1">ระบบคงรายการไว้แม้อยู่คนละหน้าหรือไม่ตรงกับตัวกรอง และจะยังไม่อนุญาตให้บันทึก</div><ul v-if="invalidSelectedOptions.length" class="mt-2 mb-0 pl-5"><li v-for="item in invalidSelectedOptions" :key="item.id">{{ item.displayName }}</li></ul></Message>
        <Message v-if="!form.reportKeys.length" severity="info" :closable="false">เลือกรายงานในขั้นตอนที่ 1 ก่อน ระบบจึงจะตรวจความพร้อมของผู้รับได้</Message>
        <DataTable v-else v-model:filters="recipientPrimeFilters" :value="recipients" data-key="id" lazy paginator :first="recipientPage * recipientPageSize" :rows="recipientPageSize" :total-records="recipientTotal" :rows-per-page-options="[25, 50, 100]" current-page-report-template="หน้า {currentPage} จาก {totalPages} · ทั้งหมด {totalRecords} รายการ" paginator-template="RowsPerPageDropdown FirstPageLink PrevPageLink PageLinks NextPageLink LastPageLink CurrentPageReport" filter-display="menu" row-hover show-gridlines striped-rows responsive-layout="scroll" :loading="loadingRecipients" @page="changeRecipientPage" @filter="applyRecipientFilters"><template #header><SakaiTableHeader v-model:global-search="recipientSearch" :loading="loadingRecipients" :has-filters="recipientHasFilters" @clear="clearRecipientFilters"><template #start><small class="text-muted-color">เลือกแล้ว {{ form.recipientIds.length.toLocaleString('th-TH') }} คน</small></template></SakaiTableHeader></template><Column header="เลือก" style="width:5rem" header-class="table-select-column" body-class="table-select-column"><template #body="{ data }"><Checkbox :model-value="selectedRecipientSet.has(data.id)" binary :disabled="readOnly || !data.eligible" :aria-label="`เลือกผู้รับ ${data.displayName}`" @update:model-value="toggleRecipient(data, $event)" /></template></Column><Column field="displayName" header="ผู้รับ" /><Column field="status" header="สถานะ LINE" :show-filter-match-modes="false"><template #body="{ data }"><Tag :value="data.status === 'ACTIVE' ? 'ใช้งาน' : 'ยังไม่ยืนยัน LINE'" :severity="data.status === 'ACTIVE' ? 'success' : 'warn'" /></template><template #filter="{ filterModel }"><MultiSelect v-model="filterModel.value" :options="[{ label: 'ใช้งาน', value: 'ACTIVE' }, { label: 'ยังไม่ยืนยัน LINE', value: 'PENDING' }]" option-label="label" option-value="value" placeholder="ทุกสถานะ" /></template></Column><Column field="eligibility" header="ความพร้อม" :show-filter-match-modes="false"><template #body="{ data }"><Tag v-if="data.status !== 'ACTIVE'" value="ยังไม่ยืนยัน LINE" severity="warn" /><Tag v-else-if="!data.eligible" :value="`ขาดสิทธิ์ ${data.missingReportKeys.length} รายงาน`" severity="danger" /><Tag v-else value="พร้อมรับรายงานครบ" severity="success" /></template><template #filter="{ filterModel }"><MultiSelect v-model="filterModel.value" :options="[{ label: 'พร้อมรับรายงานครบ', value: 'ELIGIBLE' }, { label: 'ยังไม่ยืนยัน LINE', value: 'NOT_ACTIVE' }, { label: 'ขาดสิทธิ์รายงาน', value: 'MISSING_PERMISSIONS' }]" option-label="label" option-value="value" placeholder="ทุกความพร้อม" /></template></Column><Column header="การจัดการ" header-class="table-action-column" body-class="table-action-column"><template #body="{ data }"><RouterLink v-if="data.missingReportKeys.length" :to="{ name: 'admin-recipient-permissions', params: { tenantId, recipientId: data.id } }" target="_blank" rel="noopener noreferrer" class="permission-link"><i class="pi pi-external-link" /> แก้สิทธิ์</RouterLink></template></Column><template #empty><div class="py-8 text-center text-muted-color">ไม่พบผู้รับที่ตรงกับตัวกรอง <Button v-if="recipientHasFilters" label="ล้างตัวกรอง" text size="small" @click="clearRecipientFilters" /></div></template></DataTable>
      </div>

      <div class="card">
        <h2 class="text-lg font-semibold mt-0 mb-5">3. ตั้งวันส่งและวันที่ของข้อมูล</h2>
        <div class="grid grid-cols-1 md:grid-cols-2 gap-4 max-w-5xl"><div class="grid gap-2"><label for="schedule-days">วันส่ง</label><MultiSelect input-id="schedule-days" v-model="form.daysOfWeek" :options="days" option-label="label" option-value="value" display="chip" :disabled="readOnly" fluid /></div><div class="grid gap-2"><label for="schedule-time">เวลาส่ง (เวลาไทย)</label><InputMask id="schedule-time" v-model="form.localTime" mask="99:99" placeholder="09:00" :disabled="readOnly" fluid /></div><div class="grid gap-2"><label for="schedule-period">ต้องการให้ LINE สรุปข้อมูลช่วงไหน?</label><Select input-id="schedule-period" v-model="form.periodPreset" :options="presets" option-label="label" option-value="value" :disabled="readOnly" aria-describedby="schedule-period-help schedule-period-example" fluid /><small id="schedule-period-help" class="text-muted-color">ระบบจะคำนวณวันที่ของข้อมูลใหม่ในแต่ละรอบส่ง</small></div><div class="grid gap-2"><label>เขตเวลา</label><div class="timezone-static"><i class="pi pi-clock" /> เวลาไทย</div></div></div>
        <Message id="schedule-period-example" severity="info" :closable="false" class="mt-4"><strong>{{ periodExample }}</strong></Message>
        <Message v-if="!periodModesReady && form.reportKeys.length" severity="error" :closable="false" class="mt-4">ข้อมูลความสามารถของรายงานไม่ครบ ระบบจึงไม่คาดเดาช่วงข้อมูล กรุณาโหลดหน้าใหม่ก่อน Preview หรือบันทึก</Message>
        <div v-if="form.reportKeys.length" class="period-explanation mt-5" aria-live="polite"><div class="font-semibold mb-3">รายงานที่เลือกจะใช้ข้อมูลดังนี้</div><div v-for="group in activePeriodGroups" :key="group.mode" class="period-explanation-row"><div><strong>{{ group.title }}</strong><div class="text-sm text-muted-color mt-1">{{ group.detail }}</div><div class="text-sm mt-1">{{ group.reportLabels }}</div></div><Tag :value="`${group.reports.length} รายงาน`" severity="secondary" /></div><Message v-if="activePeriodGroups.find((group) => group.mode === 'CURRENT_ONLY')" severity="warn" :closable="false" class="mt-3">“สินค้าถึงจุดสั่งซื้อ” แสดงสถานะปัจจุบัน ณ เวลาส่งเสมอ จึงไม่สามารถดูย้อนหลังตามช่วงที่เลือกได้</Message></div>
      </div>

      <div v-if="preview" class="card"><div class="flex flex-wrap items-start justify-between gap-3 mb-4"><div><h2 class="text-lg font-semibold m-0">4. ตรวจสอบตัวอย่าง LINE Card</h2><p class="text-sm text-muted-color mt-1 mb-0">รอบส่งถัดไป {{ formatDateTime(preview.exampleScheduledFor) }} · {{ preview.mixedPeriods ? 'แต่ละรายงานใช้ช่วงตามความสามารถจริง' : 'ทุกรายงานใช้ช่วงเดียวกัน' }}</p></div></div><LineFlexPreview :preview="preview" /></div>
      <div class="card"><div class="flex flex-wrap items-center justify-between gap-3"><small v-if="previewDisabledReason" class="text-muted-color"><i class="pi pi-info-circle mr-1" />{{ previewDisabledReason }}</small><span v-else /><div class="flex flex-wrap justify-end gap-2"><Button type="button" label="ยกเลิก" text class="touch-action" @click="back" /><Button type="button" label="ดูตัวอย่าง LINE Card" icon="pi pi-eye" outlined class="touch-action" :disabled="!previewReady" :loading="previewing" @click="createPreview" /><Button type="submit" label="บันทึกตารางส่ง LINE" icon="pi pi-save" class="touch-action" :disabled="!valid || readOnly" :loading="saving" /></div></div></div>
    </form>
  </template>
</template>

<style scoped>
.timezone-static { min-height: 2.85rem; display: flex; align-items: center; gap: .65rem; padding: 0 .85rem; border: 1px solid var(--surface-border); border-radius: var(--content-border-radius); background: var(--surface-50); color: var(--text-color-secondary); }
:deep(.p-multiselect) { min-width: 0; max-width: 100%; }
:deep(.p-multiselect-display-chip .p-multiselect-label) { flex-wrap: wrap; }
.period-explanation { max-width: 64rem; padding: 1rem; border: 1px solid var(--surface-border); border-radius: var(--content-border-radius); background: var(--surface-50); }
.period-explanation-row { display: flex; align-items: start; justify-content: space-between; gap: 1rem; padding-block: .7rem; border-bottom: 1px solid var(--surface-border); }
.period-explanation-row:last-of-type { border-bottom: 0; }
.permission-link { display: inline-flex; min-height: 44px; align-items: center; gap: .5rem; color: var(--p-primary-color); text-decoration: none; font-weight: 600; }
.permission-link:hover { text-decoration: underline; }
</style>
