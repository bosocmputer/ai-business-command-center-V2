<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useConfirm } from 'primevue/useconfirm';
import { useToast } from 'primevue/usetoast';
import ExecutiveChart from '@/components/dashboard/ExecutiveChart.vue';
import ReportPeriodToolbar from '@/components/dashboard/ReportPeriodToolbar.vue';
import SakaiTableHeader from '@/components/table/SakaiTableHeader.vue';
import { cloneSakaiFilterState, useSakaiFilterMenu } from '@/composables/useSakaiFilterMenu';
import { ApiError, reportDefinitionByKey, viewerApi, type CreateReportRunInput, type DashboardSnapshot, type ReportDashboard, type ReportKey, type ReportRowQueryInput, type ReportRun } from '@/api';
import { newIdempotencyKey } from '@/api/client';
import { useViewerSession } from '@/stores/viewer';
import { comparisonPeriodText, formatDashboardValue, formatPeriodRange, periodLabel } from '@/utils/dashboard';
import { errorMessage, formatDateTime, formatSourceCollection } from '@/utils/format';
import { periodModeForReport, selectionForMode, selectionFromReportPeriod, selectionToRunInput, type ReportPeriodSelection } from '@/utils/reportPeriod';
import { formatReportCell, presentationFor, reportColumnClass, visibleReportColumns, type ReportColumnDefinition } from '@/utils/reportPresentation';
import { cleanViewerQuery, validSnapshotRunId, validViewerRunId } from '@/utils/viewerSnapshot';
import { freshnessPresentation, progressPresentation, typicalDurationText } from '@/utils/freshness';

const route = useRoute();
const router = useRouter();
const confirm = useConfirm();
const toast = useToast();
const { state, selectTenant, ensureReports, periodSelection, setPeriodSelection } = useViewerSession();
const tenantId = computed(() => String(route.params.tenantId));
const reportKey = computed(() => String(route.params.reportKey) as ReportKey);
const definition = computed(() => state.reportsByTenant[tenantId.value]?.find((item) => item.reportKey === reportKey.value) ?? reportDefinitionByKey.get(reportKey.value));
const periodMode = computed(() => definition.value?.periodMode ?? periodModeForReport(reportKey.value));
const tenant = computed(() => state.tenants.find((item) => item.id === tenantId.value));
const selectedPeriod = ref<ReportPeriodSelection>({ periodPreset: 'MONTH_TO_DATE' });
const run = ref<ReportRun>();
const dashboard = ref<ReportDashboard>();
const cachedSnapshot = ref<DashboardSnapshot>();
const expiredSnapshot = ref<DashboardSnapshot>();
const rows = ref<Record<string, unknown>[]>([]);
const columns = ref<string[]>([]);
const rowPage = ref(0);
type TablePageSize = 25 | 50 | 100;
const rowPageSize = ref<TablePageSize>(25);
const rowTotal = ref(0);
const appliedRowFilters = ref<ReportRowQueryInput['filters']>([]);
const rowGlobalSearch = ref('');
const appliedRowGlobalSearch = ref('');
type RowFilterCapability = { columnKey: string; dataType: 'TEXT' | 'IDENTIFIER' | 'DATE' | 'NUMBER'; operators: ReportRowQueryInput['filters'][number]['operator'][]; globalSearchable: boolean };
type PrimeRowFilter = { value: string | Date[] | null; matchMode: string };
const rowFilterCapabilities = ref<RowFilterCapability[]>([]);
const rowPrimeFilters = ref<Record<string, PrimeRowFilter>>({});
useSakaiFilterMenu(rowPrimeFilters);
const rowFilterOperators = ref<Record<string, ReportRowQueryInput['filters'][number]['operator']>>({});
const rowFilterError = ref('');
const mobileRowFiltersOpen = ref(false);
const mobileRowFilterBaseline = ref<{ filters: Record<string, PrimeRowFilter>; operators: Record<string, ReportRowQueryInput['filters'][number]['operator']> }>();
const rowsLoaded = ref(false);
const rowsUnavailable = ref(false);
const activeTab = ref('overview');
const selectedColumnKeys = ref<string[]>([]);
const expandedMobileRows = ref(new Set<string>());
const snapshotMode = ref(false);
const snapshotMissing = ref(false);
const loading = ref(false);
const cacheLoading = ref(false);
const backgroundRefreshing = ref(false);
const forceConfirmOpen = ref(false);
const loadingRows = ref(false);
const error = ref('');
let pollTimer: number | undefined;
let pollCount = 0;
let generation = 0;
let pollDeadline = 0;
let pollInFlight = false;
let initializeController: AbortController | undefined;
let pollController: AbortController | undefined;
let rowsController: AbortController | undefined;
let rowSearchTimer: ReturnType<typeof setTimeout> | undefined;
let runAction: { fingerprint: string; key: string } | undefined;

const active = computed(() => !!run.value && ['QUEUED', 'CLAIMED', 'RUNNING'].includes(run.value.status));
const snapshotRunId = computed(() => validSnapshotRunId(route.query.snapshotRunId));
const viewerRunId = computed(() => validViewerRunId(route.query.runId));
const statusLabel = computed(() => {
  const labels: Record<string, string> = { QUEUED: 'รอคิว', CLAIMED: 'กำลังเตรียม', RUNNING: 'กำลังดึงข้อมูล', SUCCEEDED: 'พร้อมใช้งาน', FAILED: 'ไม่สำเร็จ', CANCELLED: 'ยกเลิกแล้ว', EXPIRED: 'หมดอายุ' };
  return run.value ? labels[run.value.status] ?? run.value.status : '';
});
const showRunStatus = computed(() => !!run.value && run.value.status !== 'SUCCEEDED' && !(snapshotMode.value && !!dashboard.value));
const statusSeverity = computed(() => run.value?.status === 'FAILED' ? 'danger' : active.value ? 'info' : 'secondary');
const detailRowsUnavailable = computed(() => rowsUnavailable.value || run.value?.status === 'EXPIRED' || cachedSnapshot.value?.detailsAvailable === false);
const freshness = computed(() => freshnessPresentation(cachedSnapshot.value?.freshnessStatus));
const progressLabel = computed(() => progressPresentation(run.value?.progress?.phase));
const typicalDuration = computed(() => typicalDurationText(run.value?.progress));
const elapsedSeconds = computed(() => {
  if (!run.value) return 0;
  const end = run.value.finishedAt ? Date.parse(run.value.finishedAt) : Date.now();
  return Math.max(0, Math.round((end - Date.parse(run.value.queuedAt)) / 1000));
});
const sourceFinishedAt = computed(() => cachedSnapshot.value?.sourceFinishedAt ?? run.value?.finishedAt ?? dashboard.value?.generatedAt);
const sourceLabel = computed(() => sourceFinishedAt.value ? formatSourceCollection(cachedSnapshot.value?.sourceStartedAt, sourceFinishedAt.value, cachedSnapshot.value?.sourceConsistency) : undefined);
const displayedPeriodLabel = computed(() => dashboard.value ? `${periodLabel(dashboard.value.period.preset)} · ${formatPeriodRange(dashboard.value.period)}` : run.value?.dateFrom && run.value.dateTo ? formatPeriodRange({ preset: run.value.periodPreset as ReportDashboard['period']['preset'], dateFrom: run.value.dateFrom, dateTo: run.value.dateTo }) : 'ยังไม่มีข้อมูล');
const columnOptions = computed(() => presentationFor(reportKey.value, columns.value));
const displayedColumns = computed(() => visibleReportColumns(reportKey.value, columns.value, selectedColumnKeys.value));
const mobileSummaryColumns = computed(() => displayedColumns.value.filter((column) => column.mobilePriority >= 4).slice(0, 6));
const rowCapabilityByKey = computed(() => new Map(rowFilterCapabilities.value.map((item) => [item.columnKey, item])));
const filterableDisplayedColumns = computed(() => displayedColumns.value.filter((column) => rowCapabilityByKey.value.has(column.key)));
const rowHasFilters = computed(() => Boolean(appliedRowGlobalSearch.value || appliedRowFilters.value.length));

async function resolveSnapshot(selection: ReportPeriodSelection) {
  if (!definition.value || loading.value || cacheLoading.value || backgroundRefreshing.value) return;
  let payload: CreateReportRunInput;
  try { payload = selectionToRunInput(periodMode.value, selection); }
  catch (cause) { error.value = errorMessage(cause); return; }
  const context = ++generation;
  const selectedTenantId = tenantId.value;
  const selectedReportKey = reportKey.value;
  initializeController?.abort('new-period'); initializeController = new AbortController(); stopPolling(); resetRows();
  loading.value = !dashboard.value; cacheLoading.value = true; backgroundRefreshing.value = false; error.value = '';
  try {
    const snapshot = await viewerApi.exactSnapshot(selectedTenantId, selectedReportKey, payload, initializeController.signal);
    if (!isCurrent(context, selectedTenantId, selectedReportKey)) return;
    selectedPeriod.value = { ...selection }; setPeriodSelection(selectedTenantId, selection);
    snapshotMode.value = false; snapshotMissing.value = false;
    expiredSnapshot.value = undefined;
    if (snapshot.freshnessStatus === 'EXPIRED') {
      expiredSnapshot.value = snapshot;
      dashboard.value = undefined; cachedSnapshot.value = undefined; run.value = undefined;
    } else {
      applyCachedSnapshot(snapshot);
      try { run.value = await viewerApi.run(selectedTenantId, selectedReportKey, snapshot.runId, initializeController.signal); }
      catch (cause) { if (!(cause instanceof ApiError) || cause.status !== 404) throw cause; }
    }
    loading.value = false; backgroundRefreshing.value = false;
  } catch (cause) {
    if (!isCancelled(cause) && isCurrent(context, selectedTenantId, selectedReportKey)) {
      selectedPeriod.value = { ...selection }; setPeriodSelection(selectedTenantId, selection);
      if (cause instanceof ApiError && cause.status === 404) {
        error.value = 'ยังไม่มี Snapshot สำหรับช่วงนี้ หากต้องการข้อมูลใหม่ให้กด “ดึงใหม่จาก SML”';
      } else error.value = errorMessage(cause);
    }
    loading.value = false; backgroundRefreshing.value = false;
  } finally {
    if (isCurrent(context, selectedTenantId, selectedReportKey)) cacheLoading.value = false;
  }
}

function applyCachedSnapshot(snapshot: DashboardSnapshot) {
	expiredSnapshot.value = undefined;
  cachedSnapshot.value = snapshot;
  dashboard.value = snapshot.dashboard;
  selectedPeriod.value = selectionFromReportPeriod(periodMode.value, snapshot.dashboard.period);
}

async function showExpiredSnapshot() {
  const snapshot = expiredSnapshot.value;
  if (!snapshot) return;
  applyCachedSnapshot(snapshot);
  if (!run.value || !['QUEUED', 'CLAIMED', 'RUNNING'].includes(run.value.status)) {
    try { run.value = await viewerApi.run(tenantId.value, reportKey.value, snapshot.runId, initializeController?.signal); }
    catch (cause) { error.value = errorMessage(cause); }
  }
}

async function startRun(selection: ReportPeriodSelection) {
  if (loading.value) return;
  if (!definition.value) { error.value = 'ไม่พบรายงานหรือคุณไม่มีสิทธิ์เปิดรายงานนี้'; return; }
  let payload: CreateReportRunInput;
  try { payload = selectionToRunInput(periodMode.value, selection); }
  catch (cause) { error.value = errorMessage(cause); return; }
  const context = ++generation;
  const selectedTenantId = tenantId.value;
  const selectedReportKey = reportKey.value;
  stopPolling(); rowsController?.abort('new-run'); rowsController = undefined; cacheLoading.value = false;
  loading.value = true; error.value = ''; activeTab.value = 'overview'; pollCount = 0;
  const fingerprint = JSON.stringify([selectedTenantId, selectedReportKey, payload]);
  if (!runAction || runAction.fingerprint !== fingerprint) runAction = { fingerprint, key: newIdempotencyKey('viewer-run') };
  try {
    const created = await viewerApi.createRun(selectedTenantId, selectedReportKey, payload, runAction.key);
    if (!isCurrent(context, selectedTenantId, selectedReportKey)) return;
    selectedPeriod.value = { ...selection }; setPeriodSelection(selectedTenantId, selection);
    snapshotMode.value = false; snapshotMissing.value = false; run.value = created; resetRows(); runAction = undefined;
    backgroundRefreshing.value = !!dashboard.value;
    const query = cleanViewerQuery(route.query); delete query.snapshotRunId; query.runId = created.id;
    await router.replace({ path: route.path, query, hash: route.hash });
    if (!isCurrent(context, selectedTenantId, selectedReportKey)) return;
    pollDeadline = Date.now() + 12 * 60_000; schedulePoll(context);
  } catch (cause) {
    if (!isCurrent(context, selectedTenantId, selectedReportKey) || isCancelled(cause)) return;
    if (!(cause instanceof ApiError) || !cause.retryable) runAction = undefined;
    error.value = errorMessage(cause); loading.value = false;
  }
}

function requestForceRefresh(selection: ReportPeriodSelection) {
  if (forceConfirmOpen.value || cacheLoading.value || active.value) return;
  const expectedTenantId = tenantId.value;
  const expectedReportKey = reportKey.value;
  const expectedGeneration = generation;
  forceConfirmOpen.value = true;
  confirm.require({
    header: `ดึงข้อมูลใหม่จาก ${tenant.value?.name ?? 'ร้านนี้'}`,
    message: `ระบบจะ Query SML ใหม่สำหรับ “${definition.value?.label ?? expectedReportKey}” และอาจใช้เวลานาน โดยเฉพาะรายงานสต็อก`,
    icon: 'pi pi-database', defaultFocus: 'reject', rejectLabel: 'ยกเลิก', acceptLabel: 'ดึงใหม่จาก SML',
    rejectProps: { severity: 'secondary', text: true },
    accept: () => {
      forceConfirmOpen.value = false;
      if (expectedGeneration !== generation || expectedTenantId !== tenantId.value || expectedReportKey !== reportKey.value) {
        toast.add({ severity: 'warn', summary: 'หน้ารายงานเปลี่ยนแล้ว', detail: 'กรุณาตรวจสอบช่วงข้อมูลและลองใหม่', life: 3500 });
        return;
      }
      void startRun(selection);
    },
    reject: () => { forceConfirmOpen.value = false; },
    onHide: () => { forceConfirmOpen.value = false; }
  });
}

function applyRunPeriod(item: ReportRun) {
  if (!item.dateFrom || !item.dateTo) return;
  selectedPeriod.value = selectionFromReportPeriod(periodMode.value, { preset: item.periodPreset, dateFrom: item.dateFrom, dateTo: item.dateTo });
}

async function loadExistingRun(context: number, selectedTenantId: string, selectedReportKey: ReportKey, selectedRunId: string, fromLine: boolean) {
  snapshotMode.value = fromLine; snapshotMissing.value = false; loading.value = true;
  pollCount = 0;
  dashboard.value = undefined; cachedSnapshot.value = undefined; expiredSnapshot.value = undefined; run.value = undefined; activeTab.value = 'overview'; error.value = '';
  resetRows();
  const runResult = await viewerApi.run(selectedTenantId, selectedReportKey, selectedRunId, initializeController?.signal);
  if (!isCurrent(context, selectedTenantId, selectedReportKey)) return;
  run.value = runResult; applyRunPeriod(runResult);
  if (['QUEUED', 'CLAIMED', 'RUNNING'].includes(runResult.status)) {
    pollDeadline = Date.now() + 12 * 60_000; schedulePoll(context); return;
  }
  if (runResult.status === 'SUCCEEDED' || runResult.status === 'EXPIRED') {
    try { dashboard.value = await viewerApi.dashboard(selectedTenantId, selectedReportKey, selectedRunId, initializeController?.signal); }
    catch (cause) {
      if (fromLine && cause instanceof ApiError && cause.status === 404) snapshotMissing.value = true;
      else throw cause;
    }
    if (dashboard.value) cachedSnapshot.value = {
      runId: runResult.id, dashboard: dashboard.value,
      periodFrom: runResult.dateFrom ?? undefined, periodTo: runResult.dateTo ?? undefined,
      sourceStartedAt: runResult.startedAt, sourceFinishedAt: runResult.finishedAt,
      freshnessStatus: 'FRESH', detailsAvailable: runResult.resultKind !== 'SUMMARY' && runResult.status !== 'EXPIRED',
      detailsExpiresAt: runResult.expiresAt
    };
    loading.value = false; return;
  }
  loading.value = false; error.value = runResult.safeErrorMessage || 'สร้างรายงานไม่สำเร็จ กรุณาลองใหม่';
}

async function replaySnapshot() {
  if (!run.value) return;
  await startRun(selectedPeriod.value);
}

function schedulePoll(context: number) {
  stopPolling();
  if (document.visibilityState === 'hidden') return;
  const delay = pollCount < 15 ? 2000 : 5000;
  pollTimer = window.setTimeout(() => void poll(context), delay);
}

async function poll(context: number) {
  if (!run.value || pollInFlight || context !== generation) return;
  if (Date.now() >= pollDeadline) { loading.value = false; error.value = 'รายงานใช้เวลานานกว่าปกติ คุณสามารถออกจากหน้านี้แล้วกลับมาตรวจสอบอีกครั้ง'; return; }
  const selectedTenantId = tenantId.value;
  const selectedReportKey = reportKey.value;
  const selectedRunId = run.value.id;
  pollInFlight = true; pollController = new AbortController();
  try {
    const latest = await viewerApi.run(selectedTenantId, selectedReportKey, selectedRunId, pollController.signal);
    if (!isCurrent(context, selectedTenantId, selectedReportKey) || run.value?.id !== selectedRunId) return;
    run.value = latest; pollCount++;
    if (run.value.status === 'SUCCEEDED') {
      await loadDashboard(); await refreshCachedMetadata(); loading.value = false; backgroundRefreshing.value = false; return;
    }
    if (['FAILED', 'CANCELLED', 'EXPIRED'].includes(run.value.status)) {
      loading.value = false; backgroundRefreshing.value = false;
      if (cachedSnapshot.value && dashboard.value) {
        cachedSnapshot.value = { ...cachedSnapshot.value, freshnessStatus: 'REFRESH_FAILED' };
        error.value = 'อัปเดตไม่สำเร็จ กำลังแสดง Snapshot เดิมพร้อมเวลาที่ดึงจาก SML';
      } else error.value = run.value.safeErrorMessage || 'สร้างรายงานไม่สำเร็จ กรุณาลองใหม่';
      return;
    }
    schedulePoll(context);
  } catch (cause) {
    if (!isCancelled(cause) && isCurrent(context, selectedTenantId, selectedReportKey)) { loading.value = false; error.value = errorMessage(cause); }
  } finally { pollInFlight = false; pollController = undefined; }
}

async function refreshCachedMetadata() {
  if (!run.value) return;
  const payload = selectionToRunInput(periodMode.value, selectedPeriod.value);
  try {
    const snapshot = await viewerApi.exactSnapshot(tenantId.value, reportKey.value, payload, pollController?.signal);
    if (run.value && snapshot.runId === run.value.id) applyCachedSnapshot(snapshot);
  } catch (cause) {
    if (!isCancelled(cause)) cachedSnapshot.value = undefined;
  }
}

async function loadDashboard() {
  if (!run.value) return;
  const context = generation; const selectedTenantId = tenantId.value; const selectedReportKey = reportKey.value; const selectedRunId = run.value.id;
  try {
    const result = await viewerApi.dashboard(selectedTenantId, selectedReportKey, selectedRunId, pollController?.signal);
    if (isCurrent(context, selectedTenantId, selectedReportKey) && run.value?.id === selectedRunId) dashboard.value = result;
  } catch (cause) {
    if (!isCancelled(cause) && context === generation) {
      if (snapshotMode.value && cause instanceof ApiError && cause.code === 'REPORT_ROWS_EXPIRED') rowsUnavailable.value = true;
      else error.value = errorMessage(cause);
    }
  }
}

async function loadRows(pageNumber = 0, pageSize: TablePageSize = rowPageSize.value, allowClamp = true) {
  if (!run.value) return;
  rowsController?.abort('new-page'); rowsController = new AbortController();
  const context = generation; const selectedTenantId = tenantId.value; const selectedReportKey = reportKey.value; const selectedRunId = run.value.id;
  const initializeColumns = columns.value.length === 0;
  loadingRows.value = true;
  try {
    const page = await viewerApi.queryRows(selectedTenantId, selectedReportKey, selectedRunId, {
      globalSearch: appliedRowGlobalSearch.value || undefined,
      filters: appliedRowFilters.value, page: pageNumber, pageSize
    }, rowsController.signal);
    if (!isCurrent(context, selectedTenantId, selectedReportKey) || run.value?.id !== selectedRunId) return;
    if (allowClamp && page.total > 0 && page.data.length === 0 && pageNumber > 0) {
      await loadRows(Math.max(0, Math.ceil(page.total / pageSize) - 1), pageSize, false);
      return;
    }
    const keyedRows = page.data.map((row, index) => ({ ...row, __rowKey: `${selectedRunId}:${page.rowOrdinals[index]}` }));
    rows.value = keyedRows;
    columns.value = [...new Set([...columns.value, ...page.columns])];
    rowFilterCapabilities.value = page.filterCapabilities;
    initializeRowFilterState(page.filterCapabilities);
    if (initializeColumns) selectedColumnKeys.value = visibleReportColumns(selectedReportKey, columns.value).map((column) => column.key);
    rowPage.value = page.page;
    rowPageSize.value = page.pageSize;
    rowTotal.value = page.total;
    rowsLoaded.value = true;
  } catch (cause) {
    if (!isCancelled(cause) && context === generation) {
      if (cause instanceof ApiError && cause.code === 'REPORT_ROWS_EXPIRED') rowsUnavailable.value = true;
      else error.value = errorMessage(cause);
    }
  }
  finally { if (context === generation) loadingRows.value = false; }
}

function initializeRowFilterState(capabilities: RowFilterCapability[]) {
  const filters = { ...rowPrimeFilters.value };
  const operators = { ...rowFilterOperators.value };
  for (const capability of capabilities) {
    filters[capability.columnKey] ??= { value: null, matchMode: capability.dataType === 'DATE' ? 'between' : 'contains' };
    operators[capability.columnKey] ??= capability.dataType === 'DATE' && capability.operators.includes('BETWEEN')
      ? 'BETWEEN'
      : capability.operators[0] ?? 'EQUALS';
  }
  rowPrimeFilters.value = filters;
  rowFilterOperators.value = operators;
}

function bangkokDateValue(value: Date): string {
  return `${value.getFullYear()}-${String(value.getMonth() + 1).padStart(2, '0')}-${String(value.getDate()).padStart(2, '0')}`;
}

function rowOperatorOptions(capability: RowFilterCapability) {
  const labels: Record<string, string> = { CONTAINS: 'มีคำว่า', EQUALS: 'เท่ากับ', GTE: 'มากกว่าหรือเท่ากับ', LTE: 'น้อยกว่าหรือเท่ากับ', BETWEEN: 'อยู่ในช่วงวันที่' };
  return capability.operators.map((value) => ({ label: labels[value] ?? value, value }));
}
function dateRowFilterValue(columnKey: string): Date[] | null {
  const value = rowPrimeFilters.value[columnKey]?.value;
  return Array.isArray(value) ? value : null;
}
function textRowFilterValue(columnKey: string): string {
  const value = rowPrimeFilters.value[columnKey]?.value;
  return typeof value === 'string' ? value : '';
}
function setRowFilterValue(columnKey: string, value: string | Date[] | null | undefined) {
  const filter = rowPrimeFilters.value[columnKey];
  if (filter) filter.value = value ?? null;
}

function openMobileRowFilters() {
  mobileRowFilterBaseline.value = {
    filters: cloneSakaiFilterState(rowPrimeFilters.value),
    operators: { ...rowFilterOperators.value }
  };
  mobileRowFiltersOpen.value = true;
}

function closeMobileRowFilters(applied = false) {
  if (!applied && mobileRowFilterBaseline.value) {
    rowPrimeFilters.value = cloneSakaiFilterState(mobileRowFilterBaseline.value.filters);
    rowFilterOperators.value = { ...mobileRowFilterBaseline.value.operators };
  }
  mobileRowFilterBaseline.value = undefined;
  mobileRowFiltersOpen.value = false;
}

function buildAppliedRowFilters(): ReportRowQueryInput['filters'] | undefined {
  const filters: ReportRowQueryInput['filters'] = [];
  rowFilterError.value = '';
  for (const capability of rowFilterCapabilities.value) {
    const raw = rowPrimeFilters.value[capability.columnKey]?.value;
    if (raw == null || raw === '') continue;
    if (capability.dataType === 'DATE') {
      const range = Array.isArray(raw) ? raw.filter((item): item is Date => item instanceof Date && !Number.isNaN(item.getTime())) : [];
      if (range.length !== 2) {
        rowFilterError.value = 'เลือกวันเริ่มต้นและวันสิ้นสุดให้ครบก่อนใช้ตัวกรอง';
        return undefined;
      }
      filters.push({ columnKey: capability.columnKey, operator: 'BETWEEN', value: bangkokDateValue(range[0]!), valueTo: bangkokDateValue(range[1]!) });
    } else {
      const value = String(raw).trim();
      if (!value) continue;
      if (capability.dataType === 'NUMBER' && !/^[+-]?(?:\d+(?:\.\d+)?|\.\d+)(?:[eE][+-]?\d+)?$/.test(value)) {
        rowFilterError.value = 'กรอกค่าตัวเลขให้ถูกต้อง';
        return undefined;
      }
      filters.push({ columnKey: capability.columnKey, operator: rowFilterOperators.value[capability.columnKey] ?? capability.operators[0] ?? 'EQUALS', value });
    }
    if (filters.length > 5) {
      rowFilterError.value = 'ใช้ตัวกรองพร้อมกันได้สูงสุด 5 คอลัมน์';
      return undefined;
    }
  }
  return filters;
}

function applyRowFilters() {
  const filters = buildAppliedRowFilters();
  if (!filters) return;
  appliedRowFilters.value = filters;
  expandedMobileRows.value = new Set();
  closeMobileRowFilters(true);
  void loadRows(0);
}

function clearRowFilter() {
  for (const filter of Object.values(rowPrimeFilters.value)) filter.value = null;
  rowGlobalSearch.value = '';
  appliedRowGlobalSearch.value = '';
  appliedRowFilters.value = [];
  rowFilterError.value = '';
  expandedMobileRows.value = new Set();
  mobileRowFilterBaseline.value = undefined;
  mobileRowFiltersOpen.value = false;
  void loadRows(0);
}

function changeRowsPage(event: { page: number; rows: number }) {
  if (event.rows !== 25 && event.rows !== 50 && event.rows !== 100) return;
  void loadRows(event.rows === rowPageSize.value ? event.page : 0, event.rows);
}

async function cancel() {
  if (!run.value || run.value.status !== 'QUEUED') return;
  try { run.value = await viewerApi.cancelRun(tenantId.value, reportKey.value, run.value.id); loading.value = false; stopPolling(); toast.add({ severity: 'info', summary: 'ยกเลิกการสร้างรายงานแล้ว', life: 2500 }); }
  catch (cause) { error.value = errorMessage(cause); }
}

function stopFollowing() {
  stopPolling(); loading.value = false; backgroundRefreshing.value = false;
  toast.add({ severity: 'info', summary: 'หยุดติดตามแล้ว', detail: 'Query ที่เริ่มทำงานอาจยังทำต่อใน SML คุณกลับมาตรวจสอบภายหลังได้', life: 4500 });
}

function stopPolling() { if (pollTimer) window.clearTimeout(pollTimer); pollTimer = undefined; pollController?.abort('poll-stopped'); pollController = undefined; }
function stopLifecycle() { generation++; stopPolling(); initializeController?.abort('route-changed'); rowsController?.abort('route-changed'); initializeController = undefined; rowsController = undefined; pollInFlight = false; backgroundRefreshing.value = false; }
function isCancelled(cause: unknown) { return cause instanceof ApiError && cause.code === 'CANCELLED'; }
function isCurrent(context: number, selectedTenantId: string, selectedReportKey: ReportKey) { return context === generation && tenantId.value === selectedTenantId && reportKey.value === selectedReportKey; }
function handleVisibilityChange() {
  if (document.visibilityState === 'visible' && loading.value && active.value && !pollInFlight) {
    if (pollTimer) window.clearTimeout(pollTimer);
    pollTimer = undefined; void poll(generation);
  }
}
function mobileColumns(row: Record<string, unknown>): ReportColumnDefinition[] {
  return expandedMobileRows.value.has(String(row.__rowKey)) ? displayedColumns.value : mobileSummaryColumns.value;
}
function toggleMobileRow(row: Record<string, unknown>) {
  const key = String(row.__rowKey); const next = new Set(expandedMobileRows.value);
  if (next.has(key)) next.delete(key); else next.add(key);
  expandedMobileRows.value = next;
}
function resetRows() {
  rows.value = []; columns.value = []; selectedColumnKeys.value = []; expandedMobileRows.value = new Set();
  rowPage.value = 0; rowPageSize.value = 25; rowTotal.value = 0; rowFilterCapabilities.value = [];
  rowPrimeFilters.value = {}; rowFilterOperators.value = {}; appliedRowFilters.value = [];
  rowGlobalSearch.value = ''; appliedRowGlobalSearch.value = ''; rowFilterError.value = '';
  mobileRowFiltersOpen.value = false; rowsLoaded.value = false; rowsUnavailable.value = false;
}
async function initialize() {
  stopLifecycle();
  const context = generation;
  const selectedTenantId = tenantId.value;
  const selectedReportKey = reportKey.value;
  initializeController = new AbortController();
  error.value = '';
  try {
    selectTenant(selectedTenantId);
    const permittedReports = await ensureReports(selectedTenantId, false, initializeController.signal);
    if (!isCurrent(context, selectedTenantId, selectedReportKey)) return;
    if (!permittedReports.some((item) => item.reportKey === selectedReportKey)) {
      error.value = 'คุณไม่มีสิทธิ์เปิดรายงานนี้ กรุณาเลือกจากเมนูรายงาน';
      return;
    }
    selectedPeriod.value = selectionForMode(periodMode.value, periodSelection(selectedTenantId));
    if (route.query.snapshotRunId !== undefined && !snapshotRunId.value) {
      error.value = 'ลิงก์ Snapshot ไม่ถูกต้อง กรุณาเปิดจากข้อความ LINE อีกครั้ง'; loading.value = false; return;
    }
    if (route.query.runId !== undefined && !viewerRunId.value) {
      error.value = 'ลิงก์รายงานไม่ถูกต้อง กรุณาเลือกช่วงข้อมูลแล้วลองใหม่'; loading.value = false; return;
    }
    if (snapshotRunId.value) await loadExistingRun(context, selectedTenantId, selectedReportKey, snapshotRunId.value, true);
    else if (viewerRunId.value) await loadExistingRun(context, selectedTenantId, selectedReportKey, viewerRunId.value, false);
    else {
      snapshotMode.value = false; snapshotMissing.value = false; loading.value = false;
      if (isCurrent(context, selectedTenantId, selectedReportKey)) void resolveSnapshot(selectedPeriod.value);
    }
  } catch (cause) { if (!isCancelled(cause) && context === generation) { error.value = errorMessage(cause); loading.value = false; } }
}

watch(activeTab, (value) => { if (value === 'detail' && run.value?.status === 'SUCCEEDED' && !rowsLoaded.value && !rowsUnavailable.value) void loadRows(0); });
watch(rowGlobalSearch, (value) => {
  if (rowSearchTimer) clearTimeout(rowSearchTimer);
  const normalized = value.trim();
  if (normalized.length === 1) return;
  rowSearchTimer = setTimeout(() => {
    if (normalized === appliedRowGlobalSearch.value) return;
    appliedRowGlobalSearch.value = normalized;
    expandedMobileRows.value = new Set();
    void loadRows(0);
  }, 400);
});
watch([tenantId, reportKey, () => route.query.runId, () => route.query.snapshotRunId], () => void initialize());
onMounted(() => { document.addEventListener('visibilitychange', handleVisibilityChange); void initialize(); });
onBeforeUnmount(() => { document.removeEventListener('visibilitychange', handleVisibilityChange); if (rowSearchTimer) clearTimeout(rowSearchTimer); forceConfirmOpen.value = false; confirm.close(); stopLifecycle(); });
</script>

<template>
  <AppPageHeader :title="definition?.label ?? 'รายงาน'" desktop-mode="viewerCompact"><template #back><Button label="ภาพรวมร้าน" icon="pi pi-arrow-left" text class="report-back-action touch-action" @click="router.push(`/app/tenant/${tenantId}`)" /></template><template #actions><div class="report-heading-actions"><div class="flex flex-wrap gap-2"><Tag v-if="snapshotMode" severity="info" value="ข้อมูลจาก LINE" /><Tag v-if="cachedSnapshot" :severity="freshness.severity" :icon="freshness.icon" :value="freshness.label" /><Tag v-if="showRunStatus" :severity="statusSeverity" :value="statusLabel" /></div><Button v-if="run?.status === 'QUEUED'" icon="pi pi-times" severity="danger" outlined rounded aria-label="ยกเลิกงานที่รอคิว" @click="cancel" /><Button v-else-if="active" icon="pi pi-eye-slash" severity="secondary" outlined rounded aria-label="หยุดติดตามความคืบหน้า" @click="stopFollowing" /></div></template></AppPageHeader>

  <ReportPeriodToolbar desktop-mode="compact" :mode="periodMode" :selection="selectedPeriod" :displayed-label="displayedPeriodLabel" :source-label="sourceLabel" :action-label="periodMode === 'CURRENT_ONLY' ? 'ดูสถานะล่าสุด' : 'ดูช่วงนี้'" force-action-label="ดึงใหม่จาก SML" :loading="loading || cacheLoading" :disabled="forceConfirmOpen" @apply="resolveSnapshot" @force="requestForceRefresh" />

  <Message v-if="cachedSnapshot?.sourceConsistency === 'CHUNK_WINDOW'" severity="info" :closable="false" class="mb-4">รายงานนี้รวบรวมจาก SML หลายช่วงย่อยและไม่ใช่ Snapshot ณ วินาทีเดียว · {{ formatSourceCollection(cachedSnapshot.sourceStartedAt, cachedSnapshot.sourceFinishedAt, cachedSnapshot.sourceConsistency) }}</Message>

  <Message v-if="error" severity="error" :closable="false" class="mb-4">{{ error }}</Message>
  <Message v-if="expiredSnapshot && !cachedSnapshot" severity="warn" :closable="false" class="mb-4"><div class="flex flex-wrap items-center justify-between gap-3"><span>ข้อมูลล่าสุดไม่พร้อมใช้งาน Snapshot เดิมดึงจาก SML เมื่อ {{ formatDateTime(expiredSnapshot.sourceFinishedAt) }} และจะไม่แสดงเป็น KPI หลักโดยอัตโนมัติ</span><Button label="เปิดดู Snapshot เดิม" icon="pi pi-history" size="small" severity="secondary" @click="showExpiredSnapshot" /></div></Message>
  <Message v-if="cachedSnapshot?.freshnessStatus === 'STALE' || cachedSnapshot?.freshnessStatus === 'REFRESHING'" severity="warn" :closable="false" class="mb-4">กำลังแสดง Snapshot ที่ดึงจาก SML เมื่อ {{ formatDateTime(cachedSnapshot.sourceFinishedAt) }}<template v-if="backgroundRefreshing"> ระหว่างดึงข้อมูลใหม่</template><template v-else> หากต้องการข้อมูลล่าสุดให้กด “ดึงใหม่จาก SML”</template></Message>
  <Message v-if="snapshotMissing && run" severity="warn" :closable="false" class="mb-4"><div class="flex flex-wrap items-center justify-between gap-3"><span>ข้อมูลสรุปชุดนี้หมดอายุแล้ว กรุณาดึงข้อมูลใหม่จากฐานร้าน</span><Button :label="reportKey === 'stock_reorder' ? 'ดูสถานะปัจจุบัน' : 'ดึงข้อมูลช่วงเดิมใหม่'" icon="pi pi-refresh" size="small" @click="replaySnapshot" /></div></Message>
  <section v-if="loading || backgroundRefreshing" class="card report-panel" aria-live="polite"><div class="flex flex-col items-center text-center gap-3 py-3"><ProgressSpinner style="width: 2.5rem; height: 2.5rem" stroke-width="6" /><div><h2 class="text-lg m-0">{{ snapshotMode ? 'กำลังเปิดข้อมูลจาก LINE' : cacheLoading ? 'กำลังค้นหา Snapshot' : progressLabel }}</h2><p class="text-muted-color mt-1 mb-0">{{ snapshotMode ? 'กำลังโหลด Snapshot ที่ใช้สร้างข้อความ โดยไม่ดึง SQL ใหม่' : cacheLoading ? 'กำลังอ่าน Cache โดยไม่ดึง SQL ใหม่' : `ใช้เวลาแล้ว ${elapsedSeconds.toLocaleString('th-TH')} วินาที` }}<span v-if="!cacheLoading && typicalDuration"> · {{ typicalDuration }}</span><span v-if="!snapshotMode && !cacheLoading && run?.queuePosition"> · ลำดับคิว {{ run.queuePosition }}</span></p><p v-if="!cacheLoading && run?.progress?.expectedP90Ms && elapsedSeconds * 1000 > run.progress.expectedP90Ms" class="text-orange-600 mt-2 mb-0">ใช้เวลานานกว่าปกติ แต่ระบบยังทำงานอยู่</p></div></div><ProgressBar mode="indeterminate" style="height: .35rem" class="mt-4" /></section>

  <Tabs v-if="dashboard && run" v-model:value="activeTab" class="report-tabs">
    <TabList><Tab value="overview"><i class="pi pi-chart-bar mr-2" />ภาพรวมและกราฟ</Tab><Tab value="detail"><i class="pi pi-table mr-2" />ข้อมูลรายละเอียด</Tab></TabList>
    <TabPanels>
      <TabPanel value="overview">
        <Message v-if="dashboard.quality.status === 'WARNING'" severity="warn" :closable="false" class="mb-4">ข้อมูลหลักพร้อมใช้งาน แต่ไม่มีช่วงเปรียบเทียบบางส่วน</Message>
        <div class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4 mb-5">
          <article v-for="metric in dashboard.kpis" :key="metric.key" class="card dashboard-card report-kpi"><span class="text-sm text-muted-color">{{ metric.label }}</span><strong>{{ formatDashboardValue(metric.value, metric.unit) }}</strong><span v-if="metric.comparison.availability === 'AVAILABLE'" class="metric-comparison"><span><i :class="metric.comparison.direction === 'UP' ? 'pi pi-arrow-up-right' : metric.comparison.direction === 'DOWN' ? 'pi pi-arrow-down-right' : 'pi pi-minus'" /> {{ formatDashboardValue(metric.comparison.delta, metric.unit) }}</span><span>{{ comparisonPeriodText(dashboard.comparisonPeriod) }}</span></span><span v-else class="metric-comparison"><span>ไม่มีข้อมูลเปรียบเทียบที่เทียบช่วงเวลาเดียวกันได้</span></span></article>
        </div>
        <div class="grid grid-cols-1 2xl:grid-cols-2 gap-5"><article v-for="visualization in dashboard.visualizations" :key="visualization.key" class="card dashboard-card report-panel"><h2 class="chart-title">{{ visualization.title }}</h2><ExecutiveChart :visualization="visualization" /></article></div>
        <div v-if="!dashboard.visualizations.length" class="card report-panel text-center text-muted-color"><i class="pi pi-chart-bar text-3xl" /><p class="mb-0">ช่วงนี้ไม่มีข้อมูลเพียงพอสำหรับสร้างกราฟ</p></div>
      </TabPanel>
      <TabPanel value="detail">
        <Message v-if="detailRowsUnavailable" severity="info" :closable="false"><div class="flex flex-wrap items-center justify-between gap-3"><span>ข้อมูลแถวรายละเอียดเป็นข้อมูลชั่วคราวและหมดอายุแล้ว แต่ภาพรวมยังเปิดดูได้ กรุณาดึง SQL ใหม่เมื่อต้องการตรวจรายการ</span><Button :label="reportKey === 'stock_reorder' ? 'ดูสถานะปัจจุบัน' : 'ดึงข้อมูลช่วงเดิมใหม่'" icon="pi pi-refresh" size="small" @click="replaySnapshot" /></div></Message>
        <template v-else>
          <div class="flex flex-wrap items-start justify-between gap-3 mb-4">
            <div><h2 class="text-lg font-semibold m-0">{{ snapshotMode ? 'ข้อมูล Snapshot จาก SQL' : 'ข้อมูลจาก SQL' }}</h2><p class="text-sm text-muted-color mt-1 mb-0">{{ rowTotal.toLocaleString('th-TH') }} แถวที่ตรงกับตัวกรอง · ข้อมูลรายละเอียดเก็บชั่วคราว 24 ชั่วโมง</p></div>
            <div class="flex flex-wrap gap-2"><MultiSelect v-model="selectedColumnKeys" :options="columnOptions" option-label="label" option-value="key" display="chip" filter aria-label="เลือกคอลัมน์ที่ต้องการแสดง" placeholder="เลือกคอลัมน์" class="w-full sm:w-80"><template #option="{ option }"><div class="flex items-center justify-between gap-3 w-full"><span>{{ option.label }}</span><Tag v-if="option.technical" severity="secondary" value="เทคนิค" /></div></template></MultiSelect><Tag v-if="run.isTruncated" severity="warn" value="ผลลัพธ์ถูกจำกัดตามจำนวนแถวสูงสุด" /></div>
          </div>
          <div class="card table-card report-panel">
            <small v-if="rowFilterError" class="block text-red-600 mt-2" role="alert">{{ rowFilterError }}</small>
            <small class="block text-muted-color mt-2">การกรองและเปลี่ยนหน้าอ่านจาก Snapshot ที่บันทึกไว้เท่านั้น ไม่ดึงข้อมูลใหม่จาก SML</small>
            <div class="hidden md:block mt-4"><DataTable v-model:filters="rowPrimeFilters" :value="rows" :loading="loadingRows" data-key="__rowKey" lazy paginator :first="rowPage * rowPageSize" :rows="rowPageSize" :total-records="rowTotal" :rows-per-page-options="[25, 50, 100]" filter-display="menu" row-hover show-gridlines scrollable striped-rows current-page-report-template="หน้า {currentPage} จาก {totalPages} · ทั้งหมด {totalRecords} รายการ" paginator-template="RowsPerPageDropdown FirstPageLink PrevPageLink PageLinks NextPageLink LastPageLink CurrentPageReport" @page="changeRowsPage" @filter="applyRowFilters"><template #header><SakaiTableHeader v-model:global-search="rowGlobalSearch" :loading="loadingRows" :has-filters="rowHasFilters" mobile-filters @clear="clearRowFilter" @open-mobile-filters="openMobileRowFilters" /></template><Column v-for="column in displayedColumns" :key="column.key" :field="column.key" :header="column.label" :frozen="column.frozen" align-frozen="left" :header-class="reportColumnClass(column)" :body-class="reportColumnClass(column)" :show-filter-menu="rowCapabilityByKey.has(column.key)" :show-filter-match-modes="false"><template #body="{ data }"><span class="safe-wrap" :class="{ 'metric-value': column.dataType === 'NUMBER' }">{{ formatReportCell(data[column.key], column) }}</span></template><template v-if="rowCapabilityByKey.has(column.key)" #filter="{ filterModel }"><DatePicker v-if="rowCapabilityByKey.get(column.key)?.dataType === 'DATE'" v-model="filterModel.value" selection-mode="range" date-format="dd/mm/yy" placeholder="เลือกช่วงวันที่" /><div v-else class="grid gap-2"><Select v-model="rowFilterOperators[column.key]" :options="rowOperatorOptions(rowCapabilityByKey.get(column.key)!)" option-label="label" option-value="value" aria-label="เงื่อนไขตัวกรอง" /><InputText v-model="filterModel.value" :placeholder="rowCapabilityByKey.get(column.key)?.dataType === 'NUMBER' ? 'กรอกตัวเลข' : 'กรอกคำค้นหา'" /></div></template></Column><template #empty><div class="py-8 text-center text-muted-color">{{ loadingRows ? 'กำลังโหลดข้อมูล' : 'ไม่พบข้อมูลที่ตรงกับตัวกรอง' }} <Button v-if="rowHasFilters" label="ล้างตัวกรอง" text size="small" @click="clearRowFilter" /></div></template></DataTable></div>
            <div class="md:hidden grid gap-3 mt-4" aria-label="รายละเอียดรายงานแบบมือถือ"><article v-for="row in rows" :key="String(row.__rowKey)" class="mobile-row"><div v-for="column in mobileColumns(row)" :key="column.key" class="flex items-start justify-between gap-4"><span class="text-xs text-muted-color safe-wrap">{{ column.label }}</span><strong class="text-sm safe-wrap" :class="column.dataType === 'NUMBER' ? 'text-right metric-value' : 'text-left'">{{ formatReportCell(row[column.key], column) }}</strong></div><Button v-if="displayedColumns.length > mobileSummaryColumns.length" :label="expandedMobileRows.has(String(row.__rowKey)) ? 'แสดงน้อยลง' : 'ดูรายละเอียดเพิ่ม'" :icon="expandedMobileRows.has(String(row.__rowKey)) ? 'pi pi-angle-up' : 'pi pi-angle-down'" text size="small" class="justify-self-start" @click="toggleMobileRow(row)" /></article><div v-if="!loadingRows && !rows.length" class="py-8 text-center text-muted-color">ไม่พบข้อมูลที่ตรงกับตัวกรอง</div></div>
            <div class="md:hidden mt-3"><SakaiTableHeader v-model:global-search="rowGlobalSearch" :loading="loadingRows" :has-filters="rowHasFilters" mobile-filters @clear="clearRowFilter" @open-mobile-filters="openMobileRowFilters" /><Paginator :first="rowPage * rowPageSize" :rows="rowPageSize" :total-records="rowTotal" :rows-per-page-options="[25, 50, 100]" template="RowsPerPageDropdown PrevPageLink CurrentPageReport NextPageLink" current-page-report-template="หน้า {currentPage} จาก {totalPages} · ทั้งหมด {totalRecords} รายการ" @page="changeRowsPage" /></div>
          </div>
        </template>
      </TabPanel>
    </TabPanels>
  </Tabs>
  <Dialog :visible="mobileRowFiltersOpen" modal header="กรองข้อมูลรายละเอียด" class="responsive-dialog" :style="{ width: '34rem' }" @update:visible="(value) => { if (!value) closeMobileRowFilters(); }">
    <div class="grid gap-4">
      <div v-for="column in filterableDisplayedColumns" :key="column.key" class="grid gap-2">
        <label :for="`mobile-filter-${column.key}`" class="font-semibold">{{ column.label }}</label>
        <DatePicker v-if="rowCapabilityByKey.get(column.key)?.dataType === 'DATE'" :input-id="`mobile-filter-${column.key}`" :model-value="dateRowFilterValue(column.key)" selection-mode="range" date-format="dd/mm/yy" placeholder="เลือกวันเริ่มต้นและวันสิ้นสุด" fluid @update:model-value="setRowFilterValue(column.key, $event as Date[] | null)" />
        <template v-else>
          <Select v-model="rowFilterOperators[column.key]" :options="rowOperatorOptions(rowCapabilityByKey.get(column.key)!)" option-label="label" option-value="value" aria-label="เงื่อนไขตัวกรอง" fluid />
          <InputText :id="`mobile-filter-${column.key}`" :model-value="textRowFilterValue(column.key)" :placeholder="rowCapabilityByKey.get(column.key)?.dataType === 'NUMBER' ? 'กรอกตัวเลข' : 'กรอกคำค้นหา'" fluid @update:model-value="setRowFilterValue(column.key, String($event ?? ''))" />
        </template>
      </div>
    </div>
    <template #footer><Button label="ยกเลิก" text @click="closeMobileRowFilters()" /><Button label="ล้างตัวกรอง" icon="pi pi-filter-slash" text @click="clearRowFilter" /><Button label="ใช้ตัวกรอง" icon="pi pi-filter" :loading="loadingRows" @click="applyRowFilters()" /></template>
  </Dialog>
</template>

<style scoped>
.report-panel { border-radius: var(--content-border-radius); }
.report-heading-actions { display: flex; align-items: center; justify-content: flex-end; gap: .75rem; }
.dashboard-card { margin-bottom: 0; }
.report-tabs :deep(.p-tabpanels) { padding: 1.25rem 0 0; background: transparent; }
.report-kpi { display: grid; gap: .55rem; min-height: 7.5rem; padding: 1.15rem; border-radius: var(--content-border-radius); }
.report-kpi strong { font-size: 1.5rem; line-height: 1.1; font-variant-numeric: tabular-nums; overflow-wrap: anywhere; }
.metric-comparison { display: grid; gap: .2rem; color: var(--text-color-secondary); font-size: .75rem; }
.chart-title { margin: 0 0 .75rem; font-size: 1.05rem; }
.chart-empty { display: grid; justify-items: center; gap: .4rem; min-height: 10rem; place-content: center; color: var(--text-color-secondary); text-align: center; }
.chart-empty i { color: var(--primary-color); font-size: 1.5rem; }
.chart-empty strong { color: var(--text-color); }
.report-row-filter { display: flex; flex-wrap: wrap; align-items: center; gap: .75rem; }
.mobile-row { padding: 1rem; display: grid; gap: .65rem; border: 1px solid var(--surface-border); border-radius: var(--content-border-radius); }
@media (max-width: 767px) {
  .report-back-action { display: none; }
  .report-heading-actions { width: 100%; align-items: flex-start; justify-content: space-between; flex-wrap: wrap; }
  .report-heading-actions .p-button { width: auto; }
}
</style>
