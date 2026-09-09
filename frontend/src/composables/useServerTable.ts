import { onScopeDispose, ref, watch, type Ref } from 'vue';
import { ApiError } from '@/api';
import { errorMessage } from '@/utils/format';

export interface ServerTableQuery<F> {
  page: number;
  pageSize: 25 | 50 | 100;
  globalSearch?: string;
  filters: F;
}

export interface ServerTablePage<T> {
  data: T[];
  page: number;
  pageSize: number;
  total: number;
  totalPages: number;
}

interface ServerTableOptions<T, F> {
  initialFilters: F;
  query: (input: ServerTableQuery<F>, signal: AbortSignal) => Promise<ServerTablePage<T>>;
  immediate?: boolean;
  debounceMs?: number;
}

function cloneFilters<F>(value: F): F {
  return JSON.parse(JSON.stringify(value)) as F;
}

export function useServerTable<T, F>(options: ServerTableOptions<T, F>) {
  const rows = ref<T[]>([]) as Ref<T[]>;
  const page = ref(0);
  const pageSize = ref<25 | 50 | 100>(25);
  const total = ref(0);
  const totalPages = ref(0);
  const loading = ref(false);
  const error = ref('');
  const globalSearch = ref('');
  const appliedGlobalSearch = ref('');
  const draftFilters = ref(cloneFilters(options.initialFilters)) as Ref<F>;
  const appliedFilters = ref(cloneFilters(options.initialFilters)) as Ref<F>;
  let controller: AbortController | undefined;
  let generation = 0;
  let searchTimer: ReturnType<typeof setTimeout> | undefined;

  async function load(allowClamp = true): Promise<void> {
    generation += 1;
    const requestGeneration = generation;
    controller?.abort('table-query-replaced');
    controller = new AbortController();
    loading.value = true;
    error.value = '';
    try {
      const result = await options.query({
        page: page.value,
        pageSize: pageSize.value,
        globalSearch: appliedGlobalSearch.value || undefined,
        filters: cloneFilters(appliedFilters.value)
      }, controller.signal);
      if (requestGeneration !== generation) return;
      if (allowClamp && result.totalPages > 0 && page.value >= result.totalPages) {
        page.value = result.totalPages - 1;
        await load(false);
        return;
      }
      rows.value = result.data;
      page.value = result.page;
      total.value = result.total;
      totalPages.value = result.totalPages;
    } catch (cause) {
      if (requestGeneration !== generation || cause instanceof ApiError && cause.code === 'CANCELLED') return;
      error.value = errorMessage(cause);
    } finally {
      if (requestGeneration === generation) loading.value = false;
    }
  }

  async function applyFilters(): Promise<void> {
    appliedFilters.value = cloneFilters(draftFilters.value);
    page.value = 0;
    await load();
  }

  async function clearFilters(): Promise<void> {
    draftFilters.value = cloneFilters(options.initialFilters);
    appliedFilters.value = cloneFilters(options.initialFilters);
    globalSearch.value = '';
    appliedGlobalSearch.value = '';
    page.value = 0;
    await load();
  }

  async function changePage(event: { page: number; rows: number }): Promise<void> {
    page.value = event.rows === pageSize.value ? event.page : 0;
    pageSize.value = event.rows as 25 | 50 | 100;
    await load();
  }

  function restoreDraft(): void {
    draftFilters.value = cloneFilters(appliedFilters.value);
  }

  watch(globalSearch, (value) => {
    if (searchTimer) clearTimeout(searchTimer);
    const normalized = value.trim();
    if (normalized.length === 1) return;
    searchTimer = setTimeout(() => {
      if (normalized === appliedGlobalSearch.value) return;
      appliedGlobalSearch.value = normalized;
      page.value = 0;
      void load();
    }, options.debounceMs ?? 400);
  });

  onScopeDispose(() => {
    generation += 1;
    controller?.abort('table-unmount');
    if (searchTimer) clearTimeout(searchTimer);
  });

  if (options.immediate !== false) void load();

  return {
    rows, page, pageSize, total, totalPages, loading, error,
    globalSearch, appliedGlobalSearch, draftFilters, appliedFilters,
    applyFilters, clearFilters, changePage, restoreDraft, refresh: load
  };
}
