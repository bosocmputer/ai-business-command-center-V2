import { describe, expect, it } from 'vitest';
import type { MonitorHistoryPoint } from '@/api';
import { containerMemoryPercent, containerSeries, formatBytes, formatUptime, shortContainerName, usagePercent, usageSeverity } from './monitorPresentation';

const container = (name: string, cpu: number, mib: number, limitMib = 512) => ({ name, cpuPercent: cpu, memoryUsedBytes: mib * 1048576, memoryLimitBytes: limitMib * 1048576 });
const point = (at: string, containers: ReturnType<typeof container>[]): MonitorHistoryPoint => ({ at, cpuPercent: 0, memoryUsedPercent: 0, load1: 0, containers });

describe('monitor presentation', () => {
  it('formats bytes, percentages and uptime', () => {
    expect(formatBytes(0)).toBe('0 B');
    expect(formatBytes(512)).toBe('512 B');
    expect(formatBytes(1536)).toBe('1.5 KB');
    expect(formatBytes(48.5 * 1048576)).toBe('48.5 MB');
    expect(formatBytes(15.6 * 1073741824)).toBe('15.6 GB');
    expect(usagePercent(1, 4)).toBe(25);
    expect(usagePercent(5, 0)).toBe(0);
    expect(usagePercent(10, 4)).toBe(100);
    expect(formatUptime(90061)).toBe('1 วัน 1 ชม.');
    expect(formatUptime(3720)).toBe('1 ชม. 2 นาที');
    expect(formatUptime(300)).toBe('5 นาที');
  });

  it('warns at 75% and turns critical at 90%', () => {
    expect([usageSeverity(74.9), usageSeverity(75), usageSeverity(89.9), usageSeverity(90)]).toEqual(['success', 'warn', 'warn', 'danger']);
  });

  it('shortens compose names and keeps foreign names', () => {
    expect(shortContainerName('ai-bcc-v2-api-1')).toBe('api');
    expect(shortContainerName('ai-bcc-v2-postgres-1')).toBe('postgres');
    expect(shortContainerName('ai-bcc-api')).toBe('ai-bcc-api');
  });

  it('draws no memory bar for containers whose limit is the whole host', () => {
    const host = 16 * 1024 * 1048576;
    expect(containerMemoryPercent(container('api', 1, 256), host)).toBe(50);
    expect(containerMemoryPercent({ ...container('v1', 1, 256), memoryLimitBytes: host }, host)).toBeNull();
    expect(containerMemoryPercent({ ...container('none', 1, 256), memoryLimitBytes: 0 }, host)).toBeNull();
  });

  it('keeps the busiest containers by peak and leaves gaps where one is missing', () => {
    const points = [
      point('2026-09-30T05:00:00Z', [container('ai-bcc-v2-api-1', 5, 100), container('hermes', 1, 400)]),
      point('2026-09-30T05:01:00Z', [container('ai-bcc-v2-api-1', 9, 110)]),
      point('2026-09-30T05:02:00Z', [container('ai-bcc-v2-api-1', 2, 105), container('hermes', 30, 420), container('idle', 0, 1)])
    ];
    const cpu = containerSeries(points, 'cpu', 2);
    expect(cpu.map((series) => series.label)).toEqual(['hermes', 'api']);
    expect(cpu[0]?.data).toEqual([1, null, 30]);
    expect(cpu[1]?.data).toEqual([5, 9, 2]);
    const memory = containerSeries(points, 'memory', 6);
    expect(memory.map((series) => series.label)).toEqual(['hermes', 'api', 'idle']);
    expect(memory[0]?.data).toEqual([400, null, 420]);
  });
});
