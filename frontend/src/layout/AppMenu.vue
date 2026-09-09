<script setup lang="ts">
import type { NavigationItem } from './menu';
import AppMenuItem from './AppMenuItem.vue';

defineProps<{ model: NavigationItem[] }>();
</script>

<template>
  <ul class="layout-menu">
    <template v-for="(item, index) in model" :key="item.label">
      <AppMenuItem :item="item" :index="index" />
    </template>
  </ul>
</template>

<style scoped lang="scss"></style>
