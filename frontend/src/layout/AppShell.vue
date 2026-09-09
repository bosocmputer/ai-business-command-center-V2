<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, watch } from 'vue';
import { useLayout } from '@/layout/composables/layout';
import type { NavigationItem } from './menu';
import AppFooter from './AppFooter.vue';
import AppSidebar from './AppSidebar.vue';
import AppTopbar from './AppTopbar.vue';

defineProps<{
  menuModel: NavigationItem[];
  homeTo: string;
  mobileTitle?: string;
  mobileSubtitle?: string;
  mobileHomeTo?: string;
  accountLabel?: string;
  confirmDialogs?: boolean;
}>();

defineEmits<{ signOut: [] }>();

const { layoutConfig, layoutState, hideMobileMenu } = useLayout();
const containerClass = computed(() => ({
  'layout-overlay': layoutConfig.menuMode === 'overlay',
  'layout-static': layoutConfig.menuMode === 'static',
  'layout-overlay-active': layoutState.overlayMenuActive,
  'layout-mobile-active': layoutState.mobileMenuActive,
  'layout-static-inactive': layoutState.staticMenuInactive
}));

const focusableSelector = 'a[href], button:not([disabled]), select:not([disabled]), input:not([disabled]), [tabindex]:not([tabindex="-1"])';
let restoreFocus: HTMLElement | null = null;
let desktopQuery: MediaQueryList | undefined;
let unmounting = false;

function sidebarFocusableElements(): HTMLElement[] {
  const sidebar = document.getElementById('app-sidebar');
  return sidebar ? Array.from(sidebar.querySelectorAll<HTMLElement>(focusableSelector)) : [];
}

function handleDrawerKeydown(event: KeyboardEvent) {
  if (!layoutState.mobileMenuActive) return;
  if (event.key === 'Escape') {
    event.preventDefault();
    hideMobileMenu();
    return;
  }
  if (event.key !== 'Tab') return;
  const focusable = sidebarFocusableElements();
  if (!focusable.length) return;
  const first = focusable[0]!;
  const last = focusable.at(-1)!;
  const sidebar = document.getElementById('app-sidebar');
  if (event.shiftKey && (document.activeElement === first || !sidebar?.contains(document.activeElement))) {
    event.preventDefault(); last.focus();
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault(); first.focus();
  }
}

function cleanupDrawer() {
  document.body.classList.remove('blocked-scroll');
  document.removeEventListener('keydown', handleDrawerKeydown);
}

watch(() => layoutState.mobileMenuActive, async (open) => {
  if (open) {
    restoreFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    document.body.classList.add('blocked-scroll');
    document.addEventListener('keydown', handleDrawerKeydown);
    await nextTick();
    sidebarFocusableElements()[0]?.focus();
  } else {
    cleanupDrawer();
    if (!unmounting) {
      await nextTick();
      restoreFocus?.focus();
    }
    restoreFocus = null;
  }
});

function handleBreakpointChange(event: MediaQueryListEvent) {
  if (event.matches) hideMobileMenu();
}

onMounted(() => {
  desktopQuery = window.matchMedia('(min-width: 992px)');
  desktopQuery.addEventListener('change', handleBreakpointChange);
});
onBeforeUnmount(() => {
  unmounting = true;
  hideMobileMenu();
  cleanupDrawer();
  desktopQuery?.removeEventListener('change', handleBreakpointChange);
});
</script>

<template>
  <div class="layout-wrapper" :class="containerClass">
    <AppTopbar :home-to="homeTo" :mobile-home-to="mobileHomeTo" :mobile-title="mobileTitle" :mobile-subtitle="mobileSubtitle" :account-label="accountLabel" @sign-out="$emit('signOut')">
      <template #context><slot name="topbar-context" /></template>
    </AppTopbar>
    <AppSidebar :model="menuModel" :account-label="accountLabel" @sign-out="$emit('signOut')">
      <template #context><slot name="sidebar-context" /></template>
    </AppSidebar>
    <div class="layout-main-container">
      <main class="layout-main"><slot /></main>
      <AppFooter />
    </div>
    <div class="layout-mask animate-fadein" @click="hideMobileMenu" />
  </div>
  <Toast />
  <ConfirmDialog v-if="confirmDialogs" />
</template>
