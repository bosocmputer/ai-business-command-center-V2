export interface CursorPaginationState {
  page: number;
  pageSize: number;
  cursor?: string;
  nextCursor?: string;
  hasNext: boolean;
  cursors: Array<string | undefined>;
}

export function createCursorPagination(pageSize = 25): CursorPaginationState {
  return { page: 0, pageSize, cursor: undefined, nextCursor: undefined, hasNext: false, cursors: [undefined] };
}

export function resetCursorPagination(state: CursorPaginationState) {
  state.page = 0;
  state.cursor = undefined;
  state.nextCursor = undefined;
  state.hasNext = false;
  state.cursors = [undefined];
}

export function resizeCursorPagination(state: CursorPaginationState, pageSize: number) {
  state.pageSize = pageSize;
  resetCursorPagination(state);
}

export function acceptCursorPage(state: CursorPaginationState, nextCursor: string | undefined, hasNext: boolean) {
  state.nextCursor = nextCursor;
  state.hasNext = hasNext && Boolean(nextCursor);
  if (state.hasNext) state.cursors[state.page + 1] = nextCursor;
  state.cursors.length = state.hasNext ? state.page + 2 : state.page + 1;
}

export function moveCursorPage(state: CursorPaginationState, direction: 'previous' | 'next') {
  const nextPage = direction === 'next' ? state.page + 1 : state.page - 1;
  if (nextPage < 0 || (direction === 'next' && !state.hasNext)) return false;
  const cursor = state.cursors[nextPage];
  if (nextPage > 0 && !cursor) return false;
  state.page = nextPage;
  state.cursor = cursor;
  state.nextCursor = undefined;
  state.hasNext = false;
  return true;
}
