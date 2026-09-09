const standardJavaWSPath = '/SMLJavaWebService/DotNetFrameWork';

export function resolveSMLJavaWSEndpoint(value?: string | null): string {
  const trimmed = value?.trim().replace(/\/+$/, '') ?? '';
  if (!trimmed) return '';
  return trimmed.toLowerCase().endsWith(standardJavaWSPath.toLowerCase())
    ? trimmed
    : `${trimmed}${standardJavaWSPath}`;
}

export function formatSMLTestCooldown(milliseconds: number): string {
  const totalSeconds = Math.max(0, Math.ceil(milliseconds / 1_000));
  if (totalSeconds === 0) return 'สักครู่';
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  if (minutes === 0) return `${seconds} วินาที`;
  if (seconds === 0) return `${minutes} นาที`;
  return `${minutes} นาที ${seconds} วินาที`;
}
