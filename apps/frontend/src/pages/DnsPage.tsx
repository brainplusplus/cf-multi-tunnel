import { useCallback, useEffect, useMemo, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { Globe2, Lock, Pencil, Plus, RefreshCw, Trash2 } from 'lucide-react';
import { apiMessage } from '../api/client';
import { dnsApi } from '../api/endpoints';
import { DataTable, type Column } from '../components/DataTable';
import { DnsRecordModal } from '../components/DnsRecordModal';
import { FilterToggle, QueryBuilder, filterCount } from '../components/QueryBuilder';
import { createGroup, matchesQuery, type FieldDefinition, type QueryGroup } from '../components/query';
import { Badge, EmptyState, ErrorBox, PageHeader, Spinner } from '../components/ui';
import { useToast } from '../components/Toast';
import { useTranslation } from '../i18n/useTranslation';
import type { DNSRecord, DNSZone } from '../types';

// Filtering and sorting read these derived strings instead of booleans, so a
// select filter can offer "DNS only" and "Tunnel-managed" like the dashboard.
type Row = DNSRecord & { proxy_state: string; managed_state: string };

export default function DnsPage() {
  const { t } = useTranslation('dns');
  const { t: tc } = useTranslation('common');
  const toast = useToast();
  const [params, setParams] = useSearchParams();
  const zoneParam = params.get('zone') ?? '';

  const [zones, setZones] = useState<DNSZone[] | null>(null);
  const [accountFilter, setAccountFilter] = useState('');
  const [zoneId, setZoneId] = useState(zoneParam);
  const [records, setRecords] = useState<DNSRecord[] | null>(null);
  const [truncated, setTruncated] = useState(false);
  const [error, setError] = useState('');
  const [syncing, setSyncing] = useState(false);
  const [loadingRecords, setLoadingRecords] = useState(false);
  const [modal, setModal] = useState(false);
  const [editing, setEditing] = useState<DNSRecord | null>(null);
  const [search, setSearch] = useState('');
  const [showFilters, setShowFilters] = useState(false);
  const [filters, setFilters] = useState<QueryGroup>(() => createGroup());

  const loadZones = useCallback(async () => {
    try {
      const list = await dnsApi.zones();
      setZones(list);
      setError('');
    } catch (err) {
      setError(apiMessage(err));
    }
  }, []);

  useEffect(() => {
    void loadZones();
  }, [loadZones]);

  // Keep the selection valid as the account filter or the zone list changes.
  // This is why the page never sits on a selector whose value no longer exists.
  useEffect(() => {
    if (!zones) return;
    const visible = zones.filter((z) => !accountFilter || z.cf_account_id === accountFilter);
    if (visible.length === 0) return;
    if (zoneId && visible.some((z) => z.id === zoneId)) return;
    setZoneId(visible[0].id);
  }, [zones, accountFilter, zoneId]);

  // The zone lives in the URL so a refresh or a pasted link reopens the same one.
  useEffect(() => {
    if (!zoneId || params.get('zone') === zoneId) return;
    const next = new URLSearchParams(params);
    next.set('zone', zoneId);
    setParams(next);
  }, [zoneId, params, setParams]);

  const loadRecords = useCallback(async (id: string) => {
    if (!id) {
      setRecords([]);
      setTruncated(false);
      return;
    }
    setLoadingRecords(true);
    try {
      const res = await dnsApi.records(id);
      setRecords(res.items);
      setTruncated(res.truncated);
      setError('');
    } catch (err) {
      setError(apiMessage(err));
      setRecords([]);
    } finally {
      setLoadingRecords(false);
    }
  }, []);

  useEffect(() => {
    void loadRecords(zoneId);
  }, [zoneId, loadRecords]);

  const pickZone = (id: string) => setZoneId(id);

  const sync = async () => {
    setSyncing(true);
    try {
      const result = await dnsApi.syncZones();
      toast.push(
        result.failed.length ? 'info' : 'success',
        result.failed.length
          ? t('sync_partial', { first: result.failed[0] })
          : t('sync_done', { zones: result.zones, accounts: result.accounts }),
      );
      await loadZones();
      await loadRecords(zoneId);
    } catch (err) {
      toast.push('error', apiMessage(err));
    } finally {
      setSyncing(false);
    }
  };

  const remove = async (record: DNSRecord) => {
    if (!window.confirm(t('delete_confirm', { type: record.type, name: record.name }))) return;
    try {
      await dnsApi.deleteRecord(zoneId, record.id);
      toast.push('success', t('deleted'));
      await loadRecords(zoneId);
    } catch (err) {
      toast.push('error', apiMessage(err));
    }
  };

  const visibleZones = useMemo(
    () => (zones ?? []).filter((z) => !accountFilter || z.cf_account_id === accountFilter),
    [zones, accountFilter],
  );
  const zone = useMemo(() => (zones ?? []).find((z) => z.id === zoneId), [zones, zoneId]);
  const accounts = useMemo(() => {
    const map = new Map<string, string>();
    for (const z of zones ?? []) map.set(z.cf_account_id, z.account_name);
    return [...map.entries()].map(([id, name]) => ({ id, name }));
  }, [zones]);

  const fields: FieldDefinition[] = useMemo(
    () => [
      { name: 'name', label: t('columns.name'), type: 'string' },
      {
        name: 'type',
        label: t('columns.type'),
        type: 'select',
        options: ['A', 'AAAA', 'CNAME', 'Tunnel', 'TXT', 'MX', 'NS', 'SRV', 'CAA'].map((v) => ({ value: v, label: v })),
      },
      { name: 'content', label: t('columns.content'), type: 'string' },
      {
        name: 'proxy_state',
        label: t('columns.proxy'),
        type: 'select',
        options: [
          { value: t('proxy.proxied'), label: t('proxy.proxied') },
          { value: t('proxy.dns_only'), label: t('proxy.dns_only') },
        ],
      },
      {
        name: 'managed_state',
        label: t('managed_label'),
        type: 'select',
        options: [
          { value: t('managed_label'), label: t('managed_label') },
          { value: '—', label: '—' },
        ],
      },
    ],
    [t],
  );

  const rows: Row[] = useMemo(() => {
    const term = search.trim().toLowerCase();
    return (records ?? [])
      .map((rec) => ({
        ...rec,
        proxy_state: rec.proxiable ? (rec.proxied ? t('proxy.proxied') : t('proxy.dns_only')) : '—',
        managed_state: rec.managed ? t('managed_label') : '—',
      }))
      .filter((rec) => {
        if (term && !`${rec.name} ${rec.content} ${rec.display_type}`.toLowerCase().includes(term)) return false;
        return matchesQuery(rec as unknown as Record<string, unknown>, filters);
      });
  }, [records, search, filters, t]);

  const columns: Column<Row>[] = [
    {
      key: 'name',
      label: t('columns.name'),
      sortable: true,
      minWidth: 180,
      render: (rec) => (
        <div>
          <span className="font-mono text-xs text-slate-200">{rec.name}</span>
          {rec.managed && (
            <p className="mt-1 inline-flex items-center gap-1 text-[11px] text-accent-300">
              <Lock className="h-3 w-3" /> {rec.hostname}
            </p>
          )}
        </div>
      ),
    },
    {
      key: 'display_type',
      label: t('columns.type'),
      sortable: true,
      width: 110,
      render: (rec) =>
        rec.display_type === 'Tunnel' ? (
          <Badge tone="violet">Tunnel</Badge>
        ) : (
          <span className="font-mono text-xs">{rec.display_type}</span>
        ),
    },
    {
      key: 'content',
      label: t('columns.content'),
      sortable: true,
      minWidth: 200,
      render: (rec) => {
        // A tunnel row shows the name the operator recognises; the row itself
        // is still the CNAME, so the raw target stays visible underneath.
        if (rec.display_type === 'Tunnel') {
          return (
            <div>
              {rec.tunnel_id ? (
                <Link to="/tunnels" className="text-xs text-accent-400 hover:text-accent-300">
                  {rec.tunnel_name || rec.tunnel_id}
                </Link>
              ) : (
                <span className="text-xs text-slate-300">{rec.tunnel_id}</span>
              )}
              <p className="mt-0.5 font-mono text-[10px] text-slate-500">{rec.content}</p>
            </div>
          );
        }
        const content = rec.content || rec.data?.target || '';
        return <span className="break-all font-mono text-[11px] text-slate-300">{content}</span>;
      },
    },
    {
      key: 'proxy_state',
      label: t('columns.proxy'),
      width: 110,
      render: (rec) =>
        rec.proxiable ? (
          <Badge tone={rec.proxied ? 'amber' : 'slate'} dot>
            {rec.proxied ? t('proxy.proxied') : t('proxy.dns_only')}
          </Badge>
        ) : (
          <span className="text-xs text-slate-600">—</span>
        ),
    },
    {
      key: 'ttl',
      label: t('columns.ttl'),
      sortable: true,
      width: 100,
      render: (rec) => (
        <span className="text-xs text-slate-400">{rec.ttl === 1 ? t('ttl.auto') : t('ttl.seconds', { count: rec.ttl })}</span>
      ),
    },
    {
      key: 'comment',
      label: t('columns.comment'),
      hiddenByDefault: true,
      render: (rec) => <span className="text-xs text-slate-400">{rec.comment || '—'}</span>,
    },
    {
      key: 'actions',
      label: t('columns.actions'),
      width: 160,
      render: (rec) =>
        rec.managed ? (
          <span className="inline-flex items-center gap-1 text-[11px] text-slate-500" title={t('managed_hint')}>
            <Lock className="h-3.5 w-3.5" /> {t('managed_label')}
          </span>
        ) : (
          <div className="flex items-center justify-end gap-1">
            <button
              className="btn-ghost !px-2 !py-1 !text-xs"
              onClick={() => {
                setEditing(rec);
                setModal(true);
              }}
            >
              <Pencil className="h-3.5 w-3.5" /> {tc('edit')}
            </button>
            <button className="btn-ghost !px-2 !py-1 !text-xs !text-rose-300" onClick={() => void remove(rec)}>
              <Trash2 className="h-3.5 w-3.5" /> {tc('delete')}
            </button>
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
              onChange={(e) => setAccountFilter(e.target.value)}
            >
              <option value="">{t('all_accounts')}</option>
              {accounts.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name}
                </option>
              ))}
            </select>
            <select
              className="field !w-auto !py-1.5 text-xs"
              value={zoneId}
              onChange={(e) => pickZone(e.target.value)}
            >
              {visibleZones.length === 0 && <option value="">{t('pick_zone')}</option>}
              {visibleZones.map((z) => (
                <option key={z.id} value={z.id}>
                  {z.name}
                </option>
              ))}
            </select>
            <button className="btn-ghost" onClick={() => void sync()} disabled={syncing}>
              {syncing ? <Spinner className="h-4 w-4" /> : <RefreshCw className="h-4 w-4" />} {t('sync')}
            </button>
            <button className="btn-primary" onClick={() => { setEditing(null); setModal(true); }} disabled={!zoneId}>
              <Plus className="h-4 w-4" /> {t('add_record')}
            </button>
          </>
        }
      />

      {error && <div className="mb-4"><ErrorBox message={error} /></div>}

      {!zones ? (
        <div className="flex justify-center py-16 text-slate-400">
          <Spinner className="h-6 w-6" />
        </div>
      ) : zones.length === 0 ? (
        <div className="card">
          <EmptyState
            icon={<Globe2 className="h-8 w-8" />}
            title={t('empty_zones_title')}
            description={t('empty_zones_desc')}
            action={
              <button className="btn-primary" onClick={() => void sync()} disabled={syncing}>
                <RefreshCw className="h-4 w-4" /> {t('sync_now')}
              </button>
            }
          />
        </div>
      ) : !zone ? (
        <div className="card">
          <EmptyState icon={<Globe2 className="h-8 w-8" />} title={t('no_zone')} description={t('empty_zones_desc')} />
        </div>
      ) : (
        <div className="card">
          <div className="mb-3 flex flex-wrap items-center gap-2">
            <input
              className="field !w-64 !py-1.5 text-xs"
              placeholder={t('search')}
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
            <FilterToggle
              open={showFilters}
              count={filterCount(filters)}
              onClick={() => setShowFilters((v) => !v)}
            />
            <span className="text-[11px] text-slate-500">{tc('rows_found', { count: rows.length })}</span>
            {truncated && (
              <span className="text-[11px] text-amber-300">
                {t('truncated', { shown: records?.length ?? 0 })}
              </span>
            )}
          </div>

          {showFilters && (
            <div className="mb-3">
              <QueryBuilder fields={fields} value={filters} onChange={setFilters} />
            </div>
          )}

          {loadingRecords ? (
            <div className="flex justify-center py-12 text-slate-400">
              <Spinner className="h-5 w-5" />
            </div>
          ) : (
            <DataTable
              tableId="dns"
              data={rows}
              columns={columns}
              keyField="id"
              emptyMessage={records && records.length === 0 ? t('empty_records') : tc('no_match')}
              initialSort={{ field: 'name', direction: 'asc' }}
            />
          )}

          <div className="mt-4 rounded-lg border border-surface-700 bg-surface-950/40 px-3 py-2">
            <p className="text-[11px] font-medium text-slate-300">{t('guide.title')}</p>
            <ul className="mt-1 space-y-1 text-[11px] leading-relaxed text-slate-500">
              <li>{t('guide.tunnel')}</li>
              <li>{t('guide.locked')}</li>
              <li>{t('guide.permissions')}</li>
              <li>{t('guide.ttl')}</li>
            </ul>
          </div>
        </div>
      )}

      {zone && (
        <DnsRecordModal
          open={modal}
          zoneRowId={zone.id}
          zoneName={zone.name}
          editing={editing}
          onClose={() => setModal(false)}
          onSaved={() => {
            setModal(false);
            void loadRecords(zone.id);
          }}
        />
      )}
    </div>
  );
}
