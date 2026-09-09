<script setup lang="ts">
import { computed } from 'vue';
import type { ChartPresentationModel } from '@/utils/chartPresentation';

const props = defineProps<{
  model: ChartPresentationModel;
  compact?: boolean;
}>();

const itemLimit = computed(() => props.compact ? 5 : 10);
const rankingItems = computed(() => props.model.rankingItems.slice(0, itemLimit.value));
const compositionRows = computed(() => props.model.compositionRows.slice(0, itemLimit.value));
const fillClass = computed(() => `chart-fill-${props.model.tone}`);
</script>

<template>
  <ol v-if="model.rankingItems.length" class="mobile-ranking" data-testid="mobile-ranking">
    <li v-for="item in rankingItems" :key="`${item.index}-${item.label}`" class="ranking-item" data-testid="ranking-item">
      <div class="ranking-heading">
        <span class="ranking-order" aria-hidden="true">{{ item.index + 1 }}</span>
        <span class="ranking-label">{{ item.label }}</span>
        <strong class="ranking-value metric-value">{{ item.formattedValue }}</strong>
      </div>
      <div class="chart-track" aria-hidden="true"><span class="chart-fill" :class="fillClass" :style="{ width: `${item.widthPercent}%` }" /></div>
    </li>
  </ol>

  <ul v-else class="mobile-composition" data-testid="mobile-composition">
    <li v-for="row in compositionRows" :key="`${row.index}-${row.label}`" class="composition-item">
      <div class="composition-heading">
        <span class="ranking-label">{{ row.label }}</span>
        <span v-if="row.percent !== undefined" class="composition-percent metric-value">{{ row.percent.toLocaleString('th-TH', { maximumFractionDigits: 1 }) }}%</span>
      </div>
      <div v-for="value in row.values" :key="value.key" class="composition-series">
        <div class="composition-value"><span>{{ model.series.length > 1 ? value.label : 'มูลค่า' }}</span><strong class="metric-value">{{ value.formattedValue }}</strong></div>
        <div class="chart-track" aria-hidden="true"><span class="chart-fill" :class="fillClass" :style="{ width: `${row.percent ?? value.widthPercent}%` }" /></div>
      </div>
    </li>
  </ul>
</template>

<style scoped>
.mobile-ranking, .mobile-composition { display: grid; gap: 1rem; margin: .75rem 0 0; padding: 0; list-style: none; }
.ranking-item, .composition-item { display: grid; gap: .5rem; min-width: 0; }
.ranking-heading { display: grid; grid-template-columns: 1.75rem minmax(0, 1fr) auto; align-items: start; gap: .6rem; }
.ranking-order { display: grid; place-items: center; width: 1.75rem; height: 1.75rem; border-radius: 999px; background: var(--p-surface-100); color: var(--text-color-secondary); font-size: .75rem; font-weight: 700; }
.ranking-label { min-width: 0; color: var(--text-color); font-size: .875rem; font-weight: 600; line-height: 1.45; overflow-wrap: anywhere; }
.ranking-value { padding-top: .15rem; font-size: .875rem; white-space: nowrap; }
.chart-track { height: .5rem; overflow: hidden; border-radius: 999px; background: var(--p-surface-100); }
.chart-fill { display: block; height: 100%; border-radius: inherit; }
.chart-fill-primary { background: var(--primary-color); }
.chart-fill-warning { background: var(--p-orange-500); }
.chart-fill-danger { background: var(--p-red-500); }
.composition-heading, .composition-value { display: flex; align-items: baseline; justify-content: space-between; gap: 1rem; }
.composition-percent { color: var(--text-color-secondary); font-size: .8rem; white-space: nowrap; }
.composition-series { display: grid; gap: .35rem; }
.composition-value { color: var(--text-color-secondary); font-size: .75rem; }
.composition-value strong { color: var(--text-color); font-size: .825rem; white-space: nowrap; }
</style>
