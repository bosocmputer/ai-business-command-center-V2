<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { adminApi } from '@/api';
import { useAdminSession } from '@/stores/session';
import { beginAdminTenantContext, clearAdminTenantContext, resolveAdminMobileContext } from '@/stores/adminTenantContext';
import type { NavigationItem } from './menu';
import AppShell from './AppShell.vue';

const router = useRouter();
const route = useRoute();
const { state, logout } = useAdminSession();
const routeTenantId = computed(() => typeof route.params.tenantId === 'string' ? route.params.tenantId : '');
const pageTitle = computed(() => typeof route.meta.pageTitle === 'string' ? route.meta.pageTitle : 'Nextstep Admin');
const mobileContext = computed(() => resolveAdminMobileContext(routeTenantId.value, pageTitle.value));
watch(routeTenantId, (tenantId) => {
  if (tenantId) beginAdminTenantContext(tenantId);
  else clearAdminTenantContext();
}, { immediate: true, flush: 'sync' });
const openP1Count = ref(0);
const openP1HasMore = ref(false);
const openP1Badge = computed(() => openP1Count.value ? (openP1HasMore.value ? `${openP1Count.value}+` : openP1Count.value) : undefined);
let incidentPollTimer: number | undefined;
let incidentRequest: AbortController | undefined;

const model = computed<NavigationItem[]>(() => [
  { label: 'จัดการระบบ', items: [
    { label: 'ภาพรวม', icon: 'pi pi-fw pi-home', to: '/admin' },
    { label: 'ร้านค้า', icon: 'pi pi-fw pi-building', to: '/admin/tenants', activePrefix: '/admin/tenants' }
  ] },
  { label: 'ติดตามการทำงาน', items: [
    { label: 'เหตุสำคัญ', icon: 'pi pi-fw pi-exclamation-triangle', to: '/admin/operational-incidents', activePrefix: '/admin/operational-incidents', badge: openP1Badge.value, badgeSeverity: 'danger' },
    { label: 'การสร้างรายงาน', icon: 'pi pi-fw pi-database', to: '/admin/report-runs' },
    { label: 'การส่ง LINE', icon: 'pi pi-fw pi-send', to: '/admin/deliveries' },
    { label: 'ประวัติการใช้งาน', icon: 'pi pi-fw pi-history', to: '/admin/audit' }
  ] }
]);

async function refreshIncidentBadge() {
  if (document.visibilityState !== 'visible') return;
  incidentRequest?.abort('superseded');
  incidentRequest = new AbortController();
  try {
    const page = await adminApi.incidents({ status: 'OPEN', severity: 'P1', pageSize: 100 }, incidentRequest.signal);
    openP1Count.value = page.data.length;
    openP1HasMore.value = page.page.hasMore;
  } catch { /* Keep the last safe count; the incident page exposes load errors. */ }
}

function scheduleIncidentPoll() {
  if (incidentPollTimer) window.clearInterval(incidentPollTimer);
  if (document.visibilityState !== 'visible') {
    incidentPollTimer = undefined;
    return;
  }
  incidentPollTimer = window.setInterval(() => void refreshIncidentBadge(), 60_000);
}

function handleVisibility() {
  scheduleIncidentPoll();
  if (document.visibilityState === 'visible') void refreshIncidentBadge();
}

onMounted(() => {
  void refreshIncidentBadge();
  scheduleIncidentPoll();
  document.addEventListener('visibilitychange', handleVisibility);
});
onBeforeUnmount(() => {
  incidentRequest?.abort('unmounted');
  if (incidentPollTimer) window.clearInterval(incidentPollTimer);
  document.removeEventListener('visibilitychange', handleVisibility);
});

async function signOut() {
  await logout();
  await router.replace('/admin/login');
}
</script>

<template>
  <AppShell :menu-model="model" home-to="/admin" :mobile-title="mobileContext.title" :mobile-subtitle="mobileContext.subtitle" :mobile-home-to="mobileContext.homeTo" :account-label="state.session?.username" confirm-dialogs @sign-out="signOut">
    <template #topbar-context><RouterLink v-if="openP1Badge" to="/admin/operational-incidents" class="no-underline" aria-label="เปิดดูเหตุสำคัญระดับ P1"><Tag severity="danger" :value="`${openP1Badge} เหตุ P1`" /></RouterLink></template>
    <RouterView />
  </AppShell>
</template>
