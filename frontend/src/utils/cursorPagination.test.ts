import { describe, expect, it } from 'vitest';
import {
  acceptCursorPage,
  createCursorPagination,
  moveCursorPage,
  resetCursorPagination,
  resizeCursorPagination
} from '@/utils/cursorPagination';

describe('cursor pagination', () => {
  it('moves forward and returns to a previously loaded cursor without accumulating rows', () => {
    const state = createCursorPagination(25);

    acceptCursorPage(state, 'page-2', true);
    expect(moveCursorPage(state, 'next')).toBe(true);
    expect(state.page).toBe(1);
    expect(state.cursor).toBe('page-2');

    acceptCursorPage(state, 'page-3', true);
    expect(moveCursorPage(state, 'previous')).toBe(true);
    expect(state.page).toBe(0);
    expect(state.cursor).toBeUndefined();
  });

  it('resets cursor history whenever filters or page size change', () => {
    const state = createCursorPagination(25);
    acceptCursorPage(state, 'page-2', true);
    moveCursorPage(state, 'next');

    resetCursorPagination(state);
    expect(state.page).toBe(0);
    expect(state.cursor).toBeUndefined();
    expect(state.hasNext).toBe(false);

    acceptCursorPage(state, 'page-2', true);
    moveCursorPage(state, 'next');
    resizeCursorPagination(state, 50);
    expect(state.pageSize).toBe(50);
    expect(state.page).toBe(0);
    expect(state.cursor).toBeUndefined();
  });

  it('does not move past either known boundary', () => {
    const state = createCursorPagination(25);
    expect(moveCursorPage(state, 'previous')).toBe(false);
    expect(moveCursorPage(state, 'next')).toBe(false);

    acceptCursorPage(state, undefined, false);
    expect(moveCursorPage(state, 'next')).toBe(false);
  });
});
