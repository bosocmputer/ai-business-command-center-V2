<script setup lang="ts">
import { useLayout } from '@/layout/composables/layout';
import { onBeforeUnmount, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import type { NavigationItem } from './menu';
import AppMenu from './AppMenu.vue';

defineProps<{ model: NavigationItem[]; accountLabel?: string }>();
const emit = defineEmits<{ signOut: [] }>();

const { layoutState, isDesktop, hasOpenOverlay, hideMobileMenu } = useLayout();
const route = useRoute();
const sidebarRef = ref<HTMLElement | null>(null);
let outsideClickListener: ((event: MouseEvent) => void) | null = null;

watch(
    () => route.path,
    (newPath) => {
        if (isDesktop()) layoutState.activePath = null;
        else layoutState.activePath = newPath;

        layoutState.overlayMenuActive = false;
        layoutState.mobileMenuActive = false;
        layoutState.menuHoverActive = false;
    },
    { immediate: true }
);

watch(hasOpenOverlay, (newVal) => {
    if (isDesktop()) {
        if (newVal) bindOutsideClickListener();
        else unbindOutsideClickListener();
    }
});

const bindOutsideClickListener = () => {
    if (!outsideClickListener) {
        outsideClickListener = (event: MouseEvent) => {
            if (isOutsideClicked(event)) {
                layoutState.overlayMenuActive = false;
            }
        };

        document.addEventListener('click', outsideClickListener);
    }
};

const unbindOutsideClickListener = () => {
    if (outsideClickListener) {
        document.removeEventListener('click', outsideClickListener);
        outsideClickListener = null;
    }
};

const isOutsideClicked = (event: MouseEvent) => {
    const topbarButtonEl = document.querySelector('.layout-menu-button');
    const sidebar = sidebarRef.value;

    return !(sidebar?.isSameNode(event.target as Node) || sidebar?.contains(event.target as Node) || topbarButtonEl?.isSameNode(event.target as Node) || topbarButtonEl?.contains(event.target as Node));
};

function signOut() {
    hideMobileMenu();
    emit('signOut');
}

onBeforeUnmount(() => {
    unbindOutsideClickListener();
});
</script>

<template>
    <aside id="app-sidebar" ref="sidebarRef" class="layout-sidebar" aria-label="เมนูหลัก">
        <div v-if="$slots.context" class="layout-sidebar-context"><slot name="context" /></div>
        <div class="layout-sidebar-menu"><AppMenu :model="model" /></div>
        <div class="layout-sidebar-footer">
            <span v-if="accountLabel" class="safe-wrap"><i class="pi pi-user mr-2" />{{ accountLabel }}</span>
            <Button label="ออกจากระบบ" icon="pi pi-sign-out" severity="secondary" text fluid class="touch-action" @click="signOut" />
        </div>
    </aside>
</template>

<style scoped lang="scss">
.layout-sidebar-context, .layout-sidebar-footer { display: none; }
@media (max-width: 991px) {
    .layout-sidebar-context { display: block; padding: .75rem 0 1rem; border-bottom: 1px solid var(--surface-border); }
    .layout-sidebar-footer { display: grid; gap: .5rem; padding: .75rem 0 max(.75rem, env(safe-area-inset-bottom)); border-top: 1px solid var(--surface-border); color: var(--text-color-secondary); font-size: .8rem; }
}
</style>
