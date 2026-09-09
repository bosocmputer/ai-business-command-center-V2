<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useConfirm } from 'primevue/useconfirm';
import { useToast } from 'primevue/usetoast';
import ExecutiveChart from '@/components/dashboard/ExecutiveChart.vue';
import ReportPeriodToolbar from '@/components/dashboard/ReportPeriodToolbar.vue';
import { ApiError, viewerApi, type DashboardRefresh, type DashboardRefreshInput, type DashboardRefreshResult, type DashboardSnapshot, type ExecutiveOverview, type ReportKey } from '@/api';
import { newIdempotencyKey } from '@/api/client';
import { useViewerSession } from '@/stores/viewer';
import { buildExecutiveKpis, comparisonPeriodText, executiveFeaturedVisualizationKeys, formatDashboardValue, formatPeriodRange, snapshotForReport } from '@/utils/dashboard';
import { errorMessage, formatSourceCollection } from '@/utils/format';
import { freshnessPresentation, progressPresentation } from '@/utils/freshness';
import { selectionFromReportPeriod, selectionLabel, type ReportPeriodSelection } from '@/utils/reportPeriod';

const route = useRoute();
const router = useRouter();
const confirm = useConfirm();
const toast = useToast();
const { state, ensureReports, selectTenant, periodSelection, setPeriodSelection } = useViewerSession();
const overview = ref<ExecutiveOverview>();
const refresh = ref<DashboardRefresh>();
const refreshFailures = ref<DashboardRefreshResult['failures']>([]);
const selectedPeriod = ref<ReportPeriodSelection>({ periodPreset: 'MONTH_TO_DATE' });
const displayedPeriodLabel = ref('ข้อมูลล่าสุดแต่ละรายงาน');
const exactRefreshLoaded = ref(false);
const loading = ref(true);
const lookingUp = ref(false);
const refreshing = ref(false);
const cacheRevalidating = ref(false);
const cacheRevalidationMessage = ref('');
const refreshConfirmOpen = ref(false);
const error = ref('');
let pollTimer: number | undefined;
let pollCount = 0;
let generation = 0;
let pollDeadline = 0;
let pollInFlight = false;
let pageController: AbortController | undefined;
let pollController: AbortController | undefined;
let refreshAction: { fingerprint: string; key: string } | undefined;
let revalidationTimer: number | undefined;
let revalidationDeadline = 0;

const tenantId = computed(() => String(route.params.tenantId));
const tenant = computed(() => state.tenants.find((item) => item.id === tenantId.value));
const hasReportPermissions = computed(() => (tenant.value?.reportKeys.length ?? 0) > 0);
const reports = computed(() => state.reportsByTenant[tenantId.value] ?? []);
const snapshots = computed(() => overview.value?.items ?? []);
const kpis = computed(() => buildExecutiveKpis(snapshots.value));
const missingReportCount = computed(() => Math.max(0, reports.value.length - new Set(snapshots.value.map((item) => item.dashboard.reportKey)).size));
const featuredCharts = computed(() => {
  const choices = Object.entries(executiveFeaturedVisualizationKeys) as Array<[ReportKey, string]>;
  const selected: { snapshot: DashboardSnapshot; visualization: DashboardSnapshot['dashboard']['visualizations'][number] }[] = [];
  for (const [reportKey, preferredKey] of choices) {
    const snapshot = snapshotForReport(snapshots.value, reportKey);
    const visualization = snapshot?.dashboard.visualizations.find((item) => item.key === preferredKey) ?? snapshot?.dashboard.visualizations[0];
    if (snapshot && visualization) selected.push({ snapshot, visualization });
  }
  if (selected.length < 2) {
    for (const snapshot of snapshots.value) {
      const visualization = snapshot.dashboard.visualizations[0];
      if (visualization && !selected.some((item) => item.visualization.key === visualization.key)) selected.push({ snapshot, visualization });
      if (selected.length >= 4) break;
    }
  }
  return selected.slice(0, 4);
});
const refreshPercent = computed(() => refresh.value?.total ? Math.round(((refresh.value.completed + refresh.value.failed) / refresh.value.total) * 100) : 0);
const refreshCompletedLabel = computed(() => refresh.value?.completed ?? 0);
const refreshTotalLabel = computed(() => refresh.value?.total ?? reports.value.length);
const activeRefreshRun = computed(() => refresh.value?.runs.find((item) => ['CLAIMED', 'RUNNING'].includes(item.status)) ?? refresh.value?.runs.find((item) => item.status === 'QUEUED'));
const activeRefreshLabel = computed(() => activeRefreshRun.value
  ? `${progressPresentation(activeRefreshRun.value.progressPhase)} · ${reports.value.find((item) => item.reportKey === activeRefreshRun.value?.reportKey)?.label ?? activeRefreshRun.value.reportKey}`
  : 'กำลังเตรียมรายงาน');
const activeRefreshTypicalDuration = computed(() => {
  const item = activeRefreshRun.value;
  if (!item || (item.expectedSampleCount ?? 0) < 5 || !item.expectedP50Ms || !item.expectedP90Ms) return '';
  const lower = Math.max(1, Math.round(item.expectedP50Ms / 1000));
  const upper = Math.max(lower, Math.round(item.expectedP90Ms / 1000));
  return `โดยปกติประมาณ ${lower.toLocaleString('th-TH')}–${upper.toLocaleString('th-TH')} วินาที`;
});
const failedReportLabels = computed(() => refreshFailures.value.map((failure) => reports.value.find((report) => report.reportKey === failure.reportKey)?.label ?? failure.reportKey));

async function initializeOverview() {
  const context = ++generation;
  const selectedTenantId = tenantId.value;
  pageController?.abort('new-overview'); pageController = new AbortController(); stopPolling(); stopRevalidation();
  loading.value = true; lookingUp.value = false; refreshing.value = false; error.value = ''; refreshFailures.value = []; exactRefreshLoaded.value = false;
  try {
    selectTenant(selectedTenantId);
    if (!hasReportPermissions.value) {
      overview.value = undefined;
      refresh.value = undefined;
      displayedPeriodLabel.value = 'ยังไม่ได้กำหนดสิทธิ์รายงาน';
      return;
    }
    await ensureReports(selectedTenantId, true, pageController.signal);
    selectedPeriod.value = periodSelection(selectedTenantId);
    const refreshId = validUUID(route.query.refreshId);
    if (route.query.refreshId !== undefined && !refreshId) {
      await removeRefreshReference();
      error.value = 'ลิงก์ชุดข้อมูลไม่ถูกต้อง กำลังแสดงข้อมูลล่าสุดแทน';
    }
    if (refreshId) {
      refresh.value = await viewerApi.dashboardRefresh(selectedTenantId, refreshId, pageController.signal);
      if (!isCurrent(context, selectedTenantId)) return;
      if (isTerminal(refresh.value.status)) await loadRefreshResult(context, selectedTenantId, refreshId, pageController.signal);
      else { refreshing.value = true; pollDeadline = Date.now() + 12 * 60_000; schedulePoll(context); }
    } else {
      await loadLatestOverview(context, selectedTenantId, pageController.signal);
    }
  } catch (cause) {
    if (!isCancelled(cause) && context === generation) {
      if (cause instanceof ApiError && cause.status === 404 && route.query.refreshId !== undefined) {
        await removeRefreshReference();
        try {
          await loadLatestOverview(context, selectedTenantId, pageController.signal);
          error.value = 'ชุดข้อมูลเดิมไม่พร้อมใช้งานแล้ว กำลังแสดง Snapshot ของช่วงที่เลือกแทน';
        } catch (fallbackCause) { error.value = errorMessage(fallbackCause); }
      } else error.value = errorMessage(cause);
    }
  }
  finally { if (context === generation) loading.value = false; }
}

function overviewInput(selection: ReportPeriodSelection): DashboardRefreshInput {
  return {
    periodPreset: selection.periodPreset,
    reportKeys: reports.value.map((report) => report.reportKey),
    ...(selection.periodPreset === 'CUSTOM' ? { dateFrom: selection.dateFrom, dateTo: selection.dateTo } : {})
  };
}

async function loadLatestOverview(context: number, selectedTenantId: string, signal?: AbortSignal) {
  const result = await viewerApi.overview(selectedTenantId, signal);
  if (!isCurrent(context, selectedTenantId)) return;
  overview.value = result;
  exactRefreshLoaded.value = false;
  displayedPeriodLabel.value = result.items.length ? 'ข้อมูลล่าสุดแต่ละรายงาน' : 'ยังไม่มี Snapshot';
  const needsRevalidation = missingReportCount.value > 0 || result.items.some((item) => !item.freshnessStatus || item.freshnessStatus !== 'FRESH');
  if (needsRevalidation) {
    revalidationDeadline = Date.now() + 12 * 60_000;
    void revalidateCachedOverview(context, selectedTenantId);
  }
}

async function revalidateCachedOverview(context: number, selectedTenantId: string) {
  if (!isCurrent(context, selectedTenantId) || refreshing.value || Date.now() >= revalidationDeadline) {
    stopRevalidation();
    return;
  }
  try {
    const result = await viewerApi.revalidateOverview(selectedTenantId, overviewInput(selectedPeriod.value), pageController?.signal);
    if (!isCurrent(context, selectedTenantId) || refreshing.value) return;
    const overviewItems = result.overview.items ?? [];
    const runs = result.runs ?? [];
    const exactComplete = overviewItems.length === reports.value.length && overviewItems.every((item) => item.freshnessStatus === 'FRESH');
    if (exactComplete) {
      overview.value = { ...result.overview, items: overviewItems };
      displayedPeriodLabel.value = selectionLabel(selectedPeriod.value, 'SMART_OVERVIEW');
      cacheRevalidationMessage.value = 'อัปเดต Snapshot ทั้งชุดแล้ว';
      stopRevalidation(false);
      return;
    }
    if (result.disposition === 'CIRCUIT_OPEN') {
      cacheRevalidationMessage.value = 'พักการดึงข้อมูลชั่วคราวเพื่อป้องกัน Query ซ้อนใน SML';
      stopRevalidation(false);
      return;
    }
    if (result.disposition === 'DISABLED' || !runs.length) {
      stopRevalidation();
      return;
    }
    const terminalRuns = runs.filter((item) => ['SUCCEEDED', 'FAILED', 'CANCELLED', 'EXPIRED'].includes(item.status));
    if (terminalRuns.length === runs.length && terminalRuns.some((item) => item.status !== 'SUCCEEDED')) {
      cacheRevalidationMessage.value = 'อัปเดตอัตโนมัติไม่สำเร็จ กำลังแสดง Snapshot เดิม';
      stopRevalidation(false);
      return;
    }
    cacheRevalidating.value = true;
    cacheRevalidationMessage.value = 'กำลังอัปเดต Cache เบื้องหลัง · ข้อมูลบนหน้าจอยังเป็น Snapshot เดิม';
    revalidationTimer = window.setTimeout(() => void revalidateCachedOverview(context, selectedTenantId), document.visibilityState === 'hidden' ? 15_000 : 5_000);
  } catch (cause) {
    if (!isCancelled(cause) && isCurrent(context, selectedTenantId)) {
      cacheRevalidationMessage.value = 'ตรวจสอบข้อมูลใหม่ไม่สำเร็จ กำลังแสดง Snapshot เดิม';
      stopRevalidation(false);
    }
  }
}

async function startRefresh(selection: ReportPeriodSelection) {
  if (refreshing.value) return;
  const context = generation;
  const selectedTenantId = tenantId.value;
  stopPolling(); stopRevalidation(); refreshing.value = true; error.value = ''; pollCount = 0;
  const input: DashboardRefreshInput = {
    periodPreset: selection.periodPreset,
    reportKeys: reports.value.map((report) => report.reportKey),
    ...(selection.periodPreset === 'CUSTOM' ? { dateFrom: selection.dateFrom, dateTo: selection.dateTo } : {})
  };
  const fingerprint = JSON.stringify([selectedTenantId, input]);
  if (!refreshAction || refreshAction.fingerprint !== fingerprint) refreshAction = { fingerprint, key: newIdempotencyKey('dashboard-refresh') };
  try {
    const created = await viewerApi.createDashboardRefresh(selectedTenantId, input, refreshAction.key);
    if (!isCurrent(context, selectedTenantId)) return;
    selectedPeriod.value = { ...selection }; setPeriodSelection(selectedTenantId, selection);
    refresh.value = created; refreshFailures.value = []; exactRefreshLoaded.value = false; refreshAction = undefined;
    await router.replace({ path: route.path, query: { ...route.query, refreshId: created.id }, hash: route.hash });
    pollDeadline = Date.now() + 12 * 60_000; schedulePoll(context);
  } catch (cause) {
    if (!isCancelled(cause) && context === generation) { refreshing.value = false; error.value = errorMessage(cause); }
    if (!(cause instanceof ApiError) || !cause.retryable) refreshAction = undefined;
  }
}

async function viewOverviewPeriod(selection: ReportPeriodSelection) {
  if (loading.value || lookingUp.value || refreshing.value) return;
  const context = ++generation;
  const selectedTenantId = tenantId.value;
  pageController?.abort('new-overview-period'); pageController = new AbortController(); stopPolling();
  selectedPeriod.value = { ...selection }; setPeriodSelection(selectedTenantId, selection);
  lookingUp.value = true; error.value = ''; refreshFailures.value = [];
  try {
    const result = await viewerApi.exactOverview(selectedTenantId, overviewInput(selection), pageController.signal);
    if (!isCurrent(context, selectedTenantId)) return;
    selectedPeriod.value = { ...selection }; setPeriodSelection(selectedTenantId, selection);
    overview.value = result;
    exactRefreshLoaded.value = true;
    displayedPeriodLabel.value = result.items.length ? selectionLabel(selection, 'SMART_OVERVIEW') : 'ยังไม่มี Snapshot สำหรับช่วงนี้';
  }
  catch (cause) { if (!isCancelled(cause) && isCurrent(context, selectedTenantId)) error.value = errorMessage(cause); }
  finally { if (isCurrent(context, selectedTenantId)) lookingUp.value = false; }
}

function requestRefresh(selection: ReportPeriodSelection) {
  if (loading.value || lookingUp.value || refreshing.value || refreshConfirmOpen.value || !reports.value.length) return;
  const expectedTenantId = tenantId.value;
  const expectedGeneration = generation;
  const expectedReportCount = reports.value.length;
  const expectedReportKeys = reports.value.map((report) => report.reportKey).sort().join('\u0000');
  const expectedTenantName = tenant.value?.name ?? 'ร้านนี้';
  const expectedSelection = JSON.stringify(selection);
  refreshConfirmOpen.value = true;
  confirm.require({
    header: `อัปเดตข้อมูล ${expectedTenantName}`,
    message: `ช่วงที่จะดึง: ${selectionLabel(selection, 'SMART_OVERVIEW')} · ระบบจะดึง SQL ใหม่ ${expectedReportCount.toLocaleString('th-TH')} รายงาน และอาจใช้เวลาหลายนาที`,
    icon: 'pi pi-database',
    blockScroll: true,
    defaultFocus: 'reject',
    rejectLabel: 'ยกเลิก',
    acceptLabel: 'ดึง SQL ใหม่',
    rejectProps: { severity: 'secondary', text: true },
    accept: () => {
      refreshConfirmOpen.value = false;
      const currentReportKeys = reports.value.map((report) => report.reportKey).sort().join('\u0000');
      if (expectedGeneration !== generation || expectedTenantId !== tenantId.value || expectedReportCount !== reports.value.length || expectedReportKeys !== currentReportKeys || expectedSelection !== JSON.stringify(selection)) {
        toast.add({ severity: 'warn', summary: 'บริบทของร้านเปลี่ยนแล้ว', detail: 'กรุณาตรวจสอบร้านและกดอัปเดตอีกครั้ง', life: 4000 });
        return;
      }
      void startRefresh(selection);
    },
    reject: () => { refreshConfirmOpen.value = false; },
    onHide: () => { refreshConfirmOpen.value = false; }
  });
}

function schedulePoll(context: number) {
  stopPolling();
  if (document.visibilityState === 'hidden') return;
  const delay = pollCount < 15 ? 2000 : 5000;
  pollTimer = window.setTimeout(() => void pollRefresh(context), delay);
}

async function pollRefresh(context: number) {
  if (!refresh.value || pollInFlight || context !== generation) return;
  if (Date.now() >= pollDeadline) { refreshing.value = false; error.value = 'การอัปเดตใช้เวลานานกว่าปกติ กรุณากลับมาตรวจสอบอีกครั้ง'; return; }
  const selectedTenantId = tenantId.value; const refreshId = refresh.value.id;
  pollInFlight = true; pollController = new AbortController();
  try {
    const latest = await viewerApi.dashboardRefresh(selectedTenantId, refreshId, pollController.signal);
    if (context !== generation || selectedTenantId !== tenantId.value || refresh.value?.id !== refreshId) return;
    refresh.value = latest; pollCount++;
    if (isTerminal(refresh.value.status)) {
      refreshing.value = false; await loadRefreshResult(context, selectedTenantId, refreshId, pollController.signal); return;
    }
    schedulePoll(context);
  } catch (cause) { if (!isCancelled(cause) && context === generation) { refreshing.value = false; error.value = errorMessage(cause); } }
  finally { pollInFlight = false; pollController = undefined; }
}

async function loadRefreshResult(context: number, selectedTenantId: string, refreshId: string, signal?: AbortSignal) {
  const result = await viewerApi.dashboardRefreshResult(selectedTenantId, refreshId, signal);
  if (!isCurrent(context, selectedTenantId) || refresh.value?.id !== refreshId) return;
  refreshFailures.value = result.failures;
  if (result.items.length) {
    overview.value = {
      tenantId: selectedTenantId, timezone: 'Asia/Bangkok', items: result.items,
      generationId: result.generationId, generationKey: result.generationKey,
      sourceConsistency: result.sourceConsistency, sourceStartedAt: result.sourceStartedAt,
      sourceFinishedAt: result.sourceFinishedAt, publishedAt: result.publishedAt,
      dataStatus: 'FRESH'
    };
    exactRefreshLoaded.value = true;
    const inferred = inferSelection(result);
    if (inferred) { selectedPeriod.value = inferred; setPeriodSelection(selectedTenantId, inferred); }
    const onlyCurrent = result.items.length > 0 && result.items.every((snapshot) => reports.value.find((report) => report.reportKey === snapshot.dashboard.reportKey)?.periodMode === 'CURRENT_ONLY');
    displayedPeriodLabel.value = onlyCurrent ? 'สถานะปัจจุบัน' : selectionLabel(inferred ?? selectedPeriod.value, 'SMART_OVERVIEW');
  }
  if (result.status === 'FAILED') error.value = overview.value ? 'อัปเดตไม่สำเร็จ กำลังแสดงข้อมูลก่อนหน้า' : 'อัปเดตข้อมูลไม่สำเร็จ กรุณาลองใหม่';
}

function inferSelection(result: DashboardRefreshResult): ReportPeriodSelection | undefined {
  for (const preferredMode of ['DATE_RANGE', 'AS_OF_DATE'] as const) {
    const item = result.items.find((snapshot) => reports.value.find((report) => report.reportKey === snapshot.dashboard.reportKey)?.periodMode === preferredMode);
    if (item) return selectionFromReportPeriod(preferredMode, item.dashboard.period);
  }
}

function openReport(reportKey: ReportKey) {
  const snapshot = exactRefreshLoaded.value ? snapshotForReport(snapshots.value, reportKey) : undefined;
  void router.push({ path: `/app/tenant/${tenantId.value}/report/${reportKey}`, query: snapshot ? { runId: snapshot.runId } : undefined });
}
function snapshotFreshness(snapshot: DashboardSnapshot) { return freshnessPresentation(snapshot.freshnessStatus); }
function snapshotSource(snapshot: DashboardSnapshot) { return formatSourceCollection(snapshot.sourceStartedAt, snapshot.sourceFinishedAt ?? snapshot.dashboard.generatedAt, snapshot.sourceConsistency); }
function isTerminal(status: DashboardRefresh['status']) { return status === 'SUCCEEDED' || status === 'PARTIAL' || status === 'FAILED'; }
function isCurrent(context: number, selectedTenantId: string) { return context === generation && selectedTenantId === tenantId.value; }
function validUUID(value: unknown): string | undefined { return typeof value === 'string' && /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(value) ? value : undefined; }
async function removeRefreshReference() { const query = { ...route.query }; delete query.refreshId; await router.replace({ path: route.path, query, hash: route.hash }); }
function stopPolling() { if (pollTimer) window.clearTimeout(pollTimer); pollTimer = undefined; pollController?.abort('poll-stopped'); pollController = undefined; }
function stopRevalidation(clearMessage = true) {
  if (revalidationTimer) window.clearTimeout(revalidationTimer);
  revalidationTimer = undefined;
  cacheRevalidating.value = false;
  if (clearMessage) cacheRevalidationMessage.value = '';
}
function isCancelled(cause: unknown) { return cause instanceof ApiError && cause.code === 'CANCELLED'; }
function handleVisibilityChange() {
  if (document.visibilityState === 'visible' && refreshing.value && refresh.value && !pollInFlight) {
    if (pollTimer) window.clearTimeout(pollTimer);
    pollTimer = undefined; void pollRefresh(generation);
  }
}

onMounted(() => { document.addEventListener('visibilitychange', handleVisibilityChange); void initializeOverview(); });
onBeforeUnmount(() => { generation++; refreshConfirmOpen.value = false; confirm.close(); document.removeEventListener('visibilitychange', handleVisibilityChange); pageController?.abort('unmounted'); stopPolling(); stopRevalidation(); });
watch(tenantId, () => { refresh.value = undefined; refreshing.value = false; void initializeOverview(); });
</script>

<template>
  <AppPageHeader :title="`ภาพรวมร้าน ${tenant?.name ?? ''}`" desktop-mode="viewerCompact" />

  <section v-if="!hasReportPermissions" class="card executive-panel permission-empty-state" aria-labelledby="permission-empty-title">
    <span class="permission-empty-icon"><i class="pi pi-check-circle" /></span>
    <div>
      <h2 id="permission-empty-title">เข้าร่วมร้านนี้เรียบร้อยแล้ว</h2>
      <Tag severity="warn" value="ยังไม่ได้กำหนดสิทธิ์รายงาน" />
      <p>บัญชี LINE ของคุณเชื่อมกับร้านนี้สำเร็จแล้ว แต่ผู้ดูแลยังไม่ได้เลือกรายงานที่เปิดดูได้</p>
      <p class="permission-empty-hint">เมื่อผู้ดูแลกำหนดสิทธิ์แล้ว ให้รีเฟรชหน้านี้ รายงานจะปรากฏโดยไม่ต้องเปิดลิงก์เชิญหรือเข้าสู่ระบบใหม่</p>
    </div>
  </section>

  <template v-else>
  <ReportPeriodToolbar desktop-mode="compact" mode="SMART_OVERVIEW" :selection="selectedPeriod" :displayed-label="displayedPeriodLabel" action-label="ดูภาพรวมช่วงนี้" force-action-label="ดึงใหม่จาก SML" :loading="lookingUp || refreshing" :disabled="loading || refreshConfirmOpen || !reports.length" @apply="viewOverviewPeriod" @force="requestRefresh" />

  <Message v-if="error" severity="error" :closable="false" class="mb-4">{{ error }} <Button label="ลองโหลดอีกครั้ง" text size="small" @click="initializeOverview" /></Message>
  <Message v-if="refreshFailures.length" severity="warn" :closable="false" class="mb-4">อัปเดตไม่สำเร็จ {{ refreshFailures.length }} รายงาน: {{ failedReportLabels.join(', ') }} — ช่องดังกล่าวจะไม่ใช้ตัวเลขเดิมมาปะปน</Message>
  <Message v-if="cacheRevalidationMessage" :severity="cacheRevalidating ? 'info' : 'secondary'" :closable="false" class="mb-4">{{ cacheRevalidationMessage }}</Message>
  <Message v-if="overview?.sourceConsistency === 'CHUNK_WINDOW'" severity="info" :closable="false" class="mb-4">ข้อมูลชุดนี้มาจากหลายคำขอไปยัง SML และไม่ใช่ Snapshot ณ วินาทีเดียว · {{ formatSourceCollection(overview.sourceStartedAt, overview.sourceFinishedAt, overview.sourceConsistency) }}</Message>
  <div v-if="refreshing" class="card executive-panel text-center" aria-live="polite"><div class="flex flex-col items-center gap-1 mb-3"><span class="font-medium">กำลังอัปเดต {{ refreshCompletedLabel }} จาก {{ refreshTotalLabel }} รายงาน</span><strong class="metric-value text-lg">{{ refreshPercent }}%</strong><span class="text-sm text-muted-color">{{ activeRefreshLabel }}<template v-if="activeRefreshTypicalDuration"> · {{ activeRefreshTypicalDuration }}</template></span></div><ProgressBar :value="refreshPercent" :show-value="false" style="height: .45rem" /><p class="text-xs text-muted-color mb-0 mt-3">ข้อมูลด้านล่างยังเป็น Snapshot เดิม ระบบจะเปลี่ยนข้อมูลทั้งชุดเมื่อการอัปเดตเสร็จ</p></div>
  <Message v-if="!loading && missingReportCount" severity="warn" :closable="false" class="mb-4">ยังไม่มี Snapshot {{ exactRefreshLoaded ? 'ตรงช่วงที่เลือก' : 'ล่าสุด' }} {{ missingReportCount }} รายงาน หากต้องการข้อมูลใหม่ให้กด “ดึงใหม่จาก SML”</Message>

  <div v-if="loading" class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4 mb-5"><Skeleton v-for="index in 4" :key="index" height="9rem" /></div>
  <div v-else class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4 mb-5">
    <button v-for="item in kpis" :key="item.key" type="button" class="card executive-kpi" :title="formatDashboardValue(item.metric?.value, item.metric?.unit ?? 'THB')" @click="openReport(item.reportKey)"><span class="kpi-icon"><i :class="item.icon" /></span><span class="kpi-copy"><span class="kpi-label">{{ item.label }}</span><span v-if="item.period" class="kpi-period">ข้อมูล {{ formatPeriodRange(item.period) }}</span><Tag v-if="snapshotForReport(snapshots, item.reportKey)?.freshnessStatus" :severity="snapshotFreshness(snapshotForReport(snapshots, item.reportKey)!).severity" :value="snapshotFreshness(snapshotForReport(snapshots, item.reportKey)!).label" class="kpi-freshness" /><strong class="kpi-value">{{ formatDashboardValue(item.metric?.value, item.metric?.unit ?? 'THB') }}</strong><span v-if="item.metric?.comparison.availability === 'AVAILABLE'" class="kpi-comparison"><span><i :class="item.metric.comparison.direction === 'UP' ? 'pi pi-arrow-up-right' : item.metric.comparison.direction === 'DOWN' ? 'pi pi-arrow-down-right' : 'pi pi-minus'" /> {{ formatDashboardValue(item.metric.comparison.delta, item.metric.unit) }}</span><span>{{ comparisonPeriodText(item.comparisonPeriod) }}</span></span><span v-else class="kpi-comparison">{{ item.metric ? 'ไม่มีข้อมูลเทียบช่วงเวลาเดียวกัน' : 'รอการอัปเดตข้อมูล' }}</span><span v-if="snapshotForReport(snapshots, item.reportKey)?.sourceFinishedAt" class="kpi-source-time">{{ snapshotSource(snapshotForReport(snapshots, item.reportKey)!) }}</span></span><i class="pi pi-chevron-right kpi-link" /></button>
  </div>

  <div v-if="featuredCharts.length" class="grid grid-cols-1 2xl:grid-cols-2 gap-5">
    <article v-for="item in featuredCharts" :key="`${item.snapshot.dashboard.reportKey}-${item.visualization.key}`" class="card executive-panel dashboard-card"><div class="chart-heading"><div><h2>{{ item.visualization.title }}</h2><p>ข้อมูล {{ formatPeriodRange(item.snapshot.dashboard.period) }} · {{ snapshotSource(item.snapshot) }}</p></div><Button icon="pi pi-arrow-up-right" text rounded class="touch-action" aria-label="เปิดรายงาน" @click="openReport(item.snapshot.dashboard.reportKey)" /></div><ExecutiveChart :visualization="item.visualization" compact /></article>
  </div>
  <div v-else-if="!loading && !refreshing" class="card executive-panel empty-overview"><i class="pi pi-chart-bar" /><h2>ยังไม่มี Snapshot สำหรับช่วงนี้</h2><p>เลือกช่วงแล้วกด “ดูภาพรวมช่วงนี้” เพื่อค้นหา Cache ที่มีอยู่ หรือกด “ดึงใหม่จาก SML” เมื่อต้องการสร้างข้อมูลใหม่</p></div>
  </template>
</template>

<style scoped>
.executive-panel { border-radius: var(--content-border-radius); }
.dashboard-card { margin-bottom: 0; }
.executive-kpi { position: relative; display: flex; align-items: flex-start; gap: 1rem; min-height: 8.5rem; margin-bottom: 0; color: inherit; border: 0; text-align: left; cursor: pointer; transition: transform .2s; }
.executive-kpi:hover { transform: translateY(-1px); }
.kpi-icon { display: grid; place-items: center; flex: 0 0 2.75rem; height: 2.75rem; border-radius: var(--content-border-radius); background: var(--p-primary-50); color: var(--primary-color); font-size: 1.1rem; }
.kpi-copy { display: grid; gap: .3rem; min-width: 0; }
.kpi-label { color: var(--text-color-secondary); font-size: .8rem; font-weight: 600; }
.kpi-period { color: var(--text-color-secondary); font-size: .7rem; }
.kpi-freshness { justify-self: start; font-size: .65rem; }
.kpi-value { font-size: clamp(1.05rem, 1.75vw, 1.65rem); line-height: 1.1; font-variant-numeric: tabular-nums; white-space: nowrap; letter-spacing: -.02em; }
.kpi-comparison { display: grid; gap: .15rem; color: var(--text-color-secondary); font-size: .72rem; }
.kpi-source-time { color: var(--text-color-secondary); font-size: .65rem; line-height: 1.35; }
.kpi-link { position: absolute; top: 1.2rem; right: 1rem; color: var(--text-color-secondary); font-size: .75rem; }
.chart-heading { display: flex; align-items: flex-start; justify-content: space-between; gap: 1rem; margin-bottom: .5rem; }
.chart-heading h2 { margin: 0; font-size: 1.05rem; }
.chart-heading p { margin: .35rem 0 0; color: var(--text-color-secondary); font-size: .75rem; }
.chart-empty { display: grid; place-content: center; justify-items: center; gap: .45rem; min-height: 11rem; color: var(--text-color-secondary); text-align: center; }
.chart-empty i { color: var(--primary-color); font-size: 1.5rem; }
.empty-overview { display: grid; justify-items: center; padding: 3rem 1.5rem; text-align: center; }
.empty-overview > i { color: var(--primary-color); font-size: 2.75rem; }
.empty-overview h2 { margin: 1rem 0 .4rem; }
.empty-overview p { max-width: 34rem; margin: 0 0 1.25rem; color: var(--text-color-secondary); }
.permission-empty-state { display: grid; grid-template-columns: auto minmax(0, 1fr); align-items: start; gap: 1rem; max-width: 52rem; }
.permission-empty-icon { display: grid; place-items: center; width: 3rem; height: 3rem; border-radius: 50%; color: var(--p-green-600); background: var(--p-green-50); font-size: 1.4rem; }
.permission-empty-state h2 { margin: 0 0 .75rem; font-size: 1.2rem; }
.permission-empty-state p { max-width: 42rem; margin: .9rem 0 0; color: var(--text-color-secondary); }
.permission-empty-state .permission-empty-hint { margin-top: .45rem; font-size: .85rem; }
@media (prefers-reduced-motion: reduce) { .executive-kpi { transition: none; } }
@media (max-width: 767px) {
  .executive-kpi { min-height: 7rem; padding: 1.25rem; }
  .executive-panel.dashboard-card { padding: 1.25rem; }
  .permission-empty-state { grid-template-columns: 1fr; padding: 1.25rem; }
}
</style>
