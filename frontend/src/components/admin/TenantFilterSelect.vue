<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { ApiError, adminApi, type Tenant } from '@/api';

const props = defineProps<{ modelValue?: string; ariaLabel?: string }>();
const emit = defineEmits<{ 'update:modelValue': [value: string | undefined] }>();
const selected = ref<Tenant>();
const suggestions = ref<Tenant[]>([]);
const loading = ref(false);
let controller: AbortController | undefined;

async function search(event: { query: string }) {
  controller?.abort('search-changed');
  controller = new AbortController();
  loading.value = true;
  try {
    const page = await adminApi.listTenants({ pageSize: 25, search: event.query.trim() || undefined }, controller.signal);
    suggestions.value = page.data;
    if (props.modelValue && !selected.value) selected.value = page.data.find((tenant) => tenant.id === props.modelValue);
  } catch (cause) {
    if (!(cause instanceof ApiError && cause.code === 'CANCELLED')) suggestions.value = [];
  } finally { loading.value = false; }
}

function update(value: Tenant | string | undefined) {
  if (!value || typeof value === 'string') { selected.value = undefined; emit('update:modelValue', undefined); return; }
  selected.value = value; emit('update:modelValue', value.id);
}

watch(() => props.modelValue, (value) => { if (!value) selected.value = undefined; });
onMounted(() => void search({ query: '' }));
onBeforeUnmount(() => controller?.abort('unmounted'));
</script>

<template>
  <AutoComplete
    :model-value="selected"
    :suggestions="suggestions"
    option-label="name"
    dropdown
    force-selection
    :loading="loading"
    :aria-label="ariaLabel ?? 'กรองตามร้านค้า'"
    placeholder="ทุกร้านค้า"
    @complete="search"
    @update:model-value="update"
  >
    <template #option="{ option }"><div><div class="font-medium">{{ option.name }}</div><small class="text-muted-color">{{ option.slug }}</small></div></template>
  </AutoComplete>
</template>
