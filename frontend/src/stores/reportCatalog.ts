import { adminApi, type AdminReportCatalog } from '@/api';

let cached: AdminReportCatalog | undefined;
let pending: Promise<AdminReportCatalog> | undefined;

export async function loadAdminReportCatalog(signal?: AbortSignal): Promise<AdminReportCatalog> {
  if (cached) return cached;
  if (!pending) {
    pending = adminApi.reports(signal).then((catalog) => {
      cached = catalog;
      return catalog;
    }).finally(() => { pending = undefined; });
  }
  return pending;
}

export function clearAdminReportCatalog(): void {
  cached = undefined;
  pending = undefined;
}
