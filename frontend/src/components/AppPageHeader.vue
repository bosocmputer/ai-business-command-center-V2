<script setup lang="ts">
import { computed, useSlots } from 'vue';

const props = withDefaults(defineProps<{
  title: string;
  subtitle?: string;
  mobileMode?: 'context' | 'entity';
  desktopMode?: 'default' | 'viewerCompact';
}>(), { mobileMode: 'context', desktopMode: 'default' });

const slots = useSlots();
const classes = computed(() => [
  `page-header-${props.mobileMode}`,
  props.desktopMode === 'viewerCompact' ? 'page-header-viewer-compact' : undefined,
  !slots.actions && props.mobileMode === 'context' ? 'page-header-mobile-empty' : undefined
]);
</script>

<template>
  <div class="page-header" :class="classes">
    <div class="page-header-copy">
      <slot name="back" />
      <h1 class="page-title">{{ title }}</h1>
      <p v-if="subtitle" class="page-subtitle">{{ subtitle }}</p>
    </div>
    <div v-if="$slots.actions" class="page-header-actions"><slot name="actions" /></div>
  </div>
</template>

<style scoped>
.page-header-actions { display: flex; align-items: center; justify-content: flex-end; gap: .75rem; min-width: 0; }
@media (min-width: 768px) {
  .page-header-viewer-compact { align-items: center; min-height: 3.5rem; margin-bottom: .75rem; }
  .page-header-viewer-compact .page-header-copy { display: flex; align-items: center; gap: .75rem; min-width: 0; }
  .page-header-viewer-compact .page-title { min-width: 0; font-size: 1.5rem; line-height: 1.25; text-wrap: balance; }
  .page-header-viewer-compact .page-header-actions { flex: 0 0 auto; }
}
@media (max-width: 767px) {
  .page-header { gap: .75rem; margin-bottom: .75rem; }
  .page-header-context .page-header-copy { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip: rect(0, 0, 0, 0); white-space: nowrap; border: 0; }
  .page-header-mobile-empty { margin-bottom: 0; }
  .page-header-actions { width: 100%; justify-content: space-between; flex-wrap: wrap; }
  .page-header-entity .page-title { font-size: 1.25rem; }
  .page-header-entity .page-subtitle { font-size: .8rem; }
}
</style>
