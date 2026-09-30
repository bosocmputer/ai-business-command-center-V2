import type { MonitorContainerStats, MonitorHistoryPoint } from '@/api';

export type UsageSeverity = 'success' | 'warn' | 'danger';

const units = ['B', 'KB', 'MB', 'GB', 'TB'];

export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B';
  const exponent = Math.min(units.length - 1, Math.floor(Math.log(bytes) / Math.log(1024)));
  const value = bytes / 1024 ** exponent;
  return `${value >= 100 || exponent === 0 ? Math.round(value) : value.toFixed(1)} ${units[exponent]}`;
}

export function usagePercent(used: number, total: number): number {
  if (!(total > 0) || !(used >= 0)) return 0;
  return Math.min(100, Math.round(used / total * 1000) / 10);
}

export function usageSeverity(percent: number): UsageSeverity {
  if (percent >= 90) return 'danger';
  if (percent >= 75) return 'warn';
  return 'success';
}

export function formatUptime(seconds: number): string {
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor(seconds % 86400 / 3600);
  const minutes = Math.floor(seconds % 3600 / 60);
  if (days > 0) return `${days} วัน ${hours} ชม.`;
  if (hours > 0) return `${hours} ชม. ${minutes} นาที`;
  return `${minutes} นาที`;
}

// "ai-bcc-v2-api-1" reads better as "api"; other stacks keep their own names.
export function shortContainerName(name: string): string {
  return name.replace(/^ai-bcc-v2-/, '').replace(/-\d+$/, '');
}

// Docker reports the host's memory as the limit when a container has none. That
// is not a ceiling worth drawing a bar against, so treat it as "no limit".
export function containerMemoryPercent(container: MonitorContainerStats, hostMemoryTotalBytes: number): number | null {
  if (container.memoryLimitBytes <= 0 || container.memoryLimitBytes >= hostMemoryTotalBytes * 0.95) return null;
  return usagePercent(container.memoryUsedBytes, container.memoryLimitBytes);
}

export type SeriesKind = 'cpu' | 'memory';
export type ChartSeries = { label: string; data: Array<number | null> };

// One series per container, ranked by its peak so the busiest ones are kept.
export function containerSeries(points: MonitorHistoryPoint[], kind: SeriesKind, limit = 6): ChartSeries[] {
  const value = (container: MonitorContainerStats) => kind === 'cpu' ? container.cpuPercent : container.memoryUsedBytes / 1048576;
  const peaks = new Map<string, number>();
  for (const point of points) {
    for (const container of point.containers) peaks.set(container.name, Math.max(peaks.get(container.name) ?? 0, value(container)));
  }
  return [...peaks.entries()]
    .sort((left, right) => right[1] - left[1] || left[0].localeCompare(right[0]))
    .slice(0, limit)
    .map(([name]) => ({
      label: shortContainerName(name),
      data: points.map((point) => {
        const container = point.containers.find((item) => item.name === name);
        return container ? Math.round(value(container) * 10) / 10 : null;
      })
    }));
}

export function timeLabel(iso: string, minutes: number): string {
  const date = new Date(iso);
  const options: Intl.DateTimeFormatOptions = minutes > 360
    ? { hour: '2-digit', minute: '2-digit', day: 'numeric', month: 'short', timeZone: 'Asia/Bangkok' }
    : { hour: '2-digit', minute: '2-digit', timeZone: 'Asia/Bangkok' };
  return date.toLocaleString('th-TH', options);
}
