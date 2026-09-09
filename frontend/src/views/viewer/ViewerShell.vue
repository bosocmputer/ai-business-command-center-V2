<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useToast } from 'primevue/usetoast';
import liff from '@line/liff';
import { ApiError, reportDefinitionByKey, viewerApi, type DeliveryContext, type ReportKey, type ViewerMe } from '@/api';
import AppShell from '@/layout/AppShell.vue';
import type { NavigationItem } from '@/layout/menu';
import { useViewerSession } from '@/stores/viewer';
import { reportIcon } from '@/utils/dashboard';
import { errorMessage } from '@/utils/format';
import { cleanViewerQuery } from '@/utils/viewerSnapshot';
import {
  clearViewerLineLoginRetry,
  exchangeViewerLineSession,
  lineLoginRedirectUri,
  viewerAuthenticationPlan,
  ViewerLineSessionError
} from '@/utils/viewerLineSession';
import {
  deliveryContextRoute,
  explicitViewerTenantId,
  parseViewerEntryReferences,
  resolveViewerHome,
  resolveViewerNavigationReportKeys,
  resolveViewerTenantRoute,
  viewerNavigationScope,
  type ViewerNavigationScope
} from '@/utils/viewerRouting';

const route = useRoute(); const router = useRouter();
const toast = useToast();
const { state, loadViewer, setViewer, setTenants, selectTenant, setReports, setDeliveryContext, ensureReports, clearViewer } = useViewerSession();
type ViewerStage = 'INITIALIZING' | 'CHOOSING' | 'VALIDATING' | 'READY' | 'SIGNING_OUT' | 'SIGNED_OUT' | 'UNAVAILABLE';

const stage = ref<ViewerStage>('INITIALIZING'); const message = ref('');
const lineIdentityFailure = ref(false);
const switchingTenantId = ref('');
let contextController: AbortController | undefined;
let contextGeneration = 0;
let routeRecovery = false;
const shellReady = computed(() => stage.value === 'READY' || stage.value === 'CHOOSING');
const navigationScope = computed<ViewerNavigationScope>(() => viewerNavigationScope(route.name));
const selectedTenant = computed(() => state.tenants.find((tenant) => tenant.id === state.selectedTenantId));
const routeTenant = computed(() => typeof route.params.tenantId === 'string' ? state.tenants.find((tenant) => tenant.id === route.params.tenantId) : undefined);
const contextTenantId = computed(() => routeTenant.value?.id ?? '');
const deliveryId = computed(() => typeof route.params.deliveryId === 'string' ? route.params.deliveryId : '');
const activeDeliveryContext = computed(() => {
  if (navigationScope.value !== 'DELIVERY' || !deliveryId.value) return undefined;
  const context = state.deliveryContexts[deliveryId.value];
  return context?.tenantId === contextTenantId.value ? context : undefined;
});
const selectedTenantId = computed({
  get: () => switchingTenantId.value || state.selectedTenantId,
  set: (tenantId: string) => { void switchTenant(tenantId); }
});
const tenantOptions = computed(() => [...state.tenants]);
const tenantReports = computed(() => state.reportsByTenant[contextTenantId.value]
  ?? routeTenant.value?.reportKeys.map((reportKey) => reportDefinitionByKey.get(reportKey)).filter((item) => item !== undefined)
  ?? []);
const navigationReportKeys = computed(() => resolveViewerNavigationReportKeys(
  navigationScope.value,
  tenantReports.value.map((report) => report.reportKey),
  activeDeliveryContext.value?.reports.map((report) => report.reportKey)
));
const navigationReports = computed(() => {
  if (navigationScope.value === 'DELIVERY') {
    const reportsByKey = new Map(activeDeliveryContext.value?.reports.map((report) => [report.reportKey, report]) ?? []);
    return navigationReportKeys.value.map((reportKey) => reportsByKey.get(reportKey)).filter((item) => item !== undefined);
  }
  const reportsByKey = new Map(tenantReports.value.map((report) => [report.reportKey, report]));
  return navigationReportKeys.value.map((reportKey) => reportsByKey.get(reportKey)).filter((item) => item !== undefined);
});
const mobileTitle = computed(() => {
  if (switchingTenantId.value) return 'กำลังเปลี่ยนร้าน';
  if (typeof route.params.tenantId === 'string' && !routeTenant.value) return 'กำลังเปลี่ยนร้าน';
  return routeTenant.value?.name ?? 'Nextstep Dashboard';
});
const mobileSubtitle = computed(() => {
  if (route.name === 'viewer-overview') return 'ภาพรวม';
  if (route.name === 'viewer-report') {
    const key = typeof route.params.reportKey === 'string' ? route.params.reportKey : '';
    return navigationReports.value.find((report) => report.reportKey === key)?.label ?? 'รายงาน';
  }
  if (route.name === 'viewer-delivery') return 'รายงานจาก LINE';
  if (route.name === 'viewer-delivery-report') {
    const key = typeof route.params.reportKey === 'string' ? route.params.reportKey : '';
    return navigationReports.value.find((report) => report.reportKey === key)?.label ?? 'รายงานจาก LINE';
  }
  return 'Dashboard';
});
const scopeHomeTo = computed(() => {
  if (navigationScope.value === 'DELIVERY' && activeDeliveryContext.value) {
    return deliveryContextRoute(activeDeliveryContext.value.tenantId, activeDeliveryContext.value.deliveryId);
  }
  return routeTenant.value ? `/app/tenant/${routeTenant.value.id}` : '/app';
});
const mobileHomeTo = computed(() => scopeHomeTo.value);
const menuModel = computed<NavigationItem[]>(() => {
  if (navigationScope.value === 'DELIVERY') {
    const context = activeDeliveryContext.value;
    if (!context) return [];
    return [
      { label: 'รายงานใน LINE รอบนี้', items: [
        { label: 'สรุปรอบส่ง', icon: 'pi pi-fw pi-inbox', to: deliveryContextRoute(context.tenantId, context.deliveryId) },
        ...navigationReports.value.map((report) => ({
          label: report.label,
          icon: reportIcon(report.reportKey),
          to: deliveryContextRoute(context.tenantId, context.deliveryId, report.reportKey)
        }))
      ] },
      { label: 'Dashboard ร้าน', items: [
        { label: 'ไป Dashboard ร้าน', icon: 'pi pi-fw pi-arrow-right', to: `/app/tenant/${context.tenantId}` }
      ] }
    ];
  }
  if (!contextTenantId.value) return [];
  const groups: NavigationItem[] = [
    { label: 'ภาพรวมผู้บริหาร', items: [
      { label: 'ภาพรวมร้าน', icon: 'pi pi-fw pi-home', to: `/app/tenant/${contextTenantId.value}` }
    ] }
  ];
  if (navigationReports.value.length) groups.push({ label: 'รายงานที่มีสิทธิ์เปิดดู', items: navigationReports.value.map((report) => ({
      label: report.label,
      icon: reportIcon(report.reportKey),
      to: `/app/tenant/${contextTenantId.value}/report/${report.reportKey}`
    })) });
  return groups;
});

async function prepareTenantContext(generation: number, signal: AbortSignal) {
  const resolution = resolveViewerTenantRoute(route.params.tenantId, state.tenants.map((tenant) => tenant.id), state.selectedTenantId);
  if (resolution.kind === 'UNAVAILABLE') throw new ViewerContextUnavailableError();
  const targetTenant = state.tenants.find((tenant) => tenant.id === resolution.tenantId);
  if (!targetTenant) throw new ViewerContextUnavailableError();
  if (viewerNavigationScope(route.name) === 'DELIVERY') {
    const currentDeliveryId = typeof route.params.deliveryId === 'string' ? route.params.deliveryId : '';
    if (!currentDeliveryId) throw new ViewerContextUnavailableError();
    let context = state.deliveryContexts[currentDeliveryId];
    if (!context || context.tenantId !== resolution.tenantId) {
      const fetchedContext = await viewerApi.deliveryContext(resolution.tenantId, currentDeliveryId, signal);
      setDeliveryContext(fetchedContext);
      context = state.deliveryContexts[currentDeliveryId];
    }
    if (!context) throw new ViewerContextUnavailableError();
    if (context.tenantId !== resolution.tenantId || context.deliveryId !== currentDeliveryId) throw new ViewerContextUnavailableError();
    const requestedReportKey = typeof route.params.reportKey === 'string' ? route.params.reportKey : '';
    if (requestedReportKey && !context.reports.some((report) => report.reportKey === requestedReportKey)) throw new ViewerContextUnavailableError();
  } else {
    const requestedReportKey = typeof route.params.reportKey === 'string' ? route.params.reportKey : '';
    if (requestedReportKey && !targetTenant.reportKeys.includes(requestedReportKey as ReportKey)) throw new ViewerContextUnavailableError();
    if (targetTenant.reportKeys.length) await ensureReports(resolution.tenantId, false, signal);
    else setReports(resolution.tenantId, []);
  }
  if (signal.aborted || generation !== contextGeneration) return false;
  selectTenant(resolution.tenantId);
  return true;
}

async function openViewerHome(generation: number) {
  const resolution = resolveViewerHome(state.tenants.map((tenant) => tenant.id));
  if (resolution.kind === 'AUTO') {
    await router.replace(`/app/tenant/${resolution.tenantId}`);
    return;
  }
  if (generation === contextGeneration) stage.value = 'CHOOSING';
}

async function exchangeLineIdentity(invitationReference?: string, deliveryReference?: string): Promise<ViewerMe | undefined> {
  const liffId = import.meta.env.VITE_LINE_LIFF_ID;
  if (!liffId || liffId.startsWith('replace-')) throw new Error('ยังไม่ได้ตั้งค่า LINE LIFF ID สำหรับ environment นี้');
  const expectedTenantId = deliveryReference ? explicitViewerTenantId(route.params.tenantId) : undefined;
  const result = await exchangeViewerLineSession({
    liff,
    liffId,
    redirectUri: lineLoginRedirectUri(window.location.href, invitationReference),
    exchange: (idToken) => viewerApi.exchange(idToken, invitationReference, expectedTenantId ? deliveryReference : undefined, expectedTenantId),
    retryStorage: viewerLineRetryStorage()
  });
  return result.kind === 'EXCHANGED' ? result.value : undefined;
}

async function applyLineExchange(me: ViewerMe, invitationReference: string | undefined, deliveryReference: string | undefined, generation: number, signal: AbortSignal) {
  if (signal.aborted || generation !== contextGeneration) return;
  const tenants = (await viewerApi.tenants()).data;
  if (signal.aborted || generation !== contextGeneration) return;
  setViewer(me); setTenants(tenants);
  const expectedTenantId = deliveryReference ? explicitViewerTenantId(route.params.tenantId) : undefined;
  if (deliveryReference) {
    await removeDeliveryReference();
    if (!expectedTenantId || me.deliveryContextErrorCode || !me.deliveryContext) throw new ViewerContextUnavailableError();
    await openDeliveryContext(me.deliveryContext, generation, signal);
    return;
  }
  if (invitationReference || route.name === 'viewer-home' || route.name === 'viewer-invite') {
    if (invitationReference || route.name !== 'viewer-home') await router.replace({ path: '/app' });
    await openViewerHome(generation);
    return;
  }
  if (!await prepareTenantContext(generation, signal)) return;
  stage.value = 'READY';
}

function viewerLineRetryStorage(): Storage | undefined {
  try { return window.sessionStorage; }
  catch { return undefined; }
}

async function validateTenantRoute() {
  const generation = ++contextGeneration;
  contextController?.abort('viewer-context-changed');
  contextController = new AbortController();
  stage.value = 'VALIDATING'; message.value = '';
  try {
    if (route.name === 'viewer-home') {
      await openViewerHome(generation);
      return;
    }
    if (await prepareTenantContext(generation, contextController.signal)) stage.value = 'READY';
  } catch (cause) {
    if (contextController.signal.aborted || generation !== contextGeneration) return;
    stage.value = 'UNAVAILABLE';
    message.value = cause instanceof ViewerContextUnavailableError
      ? 'ไม่สามารถเปิดร้านหรือไม่มีสิทธิ์ใช้งาน'
      : errorMessage(cause);
  }
}

async function initialize() {
  const generation = ++contextGeneration;
  contextController?.abort('viewer-initializing');
  contextController = new AbortController();
  stage.value = 'INITIALIZING'; message.value = ''; lineIdentityFailure.value = false;
  const references = parseViewerEntryReferences(route.query, route.path.endsWith('/invite'));
  if (references.error) {
    stage.value = 'UNAVAILABLE'; message.value = 'ลิงก์ Dashboard ไม่ถูกต้องหรือไม่พร้อมใช้งาน'; return;
  }
  const { invitationReference, deliveryReference } = references;
  try {
    if (viewerAuthenticationPlan(invitationReference) === 'LINE_IDENTITY_REQUIRED') {
      const me = await exchangeLineIdentity(invitationReference, deliveryReference);
      if (!me) return;
      await applyLineExchange(me, invitationReference, deliveryReference, generation, contextController.signal);
      return;
    }
    try {
      await loadViewer();
      if (deliveryReference) {
        const expectedTenantId = explicitViewerTenantId(route.params.tenantId);
        await removeDeliveryReference();
        if (!expectedTenantId) throw new ViewerContextUnavailableError();
        const deliveryContext = await viewerApi.resolveDeliveryContext(deliveryReference, expectedTenantId, contextController.signal);
        await openDeliveryContext(deliveryContext, generation, contextController.signal);
        return;
      }
      if (route.name === 'viewer-home') {
        await openViewerHome(generation);
        return;
      }
      if (!await prepareTenantContext(generation, contextController.signal)) return;
      stage.value = 'READY'; return;
    }
    catch (cause) { if (!(cause instanceof ApiError && cause.status === 401)) throw cause; }
    const me = await exchangeLineIdentity(invitationReference, deliveryReference);
    if (!me) return;
    await applyLineExchange(me, invitationReference, deliveryReference, generation, contextController.signal);
  } catch (cause) {
    if (contextController.signal.aborted || generation !== contextGeneration) return;
    stage.value = 'UNAVAILABLE';
    if (cause instanceof ViewerContextUnavailableError) message.value = 'ไม่สามารถเปิดร้านหรือไม่มีสิทธิ์ใช้งาน';
    else if (cause instanceof ApiError && ['DELIVERY_CONTEXT_UNAVAILABLE', 'DELIVERY_CONTEXT_PERMISSION_CHANGED', 'DELIVERY_REFERENCE_FORBIDDEN'].includes(cause.code)) message.value = 'ลิงก์รายงานนี้ไม่ถูกต้องหรือไม่พร้อมใช้งาน';
    else if (cause instanceof ApiError && cause.code === 'LINE_IDENTITY_FORBIDDEN') message.value = 'LINE บัญชีนี้ยังไม่ได้รับเชิญหรือถูกยกเลิกสิทธิ์';
    else if (cause instanceof ApiError && cause.code === 'LINE_ID_TOKEN_INVALID') {
      lineIdentityFailure.value = true;
      message.value = liff.isInClient()
        ? 'LINE ไม่สามารถยืนยันตัวตนได้ กรุณาปิดหน้านี้แล้วเปิดลิงก์เชิญจาก LINE อีกครั้ง'
        : 'LINE Login หมดอายุหรือยืนยันไม่สำเร็จ กรุณากด “ยืนยัน LINE ใหม่”';
    }
    else if (cause instanceof ViewerLineSessionError) {
      lineIdentityFailure.value = true;
      message.value = 'LINE ไม่ส่งข้อมูลยืนยันตัวตน กรุณากด “ยืนยัน LINE ใหม่”';
    }
    else message.value = errorMessage(cause);
  }
}

async function retryAuthentication() {
  if (lineIdentityFailure.value) clearViewerLineLoginRetry(viewerLineRetryStorage());
  await initialize();
}

async function removeDeliveryReference() {
  await router.replace({ path: route.path, query: cleanViewerQuery(route.query), hash: route.hash });
}

async function openDeliveryContext(context: DeliveryContext, generation: number, signal: AbortSignal) {
  const expectedTenantId = explicitViewerTenantId(route.params.tenantId);
  if (!expectedTenantId || context.tenantId !== expectedTenantId) throw new ViewerContextUnavailableError();
  if (!state.tenants.some((tenant) => tenant.id === expectedTenantId)) throw new ViewerContextUnavailableError();
  setDeliveryContext(context);
  const requestedReportKey = typeof route.params.reportKey === 'string' && context.reports.some((item) => item.reportKey === route.params.reportKey)
    ? route.params.reportKey as ReportKey
    : undefined;
  if (signal.aborted || generation !== contextGeneration) return;
  selectTenant(context.tenantId);
  await router.replace(deliveryContextRoute(context.tenantId, context.deliveryId, requestedReportKey));
  if (generation === contextGeneration) stage.value = 'READY';
}

async function switchTenant(tenantId: string) {
  if (!tenantId || tenantId === state.selectedTenantId && route.params.tenantId === tenantId) return;
  switchingTenantId.value = tenantId;
  stage.value = 'VALIDATING';
  try {
    const targetTenant = state.tenants.find((tenant) => tenant.id === tenantId);
    if (!targetTenant) throw new ViewerContextUnavailableError();
    if (targetTenant.reportKeys.length) await ensureReports(tenantId);
    else setReports(tenantId, []);
    selectTenant(tenantId);
    await router.push(`/app/tenant/${tenantId}`);
  } catch (cause) {
    stage.value = 'UNAVAILABLE';
    message.value = 'ไม่สามารถเปิดร้านหรือไม่มีสิทธิ์ใช้งาน';
    toast.add({ severity: 'error', summary: 'เปลี่ยนร้านไม่สำเร็จ', detail: errorMessage(cause), life: 4500 });
  }
  finally { if (switchingTenantId.value === tenantId) switchingTenantId.value = ''; }
}

async function logout() {
  if (stage.value === 'SIGNING_OUT') return;
  const previousStage = stage.value;
  ++contextGeneration;
  contextController?.abort('viewer-signing-out');
  stage.value = 'SIGNING_OUT';
  message.value = '';
  try {
    await viewerApi.logout();
  } catch (cause) {
    if (!(cause instanceof ApiError && cause.status === 401)) {
      stage.value = previousStage;
      toast.add({ severity: 'error', summary: 'ออกจากระบบไม่สำเร็จ', detail: errorMessage(cause), life: 4500 });
      return;
    }
  }
  clearViewer();
  clearViewerLineLoginRetry(viewerLineRetryStorage());
  await router.replace('/app');
  stage.value = 'SIGNED_OUT';
}
async function signInAgain() { await initialize(); }
async function goToAccessibleTenant() {
  routeRecovery = true;
  try { await router.replace('/app'); }
  finally { routeRecovery = false; }
  await initialize();
}
function handleUnauthorized(event: Event) {
  if ((event as CustomEvent<{ scope?: string }>).detail?.scope !== 'viewer' || !shellReady.value) return;
  clearViewer(); stage.value = 'UNAVAILABLE'; message.value = 'Session LINE หมดอายุ กรุณากด “ลองใหม่” เพื่อยืนยันตัวตนอีกครั้ง';
}
onMounted(() => { window.addEventListener('nextstep:unauthorized', handleUnauthorized); void initialize(); });
onBeforeUnmount(() => { contextController?.abort('viewer-shell-unmounted'); window.removeEventListener('nextstep:unauthorized', handleUnauthorized); });
watch([
  () => route.name,
  () => route.params.tenantId,
  () => route.params.deliveryId,
  () => route.params.reportKey
], () => {
  if (!routeRecovery && state.ready) void validateTenantRoute();
}, { flush: 'sync' });

class ViewerContextUnavailableError extends Error {}
</script>

<template>
  <div v-if="!shellReady" class="viewer-gate min-h-screen grid place-items-center px-4 bg-surface-50 dark:bg-surface-950">
    <div v-if="stage === 'INITIALIZING' || stage === 'VALIDATING' || stage === 'SIGNING_OUT'" class="w-full max-w-2xl card"><div class="flex flex-col items-center text-center gap-3 mb-5 py-2"><ProgressSpinner style="width: 2.5rem; height: 2.5rem" stroke-width="6" /><div><h1 class="text-xl font-semibold m-0">{{ stage === 'INITIALIZING' ? 'กำลังยืนยัน LINE' : stage === 'VALIDATING' ? 'กำลังตรวจสอบสิทธิ์' : 'กำลังออกจากระบบ' }}</h1><p class="text-muted-color mt-1 mb-0">{{ stage === 'SIGNING_OUT' ? 'กำลังสิ้นสุด Session Dashboard อย่างปลอดภัย' : 'ตรวจสอบตัวตน ร้านค้า และสิทธิ์ล่าสุด' }}</p></div></div><Skeleton v-if="stage !== 'SIGNING_OUT'" height="10rem" /></div>
    <section v-else-if="stage === 'SIGNED_OUT'" class="w-full max-w-xl card text-center"><i class="pi pi-check-circle text-5xl text-green-500" /><h1 class="text-2xl font-bold mb-2">ออกจากระบบแล้ว</h1><p class="text-muted-color safe-wrap">Session Dashboard บนอุปกรณ์นี้สิ้นสุดแล้ว</p><Button class="mt-4" label="เข้าสู่ระบบด้วย LINE" icon="pi pi-sign-in" @click="signInAgain" /></section>
    <section v-else class="w-full max-w-xl card text-center"><i class="pi pi-shield text-5xl text-red-500" /><h1 class="text-2xl font-bold mb-2">ไม่สามารถเปิด Dashboard</h1><p class="text-muted-color safe-wrap">{{ message }}</p><div class="flex flex-wrap justify-center gap-2 mt-4"><Button label="กลับไปร้านที่ฉันเข้าถึงได้" icon="pi pi-home" @click="goToAccessibleTenant" /><Button :label="lineIdentityFailure ? 'ยืนยัน LINE ใหม่' : 'ลองใหม่'" icon="pi pi-refresh" severity="secondary" outlined @click="retryAuthentication" /></div></section>
  </div>
  <AppShell v-else :menu-model="menuModel" :home-to="scopeHomeTo" :mobile-title="mobileTitle" :mobile-subtitle="mobileSubtitle" :mobile-home-to="mobileHomeTo" :account-label="state.me?.displayName" confirm-dialogs @sign-out="logout">
    <template #topbar-context>
      <span v-if="navigationScope === 'DELIVERY' && routeTenant" class="self-center text-sm font-medium"><i class="pi pi-inbox mr-2 text-primary" />{{ routeTenant.name }} · รายงานจาก LINE</span>
      <Select v-else-if="route.name !== 'viewer-home' && state.tenants.length > 1" v-model="selectedTenantId" :options="tenantOptions" option-label="name" option-value="id" aria-label="เลือกร้านค้า" class="viewer-tenant-select" />
      <span v-else-if="route.name !== 'viewer-home' && selectedTenant" class="self-center text-sm font-medium"><i class="pi pi-building mr-2 text-primary" />{{ selectedTenant.name }}</span>
    </template>
    <template #sidebar-context>
      <div v-if="navigationScope === 'DELIVERY' && routeTenant" class="grid gap-1"><span class="text-xs font-semibold text-muted-color">รายงานจาก LINE</span><span class="font-semibold safe-wrap"><i class="pi pi-building mr-2 text-primary" />{{ routeTenant.name }}</span></div>
      <div v-else-if="route.name !== 'viewer-home' && state.tenants.length > 1" class="grid gap-2"><label for="mobile-tenant" class="text-xs font-semibold">ร้านที่กำลังดู</label><Select input-id="mobile-tenant" v-model="selectedTenantId" :options="tenantOptions" option-label="name" option-value="id" aria-label="ร้านที่กำลังดู" fluid /></div>
      <div v-else-if="route.name !== 'viewer-home' && selectedTenant" class="font-semibold safe-wrap"><i class="pi pi-building mr-2 text-primary" />{{ selectedTenant.name }}</div>
    </template>
    <RouterView />
  </AppShell>
</template>

<style scoped>
.viewer-tenant-select { width: min(15rem, 30vw); }
@media (max-width: 575px) { .viewer-tenant-select { width: 8rem; } }
</style>
