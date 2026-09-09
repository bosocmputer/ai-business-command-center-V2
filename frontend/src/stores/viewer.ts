import { reactive, readonly } from 'vue';
import { viewerApi, type DeliveryContext, type ReportDefinition, type ViewerMe, type ViewerTenant } from '@/api';
import { clearStoredPeriodSelections, defaultPeriodSelection, loadPeriodSelection, savePeriodSelection, type ReportPeriodSelection } from '@/utils/reportPeriod';

type ViewerState = {
  me: ViewerMe | null;
  tenants: ViewerTenant[];
  selectedTenantId: string;
  reportsByTenant: Record<string, ReportDefinition[]>;
  periodByTenant: Record<string, ReportPeriodSelection>;
  deliveryContexts: Record<string, DeliveryContext>;
  ready: boolean;
};

const state = reactive<ViewerState>({ me: null, tenants: [], selectedTenantId: '', reportsByTenant: {}, periodByTenant: {}, deliveryContexts: {}, ready: false });

async function loadViewer(): Promise<void> {
  state.me = await viewerApi.me();
  setTenants((await viewerApi.tenants()).data);
  state.ready = true;
}

function setViewer(me: ViewerMe): void { state.me = me; state.ready = true; }
function setTenants(tenants: ViewerTenant[]): void {
  state.tenants = tenants;
  const authorizedTenantIds = new Set(tenants.map((tenant) => tenant.id));
  clearStoredPeriodSelections([...authorizedTenantIds]);
  Object.keys(state.periodByTenant).forEach((tenantId) => { if (!authorizedTenantIds.has(tenantId)) delete state.periodByTenant[tenantId]; });
  Object.keys(state.reportsByTenant).forEach((tenantId) => { if (!authorizedTenantIds.has(tenantId)) delete state.reportsByTenant[tenantId]; });
  Object.entries(state.deliveryContexts).forEach(([deliveryId, context]) => { if (!authorizedTenantIds.has(context.tenantId)) delete state.deliveryContexts[deliveryId]; });
  if (!authorizedTenantIds.has(state.selectedTenantId)) state.selectedTenantId = tenants.length === 1 ? (tenants[0]?.id ?? '') : '';
}
function selectTenant(tenantId: string): void {
  if (state.tenants.some((tenant) => tenant.id === tenantId)) state.selectedTenantId = tenantId;
}
function setReports(tenantId: string, reports: ReportDefinition[]): void { state.reportsByTenant[tenantId] = reports; }
function setDeliveryContext(context: DeliveryContext): void { state.deliveryContexts[context.deliveryId] = context; }
function periodSelection(tenantId: string): ReportPeriodSelection {
  if (!tenantId) return defaultPeriodSelection();
  state.periodByTenant[tenantId] ??= loadPeriodSelection(tenantId);
  return { ...state.periodByTenant[tenantId] };
}
function setPeriodSelection(tenantId: string, selection: ReportPeriodSelection): void {
  state.periodByTenant[tenantId] = { ...selection };
  savePeriodSelection(tenantId, selection);
}
async function ensureReports(tenantId: string, force = false, signal?: AbortSignal): Promise<ReportDefinition[]> {
  if (!force && state.reportsByTenant[tenantId]) return state.reportsByTenant[tenantId];
  const reports = (await viewerApi.reports(tenantId, signal)).data;
  setReports(tenantId, reports);
  return reports;
}
function clearViewer(): void {
  state.me = null;
  state.tenants = [];
  state.selectedTenantId = '';
  state.reportsByTenant = {};
  state.periodByTenant = {};
  state.deliveryContexts = {};
  clearStoredPeriodSelections();
  state.ready = false;
}

export function useViewerSession() {
  return { state: readonly(state), loadViewer, setViewer, setTenants, selectTenant, setReports, setDeliveryContext, ensureReports, periodSelection, setPeriodSelection, clearViewer };
}
