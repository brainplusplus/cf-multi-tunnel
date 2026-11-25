import { useEffect, useMemo, useState } from 'react';
import { ScrollText } from 'lucide-react';
import { apiMessage } from '../api/client';
import { auditApi } from '../api/endpoints';
import { DataTable, type Column } from '../components/DataTable';
import { FilterToggle, QueryBuilder, filterCount } from '../components/QueryBuilder';
import { createGroup, matchesQuery, type FieldDefinition, type QueryGroup } from '../components/query';
import { Badge, EmptyState, ErrorBox, PageHeader, Spinner } from '../components/ui';
import { useTranslation } from '../i18n/useTranslation';
import type { AuditLog } from '../types';

export default function AuditPage() {
  const { t } = useTranslation('audit');
  const { t: tc } = useTranslation('common');
  const [logs, setLogs] = useState<AuditLog[] | null>(null);
  const [total, setTotal] = useState(0);
  const [error, setError] = useState('');
  const [search, setSearch] = useState('');
  const [showFilters, setShowFilters] = useState(false);
  const [filters, setFilters] = useState<QueryGroup>(() => createGroup());

  useEffect(() => {
    auditApi
      .list(1, 100)
      .then((result) => {
        setLogs(result.items);
        setTotal(result.total);
      })
      .catch((err) => setError(apiMessage(err)));
  }, []);

  const fields: FieldDefinition[] = useMemo(
    () => [
      { name: 'actor_email', label: t('filter.actor'), type: 'string' },
      { name: 'action', label: t('filter.action'), type: 'string' },
      { name: 'entity', label: t('filter.entity'), type: 'string' },
      { name: 'detail', label: t('filter.detail'), type: 'string' },
      { name: 'ip', label: t('filter.ip'), type: 'string' },
    ],
    [t],
  );

  const rows = useMemo(() => {
    const term = search.trim().toLowerCase();
    return (logs ?? []).filter((log) => {
      if (
        term &&
        !`${log.actor_email} ${log.action} ${log.entity} ${log.detail} ${log.ip}`.toLowerCase().includes(term)
      ) {
        return false;
      }
      return matchesQuery(log as unknown as Record<string, unknown>, filters);
    });
  }, [logs, search, filters]);

  const columns: Column<AuditLog>[] = [
    {
      key: 'created_at',
      label: t('columns.when'),
      sortable: true,
      width: 180,
      render: (log) => (
        <span className="whitespace-nowrap text-xs text-slate-400">{new Date(log.created_at).toLocaleString()}</span>
      ),
    },
    {
      key: 'actor_email',
      label: t('columns.actor'),
      sortable: true,
      width: 200,
      render: (log) => <span className="text-xs">{log.actor_email || '—'}</span>,
    },
    {
      key: 'action',
      label: t('columns.action'),
      sortable: true,
      width: 200,
      render: (log) => <Badge tone="blue">{log.action}</Badge>,
    },
    {
      key: 'entity',
      label: t('columns.entity'),
      sortable: true,
      width: 160,
      render: (log) => (
        <span className="text-xs text-slate-400">
          {log.entity}
          <span className="ml-1 font-mono text-[10px] text-slate-600">{log.entity_id.slice(0, 8)}</span>
        </span>
      ),
    },
    {
      key: 'detail',
      label: t('columns.detail'),
      minWidth: 220,
      render: (log) => <span className="text-xs text-slate-400">{log.detail || '—'}</span>,
    },
    {
      key: 'ip',
      label: t('columns.ip'),
      width: 140,
      render: (log) => <span className="font-mono text-[11px] text-slate-500">{log.ip || '—'}</span>,
    },
  ];

  return (
    <div>
      <PageHeader title={t('title')} description={t('description')} />
      {error && <ErrorBox message={error} />}
      {!logs ? (
        <div className="flex justify-center py-16 text-slate-400">
          <Spinner className="h-6 w-6" />
        </div>
      ) : logs.length === 0 ? (
        <div className="card">
          <EmptyState icon={<ScrollText className="h-8 w-8" />} title={t('empty')} />
        </div>
      ) : (
        <div className="card">
          <div className="mb-3 flex flex-wrap items-center gap-2">
            <input
              className="field !w-64 !py-1.5 text-xs"
              placeholder={tc('search')}
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
            <FilterToggle
              open={showFilters}
              count={filterCount(filters)}
              onClick={() => setShowFilters((v) => !v)}
            />
            <span className="text-[11px] text-slate-500">{tc('rows_found', { count: rows.length })}</span>
            {total > (logs?.length ?? 0) && (
              <span className="text-[11px] text-amber-300/80">
                {t('truncated', { shown: logs?.length ?? 0, total })}
              </span>
            )}
          </div>

          {showFilters && (
            <div className="mb-3">
              <QueryBuilder fields={fields} value={filters} onChange={setFilters} />
            </div>
          )}

          <DataTable
            tableId="audit"
            data={rows}
            columns={columns}
            keyField="id"
            emptyMessage={tc('no_match')}
            initialSort={{ field: 'created_at', direction: 'desc' }}
            initialPageSize={25}
          />
        </div>
      )}
    </div>
  );
}
