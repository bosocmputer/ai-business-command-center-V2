import type { RecipientQueryInput } from '@/api';
import type { ServerTableQuery } from '@/composables/useServerTable';

export type RecipientTableFilters = {
  statuses: Array<'PENDING' | 'ACTIVE'>;
  permissionStates: Array<'WITH_REPORTS' | 'WITHOUT_REPORTS'>;
};

export function toRecipientQueryInput(input: ServerTableQuery<RecipientTableFilters>): RecipientQueryInput {
  return {
    page: input.page,
    pageSize: input.pageSize,
    globalSearch: input.globalSearch,
    statuses: input.filters.statuses,
    permissionStates: input.filters.permissionStates
  };
}

export function normalizeScheduleTableRow<T extends {
  readinessBlockers: string[] | null;
  nextOccurrences: string[] | null;
}>(value: T): Omit<T, 'readinessBlockers' | 'nextOccurrences'> & {
  readinessBlockers: string[];
  nextOccurrences: string[];
} {
  return {
    ...value,
    readinessBlockers: Array.isArray(value.readinessBlockers) ? value.readinessBlockers : [],
    nextOccurrences: Array.isArray(value.nextOccurrences) ? value.nextOccurrences : []
  };
}

export function toDateFilter(value?: Date | null): string | undefined {
  if (!value || Number.isNaN(value.getTime())) return undefined;
  const year = value.getFullYear();
  const month = String(value.getMonth() + 1).padStart(2, '0');
  const day = String(value.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

export function normalizedAuditAction(value: string): string | undefined {
  const normalized = value.trim().toUpperCase().replace(/[\s-]+/g, '_');
  return normalized || undefined;
}
