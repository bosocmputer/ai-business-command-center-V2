<script setup lang="ts">
import { computed } from 'vue';

const props = withDefaults(defineProps<{
  page: number;
  pageSize: number;
  itemCount: number;
  hasNext: boolean;
  disabled?: boolean;
  rowsPerPageOptions?: number[];
}>(), { disabled: false, rowsPerPageOptions: () => [25, 50, 100] });

const emit = defineEmits<{
  previous: [];
  next: [];
  'update:pageSize': [value: number];
}>();

const first = computed(() => props.page * props.pageSize);
const syntheticTotal = computed(() => first.value + props.itemCount + (props.hasNext ? 1 : 0));

function onPage(event: { page: number; rows: number }) {
  if (event.rows !== props.pageSize) {
    emit('update:pageSize', event.rows);
    return;
  }
  if (event.page < props.page) emit('previous');
  else if (event.page > props.page) emit('next');
}
</script>

<template>
  <Paginator
    :first="first"
    :rows="pageSize"
    :total-records="syntheticTotal"
    :rows-per-page-options="rowsPerPageOptions"
    template="RowsPerPageDropdown PrevPageLink CurrentPageReport NextPageLink"
    :current-page-report-template="`หน้า ${page + 1} · ${itemCount.toLocaleString('th-TH')} รายการ`"
    :pt="{ root: { 'aria-label': 'เปลี่ยนหน้าตาราง' } }"
    :disabled="disabled"
    @page="onPage"
  />
</template>
