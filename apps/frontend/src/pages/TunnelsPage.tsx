import { useCallback, useEffect, useMemo, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { Cloud, HardDrive, Laptop, Play, Plus, RefreshCw, Square, Waypoints } from 'lucide-react';
import { apiFields, apiMessage } from '../api/client';
import { accountsApi, tunnelsApi } from '../api/endpoints';
import { DataTable, type Column } from '../components/DataTable';
import { FilterToggle, QueryBuilder, filterCount } from '../components/QueryBuilder';
import { createGroup, matchesQuery, type FieldDefinition, type QueryGroup } from '../components/query';
import { Badge, EmptyState, ErrorBox, Field, Modal, PageHeader, Spinner } from '../components/ui';
import { useToast } from '../components/Toast';
import { useTranslation } from '../i18n/useTranslation';
import type { CFAccount, Tunnel } from '../types';

const statusTone = (status: Tunnel['status']) =>
  status === 'running' ? 'green' : status === 'error' ? 'red' : status === 'stopped' ? 'slate' : 'amber';

// The table needs a couple of derived strings so a filter can select a label the
// operator recognises instead of an opaque id.
type TunnelRow = Tunnel & { account_name: string; placement: string };

export default function TunnelsPage() {
  const { t } = useTranslation('tunnels');
  const { t: tc } = useTranslation('common');
  const toast = useToast();
  const [params, setParams] = useSearchParams();
  const accountFilter = params.get('account') ?? '';
  const locationFilter = params.get('location') ?? '';

  const [accounts, setAccounts] = useState<CFAccount[]>([]);
  const [tunnels, setTunnels] = useState<Tunnel[] | null>(null);
  const [error, setError] = useState('');
  const [modal, setModal] = useState(false);
  const [busyId, setBusyId] = useState('');
  const [search, setSearch] = useState('');
  const [showFilters, setShowFilters] = useState(false);
  const [filters, setFilters] = useState<QueryGroup>(() => createGroup());

  const load = useCallback(async () => {
    try {
      const [acc, tuns] = await Promise.all([
        accountsApi.list(),
        tunnelsApi.list(accountFilter || undefined, locationFilter || undefined),
      ]);
      setAccounts(acc);
      setTunnels(tuns);
      setError('');
    } catch (err) {
      setError(apiMessage(err));
    }
  }, [accountFilter, locationFilter]);

  useEffect(() => {
    void load();
  }, [load]);

  const accountName = useMemo(() => {
    const map = new Map(accounts.map((a) => [a.id, a.name]));
    return (id: string) => map.get(id) ?? id.slice(0, 8);
  }, [accounts]);

  const setFilter = (key: 'account' | 'location', value: string) => {
    const next = new URLSearchParams(params);
    if (value) next.set(key, value);
    else next.delete(key);
    setParams(next);
  };

  const toggle = async (tunnel: Tunnel) => {
    setBusyId(tunnel.id);
    try {
      if (tunnel.status === 'running') {
        await tunnelsApi.stop(tunnel.id);
        toast.push('success', t('actions.stopped', { name: tunnel.name }));
      } else {
        await tunnelsApi.start(tunnel.id);
        toast.push('success', t('actions.started', { name: tunnel.name }));
      }
      void load();
    } catch (err) {
      toast.push('error', apiMessage(err));
    } finally {
      setBusyId('');
    }
  };

  const sync = async () => {
    if (!accountFilter) {
      toast.push('error', t('sync_pick_account'));
      return;
    }
    try {
      const { result } = await tunnelsApi.sync(accountFilter);
      const parts = [
        t('sync_tunnels', { count: result.tunnels }),
        result.tunnels_created ? t('sync_new', { count: result.tunnels_created }) : '',
        t('sync_hostnames', { count: result.apps_created }),
        result.skipped?.length ? t('sync_skipped', { count: result.skipped.length }) : '',
      ].filter(Boolean);
      toast.push(result.skipped?.length ? 'info' : 'success', parts.join(' · '));
      void load();
    } catch (err) {
      toast.push('error', apiMessage(err));
    }
  };

  const localCount = tunnels?.filter((tun) => tun.location === 'local').length ?? 0;
  const remoteCount = tunnels?.filter((tun) => tun.location === 'remote').length ?? 0;

  const fields: FieldDefinition[] = useMemo(
    () => [
      { name: 'name', label: t('filter.name'), type: 'string' },
      {
        name: 'cf_account_id',
        label: t('filter.account'),
        type: 'select',
        options: accounts.map((a) => ({ value: a.id, label: a.name })),
      },
      {
        name: 'status',
        label: t('filter.status'),
        type: 'select',
        options: (['running', 'stopped', 'error', 'unknown'] as const).map((s) => ({
          value: s,
          label: t(`status.${s}`),
        })),
      },
      { name: 'apps_count', label: t('filter.apps'), type: 'number' },
    ],
    [accounts, t],
  );

  const rows: TunnelRow[] = useMemo(() => {
    const term = search.trim().toLowerCase();
    return (tunnels ?? [])
      .map((tun) => ({
        ...tun,
        account_name: accountName(tun.cf_account_id),
        placement: tun.location === 'local' ? t('placement.local') : t('placement.remote'),
      }))
      .filter((tun) => {
        if (term && !`${tun.name} ${tun.tunnel_id} ${tun.host_label}`.toLowerCase().includes(term)) return false;
        return matchesQuery(tun as unknown as Record<string, unknown>, filters);
      });
  }, [tunnels, search, filters, accountName, t]);

  const columns: Column<TunnelRow>[] = [
    {
      key: 'name',
      label: t('columns.tunnel'),
      sortable: true,
      minWidth: 200,
      render: (tun) => (
        <div>
          <Link to={`/tunnels/${tun.id}`} className="font-medium text-slate-100 hover:text-accent-400">
            {tun.name}
          </Link>
          <p className="font-mono text-[11px] text-slate-500">{tun.tunnel_id}</p>
        </div>
      ),
    },
    {
      key: 'account_name',
      label: t('columns.account'),
      sortable: true,
      width: 150,
      render: (tun) => <span className="text-xs">{tun.account_name}</span>,
    },
    {
      key: 'placement',
      label: t('columns.placement'),
      sortable: true,
      width: 170,
      render: (tun) => (
        <div>
          <Badge tone={tun.location === 'local' ? 'blue' : 'violet'} dot>
            {tun.location === 'local' ? t('placement.local') : t('placement.remote')}
          </Badge>
          <p className="mt-1 max-w-[180px] truncate text-[11px] text-slate-500">
            {tun.host_label || (tun.location === 'local' ? t('placement.local_hint') : t('placement.remote_hint'))}
          </p>
        </div>
      ),
    },
    {
      key: 'config_src',
      label: t('columns.config'),
      width: 120,
      render: (tun) => (
        <Badge tone={tun.config_src === 'cloudflare' ? 'green' : 'amber'}>{tun.config_src}</Badge>
      ),
    },
    {
      key: 'apps_count',
      label: t('columns.apps'),
      sortable: true,
      width: 100,
      render: (tun) =>
        tun.apps_count > 0 ? (
          <Link
            to={`/tunnels/${tun.id}`}
            className="inline-flex items-center gap-1 text-xs text-accent-400 hover:text-accent-300"
          >
            {tc(tun.apps_count === 1 ? 'app_one' : 'app_other', { count: tun.apps_count })}
          </Link>
        ) : (
          <span className="text-xs text-slate-500">{tc('none')}</span>
        ),
    },
    {
      key: 'status',
      label: t('columns.status'),
      sortable: true,
      width: 170,
      render: (tun) => (
        <div>
          <Badge tone={statusTone(tun.status)} dot>
            {t(`status.${tun.status}`)}
          </Badge>
          {tun.last_seen_at && (
            <p className="mt-1 text-[11px] text-slate-500">
              {t('status.seen', { when: new Date(tun.last_seen_at).toLocaleTimeString() })}
            </p>
          )}
          {tun.status_message && (
            <p className="mt-1 max-w-[220px] truncate text-[11px] text-rose-300/80">{tun.status_message}</p>
          )}
        </div>
      ),
    },
    {
      key: 'actions',
      label: tc('actions'),
      width: 170,
      render: (tun) => (
        <div className="flex items-center justify-end gap-1">
          {/* Start/Stop is lifecycle, Open is navigation. A managed tunnel used
              to show only Start/Stop, which left no way into its detail page —
              and therefore no way to publish an app on the tunnel that runs on
              this very server. */}
          {tun.location === 'local' && tun.connector_mode === 'docker' && (
            <button
              className="btn-ghost !px-2 !py-1 !text-xs"
              disabled={busyId === tun.id}
              onClick={() => void toggle(tun)}
            >
              {busyId === tun.id ? (
                <Spinner className="h-3.5 w-3.5" />
              ) : tun.status === 'running' ? (
                <Square className="h-3.5 w-3.5" />
              ) : (
                <Play className="h-3.5 w-3.5" />
              )}
              {tun.status === 'running' ? t('actions.stop') : t('actions.start')}
            </button>
          )}
          <Link to={`/tunnels/${tun.id}`} className="btn-ghost !px-2 !py-1 !text-xs">
            {tc('open')}
          </Link>
        </div>
      ),
    },
  ];

  return (
    <div>
      <PageHeader
        title={t('title')}
        description={t('description')}
        actions={
          <>
            <select
              className="field !w-auto !py-1.5 text-xs"
              value={accountFilter}
              onChange={(e) => setFilter('account', e.target.value)}
            >
              <option value="">{tc('all')}</option>
              {accounts.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name}
                </option>
              ))}
            </select>
            <button className="btn-ghost" onClick={() => void sync()} disabled={!accountFilter}>
              <RefreshCw className="h-4 w-4" /> {tc('sync')}
            </button>
            <button className="btn-primary" onClick={() => setModal(true)}>
              <Plus className="h-4 w-4" /> {t('new_tunnel')}
            </button>
          </>
        }
      />

      <div className="mb-4 flex flex-wrap gap-2">
        {[
          { value: '', label: t('placement.all'), count: tunnels?.length ?? 0, icon: Waypoints },
          { value: 'local', label: t('placement.local'), count: localCount, icon: HardDrive },
          { value: 'remote', label: t('placement.remote'), count: remoteCount, icon: Laptop },
        ].map(({ value, label, count, icon: Icon }) => (
          <button
            key={value || 'all'}
            onClick={() => setFilter('location', value)}
            className={`inline-flex items-center gap-2 rounded-lg border px-3 py-1.5 text-xs transition-colors ${
              locationFilter === value
                ? 'border-accent-500/60 bg-accent-500/10 text-accent-300'
                : 'border-surface-600 text-slate-400 hover:text-slate-200'
            }`}
          >
            <Icon className="h-3.5 w-3.5" />
            {label}
            <span className="rounded bg-surface-800 px-1.5 py-0.5 font-mono text-[10px] text-slate-400">{count}</span>
          </button>
        ))}
      </div>

      {error && <ErrorBox message={error} />}

      {!tunnels ? (
        <div className="flex justify-center py-16 text-slate-400">
          <Spinner className="h-6 w-6" />
        </div>
      ) : tunnels.length === 0 ? (
        <div className="card">
          <EmptyState
            icon={<Waypoints className="h-8 w-8" />}
            title={t('empty')}
            description={t('empty_hint')}
            action={
              <button className="btn-primary" onClick={() => setModal(true)}>
                <Plus className="h-4 w-4" /> {t('new_tunnel')}
              </button>
            }
          />
        </div>
      ) : (
        <div className="card">
          <div className="mb-3 flex flex-wrap items-center gap-2">
            <input
              className="field !w-64 !py-1.5 text-xs"
              placeholder={t('filter.name')}
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
            tableId="tunnels"
            data={rows}
            columns={columns}
            keyField="id"
            emptyMessage={tc('no_match')}
            initialSort={{ field: 'name', direction: 'asc' }}
          />
        </div>
      )}

      <CreateTunnelModal
        open={modal}
        accounts={accounts.filter((a) => a.credential === 'api')}
        onClose={() => setModal(false)}
        onDone={() => {
          setModal(false);
          void load();
        }}
      />
    </div>
  );
}

function CreateTunnelModal({
  open,
  accounts,
  onClose,
  onDone,
}: {
  open: boolean;
  accounts: CFAccount[];
  onClose: () => void;
  onDone: () => void;
}) {
  const { t } = useTranslation('tunnels');
  const { t: tc } = useTranslation('common');
  const toast = useToast();
  const [accountId, setAccountId] = useState('');
  const [name, setName] = useState('');
  const [location, setLocation] = useState<'local' | 'remote'>('local');
  const [hostLabel, setHostLabel] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [fields, setFields] = useState<Record<string, string>>({});

  const submit = async () => {
    setBusy(true);
    setError('');
    setFields({});
    try {
      const tunnel = await tunnelsApi.create({
        account_id: accountId,
        name,
        location,
        host_label: hostLabel || undefined,
        connector_mode: location === 'local' ? 'docker' : 'external',
      });
      toast.push('success', t('create.created', { name: tunnel.name }));
      setName('');
      setHostLabel('');
      onDone();
    } catch (err) {
      setError(apiMessage(err));
      setFields(apiFields(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t('create.title')}
      description={t('create.description')}
      footer={
        <>
          <button className="btn-ghost" onClick={onClose}>
            {tc('cancel')}
          </button>
          <button className="btn-primary" disabled={busy || !accountId || !name} onClick={() => void submit()}>
            {busy && <Spinner className="h-4 w-4" />} {t('create.submit')}
          </button>
        </>
      }
    >
      {error && <ErrorBox message={error} />}
      {accounts.length === 0 && (
        <div className="rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-xs text-amber-200">
          {t('create.no_account')}
        </div>
      )}

      <Field label={t('columns.account')} error={fields.account_id}>
        <select className="field" value={accountId} onChange={(e) => setAccountId(e.target.value)}>
          <option value="">{t('select_account')}</option>
          {accounts.map((a) => (
            <option key={a.id} value={a.id}>
              {a.name}
            </option>
          ))}
        </select>
      </Field>

      <Field label={t('tunnel_name')} hint={t('tunnel_name_hint')} error={fields.name}>
        <input className="field" value={name} onChange={(e) => setName(e.target.value)} placeholder="my-app" />
      </Field>

      <div>
        <span className="label">{t('where_run')}</span>
        <div className="grid gap-2 sm:grid-cols-2">
          <button
            type="button"
            onClick={() => setLocation('local')}
            className={`rounded-lg border p-3 text-left transition-colors ${
              location === 'local'
                ? 'border-accent-500/60 bg-accent-500/10'
                : 'border-surface-600 hover:border-surface-500'
            }`}
          >
            <div className="flex items-center gap-2 text-sm font-medium text-slate-100">
              <HardDrive className="h-4 w-4" /> {t('create.this_server')}
            </div>
            <p className="mt-1 text-[11px] leading-relaxed text-slate-400">{t('create.this_server_hint')}</p>
          </button>
          <button
            type="button"
            onClick={() => setLocation('remote')}
            className={`rounded-lg border p-3 text-left transition-colors ${
              location === 'remote'
                ? 'border-accent-500/60 bg-accent-500/10'
                : 'border-surface-600 hover:border-surface-500'
            }`}
          >
            <div className="flex items-center gap-2 text-sm font-medium text-slate-100">
              <Laptop className="h-4 w-4" /> {t('create.other_device')}
            </div>
            <p className="mt-1 text-[11px] leading-relaxed text-slate-400">{t('create.other_device_hint')}</p>
          </button>
        </div>
      </div>

      {location === 'remote' && (
        <Field label={t('create.host_label')} hint={t('placement.remote_hint')} error={fields.host_label}>
          <input
            className="field"
            value={hostLabel}
            onChange={(e) => setHostLabel(e.target.value)}
            placeholder="vps-jakarta"
          />
        </Field>
      )}

      <div className="flex items-start gap-2 rounded-lg border border-surface-700 bg-surface-950/50 p-3 text-[11px] text-slate-400">
        <Cloud className="mt-0.5 h-3.5 w-3.5 shrink-0" />
        {location === 'local' ? t('after_create_start') : t('after_create_copy')}
      </div>
    </Modal>
  );
}
