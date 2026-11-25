import { useCallback, useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { Globe2, RefreshCw } from 'lucide-react';
import { apiMessage } from '../api/client';
import { accountsApi, appsApi, tunnelsApi } from '../api/endpoints';
import { DataTable, type Column } from '../components/DataTable';
import { FilterToggle, QueryBuilder, filterCount } from '../components/QueryBuilder';
import { createGroup, matchesQuery, type FieldDefinition, type QueryGroup } from '../components/query';
import { Badge, EmptyState, ErrorBox, PageHeader, Spinner } from '../components/ui';
import { useToast } from '../components/Toast';
import { useTranslation } from '../i18n/useTranslation';
import type { PublishedApp } from '../types';

type AppRow = PublishedApp & { kind_label: string; enabled_label: string };

export default function AppsPage() {
  const { t } = useTranslation('apps');
  const { t: tc } = useTranslation('common');
  const toast = useToast();
  const [apps, setApps] = useState<PublishedApp[] | null>(null);
  const [error, setError] = useState('');
  const [syncing, setSyncing] = useState(false);
  const [search, setSearch] = useState('');
  const [showFilters, setShowFilters] = useState(false);
  const [filters, setFilters] = useState<QueryGroup>(() => createGroup());

  const load = useCallback(async () => {
    try {
      setApps(await appsApi.list());
      setError('');
    } catch (err) {
      setError(apiMessage(err));
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  // A local reload cannot discover hostnames that only exist in Cloudflare, which
  // is why "Refresh" used to look like it did nothing after an import. This pulls
  // each account's remote ingress into the panel first, then reloads.
  const syncAll = async () => {
    setSyncing(true);
    try {
      const accounts = await accountsApi.list();
      const usable = accounts.filter((a) => a.credential === 'api');
      if (usable.length === 0) {
        toast.push('error', t('sync_no_account'));
        return;
      }
      let created = 0;
      let updated = 0;
      for (const account of usable) {
        const res = await tunnelsApi.sync(account.id);
        created += res.result.apps_created;
        updated += res.result.apps_updated;
      }
      await load();
      toast.push(
        'success',
        created + updated === 0 ? t('sync_done_none') : t('sync_done', { created, updated }),
      );
    } catch (err) {
      toast.push('error', apiMessage(err));
    } finally {
      setSyncing(false);
    }
  };

  const retry = async (app: PublishedApp) => {
    try {
      const result = await appsApi.retry(app.id);
      toast.push(result.sync_status === 'synced' ? 'success' : 'error', result.last_error || t('synced'));
      void load();
    } catch (err) {
      toast.push('error', apiMessage(err));
    }
  };

  const toggle = async (app: PublishedApp) => {
    try {
      await appsApi.setEnabled(app.id, !app.enabled);
      void load();
    } catch (err) {
      toast.push('error', apiMessage(err));
    }
  };

  const fields: FieldDefinition[] = useMemo(
    () => [
      { name: 'hostname', label: t('filter.hostname'), type: 'string' },
      { name: 'service_url', label: t('filter.service'), type: 'string' },
      {
        name: 'kind_label',
        label: t('filter.kind'),
        type: 'select',
        options: [
          { value: t('kind.app'), label: t('kind.app') },
          { value: t('kind.panel'), label: t('kind.panel') },
        ],
      },
      {
        name: 'sync_status',
        label: t('filter.sync'),
        type: 'select',
        options: ['synced', 'pending', 'error', 'manual'].map((s) => ({ value: s, label: s })),
      },
      {
        name: 'enabled_label',
        label: t('filter.enabled'),
        type: 'select',
        options: [
          { value: t('filter.enabled_yes'), label: t('filter.enabled_yes') },
          { value: t('filter.enabled_no'), label: t('filter.enabled_no') },
        ],
      },
    ],
    [t],
  );

  const rows: AppRow[] = useMemo(() => {
    const term = search.trim().toLowerCase();
    return (apps ?? [])
      .map((app) => ({
        ...app,
        kind_label: app.kind === 'panel' ? t('kind.panel') : t('kind.app'),
        enabled_label: app.enabled ? t('filter.enabled_yes') : t('filter.enabled_no'),
      }))
      .filter((app) => {
        if (term && !`${app.hostname} ${app.service_url}`.toLowerCase().includes(term)) return false;
        return matchesQuery(app as unknown as Record<string, unknown>, filters);
      });
  }, [apps, search, filters, t]);

  const columns: Column<AppRow>[] = [
    {
      key: 'hostname',
      label: t('columns.hostname'),
      sortable: true,
      minWidth: 200,
      render: (app) => (
        <div>
          <Link to={`/tunnels/${app.tunnel_id}`} className="font-medium text-slate-100 hover:text-accent-400">
            {app.hostname}
          </Link>
          {app.zone_name && <p className="text-[11px] text-slate-500">{app.zone_name}</p>}
        </div>
      ),
    },
    {
      key: 'service_url',
      label: t('columns.service'),
      sortable: true,
      minWidth: 180,
      render: (app) => <span className="font-mono text-xs">{app.service_url}</span>,
    },
    {
      key: 'kind_label',
      label: t('columns.kind'),
      sortable: true,
      width: 100,
      render: (app) => <Badge tone={app.kind === 'panel' ? 'violet' : 'slate'}>{app.kind_label}</Badge>,
    },
    {
      key: 'sync_status',
      label: t('columns.sync'),
      sortable: true,
      width: 150,
      render: (app) => (
        <div>
          <Badge
            tone={
              app.sync_status === 'synced'
                ? 'green'
                : app.sync_status === 'error'
                  ? 'red'
                  : app.sync_status === 'manual'
                    ? 'amber'
                    : 'slate'
            }
          >
            {app.sync_status}
          </Badge>
          {app.last_error && (
            <p className="mt-1 max-w-xs truncate text-[11px] text-rose-300/80">{app.last_error}</p>
          )}
        </div>
      ),
    },
    {
      key: 'enabled_label',
      label: t('columns.enabled'),
      width: 110,
      render: (app) =>
        app.kind === 'panel' ? (
          <Link to="/settings" className="text-[11px] text-slate-500 hover:text-accent-400">
            {t('in_settings')}
          </Link>
        ) : (
          <button
            role="switch"
            aria-checked={app.enabled}
            aria-label={t('columns.enabled')}
            onClick={() => void toggle(app)}
            className={`relative h-5 w-9 rounded-full transition-colors ${app.enabled ? 'bg-emerald-500/70' : 'bg-surface-600'}`}
          >
            <span
              className={`absolute top-0.5 h-4 w-4 rounded-full bg-white transition-all ${app.enabled ? 'left-[18px]' : 'left-0.5'}`}
            />
          </button>
        ),
    },
    {
      key: 'actions',
      label: tc('actions'),
      width: 110,
      render: (app) =>
        app.sync_status !== 'synced' ? (
          <button className="btn-ghost !px-2 !py-1 !text-xs" onClick={() => void retry(app)}>
            <RefreshCw className="h-3.5 w-3.5" /> {t('retry')}
          </button>
        ) : null,
    },
  ];

  return (
    <div>
      <PageHeader
        title={t('title')}
        description={t('description')}
        actions={
          <button className="btn-ghost" disabled={syncing} onClick={() => void syncAll()}>
            {syncing ? <Spinner className="h-4 w-4" /> : <RefreshCw className="h-4 w-4" />} {t('sync_cloudflare')}
          </button>
        }
      />

      {error && <ErrorBox message={error} />}

      {!apps ? (
        <div className="flex justify-center py-16 text-slate-400">
          <Spinner className="h-6 w-6" />
        </div>
      ) : apps.length === 0 ? (
        <div className="card">
          <EmptyState
            icon={<Globe2 className="h-8 w-8" />}
            title={t('empty')}
            description={t('empty_hint')}
            action={
              <button className="btn-primary" disabled={syncing} onClick={() => void syncAll()}>
                {syncing ? <Spinner className="h-4 w-4" /> : <RefreshCw className="h-4 w-4" />} {t('sync_cloudflare')}
              </button>
            }
          />
        </div>
      ) : (
        <div className="card">
          <div className="mb-3 flex flex-wrap items-center gap-2">
            <input
              className="field !w-64 !py-1.5 text-xs"
              placeholder={t('filter.hostname')}
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
            <FilterToggle
              open={showFilters}
              count={filterCount(filters)}
              onClick={() => setShowFilters((v) => !v)}
            />
            <span className="text-[11px] text-slate-500">{tc('rows_found', { count: rows.length })}</span>
          </div>

          {showFilters && (
            <div className="mb-3">
              <QueryBuilder fields={fields} value={filters} onChange={setFilters} />
            </div>
          )}

          <DataTable
            tableId="apps"
            data={rows}
            columns={columns}
            keyField="id"
            emptyMessage={tc('no_match')}
            initialSort={{ field: 'hostname', direction: 'asc' }}
          />
        </div>
      )}
    </div>
  );
}
