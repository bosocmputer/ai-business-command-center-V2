<script setup lang="ts">
import { computed, ref, useId, watch } from 'vue';
import { formatDateOnly } from '@/utils/format';
import {
  bangkokToday,
  validatePeriodSelection,
  type ReportPeriodMode,
  type ReportPeriodSelection,
  type SmartPeriodPreset
} from '@/utils/reportPeriod';

const props = withDefaults(defineProps<{
  mode: ReportPeriodMode | 'SMART_OVERVIEW';
  selection: ReportPeriodSelection;
  displayedLabel?: string;
  sourceLabel?: string;
  actionLabel?: string;
  forceActionLabel?: string;
  loading?: boolean;
  disabled?: boolean;
  desktopMode?: 'default' | 'compact';
}>(), {
  displayedLabel: 'ยังไม่มีข้อมูล',
  actionLabel: 'ดึงข้อมูลช่วงนี้',
  loading: false,
  disabled: false,
  desktopMode: 'default'
});

const emit = defineEmits<{ apply: [selection: ReportPeriodSelection]; force: [selection: ReportPeriodSelection] }>();
const draft = ref<ReportPeriodSelection>({ ...props.selection });
const dateFrom = ref<Date | null>(null);
const dateTo = ref<Date | null>(null);
const expanded = ref(false);
const fieldId = useId();
const presetId = `${fieldId}-preset`;
const fromId = `${fieldId}-from`;
const toId = `${fieldId}-to`;
const maxDate = computed(() => new Date(`${bangkokToday()}T00:00:00`));
const presetOptions = computed(() => props.mode === 'AS_OF_DATE'
  ? [
      { label: 'วันนี้', value: 'TODAY_TO_NOW' },
      { label: 'เมื่อวาน', value: 'YESTERDAY' },
      { label: 'เลือกวันที่', value: 'CUSTOM' }
    ]
  : [
      { label: 'วันนี้', value: 'TODAY_TO_NOW' },
      { label: 'เมื่อวาน', value: 'YESTERDAY' },
      { label: 'เดือนนี้', value: 'MONTH_TO_DATE' },
      { label: 'กำหนดช่วงเอง', value: 'CUSTOM' }
    ]);
const validation = computed(() => validatePeriodSelection(draft.value));
const applyDisabled = computed(() => props.disabled || props.loading || (props.mode !== 'CURRENT_ONLY' && !validation.value.valid));
const mobileToggleLabel = computed(() => props.mode === 'CURRENT_ONLY'
  ? (expanded.value ? 'ปิด' : 'รีเฟรช')
  : (expanded.value ? 'ปิดตัวเลือก' : 'เปลี่ยนช่วง'));

watch(() => props.selection, (selection) => resetDraft(selection), { immediate: true, deep: true });
watch(() => draft.value.periodPreset, (preset) => {
  if (preset !== 'CUSTOM') {
    dateFrom.value = null;
    dateTo.value = null;
    draft.value = { periodPreset: preset };
    return;
  }
  const fallback = new Date(`${bangkokToday()}T00:00:00`);
  dateFrom.value ??= fallback;
  dateTo.value ??= fallback;
  syncCustomDates();
});
watch(dateFrom, () => {
  if (props.mode === 'AS_OF_DATE' && dateFrom.value) dateTo.value = dateFrom.value;
  syncCustomDates();
});
watch(dateTo, syncCustomDates);

function resetDraft(selection: ReportPeriodSelection) {
  draft.value = { ...selection };
  dateFrom.value = selection.dateFrom ? new Date(`${selection.dateFrom}T00:00:00`) : null;
  dateTo.value = selection.dateTo ? new Date(`${selection.dateTo}T00:00:00`) : null;
}

function syncCustomDates() {
  if (draft.value.periodPreset !== 'CUSTOM') return;
  draft.value = {
    periodPreset: 'CUSTOM',
    dateFrom: dateFrom.value ? formatDateOnly(dateFrom.value) : undefined,
    dateTo: dateTo.value ? formatDateOnly(dateTo.value) : undefined
  };
}

function selectPreset(value: SmartPeriodPreset) {
  draft.value = { periodPreset: value };
}

function apply() {
  if (applyDisabled.value) return;
  emit('apply', { ...draft.value });
  expanded.value = false;
}

function force() {
  if (applyDisabled.value) return;
  emit('force', { ...draft.value });
  expanded.value = false;
}
</script>

<template>
  <section :class="['period-toolbar-shell', { 'period-toolbar-compact': desktopMode === 'compact' }]" aria-label="เลือกช่วงข้อมูลรายงาน">
    <Toolbar :class="['period-toolbar', { 'is-expanded': expanded, 'has-custom-period': draft.periodPreset === 'CUSTOM' && mode !== 'CURRENT_ONLY' }]">
      <template #start>
        <div class="period-context">
          <div class="period-context-details">
            <strong class="period-context-value">
              <span>{{ displayedLabel }}</span>
              <span v-if="sourceLabel" class="period-source-inline">· {{ sourceLabel }}</span>
            </strong>
          </div>
          <Button
            class="period-mobile-toggle"
            :label="mobileToggleLabel"
            :icon="expanded ? 'pi pi-angle-up' : 'pi pi-calendar'"
            severity="secondary"
            text
            size="small"
            :aria-expanded="expanded"
            @click="expanded = !expanded"
          />
        </div>
      </template>

      <template #center>
        <div class="period-controls">
          <template v-if="mode === 'CURRENT_ONLY'">
            <div class="current-only-copy">
              <i class="pi pi-bolt" aria-hidden="true" />
              <div><strong>สถานะปัจจุบัน</strong><span>ไม่รองรับข้อมูลย้อนหลัง</span></div>
            </div>
          </template>
          <template v-else>
            <div class="period-field period-preset-field">
              <label :for="presetId" class="sr-only">เลือกช่วงข้อมูล</label>
              <Select
                :input-id="presetId"
                :model-value="draft.periodPreset"
                :options="presetOptions"
                option-label="label"
                option-value="value"
                fluid
                @update:model-value="selectPreset"
              />
            </div>
            <div v-if="draft.periodPreset === 'CUSTOM'" class="period-field">
              <label :for="fromId">{{ mode === 'AS_OF_DATE' ? 'วันที่' : 'จากวันที่' }}</label>
              <DatePicker :input-id="fromId" v-model="dateFrom" date-format="dd/mm/yy" :max-date="maxDate" show-icon fluid />
            </div>
            <div v-if="draft.periodPreset === 'CUSTOM' && mode !== 'AS_OF_DATE'" class="period-field">
              <label :for="toId">ถึงวันที่</label>
              <DatePicker :input-id="toId" v-model="dateTo" date-format="dd/mm/yy" :max-date="maxDate" show-icon fluid />
            </div>
          </template>
        </div>
      </template>

      <template #end>
        <div class="period-actions">
          <Button :label="actionLabel" icon="pi pi-refresh" :loading="loading" :disabled="applyDisabled" @click="apply" />
          <Button v-if="forceActionLabel" :label="forceActionLabel" icon="pi pi-database" severity="secondary" outlined :disabled="applyDisabled" @click="force" />
        </div>
      </template>
    </Toolbar>

    <Message v-if="mode !== 'CURRENT_ONLY' && validation.message" severity="error" :closable="false" class="period-message">{{ validation.message }}</Message>
    <Message v-else-if="mode !== 'CURRENT_ONLY' && validation.warning" severity="warn" :closable="false" class="period-message">{{ validation.warning }}</Message>
  </section>
</template>

<style scoped>
.period-toolbar-shell { container-type: inline-size; display: grid; gap: .75rem; margin-bottom: 1rem; }
.period-toolbar { display: grid; grid-template-columns: minmax(18rem, 1fr) minmax(11rem, 14rem) auto; align-items: center; gap: 1rem; padding: .65rem 1rem; border-radius: var(--content-border-radius); }
.period-toolbar :deep(.p-toolbar-start), .period-toolbar :deep(.p-toolbar-center), .period-toolbar :deep(.p-toolbar-end) { align-self: center; min-width: 0; }
.period-toolbar :deep(.p-toolbar-start), .period-toolbar :deep(.p-toolbar-center) { width: 100%; }
.period-context { display: grid; grid-template-columns: minmax(0, 1fr) auto; align-items: center; gap: .75rem; min-width: 0; width: 100%; }
.period-context-details { min-width: 0; }
.period-context-value { display: flex; align-items: center; gap: .35rem; min-height: 2.75rem; min-width: 0; overflow-wrap: anywhere; color: var(--text-color); font-size: .9rem; line-height: 1.35; }
.period-source-inline { flex: 0 0 auto; color: var(--text-color-secondary); font-size: .8rem; font-weight: 400; white-space: nowrap; }
.period-controls { display: flex; align-items: end; gap: .75rem; min-width: 0; width: 100%; }
.period-field { display: grid; gap: .4rem; }
.period-field label { font-size: .8rem; font-weight: 600; }
.period-preset-field { width: 100%; }
.period-actions { display: flex; align-items: center; justify-content: flex-end; gap: .75rem; }
.period-actions .p-button { min-height: 2.75rem; width: max-content; max-width: 14rem; }
.current-only-copy { display: flex; align-items: center; gap: .75rem; min-height: 2.75rem; }
.current-only-copy > i { color: var(--primary-color); font-size: 1.1rem; }
.current-only-copy > div { display: grid; gap: .15rem; }
.current-only-copy span { color: var(--text-color-secondary); font-size: .75rem; }
.period-message { margin: 0; }
.period-mobile-toggle { display: none; margin-left: auto; }
.period-toolbar.has-custom-period { grid-template-columns: minmax(18rem, 1fr) auto; }
.period-toolbar.has-custom-period :deep(.p-toolbar-start) { grid-column: 1; grid-row: 1; }
.period-toolbar.has-custom-period :deep(.p-toolbar-end) { grid-column: 2; grid-row: 1; }
.period-toolbar.has-custom-period :deep(.p-toolbar-center) { grid-column: 1 / -1; grid-row: 2; }
.period-toolbar.has-custom-period .period-controls { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); }
@container (max-width: 899px) {
  .period-toolbar { grid-template-columns: minmax(0, 1fr) auto; gap: .75rem 1rem; }
  .period-toolbar :deep(.p-toolbar-start), .period-toolbar.has-custom-period :deep(.p-toolbar-start) { grid-column: 1 / -1; grid-row: 1; }
  .period-toolbar :deep(.p-toolbar-center), .period-toolbar.has-custom-period :deep(.p-toolbar-center) { grid-column: 1; grid-row: 2; }
  .period-toolbar :deep(.p-toolbar-end), .period-toolbar.has-custom-period :deep(.p-toolbar-end) { grid-column: 2; grid-row: 2; }
  .period-toolbar.has-custom-period .period-controls { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); }
}
@media (max-width: 767px) {
  .period-toolbar { display: grid; grid-template-columns: 1fr; gap: .75rem; padding: 1rem; }
  .period-toolbar :deep(.p-toolbar-start), .period-toolbar :deep(.p-toolbar-center), .period-toolbar :deep(.p-toolbar-end), .period-toolbar.has-custom-period :deep(.p-toolbar-start), .period-toolbar.has-custom-period :deep(.p-toolbar-center), .period-toolbar.has-custom-period :deep(.p-toolbar-end) { grid-column: 1; grid-row: auto; width: 100%; }
  .period-context { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: .65rem; width: 100%; }
  .period-context-value { display: grid; gap: .1rem; min-height: 0; }
  .period-source-inline { font-size: .75rem; }
  .period-mobile-toggle { display: inline-flex; }
  .period-toolbar:not(.is-expanded) :deep(.p-toolbar-center), .period-toolbar:not(.is-expanded) :deep(.p-toolbar-end) { display: none; }
  .period-controls, .period-toolbar.has-custom-period .period-controls, .period-actions { display: grid; grid-template-columns: 1fr; align-items: stretch; width: 100%; padding-top: .75rem; border-top: 1px solid var(--surface-border); }
  .period-field, .period-preset-field { width: 100%; }
  .period-actions .p-button { width: 100%; max-width: none; }
}
</style>
