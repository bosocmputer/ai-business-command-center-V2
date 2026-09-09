<script setup lang="ts">
import { computed } from 'vue';
import { useLayout } from '@/layout/composables/layout';

const props = defineProps<{
  homeTo: string;
  mobileTitle?: string;
  mobileSubtitle?: string;
  mobileHomeTo?: string;
  accountLabel?: string;
}>();
const { toggleMenu, toggleDarkMode, isDarkTheme, layoutState } = useLayout();
const contextualHome = computed(() => props.mobileHomeTo || props.homeTo);
const contextualLabel = computed(() => [props.mobileTitle, props.mobileSubtitle].filter(Boolean).join(' · '));
defineEmits<{ signOut: [] }>();
</script>

<template>
  <header class="layout-topbar">
    <div class="layout-topbar-logo-container">
      <button class="layout-menu-button layout-topbar-action" type="button" aria-label="เปิดหรือปิดเมนู" aria-controls="app-sidebar" :aria-expanded="layoutState.mobileMenuActive" @click="toggleMenu">
        <i class="pi pi-bars" />
      </button>
      <RouterLink :to="homeTo" class="layout-topbar-logo desktop-brand" aria-label="Nextstep Dashboard">
        <span class="brand-mark"><i class="pi pi-chart-line" /></span>
        <span class="brand-name">NEXTSTEP</span>
      </RouterLink>
      <RouterLink :to="contextualHome" class="mobile-topbar-context" :aria-label="contextualLabel" data-testid="mobile-topbar-context">
        <strong :title="mobileTitle">{{ mobileTitle || 'Nextstep Dashboard' }}</strong>
        <span v-if="mobileSubtitle" :title="mobileSubtitle">{{ mobileSubtitle }}</span>
      </RouterLink>
    </div>
    <div class="layout-topbar-actions">
      <div class="desktop-topbar-context"><slot name="context" /></div>
      <button type="button" class="layout-topbar-action topbar-button" aria-label="สลับโหมดสี" @click="toggleDarkMode">
        <i :class="['pi', isDarkTheme ? 'pi-sun' : 'pi-moon']" />
      </button>
      <span v-if="accountLabel" class="desktop-account self-center text-sm text-muted-color">{{ accountLabel }}</span>
      <button type="button" class="layout-topbar-action topbar-button desktop-sign-out" aria-label="ออกจากระบบ" @click="$emit('signOut')">
        <i class="pi pi-sign-out" />
      </button>
    </div>
  </header>
</template>

<style scoped>
.brand-mark { display: grid; place-items: center; width: 2.25rem; height: 2.25rem; border-radius: 0.75rem; color: #fff; background: var(--primary-color); }
.topbar-button { border: 0; background: transparent; padding: 0; }
.mobile-topbar-context { display: none; min-width: 0; color: var(--text-color); text-decoration: none; }
.desktop-topbar-context { display: flex; align-items: center; }
.desktop-account { display: inline; }
@media (max-width: 991px) {
  .desktop-brand, .desktop-topbar-context, .desktop-account, .desktop-sign-out { display: none; }
  .mobile-topbar-context { display: grid; gap: .1rem; line-height: 1.15; }
  .mobile-topbar-context strong, .mobile-topbar-context span { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .mobile-topbar-context strong { font-size: 1rem; }
  .mobile-topbar-context span { color: var(--text-color-secondary); font-size: .7rem; font-weight: 500; }
}
</style>
