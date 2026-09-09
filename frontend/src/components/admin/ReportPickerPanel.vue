<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import type { DataTableFilterEvent } from 'primevue/datatable';
import type { AdminReportDefinition, ReportKey } from '@/api';
import SakaiTableHeader from '@/components/table/SakaiTableHeader.vue';
import { useSakaiFilterMenu } from '@/composables/useSakaiFilterMenu';

const props = withDefaults(defineProps<{
  definitions: AdminReportDefinition[];
  modelValue: ReportKey[];
  maxSelected?: number;
  ordered?: boolean;
  disabled?: boolean;
  lockedKeys?: ReportKey[];
}>(), { maxSelected: 0, ordered: false, disabled: false, lockedKeys: () => [] });

const emit = defineEmits<{ 'update:modelValue': [value: ReportKey[]] }>();
const selectedOnly = ref(false);
const limitMessage = ref('');
const tableFilters = ref({
  global: { value: null as string | null, matchMode: 'contains' },
  categoryLabel: { value: null as string[] | null, matchMode: 'in' }
});
useSakaiFilterMenu(tableFilters);
const filteredResult = ref<AdminReportDefinition[] | null>(null);

const selectedSet = computed(() => new Set<ReportKey>(props.modelValue));
const lockedSet = computed(() => new Set<ReportKey>(props.lockedKeys));
const definitionsByKey = computed(() => new Map(props.definitions.map((item) => [item.reportKey, item])));
const categories = computed(() => {
  const labels = new Map<string, string>();
  props.definitions.forEach((item) => labels.set(item.category, item.categoryLabel));
  return Array.from(labels, ([, label]) => ({ value: label, label }));
});
const tableSource = computed(() => selectedOnly.value ? props.definitions.filter((item) => selectedSet.value.has(item.reportKey)) : props.definitions);
const visibleFiltered = computed(() => filteredResult.value ?? tableSource.value);
const globalSearch = computed({ get: () => tableFilters.value.global.value ?? '', set: (value: string) => { tableFilters.value.global.value = value || null; } });
const hasFilters = computed(() => Boolean(globalSearch.value || tableFilters.value.categoryLabel.value?.length || selectedOnly.value));
const selectedDefinitions = computed(() => props.modelValue
  .map((key) => definitionsByKey.value.get(key))
  .filter((item): item is AdminReportDefinition => !!item));

function setSelection(next: ReportKey[]) {
  limitMessage.value = '';
  const nextSet = new Set(next);
  const preserved = props.modelValue.filter((key) => lockedSet.value.has(key) && !nextSet.has(key));
  emit('update:modelValue', [...next, ...preserved]);
}

function toggle(item: AdminReportDefinition, checked: boolean) {
  if (props.disabled || lockedSet.value.has(item.reportKey)) return;
  if (checked) {
    if (item.status !== 'ACTIVE') return;
    if (props.maxSelected > 0 && props.modelValue.length >= props.maxSelected) {
      limitMessage.value = `เลือกได้สูงสุด ${props.maxSelected} รายงาน`;
      return;
    }
    if (!selectedSet.value.has(item.reportKey)) setSelection([...props.modelValue, item.reportKey]);
    return;
  }
  setSelection(props.modelValue.filter((key) => key !== item.reportKey));
}

function selectFiltered() {
  if (props.disabled) return;
  const next = [...props.modelValue];
  const nextSet = new Set(next);
  for (const item of visibleFiltered.value) {
    if (item.status !== 'ACTIVE' || nextSet.has(item.reportKey)) continue;
    if (props.maxSelected > 0 && next.length >= props.maxSelected) break;
    next.push(item.reportKey);
    nextSet.add(item.reportKey);
  }
  if (props.maxSelected > 0 && visibleFiltered.value.some((item) => item.status === 'ACTIVE' && !nextSet.has(item.reportKey))) {
    limitMessage.value = `เลือกได้สูงสุด ${props.maxSelected} รายงาน`;
  }
  emit('update:modelValue', next);
}

function captureFiltered(event: DataTableFilterEvent) {
  filteredResult.value = ((event as DataTableFilterEvent & { filteredValue?: AdminReportDefinition[] }).filteredValue ?? tableSource.value);
}
function clearFilters() {
  tableFilters.value = { global: { value: null, matchMode: 'contains' }, categoryLabel: { value: null, matchMode: 'in' } };
  selectedOnly.value = false;
  filteredResult.value = null;
}
watch([tableSource, () => props.definitions], () => { filteredResult.value = null; });

function move(index: number, direction: -1 | 1) {
  if (props.disabled) return;
  const target = index + direction;
  if (target < 0 || target >= props.modelValue.length) return;
  const next = [...props.modelValue];
  [next[index], next[target]] = [next[target]!, next[index]!];
  setSelection(next);
}
</script>

<template>
  <div class="flex flex-col gap-4">
    <Message v-if="limitMessage" severity="warn" :closable="false">{{ limitMessage }}</Message>
    <div class="grid grid-cols-1 gap-5" :class="ordered ? 'xl:grid-cols-[minmax(0,1fr)_22rem]' : ''">
      <DataTable v-model:filters="tableFilters" :value="tableSource" data-key="reportKey" :global-filter-fields="['label', 'categoryLabel']" filter-display="menu" row-hover show-gridlines paginator :rows="25" :rows-per-page-options="[25, 50, 100]" paginator-template="RowsPerPageDropdown FirstPageLink PrevPageLink PageLinks NextPageLink LastPageLink CurrentPageReport" current-page-report-template="หน้า {currentPage} จาก {totalPages} · ทั้งหมด {totalRecords} รายการ" striped-rows responsive-layout="scroll" @filter="captureFiltered">
        <template #header><SakaiTableHeader v-model:global-search="globalSearch" :has-filters="hasFilters" search-label="ค้นหารายงาน" search-placeholder="ค้นหารายงาน" @clear="clearFilters"><template #start><div class="flex items-center gap-2"><Checkbox v-model="selectedOnly" input-id="selected-only" binary :disabled="disabled" /><label for="selected-only">เฉพาะที่เลือก</label></div><Button label="เลือกผลที่กรอง" icon="pi pi-check-square" outlined class="touch-action" :disabled="disabled" @click="selectFiltered" /><Button label="ล้างที่เลือก" icon="pi pi-times" text severity="secondary" class="touch-action" :disabled="disabled || !modelValue.length" @click="setSelection([])" /></template></SakaiTableHeader></template>
        <Column header="เลือก" style="width: 5rem" header-class="table-select-column" body-class="table-select-column">
          <template #body="{ data }"><Checkbox :model-value="selectedSet.has(data.reportKey)" binary :disabled="disabled || lockedSet.has(data.reportKey) || (data.status === 'DEPRECATED' && !selectedSet.has(data.reportKey))" :aria-label="lockedSet.has(data.reportKey) ? `${data.label} ถูกใช้โดยตารางส่ง LINE ที่กำลังใช้งาน` : `เลือก ${data.label}`" @update:model-value="toggle(data, $event)" /></template>
        </Column>
        <Column field="label" header="รายงาน"><template #body="{ data }"><span class="font-medium">{{ data.label }}</span><Tag v-if="lockedSet.has(data.reportKey)" value="ใช้ในตาราง Active" severity="info" class="ml-2" /><Tag v-if="data.status === 'DEPRECATED'" value="เลิกใช้" severity="warn" class="ml-2" /></template></Column>
        <Column field="categoryLabel" header="หมวด" :show-filter-match-modes="false"><template #body="{ data }"><Tag :value="data.categoryLabel" severity="secondary" /></template><template #filter="{ filterModel }"><MultiSelect v-model="filterModel.value" :options="categories" option-label="label" option-value="value" placeholder="ทุกหมวด" /></template></Column>
        <template #empty><div class="py-8 text-center text-muted-color">ไม่พบรายงานที่ตรงกับเงื่อนไข <Button v-if="hasFilters" label="ล้างตัวกรอง" text size="small" @click="clearFilters" /></div></template>
      </DataTable>

      <div v-if="ordered" class="border border-surface rounded-md p-4 self-start">
        <div class="flex items-center justify-between mb-3"><h3 class="text-base font-semibold m-0">ลำดับใน LINE Card</h3><Badge :value="`${modelValue.length}/${maxSelected}`" /></div>
        <div v-if="selectedDefinitions.length" class="flex flex-col gap-2">
          <div v-for="(item, index) in selectedDefinitions" :key="item.reportKey" class="flex items-center gap-2 p-2 border-bottom-1 surface-border">
            <span class="font-medium flex-1">{{ index + 1 }}. {{ item.label }}</span>
            <Button icon="pi pi-angle-up" text rounded size="small" class="touch-action" :disabled="disabled || index === 0" :aria-label="`เลื่อน ${item.label} ขึ้น`" @click="move(index, -1)" />
            <Button icon="pi pi-angle-down" text rounded size="small" class="touch-action" :disabled="disabled || index === selectedDefinitions.length - 1" :aria-label="`เลื่อน ${item.label} ลง`" @click="move(index, 1)" />
          </div>
        </div>
        <p v-else class="text-muted-color m-0">เลือกรายงานจากตาราง</p>
      </div>
    </div>
    <div class="text-sm text-muted-color">เลือกแล้ว {{ modelValue.length }} จาก {{ definitions.length }} รายงาน</div>
  </div>
</template>
