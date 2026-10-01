import type { ReportDrillLink, ReportKey } from '@/api';
import { bangkokToday, type ReportPeriodMode } from '@/utils/reportPeriod';

// A drill-down opens another report filtered to one customer, item or document.
// The filter travels in the address as drillColumn and drillValue, so the link
// can be reloaded or shared, and the target report applies it as an ordinary
// equality row filter on its own stored rows once the report is open.
export const drillColumnQueryKey = 'drillColumn';
export const drillValueQueryKey = 'drillValue';
export const drillFromQueryKey = 'drillFrom';
export const drillDateFromQueryKey = 'drillDateFrom';
export const drillDateToQueryKey = 'drillDateTo';

const columnPattern = /^[a-z][a-z0-9_]*$/;
const maximumValueLength = 160;

export interface DrillRequest {
  column: string;
  value: string;
  from?: ReportKey;
  // The period the target should open for, when the source knows one that makes
  // sense. Without it a drill into a date-range report would open for whatever
  // period was last selected, which may not contain the customer or document.
  dateFrom?: string;
  dateTo?: string;
}

export interface DrillPeriod {
  dateFrom: string;
  dateTo: string;
}

const dateOnlyPattern = /^\d{4}-\d{2}-\d{2}$/;
const defaultLookbackDays = 90;

function shiftDays(value: string, days: number): string {
  const date = new Date(`${value}T00:00:00Z`);
  date.setUTCDate(date.getUTCDate() + days);
  return date.toISOString().slice(0, 10);
}

function validDate(value: unknown): value is string {
  if (typeof value !== 'string' || !dateOnlyPattern.test(value)) return false;
  const date = new Date(`${value}T00:00:00Z`);
  return !Number.isNaN(date.getTime()) && date.toISOString().slice(0, 10) === value;
}

// drillPeriodFor chooses the period the target report should open for, or
// undefined to leave the viewer's own selection alone:
//   - a document link opens a date-range target for the document's own day;
//   - a date-range source hands its range to a date-range target, and its last day
//     to an as-of target;
//   - an as-of source opens a date-range target for the 90 days up to that date.
export function drillPeriodFor(
  link: ReportDrillLink,
  targetMode: ReportPeriodMode | undefined,
  sourceMode: ReportPeriodMode,
  sourcePeriod: { dateFrom: string; dateTo: string } | undefined,
  row: Record<string, unknown>,
  now = new Date()
): DrillPeriod | undefined {
  if (targetMode !== 'DATE_RANGE' && targetMode !== 'AS_OF_DATE') return undefined;
  if (link.dateColumn) {
    const day = row[link.dateColumn];
    return targetMode === 'DATE_RANGE' && validDate(day) && day <= bangkokToday(now) ? { dateFrom: day, dateTo: day } : undefined;
  }
  if (!sourcePeriod || !validDate(sourcePeriod.dateFrom) || !validDate(sourcePeriod.dateTo)) return undefined;
  if (sourceMode === 'DATE_RANGE') {
    return targetMode === 'DATE_RANGE' ? { dateFrom: sourcePeriod.dateFrom, dateTo: sourcePeriod.dateTo } : { dateFrom: sourcePeriod.dateTo, dateTo: sourcePeriod.dateTo };
  }
  if (sourceMode === 'AS_OF_DATE' && targetMode === 'DATE_RANGE') return { dateFrom: shiftDays(sourcePeriod.dateTo, -(defaultLookbackDays - 1)), dateTo: sourcePeriod.dateTo };
  return undefined;
}

// DrillAnchor is a column on screen that a viewer can open other reports from.
// valueColumn is where the key is read from, which is not always the column
// shown: a report usually shows a customer's name and keeps the code hidden.
export interface DrillAnchor {
  valueColumn: string;
  links: ReportDrillLink[];
}

// drillAnchors decides, for the columns currently displayed, which of them carry
// drill links. A link is offered on its identifier column when that is shown, and
// otherwise on its name column, so the viewer can reach it either way.
export function drillAnchors(links: readonly ReportDrillLink[] | undefined, displayedColumns: readonly string[]): Map<string, DrillAnchor> {
  const displayed = new Set(displayedColumns);
  const anchors = new Map<string, DrillAnchor>();
  for (const link of links ?? []) {
    const anchorColumn = displayed.has(link.column) ? link.column : link.labelColumn && displayed.has(link.labelColumn) ? link.labelColumn : undefined;
    if (!anchorColumn) continue;
    const existing = anchors.get(anchorColumn);
    if (existing && existing.valueColumn !== link.column) continue;
    anchors.set(anchorColumn, { valueColumn: link.column, links: [...(existing?.links ?? []), link] });
  }
  return anchors;
}

// drillValueOf reads the value to carry from a row, or undefined when the row has
// nothing usable (an empty cell cannot be drilled into).
export function drillValueOf(cell: unknown): string | undefined {
  if (typeof cell !== 'string' && typeof cell !== 'number') return undefined;
  const value = String(cell).trim();
  return value && value.length <= maximumValueLength ? value : undefined;
}

export function drillQuery(link: ReportDrillLink, value: string, from: ReportKey, period?: DrillPeriod): Record<string, string> {
  const query: Record<string, string> = { [drillColumnQueryKey]: link.targetColumn, [drillValueQueryKey]: value, [drillFromQueryKey]: from };
  if (period) { query[drillDateFromQueryKey] = period.dateFrom; query[drillDateToQueryKey] = period.dateTo; }
  return query;
}

// drillRequestFromQuery validates what the address claims. Anything malformed is
// ignored rather than guessed at, and a column the target report cannot filter on
// is ignored too, so a hand-edited address cannot make the page send a filter the
// server would refuse.
export function drillRequestFromQuery(query: Record<string, unknown>, filterableColumns: ReadonlySet<string>): DrillRequest | undefined {
  const column = query[drillColumnQueryKey];
  const value = query[drillValueQueryKey];
  if (typeof column !== 'string' || typeof value !== 'string') return undefined;
  if (!columnPattern.test(column) || !filterableColumns.has(column)) return undefined;
  const trimmed = value.trim();
  if (!trimmed || trimmed.length > maximumValueLength) return undefined;
  const from = query[drillFromQueryKey];
  const request: DrillRequest = { column, value: trimmed, from: typeof from === 'string' ? (from as ReportKey) : undefined };
  const dateFrom = query[drillDateFromQueryKey];
  const dateTo = query[drillDateToQueryKey];
  // A period counts only when both ends are real dates in order and within the
  // 366 days a report may cover; otherwise the viewer's own selection is used.
  if (validDate(dateFrom) && validDate(dateTo) && dateFrom <= dateTo && (Date.parse(dateTo) - Date.parse(dateFrom)) / 86_400_000 < 366) {
    request.dateFrom = dateFrom;
    request.dateTo = dateTo;
  }
  return request;
}

export function withoutDrillQuery(query: Record<string, string | string[]>): Record<string, string | string[]> {
  const rest = { ...query };
  delete rest[drillColumnQueryKey];
  delete rest[drillValueQueryKey];
  delete rest[drillFromQueryKey];
  delete rest[drillDateFromQueryKey];
  delete rest[drillDateToQueryKey];
  return rest;
}
