<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import type { DataTableFilterEvent } from 'primevue/datatable';
import { onBeforeRouteLeave, useRoute, useRouter } from 'vue-router';
import { useConfirm } from 'primevue/useconfirm';
import { useToast } from 'primevue/usetoast';
import { adminApi, ApiError, type DashboardRefreshPolicy, type DashboardRefreshPolicyInput, type Recipient, type Schedule, type SMLConnectionStatus, type Tenant } from '@/api';
import { newIdempotencyKey } from '@/api/client';
import { beginAdminTenantContext, setAdminTenantContext } from '@/stores/adminTenantContext';
import { errorMessage, formatDateTime } from '@/utils/format';
import { formatSMLTestCooldown, resolveSMLJavaWSEndpoint } from '@/utils/smlConnectionPresentation';
import { statusLabel } from '@/utils/status';
import SakaiTableHeader from '@/components/table/SakaiTableHeader.vue';
import { useServerTable } from '@/composables/useServerTable';
import { useSakaiFilterMenu } from '@/composables/useSakaiFilterMenu';
import { normalizeScheduleTableRow, toRecipientQueryInput, type RecipientTableFilters } from '@/utils/adminTableFilters';

const route = useRoute();
const router = useRouter();
const confirm = useConfirm();
const toast = useToast();
const tenantId = String(route.params.tenantId);
beginAdminTenantContext(tenantId);
const activeTab = ref(typeof route.query.tab === 'string' ? route.query.tab : 'overview');
const tenant = ref<Tenant>();
const sml = ref<SMLConnectionStatus>();
const refreshPolicy = ref<DashboardRefreshPolicy>();
const showArchived = ref(false);
const loading = ref(true);
const error = ref('');
const savingTenant = ref(false);
const testingSML = ref(false);
const savingSML = ref(false);
const savingRefreshPolicy = ref(false);
const inviteOpen = ref(false);
const inviteLabel = ref('');
const inviteURL = ref('');
const inviting = ref(false);
const reissuingRecipientId = ref('');
const revokingRecipientId = ref('');
const testSendingScheduleId = ref('');
const changingScheduleId = ref('');
const tenantBaseline = ref('');
const smlBaseline = ref('');
const refreshPolicyBaseline = ref('');
const refreshPolicyConflict = ref<DashboardRefreshPolicy>();
const smlLoaded = ref(false);
const latestSMLTestLatencyMs = ref<number>();
const smlCooldownUntil = ref<number>();
const smlClock = ref(Date.now());
let smlCooldownTimer: number | undefined;
let inviteActionKey = '';
const reissueActionKeys = new Map<string, string>();
const testSendActionKeys = new Map<string, string>();

type ScheduleStatus = Schedule['status'];
type ScheduleFilters = { statuses: ScheduleStatus[]; includeArchived: boolean };
const recipientPrimeFilters = ref({
  status: { value: null as Array<'PENDING' | 'ACTIVE'> | null, matchMode: 'in' },
  permissionState: { value: null as Array<'WITH_REPORTS' | 'WITHOUT_REPORTS'> | null, matchMode: 'in' }
});
const schedulePrimeFilters = ref({ status: { value: null as ScheduleStatus[] | null, matchMode: 'in' } });
useSakaiFilterMenu(recipientPrimeFilters);
useSakaiFilterMenu(schedulePrimeFilters);
const recipientTable = useServerTable<Recipient, RecipientTableFilters>({
  immediate: false,
  initialFilters: { statuses: [], permissionStates: [] },
  query: (input, signal) => adminApi.queryRecipients(tenantId, toRecipientQueryInput(input), signal)
});
const scheduleTable = useServerTable<Schedule, ScheduleFilters>({
  immediate: false,
  initialFilters: { statuses: [], includeArchived: false },
  query: async (input, signal) => {
    const result = await adminApi.querySchedules(tenantId, input, signal);
    return { ...result, data: result.data.map((item) => normalizeScheduleTableRow(item)) };
  }
});
const recipients = recipientTable.rows;
const schedules = scheduleTable.rows;
const recipientHasFilters = computed(() => Boolean(recipientTable.appliedGlobalSearch.value || recipientTable.appliedFilters.value.statuses.length || recipientTable.appliedFilters.value.permissionStates.length));
const scheduleHasFilters = computed(() => Boolean(scheduleTable.appliedGlobalSearch.value || scheduleTable.appliedFilters.value.statuses.length || scheduleTable.appliedFilters.value.includeArchived));

const tenantForm = reactive({ name: '', status: 'DISABLED' as Tenant['status'], accessEndsAt: new Date(), version: 1 });
const smlForm = reactive({ endpointUrl: '', configFileName: 'SMLConfigDATA.xml', databaseName: '', version: 0 });
const refreshPolicyForm = reactive<DashboardRefreshPolicyInput>({ fastIntervalMinutes: 5, standardIntervalMinutes: 15, heavyIntervalMinutes: 30, version: 0 });
const tenantFingerprint = computed(() => JSON.stringify({ name: tenantForm.name.trim(), status: tenantForm.status, accessEndsAt: tenantForm.accessEndsAt.getTime() }));
const smlFingerprint = computed(() => JSON.stringify({ endpointUrl: smlForm.endpointUrl.trim(), configFileName: smlForm.configFileName.trim(), databaseName: smlForm.databaseName.trim() }));
const tenantDirty = computed(() => !!tenant.value && tenantFingerprint.value !== tenantBaseline.value);
const smlDirty = computed(() => smlLoaded.value && smlFingerprint.value !== smlBaseline.value);
const refreshPolicyFingerprint = computed(() => JSON.stringify(refreshPolicyForm));
const refreshPolicyDirty = computed(() => !!refreshPolicy.value && refreshPolicyFingerprint.value !== refreshPolicyBaseline.value);
const hasUnsavedChanges = computed(() => tenantDirty.value || smlDirty.value || refreshPolicyDirty.value);
const resolvedSMLJavaWSEndpoint = computed(() => resolveSMLJavaWSEndpoint(smlForm.endpointUrl));
const smlCooldownRemainingMs = computed(() => Math.max(0, (smlCooldownUntil.value ?? 0) - smlClock.value));
const isSMLTestCoolingDown = computed(() => smlCooldownRemainingMs.value > 0);
const smlTestDisabled = computed(() => !sml.value?.isConfigured || smlDirty.value || savingSML.value || testingSML.value || isSMLTestCoolingDown.value);
const smlTestDisabledReason = computed(() => {
  if (isSMLTestCoolingDown.value) return `ระบบพักการทดสอบเพื่อไม่ให้คำขอซ้อน ลองใหม่ได้ใน ${formatSMLTestCooldown(smlCooldownRemainingMs.value)}`;
  if (smlDirty.value) return 'บันทึกค่าชุดล่าสุดก่อนทดสอบ';
  if (!sml.value?.isConfigured) return 'ตั้งค่าและบันทึก SML ก่อนทดสอบ';
  return '';
});
const smlStatusSeverity = computed(() => {
  if (smlDirty.value || isSMLTestCoolingDown.value) return 'warn';
  if (sml.value?.readinessStatus === 'READY') return 'success';
  if (sml.value?.readinessStatus === 'FAILED') return 'error';
  return 'info';
});
const smlStatusTitle = computed(() => {
  if (smlDirty.value) return 'มีค่าการเชื่อมต่อที่ยังไม่บันทึก';
  if (isSMLTestCoolingDown.value) return 'กำลังพักการทดสอบชั่วคราว';
  if (sml.value?.readinessStatus === 'READY') return 'การเชื่อมต่อ SML พร้อมใช้งาน';
  if (sml.value?.readinessStatus === 'FAILED') return 'การทดสอบ SML ล่าสุดไม่ผ่าน';
  return 'ยังไม่ได้ทดสอบการเชื่อมต่อ SML';
});
const smlStatusDetail = computed(() => {
  if (smlDirty.value) return 'บันทึกค่าชุดล่าสุดก่อน ระบบจึงจะทดสอบการเชื่อมต่อชุดนี้ได้';
  if (isSMLTestCoolingDown.value) return `ระบบจะไม่เริ่มคำขอซ้ำขณะที่ผลของคำขอก่อนหน้าอาจยังไม่แน่นอน ลองใหม่ได้ใน ${formatSMLTestCooldown(smlCooldownRemainingMs.value)}`;
  if (sml.value?.readinessStatus === 'READY') {
    const testedAt = formatDateTime(sml.value.lastTestedAt);
    const latency = latestSMLTestLatencyMs.value == null ? '' : ` · ใช้เวลา ${latestSMLTestLatencyMs.value} ms`;
    return `Dashboard ติดต่อ Java Web Service ได้${testedAt === '—' ? '' : ` · ทดสอบล่าสุด ${testedAt}`}${latency}`;
  }
  if (sml.value?.readinessStatus === 'FAILED') {
    const testedAt = formatDateTime(sml.value.lastTestedAt);
    const safeCode = sml.value.lastSafeErrorCode;
    const reason = safeCode ? errorMessage({ code: safeCode, message: 'การทดสอบการเชื่อมต่อไม่สำเร็จ' }) : 'ระบบไม่ได้บันทึกรายละเอียดของการทดสอบครั้งล่าสุด';
    return `${reason}${testedAt === '—' ? '' : ` · ทดสอบล่าสุด ${testedAt}`}`;
  }
  return 'บันทึกค่าแล้ว แต่ต้องทดสอบแบบอ่านอย่างเดียวก่อนเปิดตารางส่ง LINE';
});
const fastIntervalOptions = [{ label: 'ปิด', value: null }, 5, 10, 15, 30, 60].map((item) => typeof item === 'number' ? { label: `ทุก ${item} นาที`, value: item } : item);
const standardIntervalOptions = [{ label: 'ปิด', value: null }, 15, 30, 60].map((item) => typeof item === 'number' ? { label: `ทุก ${item} นาที`, value: item } : item);
const heavyIntervalOptions = [{ label: 'ปิด', value: null }, 30, 60, 120].map((item) => typeof item === 'number' ? { label: `ทุก ${item} นาที`, value: item } : item);
const refreshPresets = [
  { label: 'สมดุล', description: '5 / 15 / 30 นาที', values: [5, 15, 30] as const },
  { label: 'ลดภาระ SML', description: '15 / 30 / 60 นาที', values: [15, 30, 60] as const },
  { label: 'ดึงด้วยตนเองเท่านั้น', description: 'ไม่อัปเดตเบื้องหลัง', values: [null, null, null] as const }
];
const activeRefreshPreset = computed(() => refreshPresets.find((preset) => JSON.stringify(preset.values) === JSON.stringify([refreshPolicyForm.fastIntervalMinutes, refreshPolicyForm.standardIntervalMinutes, refreshPolicyForm.heavyIntervalMinutes]))?.label ?? 'กำหนดเอง');

function updateSMLCooldownClock() {
  smlClock.value = Date.now();
  if (smlCooldownUntil.value && smlCooldownUntil.value <= smlClock.value) {
    smlCooldownUntil.value = undefined;
    if (smlCooldownTimer !== undefined) window.clearInterval(smlCooldownTimer);
    smlCooldownTimer = undefined;
  }
}

function startSMLCooldown(retryAfterMs?: number) {
  if (!retryAfterMs || retryAfterMs <= 0) return;
  smlCooldownUntil.value = Math.max(smlCooldownUntil.value ?? 0, Date.now() + retryAfterMs);
  updateSMLCooldownClock();
  if (smlCooldownTimer === undefined) smlCooldownTimer = window.setInterval(updateSMLCooldownClock, 1_000);
}

async function refreshSMLStatus() {
  try {
    sml.value = await adminApi.getSML(tenantId);
  } catch {
    // Preserve the last safe state. Loading this status never contacts JavaWS.
  }
}

function applyRefreshPolicy(policy: DashboardRefreshPolicy, resetBaseline = true) {
  refreshPolicy.value = policy;
  Object.assign(refreshPolicyForm, {
    fastIntervalMinutes: policy.fastIntervalMinutes ?? null,
    standardIntervalMinutes: policy.standardIntervalMinutes ?? null,
    heavyIntervalMinutes: policy.heavyIntervalMinutes ?? null,
    version: policy.version
  });
  if (resetBaseline) refreshPolicyBaseline.value = refreshPolicyFingerprint.value;
}

function setRefreshPreset(values: readonly [DashboardRefreshPolicyInput['fastIntervalMinutes'], DashboardRefreshPolicyInput['standardIntervalMinutes'], DashboardRefreshPolicyInput['heavyIntervalMinutes']]) {
  [refreshPolicyForm.fastIntervalMinutes, refreshPolicyForm.standardIntervalMinutes, refreshPolicyForm.heavyIntervalMinutes] = values;
}

async function load() {
  loading.value = true; error.value = '';
  try {
    tenant.value = await adminApi.getTenant(tenantId);
    setAdminTenantContext(tenantId, tenant.value.name);
    Object.assign(tenantForm, { name: tenant.value.name, status: tenant.value.status, accessEndsAt: new Date(tenant.value.accessEndsAt), version: tenant.value.version });
    tenantBaseline.value = tenantFingerprint.value;
    const [smlResult, policyResult] = await Promise.allSettled([
      adminApi.getSML(tenantId),
      adminApi.getDashboardRefreshPolicy(tenantId)
    ]);
    await Promise.all([recipientTable.refresh(), scheduleTable.refresh()]);
    if (smlResult.status === 'fulfilled') {
      sml.value = smlResult.value;
      Object.assign(smlForm, { endpointUrl: sml.value.endpointUrl ?? '', configFileName: sml.value.configFileName ?? 'SMLConfigDATA.xml', databaseName: sml.value.databaseName ?? '', version: sml.value.version });
      smlBaseline.value = smlFingerprint.value;
      smlLoaded.value = true;
    } else if (smlResult.reason instanceof ApiError && smlResult.reason.status === 404) {
      smlBaseline.value = smlFingerprint.value; smlLoaded.value = true;
    } else throw smlResult.reason;
    if (policyResult.status === 'fulfilled') {
      applyRefreshPolicy(policyResult.value);
    } else throw policyResult.reason;
  } catch (cause) { error.value = errorMessage(cause); }
  finally { loading.value = false; }
}

async function saveRefreshPolicy() {
  if (!refreshPolicy.value || savingRefreshPolicy.value || !refreshPolicyDirty.value) return;
  const isFaster = (next: number | null | undefined, previous: number | null | undefined) => next != null && (previous == null || next < previous);
  if (isFaster(refreshPolicyForm.fastIntervalMinutes, refreshPolicy.value.fastIntervalMinutes) || isFaster(refreshPolicyForm.standardIntervalMinutes, refreshPolicy.value.standardIntervalMinutes) || isFaster(refreshPolicyForm.heavyIntervalMinutes, refreshPolicy.value.heavyIntervalMinutes)) {
    confirm.require({
      header: 'ยืนยันการดึงข้อมูลถี่ขึ้น',
      message: 'ค่าที่ถี่ขึ้นอาจเพิ่มภาระ Java Web Service และฐาน SML เมื่อมีผู้ใช้เปิด Dashboard ที่ข้อมูลเก่า ต้องการบันทึกหรือไม่?',
      icon: 'pi pi-exclamation-triangle', acceptLabel: 'ยืนยันและบันทึก', rejectLabel: 'กลับไปตรวจสอบ',
      accept: () => { void persistRefreshPolicy(); }
    });
    return;
  }
  await persistRefreshPolicy();
}

async function persistRefreshPolicy() {
  if (!refreshPolicy.value || savingRefreshPolicy.value || !refreshPolicyDirty.value) return;
  savingRefreshPolicy.value = true;
  try {
    const updated = await adminApi.updateDashboardRefreshPolicy(tenantId, { ...refreshPolicyForm });
    applyRefreshPolicy(updated); refreshPolicyConflict.value = undefined;
    toast.add({ severity: 'success', summary: 'บันทึกรอบอัปเดต Dashboard แล้ว', life: 3000 });
  } catch (cause) {
    if (cause instanceof ApiError && cause.code === 'VERSION_CONFLICT') {
      const latest = await adminApi.getDashboardRefreshPolicy(tenantId);
      refreshPolicyConflict.value = latest;
      refreshPolicy.value = latest;
      refreshPolicyForm.version = latest.version;
      toast.add({ severity: 'warn', summary: 'มีผู้ดูแลแก้ไขค่าก่อนหน้า', detail: 'เก็บค่าที่คุณกำลังแก้ไว้แล้ว กรุณาเทียบกับค่าล่าสุดก่อนบันทึกอีกครั้ง', life: 6000 });
    } else toast.add({ severity: 'error', summary: 'บันทึกรอบอัปเดตไม่ได้', detail: errorMessage(cause), life: 5000 });
  } finally { savingRefreshPolicy.value = false; }
}

function saveTenant() {
  if (!tenant.value || savingTenant.value || !tenantDirty.value) return;
  const statusChanged = tenantForm.status !== tenant.value.status;
  const accessShortened = tenantForm.accessEndsAt.getTime() < new Date(tenant.value.accessEndsAt).getTime();
  if (statusChanged || accessShortened) {
    confirm.require({
      header: 'ยืนยันการเปลี่ยนสิทธิ์ร้าน',
      message: tenantForm.status !== 'ACTIVE'
        ? 'การบันทึกจะหยุดการเปิด Dashboard รอบส่งรายงานใหม่ และการส่ง LINE ของร้านนี้ทันที'
        : 'คุณกำลังเปลี่ยนสถานะหรือย่อวันสิ้นสุดสิทธิ์ โปรดตรวจสอบวันที่ก่อนบันทึก',
      icon: 'pi pi-exclamation-triangle',
      acceptLabel: 'ยืนยันและบันทึก',
      rejectLabel: 'กลับไปตรวจสอบ',
      accept: () => persistTenant()
    });
    return;
  }
  void persistTenant();
}

async function persistTenant() {
  if (!tenant.value || savingTenant.value) return;
  savingTenant.value = true;
  try {
    const updated = await adminApi.updateTenant(tenantId, { ...tenantForm, timezone: 'Asia/Bangkok', accessEndsAt: tenantForm.accessEndsAt.toISOString() });
    tenant.value = updated; tenantForm.version = updated.version; tenantBaseline.value = tenantFingerprint.value;
    setAdminTenantContext(tenantId, updated.name);
    toast.add({ severity: 'success', summary: 'บันทึกร้านค้าแล้ว', life: 2500 });
  } catch (cause) { toast.add({ severity: 'error', summary: 'บันทึกไม่สำเร็จ', detail: errorMessage(cause), life: 5000 }); }
  finally { savingTenant.value = false; }
}

function saveSML() {
  if (savingSML.value || !smlDirty.value) return;
  if (sml.value?.isConfigured) {
    confirm.require({
      header: 'ยืนยันแทนที่การเชื่อมต่อ SML',
      message: 'ตารางส่งรายงานของร้านจะไม่พร้อม จนกว่าจะบันทึกและทดสอบการเชื่อมต่อใหม่ผ่าน',
      icon: 'pi pi-exclamation-triangle',
      acceptLabel: 'แทนที่การเชื่อมต่อ',
      rejectLabel: 'ยกเลิก',
      accept: () => persistSML()
    });
    return;
  }
  void persistSML();
}

async function persistSML() {
  if (savingSML.value) return;
  savingSML.value = true;
  try {
    sml.value = await adminApi.replaceSML(tenantId, { ...smlForm });
    Object.assign(smlForm, { endpointUrl: sml.value.endpointUrl ?? '', configFileName: sml.value.configFileName ?? smlForm.configFileName, databaseName: sml.value.databaseName ?? smlForm.databaseName, version: sml.value.version });
    smlBaseline.value = smlFingerprint.value;
    latestSMLTestLatencyMs.value = undefined;
    toast.add({ severity: 'success', summary: 'บันทึก SML แล้ว', detail: 'กรุณาทดสอบการเชื่อมต่อก่อนเปิดตารางส่งรายงาน', life: 4000 });
  } catch (cause) { toast.add({ severity: 'error', summary: 'บันทึก SML ไม่สำเร็จ', detail: errorMessage(cause), life: 5000 }); }
  finally { savingSML.value = false; }
}

async function testSML() {
  if (testingSML.value || isSMLTestCoolingDown.value) return;
  if (smlDirty.value) { toast.add({ severity: 'warn', summary: 'ยังมีค่าที่ยังไม่บันทึก', detail: 'บันทึกการเชื่อมต่อก่อนทดสอบ เพื่อให้ระบบทดสอบค่าชุดล่าสุด', life: 4500 }); return; }
  testingSML.value = true;
  try {
    const result = await adminApi.testSML(tenantId);
    latestSMLTestLatencyMs.value = result.latencyMs;
    toast.add({ severity: 'success', summary: 'ทดสอบ SML สำเร็จ', detail: `Server Dashboard ติดต่อ Java Web Service ได้ · ใช้เวลา ${result.latencyMs} ms`, life: 5000 });
    await refreshSMLStatus();
  } catch (cause) {
    if (cause instanceof ApiError && (cause.code === 'SML_TEST_COOLDOWN' || cause.code === 'SML_TEST_BUSY')) {
      startSMLCooldown(cause.retryAfterMs);
      toast.add({ severity: 'warn', summary: 'ยังทดสอบซ้ำไม่ได้', detail: smlTestDisabledReason.value || errorMessage(cause), life: 6000 });
    } else {
      toast.add({ severity: 'error', summary: 'ทดสอบ SML ไม่ผ่าน', detail: errorMessage(cause), life: 6000 });
      await refreshSMLStatus();
    }
  }
  finally { testingSML.value = false; }
}

async function invite() {
  if (!inviteLabel.value.trim() || inviting.value) return;
  inviting.value = true;
  try {
    inviteActionKey ||= newIdempotencyKey('recipient');
    const created = await adminApi.inviteRecipient(tenantId, inviteLabel.value.trim(), inviteActionKey);
    inviteActionKey = '';
    inviteURL.value = created.invitationUrl ?? '';
    await loadRecipients();
    toast.add({ severity: 'success', summary: 'สร้างคำเชิญแล้ว', life: 2500 });
  } catch (cause) { if (!(cause instanceof ApiError) || !cause.retryable) inviteActionKey = ''; toast.add({ severity: 'error', summary: 'สร้างคำเชิญไม่สำเร็จ', detail: errorMessage(cause), life: 5000 }); }
  finally { inviting.value = false; }
}

function confirmReissueInvitation(item: Recipient) {
  if (reissuingRecipientId.value || revokingRecipientId.value) return;
  confirm.require({
    header: 'สร้างลิงก์เชิญใหม่',
    message: `ลิงก์เชิญเดิมของ “${item.displayName}” จะใช้งานไม่ได้ทันที ต้องการสร้างลิงก์ใหม่หรือไม่?`,
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: 'สร้างลิงก์ใหม่',
    rejectLabel: 'ยกเลิก',
    accept: () => { void reissueInvitation(item); }
  });
}

async function reissueInvitation(item: Recipient) {
  if (item.status !== 'PENDING' || reissuingRecipientId.value) return;
  reissuingRecipientId.value = item.id;
  try {
    const actionKey = reissueActionKeys.get(item.id) ?? newIdempotencyKey('recipient-reissue');
    reissueActionKeys.set(item.id, actionKey);
    const updated = await adminApi.reissueRecipientInvitation(tenantId, item.id, actionKey);
    reissueActionKeys.delete(item.id);
    inviteLabel.value = updated.displayName;
    inviteURL.value = updated.invitationUrl ?? '';
    inviteOpen.value = true;
    toast.add({ severity: 'success', summary: 'สร้างลิงก์เชิญใหม่แล้ว', detail: 'ลิงก์เดิมถูกยกเลิกและใช้งานไม่ได้แล้ว', life: 4000 });
  } catch (cause) {
    if (!(cause instanceof ApiError) || !cause.retryable) reissueActionKeys.delete(item.id);
    if (cause instanceof ApiError && (cause.code === 'INVITATION_NOT_PENDING' || cause.code === 'RECIPIENT_NOT_FOUND')) {
      try { await loadRecipients(); } catch { /* retain the current table and surface the mutation error below */ }
    }
    toast.add({ severity: 'error', summary: 'สร้างลิงก์เชิญใหม่ไม่สำเร็จ', detail: errorMessage(cause), life: 5000 });
  } finally {
    reissuingRecipientId.value = '';
  }
}

async function copyInvite() {
  if (!inviteURL.value) return;
  await navigator.clipboard.writeText(inviteURL.value);
  toast.add({ severity: 'success', summary: 'คัดลอกลิงก์แล้ว', life: 2000 });
}

async function copyDashboardLink() {
  if (!tenant.value?.viewerUrl) return;
  await navigator.clipboard.writeText(tenant.value.viewerUrl);
  toast.add({ severity: 'success', summary: 'คัดลอกลิงก์ Dashboard แล้ว', detail: 'ลิงก์ไม่เพิ่มสิทธิ์ ผู้รับต้องยืนยัน LINE และมีสิทธิ์ร้านนี้อยู่แล้ว', life: 3500 });
}

function editPermissions(item: Recipient) { void router.push({ name: 'admin-recipient-permissions', params: { tenantId, recipientId: item.id } }); }

async function revokeRecipient(item: Recipient) {
  if (revokingRecipientId.value) return;
  revokingRecipientId.value = item.id;
  try {
    await adminApi.revokeRecipient(tenantId, item.id);
    await loadRecipients();
    toast.add({
      severity: 'success',
      summary: item.status === 'PENDING' ? 'ยกเลิกคำเชิญแล้ว' : 'ลบผู้รับแล้ว',
      detail: item.status === 'PENDING' ? 'ลิงก์เชิญเดิมใช้งานไม่ได้แล้ว' : 'เพิกถอนสิทธิ์ Dashboard และการรับ LINE ของร้านนี้แล้ว',
      life: 4000
    });
  } catch (cause) {
    if (cause instanceof ApiError && cause.code === 'RECIPIENT_IN_USE') {
      const schedules = cause.fieldErrors?.map((item) => item.message).filter(Boolean) ?? [];
      toast.add({
        severity: 'warn',
        summary: 'ยังลบผู้รับไม่ได้',
        detail: schedules.length
          ? `ผู้รับยังอยู่ในตารางที่ใช้งาน: ${schedules.join(', ')} กรุณาพักหรือแก้ตารางก่อน`
          : 'ผู้รับยังอยู่ในตารางส่งรายงานที่ใช้งาน กรุณาพักหรือแก้ตารางก่อน',
        life: 7000
      });
    } else {
      toast.add({ severity: 'error', summary: 'ลบผู้รับไม่สำเร็จ', detail: errorMessage(cause), life: 6000 });
    }
  } finally {
    if (revokingRecipientId.value === item.id) revokingRecipientId.value = '';
  }
}

async function loadRecipients() { await recipientTable.refresh(); }
function filterValue<T>(event: DataTableFilterEvent, key: string): T | undefined { return (event.filters[key] as { value?: T } | undefined)?.value; }
function applyRecipientFilters(event: DataTableFilterEvent) {
  recipientTable.draftFilters.value = {
    statuses: filterValue<Array<'PENDING' | 'ACTIVE'> | null>(event, 'status') ?? [],
    permissionStates: filterValue<Array<'WITH_REPORTS' | 'WITHOUT_REPORTS'> | null>(event, 'permissionState') ?? []
  };
  void recipientTable.applyFilters();
}
function clearRecipientFilters() {
  recipientPrimeFilters.value.status.value = null;
  recipientPrimeFilters.value.permissionState.value = null;
  void recipientTable.clearFilters();
}

function confirmRevokeRecipient(item: Recipient) {
  if (revokingRecipientId.value) return;
  confirm.require({
    header: item.status === 'PENDING' ? 'ยกเลิกคำเชิญผู้รับ' : 'ยืนยันลบผู้รับ',
    message: item.status === 'PENDING'
      ? `ยกเลิกคำเชิญ “${item.displayName}” และทำให้ลิงก์เดิมใช้งานไม่ได้ทันที?`
      : `ลบ “${item.displayName}” ออกจากร้าน ${tenant.value?.name ?? ''}? ผู้รับจะเปิด Dashboard และรับ LINE ของร้านนี้ไม่ได้ แต่ประวัติการส่งและ Audit เดิมยังคงอยู่`,
    icon: 'pi pi-exclamation-triangle',
    acceptLabel: item.status === 'PENDING' ? 'ยกเลิกคำเชิญ' : 'ลบผู้รับ',
    rejectLabel: 'กลับ',
    acceptClass: 'p-button-danger',
    accept: () => revokeRecipient(item)
  });
}
function openSchedule(item?: Schedule) { void router.push(item ? { name: 'admin-schedule-edit', params: { tenantId, scheduleId: item.id } } : { name: 'admin-schedule-new', params: { tenantId } }); }

async function copySlug() {
  if (!tenant.value) return;
  await navigator.clipboard.writeText(tenant.value.slug);
  toast.add({ severity: 'success', summary: 'คัดลอกรหัสระบบแล้ว', life: 2000 });
}

async function changeScheduleState(item: Schedule) {
  if (changingScheduleId.value) return;
  changingScheduleId.value = item.id;
  try {
    await (item.status === 'ACTIVE' ? adminApi.pauseSchedule(tenantId, item.id) : adminApi.activateSchedule(tenantId, item.id));
    await scheduleTable.refresh();
    toast.add({ severity: 'success', summary: item.status === 'ACTIVE' ? 'พักตารางส่งรายงานแล้ว' : 'เปิดตารางส่งรายงานแล้ว', life: 2500 });
  } catch (cause) { toast.add({ severity: 'error', summary: 'เปลี่ยนสถานะไม่ได้', detail: errorMessage(cause), life: 6000 }); }
  finally { changingScheduleId.value = ''; }
}

function applyScheduleFilters(event: DataTableFilterEvent) {
  const statuses = filterValue<ScheduleStatus[] | null>(event, 'status') ?? [];
  if (statuses.includes('ARCHIVED')) showArchived.value = true;
  scheduleTable.draftFilters.value = {
    statuses,
    includeArchived: showArchived.value
  };
  void scheduleTable.applyFilters();
}
function toggleArchivedSchedules() {
  scheduleTable.draftFilters.value.includeArchived = showArchived.value;
  void scheduleTable.applyFilters();
}
function clearScheduleFilters() {
  schedulePrimeFilters.value.status.value = null;
  showArchived.value = false;
  void scheduleTable.clearFilters();
}

async function archiveSchedule(item: Schedule) {
  if (changingScheduleId.value || item.status === 'ACTIVE' || item.status === 'ARCHIVED') return;
  changingScheduleId.value = item.id;
  try {
    await adminApi.archiveSchedule(tenantId, item.id, item.version);
    await scheduleTable.refresh();
    toast.add({ severity: 'success', summary: 'ลบตารางส่งรายงานแล้ว', detail: 'หยุดการส่งในอนาคต และยังเก็บประวัติการส่งเดิมไว้', life: 3500 });
  } catch (cause) {
    toast.add({ severity: 'error', summary: 'ลบตารางส่งรายงานไม่ได้', detail: errorMessage(cause), life: 6000 });
  } finally {
    changingScheduleId.value = '';
  }
}

function confirmArchiveSchedule(item: Schedule) {
  if (item.status === 'ACTIVE') return;
  confirm.require({
    header: 'ยืนยันลบตารางส่งรายงาน',
    message: `ร้าน ${tenant.value?.name ?? ''} · ตาราง “${item.name}” · ${item.reportKeys.length} รายงาน · ผู้รับ ${item.recipientIds.length} คน ระบบจะไม่ส่ง LINE ตามตารางนี้อีก แต่ประวัติการส่งเดิมยังเก็บไว้ 365 วัน`,
    icon: 'pi pi-trash',
    acceptLabel: 'ลบตารางส่งรายงาน',
    rejectLabel: 'ยกเลิก',
    acceptClass: 'p-button-danger',
    accept: () => archiveSchedule(item)
  });
}

async function restoreSchedule(item: Schedule) {
  if (changingScheduleId.value || item.status !== 'ARCHIVED') return;
  changingScheduleId.value = item.id;
  try {
    await adminApi.restoreSchedule(tenantId, item.id, item.version);
    await scheduleTable.refresh();
    toast.add({ severity: 'success', summary: 'กู้คืนเป็นฉบับร่างแล้ว', detail: 'ตารางยังไม่ส่งจนกว่าจะตรวจสอบและเปิดใช้งาน', life: 3500 });
  } catch (cause) {
    toast.add({ severity: 'error', summary: 'กู้คืนตารางไม่ได้', detail: errorMessage(cause), life: 6000 });
  } finally {
    changingScheduleId.value = '';
  }
}

function confirmRestoreSchedule(item: Schedule) {
  confirm.require({
    header: 'กู้คืนตารางส่งรายงาน',
    message: `“${item.name}” จะกลับมาเป็นฉบับร่างและยังไม่ส่ง LINE จนกว่าจะเปิดใช้งานอีกครั้ง`,
    icon: 'pi pi-refresh',
    acceptLabel: 'กู้คืนเป็นฉบับร่าง',
    rejectLabel: 'ยกเลิก',
    accept: () => restoreSchedule(item)
  });
}

function confirmScheduleState(item: Schedule) {
  const activating = item.status !== 'ACTIVE';
  confirm.require({
    header: activating ? 'ยืนยันเปิดตารางส่งรายงาน' : 'ยืนยันพักตารางส่งรายงาน',
    message: activating
      ? `ระบบจะเริ่มรันตามรอบถัดไปของ “${item.name}”`
      : `ระบบจะหยุดรอบใหม่ของ “${item.name}” แต่ Run ที่กำลังทำงานจะไม่ถูกยกเลิก`,
    icon: activating ? 'pi pi-play' : 'pi pi-pause',
    acceptLabel: activating ? 'เปิดใช้งาน' : 'พักไว้',
    rejectLabel: 'ยกเลิก',
    accept: () => changeScheduleState(item)
  });
}

function confirmTestSchedule(item: Schedule) {
  if (item.readinessBlockers.length) {
    toast.add({ severity: 'warn', summary: 'ตารางส่งรายงานยังไม่พร้อม', detail: 'แก้ไขรายการความพร้อมก่อนทดสอบส่ง', life: 4500 });
    return;
  }
  confirm.require({
    header: 'ยืนยันส่ง LINE จริง',
    message: `ร้าน ${tenant.value?.name ?? ''} · ตาราง “${item.name}” · ${item.reportKeys.length} รายงาน · ผู้รับ ${item.recipientIds.length} คน ระบบจะดึง SQL ใหม่และส่ง LINE จริง ใช้ quota ของ OA กลาง และไม่เปลี่ยนรอบถัดไป`,
    icon: 'pi pi-send',
    acceptLabel: 'ดึงข้อมูลและส่งจริง',
    rejectLabel: 'ยกเลิก',
    accept: () => testSchedule(item)
  });
}

async function testSchedule(item: Schedule) {
  if (testSendingScheduleId.value) return;
  testSendingScheduleId.value = item.id;
  try {
    const actionKey = testSendActionKeys.get(item.id) ?? newIdempotencyKey('schedule-test-send');
    testSendActionKeys.set(item.id, actionKey);
    const execution = await adminApi.testSendSchedule(tenantId, item.id, actionKey);
    testSendActionKeys.delete(item.id);
    toast.add({ severity: 'success', summary: 'รับคำขอทดสอบส่งแล้ว', detail: `งาน ${execution.id} กำลังดึงข้อมูลและส่งในเบื้องหลัง`, life: 5000 });
  } catch (cause) {
    if (!(cause instanceof ApiError) || !cause.retryable) testSendActionKeys.delete(item.id);
    toast.add({ severity: 'error', summary: 'ทดสอบส่งไม่สำเร็จ', detail: errorMessage(cause), life: 6000 });
  } finally {
    testSendingScheduleId.value = '';
  }
}

function blockerLabel(code: string) {
  return ({ TENANT_INACTIVE: 'ร้านยังไม่เปิดใช้งานหรือหมดอายุ', SML_NOT_READY: 'SML ยังไม่พร้อม', RECIPIENT_NOT_ACTIVE: 'มีผู้รับที่ยังไม่ยืนยัน LINE', RECIPIENT_PERMISSION_MISMATCH: 'สิทธิ์ผู้รับไม่ตรงกับรายงาน', LINE_NOT_CONFIGURED: 'LINE OA ยังไม่พร้อม' } as Record<string, string>)[code] ?? code;
}
watch(inviteLabel, () => { if (!inviting.value) inviteActionKey = ''; });
function beforeUnload(event: BeforeUnloadEvent) { if (hasUnsavedChanges.value) { event.preventDefault(); event.returnValue = ''; } }
onBeforeRouteLeave(() => !hasUnsavedChanges.value || window.confirm('มีข้อมูลที่ยังไม่ได้บันทึก ต้องการออกจากหน้านี้หรือไม่'));
onMounted(() => { window.addEventListener('beforeunload', beforeUnload); void load(); });
onBeforeUnmount(() => {
  window.removeEventListener('beforeunload', beforeUnload);
  if (smlCooldownTimer !== undefined) window.clearInterval(smlCooldownTimer);
});
</script>

<template>
  <div v-if="loading" class="card"><Skeleton height="2rem" width="15rem" class="mb-4" /><Skeleton height="12rem" /></div>
  <Message v-else-if="error" severity="error" :closable="false">{{ error }} <Button label="ลองใหม่" text @click="load" /></Message>
  <template v-else-if="tenant">
    <div class="mb-3 flex flex-wrap items-center justify-between gap-3">
      <div class="flex min-w-0 flex-1 items-center gap-2">
        <Button label="ร้านค้าทั้งหมด" icon="pi pi-arrow-left" text class="entity-back-action -ml-3" @click="router.push('/admin/tenants')" />
        <h1 class="tenant-page-title min-w-0">{{ tenant.name }}</h1>
      </div>
      <div class="flex flex-wrap gap-2"><Tag :severity="tenant.status === 'ACTIVE' ? 'success' : 'secondary'" :value="statusLabel(tenant.status)" /><Tag :severity="sml?.readinessStatus === 'READY' ? 'success' : 'warn'" :value="`SML ${statusLabel(sml?.readinessStatus ?? 'UNCONFIGURED')}`" /></div>
    </div>
    <div class="card"><Tabs v-model:value="activeTab">
      <TabList><Tab value="overview">ข้อมูลร้าน</Tab><Tab value="sml">การเชื่อมต่อ SML</Tab><Tab value="refresh">ความสดและการดึงข้อมูล</Tab><Tab value="recipients">ผู้รับและสิทธิ์</Tab><Tab value="schedules">ตารางส่ง LINE</Tab></TabList>
      <TabPanels>
        <TabPanel value="overview"><form class="grid grid-cols-1 md:grid-cols-2 gap-5 max-w-4xl" @submit.prevent="saveTenant"><div class="grid gap-2"><label for="tenant-name">ชื่อร้าน</label><InputText id="tenant-name" v-model="tenantForm.name" fluid /></div><div class="grid gap-2"><label for="tenant-status">สถานะ</label><Select input-id="tenant-status" aria-label="สถานะ" v-model="tenantForm.status" :options="['ACTIVE','DISABLED','EXPIRED']" fluid /></div><div class="grid gap-2"><label for="tenant-access-end">สิ้นสุดสิทธิ์ (เวลาไทย)</label><DatePicker input-id="tenant-access-end" v-model="tenantForm.accessEndsAt" show-icon show-time hour-format="24" fluid /></div><div class="md:col-span-2 flex items-center gap-3"><Button type="submit" label="บันทึกข้อมูลร้าน" icon="pi pi-save" :loading="savingTenant" :disabled="savingTenant || !tenantDirty" /><small v-if="tenantDirty" class="text-orange-600">มีการแก้ไขที่ยังไม่บันทึก</small></div><div class="md:col-span-2 flex flex-wrap items-center justify-between gap-3 border-t border-surface pt-4"><div><div class="font-semibold">ลิงก์ Dashboard ของร้าน</div><small class="text-muted-color">ลิงก์นี้ไม่โอนหรือเพิ่มสิทธิ์ ผู้รับต้องยืนยัน LINE และได้รับสิทธิ์ร้านนี้อยู่แล้ว</small></div><Button label="คัดลอกลิงก์ Dashboard" icon="pi pi-copy" outlined :disabled="!tenant.viewerUrl" @click="copyDashboardLink" /></div></form><Accordion class="mt-6 max-w-4xl"><AccordionPanel value="technical"><AccordionHeader>ข้อมูลทางเทคนิค</AccordionHeader><AccordionContent><div class="flex flex-wrap items-center gap-3"><div><div class="text-sm text-muted-color">รหัสระบบ</div><code>{{ tenant.slug }}</code></div><Button label="คัดลอกรหัส" icon="pi pi-copy" text @click="copySlug" /></div></AccordionContent></AccordionPanel></Accordion></TabPanel>
        <TabPanel value="sml">
          <Message severity="info" :closable="false" class="mb-5">
            <strong>ตั้งค่า Java Web Service ของร้าน</strong>
            <div class="mt-1">กรอกเฉพาะ Base URL เช่น <code>https://shop.example.com</code> ระบบจะเติม path มาตรฐานให้เอง และไม่แสดงรหัสผ่านหรือ token กลับมา</div>
          </Message>

          <Message :severity="smlStatusSeverity" :closable="false" class="mb-5" role="status">
            <div class="flex items-start gap-2">
              <i :class="smlStatusSeverity === 'success' ? 'pi pi-check-circle mt-1' : smlStatusSeverity === 'error' ? 'pi pi-times-circle mt-1' : 'pi pi-info-circle mt-1'" aria-hidden="true" />
              <div>
                <div class="font-semibold">{{ smlStatusTitle }}</div>
                <div class="mt-1">{{ smlStatusDetail }}</div>
              </div>
            </div>
          </Message>

          <form class="grid grid-cols-1 md:grid-cols-2 gap-5 max-w-4xl" @submit.prevent="saveSML">
            <div class="grid gap-2 md:col-span-2">
              <label for="sml-endpoint">Java Web Service Base URL</label>
              <InputText id="sml-endpoint" v-model="smlForm.endpointUrl" type="url" inputmode="url" autocomplete="url" placeholder="https://shop.example.com" aria-describedby="sml-endpoint-help" fluid />
              <small id="sml-endpoint-help" class="text-muted-color">ใช้ URL สุดท้ายที่ไม่ redirect และไม่ต้องใส่ชื่อฐานข้อมูลหรือชื่อไฟล์ใน URL</small>
              <div v-if="resolvedSMLJavaWSEndpoint" class="flex flex-wrap gap-x-2 gap-y-1 text-sm">
                <span class="font-medium">ปลายทางที่ระบบจะทดสอบ:</span>
                <code class="break-all">{{ resolvedSMLJavaWSEndpoint }}</code>
              </div>
            </div>
            <div class="grid gap-2">
              <label for="sml-config-file">ไฟล์ SMLConfig</label>
              <InputText id="sml-config-file" v-model="smlForm.configFileName" placeholder="SMLConfigDATA.xml" fluid />
              <small class="text-muted-color">ต้องตรงกับไฟล์ที่ Java Web Service ของร้านเข้าถึงได้</small>
            </div>
            <div class="grid gap-2">
              <label for="sml-database">ชื่อฐานข้อมูล SML</label>
              <InputText id="sml-database" v-model="smlForm.databaseName" fluid />
              <small class="text-muted-color">ต้องตรงกับชื่อฐานข้อมูลที่กำหนดไว้ใน SMLConfig</small>
            </div>
            <div class="md:col-span-2 flex flex-wrap items-center gap-3">
              <Button type="submit" label="บันทึกการเชื่อมต่อ" icon="pi pi-save" :loading="savingSML" :disabled="savingSML || !smlDirty" />
              <span v-tooltip.top="smlTestDisabledReason">
                <Button type="button" label="ทดสอบการเชื่อมต่อ" icon="pi pi-bolt" outlined :disabled="smlTestDisabled" :loading="testingSML" @click="testSML" />
              </span>
              <small v-if="smlDirty" class="text-orange-600">บันทึกค่าก่อนทดสอบการเชื่อมต่อ</small>
              <small v-else class="text-muted-color">การทดสอบส่ง <code>select 1</code> แบบอ่านอย่างเดียว ไม่สร้างรายงานและไม่ส่ง LINE</small>
            </div>
          </form>
        </TabPanel>
        <TabPanel value="refresh">
          <Message severity="info" :closable="false" class="mb-5"><strong>กำหนดว่า Snapshot จะถือเป็น “ข้อมูลล่าสุด” ได้นานเท่าไร</strong><div class="mt-2">เมื่อผู้ใช้เปิด Dashboard หลังพ้นช่วงนี้ ระบบจึงอาจเริ่มดึงข้อมูลเบื้องหลัง ค่านี้ไม่ใช่ตารางส่ง LINE และไม่ได้ Query SML ต่อเนื่องตลอดวัน ผู้ใช้ยังกดดึงใหม่เองได้เสมอ</div></Message>
          <Message v-if="refreshPolicy?.rolloutStatus && refreshPolicy.rolloutStatus !== 'ACTIVE'" severity="warn" :closable="false" class="mb-5">การอัปเดตเบื้องหลังยังไม่เปิดใช้กับร้านนี้ใน Production ค่าที่ตั้งไว้จะมีผลเมื่อระบบเปิด feature นี้ แต่การกดดึงข้อมูลด้วยตนเองยังใช้ได้</Message>
          <Message v-if="refreshPolicyConflict" severity="warn" :closable="false" class="mb-5"><strong>ค่าถูกแก้จากอีกหน้าจอ</strong><div class="mt-1">ค่าล่าสุดบนระบบ: {{ refreshPolicyConflict.fastIntervalMinutes ?? 'ปิด' }} / {{ refreshPolicyConflict.standardIntervalMinutes ?? 'ปิด' }} / {{ refreshPolicyConflict.heavyIntervalMinutes ?? 'ปิด' }} นาที ระบบเก็บ draft ของคุณไว้และยังไม่เขียนทับอัตโนมัติ</div></Message>
          <div class="flex flex-wrap gap-3 mb-5"><Button v-for="preset in refreshPresets" :key="preset.label" :label="preset.label" :outlined="activeRefreshPreset !== preset.label" :severity="activeRefreshPreset === preset.label ? 'primary' : 'secondary'" @click="setRefreshPreset(preset.values)" /><Tag v-if="activeRefreshPreset === 'กำหนดเอง'" value="กำหนดเอง" severity="info" /></div>
          <form class="grid grid-cols-1 md:grid-cols-3 gap-5 max-w-5xl" @submit.prevent="saveRefreshPolicy"><div class="grid gap-2"><label for="refresh-fast">ข้อมูลเคลื่อนไหวเร็ว</label><Select input-id="refresh-fast" v-model="refreshPolicyForm.fastIntervalMinutes" :options="fastIntervalOptions" option-label="label" option-value="value" fluid /><small class="text-muted-color">ขาย · รับชำระ · รับเงิน · จ่ายเงิน</small></div><div class="grid gap-2"><label for="refresh-standard">รายงานทั่วไป</label><Select input-id="refresh-standard" v-model="refreshPolicyForm.standardIntervalMinutes" :options="standardIntervalOptions" option-label="label" option-value="value" fluid /><small class="text-muted-color">ซื้อ · กำไร · ลูกหนี้ · จุดสั่งซื้อ</small></div><div class="grid gap-2"><label for="refresh-heavy">รายงานหนัก</label><Select input-id="refresh-heavy" v-model="refreshPolicyForm.heavyIntervalMinutes" :options="heavyIntervalOptions" option-label="label" option-value="value" fluid /><small class="text-muted-color">สต็อกคงเหลือ</small></div><Message v-if="refreshPolicyForm.fastIntervalMinutes == null && refreshPolicyForm.standardIntervalMinutes == null && refreshPolicyForm.heavyIntervalMinutes == null" severity="secondary" :closable="false" class="md:col-span-3">Snapshot ปัจจุบันจะถือว่าเก่าทันที ระบบจะไม่ดึงเบื้องหลัง แต่ผู้ใช้ยังกดดึงใหม่เองได้ และ Snapshot ย้อนหลังยังอยู่ตาม retention เดิม</Message><div class="md:col-span-3 flex flex-wrap items-center gap-3"><Button type="submit" label="บันทึกความสดของข้อมูล" icon="pi pi-save" :loading="savingRefreshPolicy" :disabled="savingRefreshPolicy || !refreshPolicyDirty" /><small v-if="refreshPolicyDirty" class="text-orange-600">มีค่าที่ยังไม่ได้บันทึก</small></div></form>
        </TabPanel>
        <TabPanel value="recipients">
          <Message severity="info" :closable="false" class="mb-5"><strong>ขั้นที่ 1: ให้สิทธิ์เปิดดู Dashboard แก่ผู้รับ</strong><div class="mt-1">สิทธิ์กำหนดเพดานว่าผู้รับเปิดรายงานใดได้ ส่วนรายงานที่จะส่งจริงเลือกอีกครั้งใน “ตารางส่ง LINE”</div></Message>
          <Toolbar class="mb-5 border-0 p-0"><template #start><div><h2 class="text-lg font-semibold m-0">ผู้รับและสิทธิ์เปิดดู Dashboard</h2><p class="text-muted-color mt-1 mb-0">ผู้รับที่รอยืนยันสามารถออกลิงก์เชิญใหม่ได้ ส่วนผู้รับที่ยืนยันแล้วสามารถส่งลิงก์ Dashboard เดิมให้เข้าใช้งานอีกครั้ง</p></div></template><template #end><Button label="เพิ่มผู้รับ LINE" icon="pi pi-user-plus" @click="inviteOpen = true; inviteURL = ''; inviteLabel = ''" /></template></Toolbar>
          <Message v-if="recipientTable.error.value" severity="error" :closable="false" class="mb-4">โหลดข้อมูลใหม่ไม่สำเร็จ ข้อมูลเดิมยังแสดงอยู่ · {{ recipientTable.error.value }}</Message>
          <DataTable v-model:filters="recipientPrimeFilters" :value="recipients" :loading="recipientTable.loading.value" data-key="id" lazy paginator :first="recipientTable.page.value * recipientTable.pageSize.value" :rows="recipientTable.pageSize.value" :total-records="recipientTable.total.value" :rows-per-page-options="[25, 50, 100]" filter-display="menu" row-hover show-gridlines striped-rows scrollable current-page-report-template="หน้า {currentPage} จาก {totalPages} · ทั้งหมด {totalRecords} รายการ" paginator-template="RowsPerPageDropdown FirstPageLink PrevPageLink PageLinks NextPageLink LastPageLink CurrentPageReport" @page="recipientTable.changePage" @filter="applyRecipientFilters">
            <template #header><SakaiTableHeader v-model:global-search="recipientTable.globalSearch.value" :loading="recipientTable.loading.value" :has-filters="recipientHasFilters" @clear="clearRecipientFilters"><template #start><Button label="รีเฟรช" icon="pi pi-refresh" outlined :loading="recipientTable.loading.value" @click="recipientTable.refresh()" /></template></SakaiTableHeader></template>
            <Column field="displayName" header="ชื่อ" />
            <Column field="status" header="สถานะ" :show-filter-match-modes="false"><template #body="{ data }"><Tag :severity="data.status === 'ACTIVE' ? 'success' : 'warn'" :value="statusLabel(data.status)" /></template><template #filter="{ filterModel }"><MultiSelect v-model="filterModel.value" :options="[{ label: 'รอยืนยัน LINE', value: 'PENDING' }, { label: 'ใช้งาน', value: 'ACTIVE' }]" option-label="label" option-value="value" placeholder="ทุกสถานะ" /></template></Column>
            <Column field="permissionState" header="สิทธิ์เปิดดู" header-class="table-number-column" body-class="table-number-column" :show-filter-match-modes="false"><template #body="{ data }"><span>{{ data.reportKeys.length }} รายงาน</span></template><template #filter="{ filterModel }"><MultiSelect v-model="filterModel.value" :options="[{ label: 'กำหนดรายงานแล้ว', value: 'WITH_REPORTS' }, { label: 'ยังไม่กำหนดรายงาน', value: 'WITHOUT_REPORTS' }]" option-label="label" option-value="value" placeholder="ทุกสิทธิ์" /></template></Column>
            <Column field="verifiedAt" header="ยืนยันเมื่อ"><template #body="{ data }">{{ formatDateTime(data.verifiedAt) }}</template></Column>
            <Column header="การจัดการ" header-class="table-action-column" body-class="table-action-column"><template #body="{ data }"><div class="flex flex-wrap items-center justify-end gap-1">
              <Button label="กำหนดสิทธิ์" icon="pi pi-lock" text class="touch-action" :disabled="!!revokingRecipientId || !!reissuingRecipientId" @click="editPermissions(data)" />
              <Button v-if="data.status === 'PENDING'" label="สร้างลิงก์ใหม่" icon="pi pi-link" text class="touch-action" :loading="reissuingRecipientId === data.id" :disabled="!!revokingRecipientId || (!!reissuingRecipientId && reissuingRecipientId !== data.id)" @click="confirmReissueInvitation(data)" />
              <Button v-else-if="data.status === 'ACTIVE'" label="คัดลอกลิงก์ Dashboard" icon="pi pi-copy" text class="touch-action" :disabled="!tenant?.viewerUrl || !!revokingRecipientId || !!reissuingRecipientId" @click="copyDashboardLink" />
              <Button :label="data.status === 'PENDING' ? 'ยกเลิกคำเชิญ' : 'ลบผู้รับ'" icon="pi pi-trash" severity="danger" text class="touch-action" :loading="revokingRecipientId === data.id" :disabled="!!reissuingRecipientId || (!!revokingRecipientId && revokingRecipientId !== data.id)" @click="confirmRevokeRecipient(data)" />
            </div></template></Column>
            <template #empty><div class="py-8 text-center text-muted-color">ไม่พบผู้รับที่ตรงกับตัวกรอง <Button v-if="recipientHasFilters" label="ล้างตัวกรอง" text size="small" @click="clearRecipientFilters" /></div></template>
          </DataTable>
        </TabPanel>
        <TabPanel value="schedules">
          <Message severity="info" :closable="false" class="mb-5"><strong>ขั้นที่ 2–4: เลือกรายงานใน LINE รอบนี้ → เลือกผู้รับ → ตั้งวันเวลา → ตรวจสอบและเปิดใช้งาน</strong><div class="mt-1">การเลือกรายงานในตารางส่งไม่เพิ่มสิทธิ์ ผู้รับทุกคนต้องมีสิทธิ์เปิดดูรายงานนั้นจากแท็บ “ผู้รับและสิทธิ์” ก่อน</div></Message>
          <Toolbar class="mb-5 border-0 p-0">
            <template #start><p class="m-0 text-muted-color">หนึ่ง LINE Card รองรับสูงสุด 10 รายงาน รายงานละ 2 ตัวเลขสำคัญ</p></template>
            <template #end><Button label="เพิ่มตารางส่งรายงาน" icon="pi pi-calendar-plus" @click="openSchedule()" /></template>
          </Toolbar>
          <Message v-if="scheduleTable.error.value" severity="error" :closable="false" class="mb-4">โหลดข้อมูลใหม่ไม่สำเร็จ ข้อมูลเดิมยังแสดงอยู่ · {{ scheduleTable.error.value }}</Message>
          <DataTable v-model:filters="schedulePrimeFilters" :value="schedules" :loading="scheduleTable.loading.value" data-key="id" lazy paginator :first="scheduleTable.page.value * scheduleTable.pageSize.value" :rows="scheduleTable.pageSize.value" :total-records="scheduleTable.total.value" :rows-per-page-options="[25, 50, 100]" filter-display="menu" row-hover show-gridlines striped-rows scrollable current-page-report-template="หน้า {currentPage} จาก {totalPages} · ทั้งหมด {totalRecords} รายการ" paginator-template="RowsPerPageDropdown FirstPageLink PrevPageLink PageLinks NextPageLink LastPageLink CurrentPageReport" @page="scheduleTable.changePage" @filter="applyScheduleFilters">
            <template #header><SakaiTableHeader v-model:global-search="scheduleTable.globalSearch.value" :loading="scheduleTable.loading.value" :has-filters="scheduleHasFilters" @clear="clearScheduleFilters"><template #start><div class="flex items-center gap-2"><Checkbox v-model="showArchived" binary input-id="show-archived-schedules" @change="toggleArchivedSchedules" /><label for="show-archived-schedules">รวมรายการที่ลบแล้ว</label></div></template></SakaiTableHeader></template>
            <Column field="name" header="ชื่อตาราง"><template #body="{ data }"><span class="font-medium">{{ data.name }}</span><div class="text-xs text-muted-color mt-1">{{ data.localTime }} · เวลาไทย</div></template></Column>
            <Column field="status" header="สถานะ" :show-filter-match-modes="false"><template #body="{ data }"><Tag :severity="data.status === 'ACTIVE' ? 'success' : data.status === 'PAUSED' ? 'warn' : 'secondary'" :value="statusLabel(data.status)" /></template><template #filter="{ filterModel }"><MultiSelect v-model="filterModel.value" :options="[{ label: 'ฉบับร่าง', value: 'DRAFT' }, { label: 'ใช้งาน', value: 'ACTIVE' }, { label: 'พักไว้', value: 'PAUSED' }, { label: 'หมดอายุ', value: 'EXPIRED' }, { label: 'ลบแล้ว', value: 'ARCHIVED' }]" option-label="label" option-value="value" placeholder="ทุกสถานะ" /></template></Column>
            <Column header="รายงาน" header-class="table-number-column" body-class="table-number-column"><template #body="{ data }">{{ data.reportKeys.length }}</template></Column>
            <Column header="ผู้รับ" header-class="table-number-column" body-class="table-number-column"><template #body="{ data }">{{ data.recipientIds.length }}</template></Column>
            <Column header="รอบถัดไป"><template #body="{ data }">{{ data.status === 'ARCHIVED' ? '—' : formatDateTime(data.nextOccurrences[0]) }}</template></Column>
            <Column header="ความพร้อม"><template #body="{ data }"><Tag v-if="data.status === 'ARCHIVED'" severity="secondary" value="เก็บประวัติไว้" /><div v-else-if="data.readinessBlockers.length" class="flex flex-wrap gap-1"><Tag v-for="code in data.readinessBlockers" :key="code" severity="danger" :value="blockerLabel(code)" /></div><Tag v-else severity="success" value="พร้อม" /></template></Column>
            <Column header="" header-class="table-action-column" body-class="table-action-column"><template #body="{ data }"><div class="flex justify-end">
              <Button v-if="data.status === 'ARCHIVED'" icon="pi pi-refresh" text rounded class="touch-action" severity="info" aria-label="กู้คืนเป็นฉบับร่าง" v-tooltip.top="'กู้คืนเป็นฉบับร่าง โดยยังไม่เปิดส่ง'" :loading="changingScheduleId === data.id" :disabled="!!changingScheduleId" @click="confirmRestoreSchedule(data)" />
              <template v-else>
                <Button icon="pi pi-pencil" text rounded class="touch-action" aria-label="แก้ไข" v-tooltip.top="data.status === 'ACTIVE' ? 'พักตารางก่อนแก้ไข' : 'แก้ไขตาราง'" :disabled="data.status === 'ACTIVE' || !!changingScheduleId" @click="openSchedule(data)" />
                <Button icon="pi pi-send" text rounded class="touch-action" severity="info" aria-label="ทดสอบส่ง LINE" v-tooltip.top="data.readinessBlockers.length ? 'แก้ไขความพร้อมก่อนทดสอบส่ง' : 'ดึง SQL ใหม่และส่ง LINE จริง'" :disabled="data.readinessBlockers.length > 0 || !!testSendingScheduleId || !!changingScheduleId" :loading="testSendingScheduleId === data.id" @click="confirmTestSchedule(data)" />
                <Button :icon="data.status === 'ACTIVE' ? 'pi pi-pause' : 'pi pi-play'" text rounded class="touch-action" :severity="data.status === 'ACTIVE' ? 'warn' : 'success'" :aria-label="data.status === 'ACTIVE' ? 'พัก' : 'เปิด'" :loading="changingScheduleId === data.id" :disabled="!!changingScheduleId || !!testSendingScheduleId || (data.status !== 'ACTIVE' && data.readinessBlockers.length > 0)" @click="confirmScheduleState(data)" />
                <Button icon="pi pi-trash" text rounded class="touch-action" severity="danger" aria-label="ลบตารางส่งรายงาน" v-tooltip.top="data.status === 'ACTIVE' ? 'พักตารางก่อนลบ' : 'ลบตารางและหยุดการส่งในอนาคต'" :loading="changingScheduleId === data.id" :disabled="data.status === 'ACTIVE' || !!changingScheduleId || !!testSendingScheduleId" @click="confirmArchiveSchedule(data)" />
              </template>
            </div></template></Column>
            <template #empty><div class="py-8 text-center text-muted-color">ไม่พบตารางส่งรายงานที่ตรงกับตัวกรอง <Button v-if="scheduleHasFilters" label="ล้างตัวกรอง" text size="small" @click="clearScheduleFilters" /></div></template>
          </DataTable>
        </TabPanel>
      </TabPanels>
    </Tabs></div>
  </template>

  <Dialog v-model:visible="inviteOpen" modal :header="inviteURL ? 'ลิงก์เชิญผู้รับ LINE' : 'เพิ่มผู้รับ LINE'" class="responsive-dialog" :style="{ width: '32rem' }"><div v-if="!inviteURL" class="grid gap-3"><label for="recipient-invite-label">ชื่อสำหรับระบุผู้รับ</label><InputText id="recipient-invite-label" v-model="inviteLabel" placeholder="เช่น เจ้าของร้าน หรือ ผู้จัดการ" fluid /><small class="text-muted-color">หลังผู้รับเปิดลิงก์และยืนยัน LINE ระบบจะแทนชื่อนี้ด้วยชื่อจากบัญชี LINE</small></div><div v-else><Message severity="success" :closable="false">ส่งลิงก์นี้ให้ผู้รับที่ต้องการเท่านั้น ลิงก์มีอายุ 7 วันและใช้ได้หนึ่งครั้ง หากสร้างลิงก์ใหม่อีกครั้ง ลิงก์นี้จะใช้ไม่ได้ทันที</Message><InputGroup class="mt-4"><InputText :model-value="inviteURL" aria-label="ลิงก์คำเชิญ" readonly /><Button icon="pi pi-copy" aria-label="คัดลอก" @click="copyInvite" /></InputGroup></div><template #footer><Button label="ปิด" text @click="inviteOpen = false" /><Button v-if="!inviteURL" label="สร้างลิงก์เพิ่มผู้รับ" icon="pi pi-link" :loading="inviting" :disabled="!inviteLabel.trim()" @click="invite" /></template></Dialog>
</template>

<style scoped>
.tenant-page-title {
  margin: 0;
  font-size: 1.75rem;
  font-weight: 600;
  line-height: 1.25;
  overflow-wrap: anywhere;
}

@media (max-width: 991px) {
  .tenant-page-title {
    font-size: 1.5rem;
    line-height: 1.3;
  }
}
</style>
