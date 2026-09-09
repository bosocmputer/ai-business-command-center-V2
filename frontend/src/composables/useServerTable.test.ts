import { effectScope, nextTick } from 'vue';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { useServerTable } from './useServerTable';

afterEach(() => vi.useRealTimers());

describe('useServerTable', () => {
  it('does not request while a column filter is only a draft', async () => {
    const query = vi.fn().mockResolvedValue({ data: [], page: 0, pageSize: 25, total: 0, totalPages: 0 });
    const scope = effectScope();
    const table = scope.run(() => useServerTable({ initialFilters: { statuses: [] as string[] }, query, immediate: false }))!;

    table.draftFilters.value.statuses = ['FAILED'];
    await nextTick();
    expect(query).not.toHaveBeenCalled();

    await table.applyFilters();
    expect(query).toHaveBeenCalledTimes(1);
    expect(query.mock.calls[0]![0]).toMatchObject({ page: 0, pageSize: 25, filters: { statuses: ['FAILED'] } });
    scope.stop();
  });

  it('debounces global search and sends only the final value', async () => {
    vi.useFakeTimers();
    const query = vi.fn().mockResolvedValue({ data: [], page: 0, pageSize: 25, total: 0, totalPages: 0 });
    const scope = effectScope();
    const table = scope.run(() => useServerTable({ initialFilters: {}, query, immediate: false }))!;

    table.globalSearch.value = 'st';
    table.globalSearch.value = 'sto';
    table.globalSearch.value = 'stock';
    await vi.advanceTimersByTimeAsync(399);
    expect(query).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(1);
    expect(query).toHaveBeenCalledTimes(1);
    expect(query.mock.calls[0]![0].globalSearch).toBe('stock');
    scope.stop();
  });

  it('keeps visible rows and ignores stale responses', async () => {
    const resolvers: Array<(value: { data: Array<{ id: string }>; page: number; pageSize: 25; total: number; totalPages: number }) => void> = [];
    const query = vi.fn().mockImplementation(() => new Promise((resolve) => resolvers.push(resolve)));
    const scope = effectScope();
    const table = scope.run(() => useServerTable<{ id: string }, Record<string, never>>({ initialFilters: {}, query, immediate: false }))!;

    const first = table.refresh();
    const second = table.refresh();
    resolvers[1]!({ data: [{ id: 'new' }], page: 0, pageSize: 25, total: 1, totalPages: 1 });
    await second;
    resolvers[0]!({ data: [{ id: 'old' }], page: 0, pageSize: 25, total: 1, totalPages: 1 });
    await first;
    expect(table.rows.value).toEqual([{ id: 'new' }]);
    scope.stop();
  });

  it('keeps the visible page when a replacement request fails', async () => {
    const query = vi.fn()
      .mockResolvedValueOnce({ data: [{ id: 'visible' }], page: 0, pageSize: 25, total: 1, totalPages: 1 })
      .mockRejectedValueOnce(new Error('network unavailable'));
    const scope = effectScope();
    const table = scope.run(() => useServerTable<{ id: string }, Record<string, never>>({ initialFilters: {}, query, immediate: false }))!;
    await table.refresh();
    await table.refresh();
    expect(table.rows.value).toEqual([{ id: 'visible' }]);
    expect(table.error.value).toContain('network unavailable');
    scope.stop();
  });

  it('restores draft filters without changing the applied page', () => {
    const query = vi.fn().mockResolvedValue({ data: [], page: 0, pageSize: 25, total: 0, totalPages: 0 });
    const scope = effectScope();
    const table = scope.run(() => useServerTable({ initialFilters: { statuses: [] as string[] }, query, immediate: false }))!;
    table.appliedFilters.value = { statuses: ['FAILED'] };
    table.draftFilters.value = { statuses: ['SUCCEEDED'] };
    table.restoreDraft();
    expect(table.draftFilters.value).toEqual({ statuses: ['FAILED'] });
    expect(query).not.toHaveBeenCalled();
    scope.stop();
  });

  it('clamps a deleted last page once and never loops', async () => {
    const query = vi.fn()
      .mockResolvedValueOnce({ data: [], page: 4, pageSize: 25, total: 26, totalPages: 2 })
      .mockResolvedValueOnce({ data: [{ id: 'last' }], page: 1, pageSize: 25, total: 26, totalPages: 2 });
    const scope = effectScope();
    const table = scope.run(() => useServerTable<{ id: string }, Record<string, never>>({ initialFilters: {}, query, immediate: false }))!;
    table.page.value = 4;
    await table.refresh();
    expect(query).toHaveBeenCalledTimes(2);
    expect(table.page.value).toBe(1);
    expect(table.rows.value).toEqual([{ id: 'last' }]);
    scope.stop();
  });

  it('resets to the first page when the page size changes', async () => {
    const query = vi.fn().mockResolvedValue({ data: [], page: 0, pageSize: 50, total: 0, totalPages: 0 });
    const scope = effectScope();
    const table = scope.run(() => useServerTable({ initialFilters: {}, query, immediate: false }))!;
    table.page.value = 3;
    await table.changePage({ page: 1, rows: 50 });
    expect(query.mock.calls[0]![0]).toMatchObject({ page: 0, pageSize: 50 });
    scope.stop();
  });
});
