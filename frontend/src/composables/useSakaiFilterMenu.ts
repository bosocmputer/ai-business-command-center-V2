import { onMounted, onScopeDispose, type Ref } from 'vue';

export function cloneSakaiFilterState<T>(value: T): T {
  if (value instanceof Date) return new Date(value.getTime()) as T;
  if (Array.isArray(value)) return value.map((item) => cloneSakaiFilterState(item)) as T;
  if (value && typeof value === 'object') {
    return Object.fromEntries(Object.entries(value).map(([key, item]) => [key, cloneSakaiFilterState(item)])) as T;
  }
  return value;
}

function removedFilterOverlay(records: MutationRecord[]): boolean {
  return records.some((record) => [...record.removedNodes].some((node) =>
    node instanceof Element && (node.matches('.p-datatable-filter-overlay') || Boolean(node.querySelector('.p-datatable-filter-overlay')))
  ));
}

/**
 * PrimeVue keeps an unapplied menu draft in DataTable's private filter state.
 * Replacing the controlled value after the overlay closes restores that draft
 * to the last applied value without reaching into PrimeVue internals.
 */
export function useSakaiFilterMenu<T>(filters: Ref<T>): void {
  let observer: MutationObserver | undefined;
  onMounted(() => {
    if (typeof MutationObserver === 'undefined') return;
    observer = new MutationObserver((records) => {
      if (removedFilterOverlay(records)) filters.value = cloneSakaiFilterState(filters.value);
    });
    observer.observe(document.body, { childList: true, subtree: true });
  });
  onScopeDispose(() => observer?.disconnect());
}
