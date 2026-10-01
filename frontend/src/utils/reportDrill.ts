import type { ReportDrillLink, ReportKey } from '@/api';

// A drill-down opens another report filtered to one customer, item or document.
// The filter travels in the address as drillColumn and drillValue, so the link
// can be reloaded or shared, and the target report applies it as an ordinary
// equality row filter on its own stored rows once the report is open.
export const drillColumnQueryKey = 'drillColumn';
export const drillValueQueryKey = 'drillValue';
export const drillFromQueryKey = 'drillFrom';

const columnPattern = /^[a-z][a-z0-9_]*$/;
const maximumValueLength = 160;

export interface DrillRequest {
  column: string;
  value: string;
  from?: ReportKey;
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

export function drillQuery(link: ReportDrillLink, value: string, from: ReportKey): Record<string, string> {
  return { [drillColumnQueryKey]: link.targetColumn, [drillValueQueryKey]: value, [drillFromQueryKey]: from };
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
  return { column, value: trimmed, from: typeof from === 'string' ? (from as ReportKey) : undefined };
}

export function withoutDrillQuery(query: Record<string, string | string[]>): Record<string, string | string[]> {
  const rest = { ...query };
  delete rest[drillColumnQueryKey];
  delete rest[drillValueQueryKey];
  delete rest[drillFromQueryKey];
  return rest;
}
