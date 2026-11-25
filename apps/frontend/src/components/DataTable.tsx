import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { ArrowDown, ArrowUp, ArrowUpDown, ChevronLeft, ChevronRight, Columns3, RotateCcw } from 'lucide-react';
import { Spinner } from './ui';
import { useTranslation } from '../i18n/useTranslation';

/**
 * Column definition, same shape as the saas-rus DataTable so a table can be
 * moved between the two projects without rewriting its config.
 */
export interface Column<T> {
  key: keyof T | string;
  label: string;
  sortable?: boolean;
  width?: number;
  minWidth?: number;
  /** Custom cell. Returning undefined falls back to the raw value. */
  render?: (item: T, value: unknown) => ReactNode;
  /** Hidden unless the operator turns it on in the column picker. */
  hiddenByDefault?: boolean;
}

export interface SortState {
  field: string;
  direction: 'asc' | 'desc';
}

export interface DataTableProps<T> {
  data: T[];
  columns: Column<T>[];
  keyField: keyof T;
  /** Stable id used to remember column visibility per table. */
  tableId: string;
  isLoading?: boolean;
  emptyMessage?: string;
  /** Rendered when there are no rows at all, instead of emptyMessage. */
  emptyState?: ReactNode;

  // Pagination. Omit onPageChange for a table that shows everything.
  paginated?: boolean;
  pageSizeOptions?: number[];
  initialPageSize?: number;

  // Sorting is local: the caller passes the already-filtered rows.
  initialSort?: SortState;
  onSortChange?: (sort: SortState | null) => void;

  showColumnToggle?: boolean;
  onRowClick?: (item: T) => void;
  /** Extra classes for the wrapper, e.g. to control overflow. */
  className?: string;
}

const DEFAULT_PAGE_SIZES = [10, 25, 50, 100];

function readHiddenColumns(tableId: string): Set<string> {
  try {
    const raw = localStorage.getItem(`table:${tableId}:hidden`);
    return raw ? new Set(JSON.parse(raw) as string[]) : new Set();
  } catch {
    return new Set();
  }
}

function writeHiddenColumns(tableId: string, hidden: Set<string>) {
  localStorage.setItem(`table:${tableId}:hidden`, JSON.stringify([...hidden]));
}

function cellValue<T>(item: T, key: string): unknown {
  return (item as Record<string, unknown>)[key];
}

/**
 * Sorting and pagination for a table the caller has already filtered.
 *
 * The panel's data is small (tens of rows), so sorting locally avoids a second
 * round trip per click and keeps the table usable while an API is slow. Column
 * visibility is remembered per tableId, which is what makes a wide table like
 * Tunnels tolerable on a narrow screen.
 */
export function DataTable<T>({
  data,
  columns,
  keyField,
  tableId,
  isLoading = false,
  emptyMessage,
  emptyState,
  paginated = true,
  pageSizeOptions = DEFAULT_PAGE_SIZES,
  initialPageSize = 10,
  initialSort,
  onSortChange,
  showColumnToggle = true,
  onRowClick,
  className = '',
}: DataTableProps<T>) {
  const { t: tc } = useTranslation('common');
  const [hidden, setHidden] = useState<Set<string>>(() => readHiddenColumns(tableId));
  const [showPicker, setShowPicker] = useState(false);
  const [sort, setSort] = useState<SortState | null>(initialSort ?? null);
  const [page, setPage] = useState(1);
  const [limit, setLimit] = useState(initialPageSize);
  const pickerRef = useRef<HTMLDivElement | null>(null);

  const visibleColumns = useMemo(
    () => columns.filter((c) => !hidden.has(String(c.key))),
    [columns, hidden],
  );

  const toggleColumn = (key: string) => {
    setHidden((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      writeHiddenColumns(tableId, next);
      return next;
    });
  };

  const resetColumns = () => {
    const next = new Set(columns.filter((c) => c.hiddenByDefault).map((c) => String(c.key)));
    writeHiddenColumns(tableId, next);
    setHidden(next);
  };

  // A click outside the picker closes it; without this it stays open over the
  // table and the next click lands on a checkbox nobody meant to press.
  useEffect(() => {
    if (!showPicker) return;
    const onDown = (event: MouseEvent) => {
      if (pickerRef.current && !pickerRef.current.contains(event.target as Node)) setShowPicker(false);
    };
    document.addEventListener('mousedown', onDown);
    return () => document.removeEventListener('mousedown', onDown);
  }, [showPicker]);

  const sorted = useMemo(() => {
    if (!sort) return data;
    const copy = [...data];
    copy.sort((a, b) => {
      const av = cellValue(a, sort.field);
      const bv = cellValue(b, sort.field);
      const an = typeof av === 'number' ? av : null;
      const bn = typeof bv === 'number' ? bv : null;
      let cmp: number;
      if (an !== null && bn !== null) cmp = an - bn;
      else cmp = String(av ?? '').localeCompare(String(bv ?? ''), undefined, { numeric: true });
      return sort.direction === 'asc' ? cmp : -cmp;
    });
    return copy;
  }, [data, sort]);

  const total = sorted.length;
  const totalPages = paginated ? Math.max(1, Math.ceil(total / limit)) : 1;
  const safePage = Math.min(page, totalPages);
  const rows = paginated ? sorted.slice((safePage - 1) * limit, safePage * limit) : sorted;

  const changeSort = useCallback(
    (field: string) => {
      setSort((prev) => {
        const next: SortState | null =
          prev?.field !== field
            ? { field, direction: 'asc' }
            : prev.direction === 'asc'
              ? { field, direction: 'desc' }
              : null; // third click clears, so "unsorted" stays reachable
        onSortChange?.(next);
        return next;
      });
      setPage(1);
    },
    [onSortChange],
  );

  // Filtering happens in the caller, so a new result set must not leave the
  // table stranded on page 4 of a 1-page result.
  useEffect(() => {
    setPage(1);
  }, [data]);

  const rangeStart = total === 0 ? 0 : (safePage - 1) * limit + 1;
  const rangeEnd = paginated ? Math.min(safePage * limit, total) : total;

  return (
    <div className={className}>
      {showColumnToggle && (
        <div className="mb-2 flex justify-end">
          <div className="relative" ref={pickerRef}>
            <button
              className="btn-ghost !px-2 !py-1 !text-xs"
              onClick={() => setShowPicker((v) => !v)}
              title={tc('toggle_columns')}
            >
              <Columns3 className="h-3.5 w-3.5" /> {tc('columns')}
            </button>
            {showPicker && (
              <div className="absolute right-0 z-20 mt-1 w-56 rounded-lg border border-surface-700 bg-surface-900 p-2 shadow-xl">
                <div className="max-h-64 space-y-1 overflow-auto">
                  {columns.map((c) => {
                    const key = String(c.key);
                    return (
                      <label key={key} className="flex items-center gap-2 rounded px-1.5 py-1 text-xs text-slate-300 hover:bg-surface-800">
                        <input type="checkbox" checked={!hidden.has(key)} onChange={() => toggleColumn(key)} />
                        {c.label}
                      </label>
                    );
                  })}
                </div>
                <button className="mt-2 flex w-full items-center justify-center gap-1 rounded border border-surface-700 px-2 py-1 text-[11px] text-slate-400 hover:text-slate-200" onClick={resetColumns}>
                  <RotateCcw className="h-3 w-3" /> {tc('reset')}
                </button>
              </div>
            )}
          </div>
        </div>
      )}

      <div className="overflow-x-auto rounded-xl border border-surface-700/70">
        <table className="w-full min-w-[720px]">
          <thead className="border-b border-surface-700/70 bg-surface-900/60">
            <tr>
              {visibleColumns.map((c) => {
                const key = String(c.key);
                const active = sort?.field === key;
                return (
                  <th
                    key={key}
                    className="th"
                    style={{ width: c.width, minWidth: c.minWidth }}
                    aria-sort={active ? (sort.direction === 'asc' ? 'ascending' : 'descending') : 'none'}
                  >
                    {c.sortable ? (
                      <button
                        className="inline-flex items-center gap-1 hover:text-slate-200"
                        onClick={() => changeSort(key)}
                      >
                        {c.label}
                        {active ? (
                          sort.direction === 'asc' ? (
                            <ArrowUp className="h-3 w-3" />
                          ) : (
                            <ArrowDown className="h-3 w-3" />
                          )
                        ) : (
                          <ArrowUpDown className="h-3 w-3 opacity-40" />
                        )}
                      </button>
                    ) : (
                      c.label
                    )}
                  </th>
                );
              })}
            </tr>
          </thead>
          <tbody className="divide-y divide-surface-800">
            {isLoading ? (
              <tr>
                <td className="td" colSpan={visibleColumns.length}>
                  <div className="flex justify-center py-10 text-slate-400">
                    <Spinner className="h-5 w-5" />
                  </div>
                </td>
              </tr>
            ) : rows.length === 0 ? (
              <tr>
                <td className="td" colSpan={visibleColumns.length}>
                  {emptyState ?? <p className="py-10 text-center text-sm text-slate-500">{emptyMessage ?? tc('no_rows')}</p>}
                </td>
              </tr>
            ) : (
              rows.map((item) => (
                <tr
                  key={String(item[keyField])}
                  className={onRowClick ? 'cursor-pointer hover:bg-surface-900/40' : 'hover:bg-surface-900/40'}
                  onClick={onRowClick ? () => onRowClick(item) : undefined}
                >
                  {visibleColumns.map((c) => {
                    const value = cellValue(item, String(c.key));
                    return (
                      <td key={String(c.key)} className="td">
                        {c.render ? c.render(item, value) : ((value as ReactNode) ?? '—')}
                      </td>
                    );
                  })}
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      {paginated && (
        <div className="mt-3 flex flex-wrap items-center justify-between gap-3 text-xs text-slate-400">
          <div className="flex items-center gap-2">
            <span>{tc('range_of', { start: rangeStart, end: rangeEnd, total })}</span>
            <select
              className="field !w-auto !py-1 !text-xs"
              value={limit}
              onChange={(e) => {
                setLimit(Number(e.target.value));
                setPage(1);
              }}
            >
              {pageSizeOptions.map((n) => (
                <option key={n} value={n}>
                  {tc('per_page', { count: n })}
                </option>
              ))}
            </select>
          </div>
          <div className="flex items-center gap-1">
            <button
              className="btn-ghost !px-2 !py-1"
              disabled={safePage <= 1}
              onClick={() => setPage(1)}
              title={tc('first_page')}
            >
              <ChevronLeft className="h-3.5 w-3.5" />
              <ChevronLeft className="-ml-2 h-3.5 w-3.5" />
            </button>
            <button
              className="btn-ghost !px-2 !py-1"
              disabled={safePage <= 1}
              onClick={() => setPage(safePage - 1)}
              title={tc('previous_page')}
            >
              <ChevronLeft className="h-3.5 w-3.5" />
            </button>
            <span className="px-2 font-mono text-slate-300">
              {safePage} / {totalPages}
            </span>
            <button
              className="btn-ghost !px-2 !py-1"
              disabled={safePage >= totalPages}
              onClick={() => setPage(safePage + 1)}
              title={tc('next_page')}
            >
              <ChevronRight className="h-3.5 w-3.5" />
            </button>
            <button
              className="btn-ghost !px-2 !py-1"
              disabled={safePage >= totalPages}
              onClick={() => setPage(totalPages)}
              title={tc('last_page')}
            >
              <ChevronRight className="h-3.5 w-3.5" />
              <ChevronRight className="-ml-2 h-3.5 w-3.5" />
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
