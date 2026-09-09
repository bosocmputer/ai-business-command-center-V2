<script setup lang="ts">
defineProps<{
  globalSearch: string;
  loading?: boolean;
  hasFilters?: boolean;
  searchPlaceholder?: string;
  searchLabel?: string;
  mobileFilters?: boolean;
}>();

const emit = defineEmits<{
  'update:globalSearch': [value: string];
  clear: [];
  openMobileFilters: [];
}>();
</script>

<template>
  <div class="sakai-table-header">
    <div class="flex flex-wrap items-center gap-2">
      <Button type="button" icon="pi pi-filter-slash" label="ล้างตัวกรอง" outlined :disabled="loading || !hasFilters" @click="emit('clear')" />
      <Button v-if="mobileFilters" type="button" icon="pi pi-filter" label="ตัวกรอง" outlined class="md:hidden" :disabled="loading" @click="emit('openMobileFilters')" />
      <slot name="start" />
    </div>
    <div class="grid gap-1">
      <IconField>
        <InputIcon><i class="pi pi-search" /></InputIcon>
        <InputText :model-value="globalSearch" :placeholder="searchPlaceholder ?? 'ค้นหาอย่างน้อย 2 ตัวอักษร'" :aria-label="searchLabel ?? 'ค้นหาข้อมูลในตาราง'" @update:model-value="emit('update:globalSearch', String($event ?? ''))" />
      </IconField>
      <small v-if="globalSearch.trim().length === 1" class="text-muted-color">กรอกอย่างน้อย 2 ตัวอักษร</small>
    </div>
  </div>
</template>
