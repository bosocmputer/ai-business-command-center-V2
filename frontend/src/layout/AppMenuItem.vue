<script setup>
import { useLayout } from '@/layout/composables/layout';
import { computed } from 'vue';
import { useRoute } from 'vue-router';

const { layoutState, isDesktop } = useLayout();
const route = useRoute();

const props = defineProps({
    item: {
        type: Object,
        default: () => ({})
    },
    root: {
        type: Boolean,
        default: true
    },
    parentPath: {
        type: String,
        default: null
    }
});

const fullPath = computed(() => (props.item.path ? (props.parentPath ? props.parentPath + props.item.path : props.item.path) : null));

const isActive = computed(() => {
    return props.item.path ? layoutState.activePath?.startsWith(fullPath.value) : layoutState.activePath === props.item.to;
});

const isRouteActive = computed(() => {
    if (!props.item.to) return false;
    if (!props.item.activePrefix) return route.path === props.item.to;
    return route.path === props.item.activePrefix || route.path.startsWith(`${props.item.activePrefix}/`);
});

const itemClick = (event, item) => {
    if (item.disabled) {
        event.preventDefault();
        return;
    }

    if (item.command) {
        item.command({ originalEvent: event, item: item });
    }

    if (item.items) {
        if (isActive.value) {
            layoutState.activePath = layoutState.activePath.replace(item.path, '');
        } else {
            layoutState.activePath = fullPath.value;
            layoutState.menuHoverActive = true;
        }
    } else {
        layoutState.overlayMenuActive = false;
        layoutState.mobileMenuActive = false;
        layoutState.menuHoverActive = false;
    }
};

const onMouseEnter = () => {
    if (isDesktop() && props.root && props.item.items && layoutState.menuHoverActive) {
        layoutState.activePath = fullPath.value;
    }
};
</script>

<template>
    <li :class="{ 'layout-root-menuitem': root, 'active-menuitem': isActive }">
        <div v-if="root && item.visible !== false" class="layout-menuitem-root-text">{{ item.label }}</div>
        <a v-if="(!item.to || item.items) && item.visible !== false" :href="item.url" @click="itemClick($event, item)" :class="item.class" :target="item.target" tabindex="0" @mouseenter="onMouseEnter">
            <i :class="item.icon" class="layout-menuitem-icon" aria-hidden="true" />
            <span class="layout-menuitem-text">{{ item.label }}</span>
            <Badge v-if="item.badge" :value="item.badge" :severity="item.badgeSeverity || 'danger'" class="ml-auto" />
            <i class="pi pi-fw pi-angle-down layout-submenu-toggler" v-if="item.items" aria-hidden="true" />
        </a>
        <router-link v-if="item.to && !item.items && item.visible !== false" @click="itemClick($event, item)" :class="[item.class, { 'active-route': isRouteActive }]" tabindex="0" :to="item.to" @mouseenter="onMouseEnter">
            <i :class="item.icon" class="layout-menuitem-icon" aria-hidden="true" />
            <span class="layout-menuitem-text">{{ item.label }}</span>
            <Badge v-if="item.badge" :value="item.badge" :severity="item.badgeSeverity || 'danger'" class="ml-auto" />
            <i class="pi pi-fw pi-angle-down layout-submenu-toggler" v-if="item.items" aria-hidden="true" />
        </router-link>
        <Transition v-if="item.items && item.visible !== false" name="layout-submenu">
            <ul v-show="root ? true : isActive" class="layout-submenu">
                <app-menu-item v-for="child in item.items" :key="child.label + '_' + (child.to || child.path)" :item="child" :root="false" :parentPath="fullPath" />
            </ul>
        </Transition>
    </li>
</template>
