import { useCallback, useEffect, useMemo, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { Cloud, HardDrive, Laptop, Play, Plus, RefreshCw, Square, Waypoints } from 'lucide-react';
import { apiFields, apiMessage } from '../api/client';
import { accountsApi, tunnelsApi } from '../api/endpoints';
import { Badge, EmptyState, ErrorBox, Field, Modal, PageHeader, Spinner } from '../components/ui';
import { useToast } from '../components/Toast';
import type { CFAccount, Tunnel } from '../types';

const statusTone = (status: Tunnel['status']) =>
  status === 'running' ? 'green' : status === 'error' ? 'red' : status === 'stopped' ? 'slate' : 'amber';

export default function TunnelsPage() {
  const toast = useToast();
  const [params, setParams] = useSearchParams();
  const accountFilter = params.get('account') ?? '';
  const locationFilter = params.get('location') ?? '';

  const [accounts, setAccounts] = useState<CFAccount[]>([]);
  const [tunnels, setTunnels] = useState<Tunnel[] | null>(null);
  const [error, setError] = useState('');
  const [modal, setModal] = useState(false);
  const [busyId, setBusyId] = useState('');

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
        toast.push('success', `${tunnel.name} stopped`);
      } else {
        await tunnelsApi.start(tunnel.id);
        toast.push('success', `${tunnel.name} started`);
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
      toast.push('error', 'Pick an account first');
      return;
    }
    try {
      const { result } = await tunnelsApi.sync(accountFilter);
      const parts = [
        `${result.tunnels} tunnel(s)`,
        result.tunnels_created ? `${result.tunnels_created} new` : '',
        `${result.apps_created} hostname(s) imported`,
        result.skipped?.length ? `${result.skipped.length} skipped` : '',
      ].filter(Boolean);
      toast.push(result.skipped?.length ? 'info' : 'success', parts.join(' · '));
      void load();
    } catch (err) {
      toast.push('error', apiMessage(err));
    }
  };

  const localCount = tunnels?.filter((t) => t.location === 'local').length ?? 0;
  const remoteCount = tunnels?.filter((t) => t.location === 'remote').length ?? 0;

  return (
    <div>
      <PageHeader
        title="Tunnels"
        description="Tunnels hosted by this panel are supervised here. Tunnels on other devices are configured through the Cloudflare API, not through Docker."
        actions={
          <>
            <select
              className="field !w-auto !py-1.5 text-xs"
              value={accountFilter}
              onChange={(e) => setFilter('account', e.target.value)}
            >
              <option value="">All accounts</option>
              {accounts.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name}
                </option>
              ))}
            </select>
            <button className="btn-ghost" onClick={() => void sync()} disabled={!accountFilter}>
              <RefreshCw className="h-4 w-4" /> Sync
            </button>
            <button className="btn-primary" onClick={() => setModal(true)}>
              <Plus className="h-4 w-4" /> New tunnel
            </button>
          </>
        }
      />

      <div className="mb-4 flex flex-wrap gap-2">
        {[
          { value: '', label: 'All placements', count: tunnels?.length ?? 0, icon: Waypoints },
          { value: 'local', label: 'This server', count: localCount, icon: HardDrive },
          { value: 'remote', label: 'Other devices', count: remoteCount, icon: Laptop },
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
            title="No tunnels here"
            description="Create a tunnel hosted by this panel, or sync an account to import tunnels that already run elsewhere."
            action={
              <button className="btn-primary" onClick={() => setModal(true)}>
                <Plus className="h-4 w-4" /> New tunnel
              </button>
            }
          />
        </div>
      ) : (
        <div className="card overflow-x-auto">
          <table className="w-full min-w-[900px]">
            <thead className="border-b border-surface-700/70 bg-surface-900/60">
              <tr>
                <th className="th">Tunnel</th>
                <th className="th">Account</th>
                <th className="th">Placement</th>
                <th className="th">Config</th>
                <th className="th">Status</th>
                <th className="th text-right">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-surface-800">
              {tunnels.map((tunnel) => (
                <tr key={tunnel.id} className="hover:bg-surface-900/40">
                  <td className="td">
                    <Link to={`/tunnels/${tunnel.id}`} className="font-medium text-slate-100 hover:text-accent-400">
                      {tunnel.name}
                    </Link>
                    <p className="font-mono text-[11px] text-slate-500">{tunnel.tunnel_id}</p>
                  </td>
                  <td className="td text-xs">{accountName(tunnel.cf_account_id)}</td>
                  <td className="td">
                    {tunnel.location === 'local' ? (
                      <Badge tone="blue" dot>
                        this server
                      </Badge>
                    ) : (
                      <Badge tone="violet" dot>
                        remote
                      </Badge>
                    )}
                    <p className="mt-1 max-w-[180px] truncate text-[11px] text-slate-500">
                      {tunnel.host_label || (tunnel.location === 'local' ? 'panel host' : 'unknown device')}
                    </p>
                  </td>
                  <td className="td">
                    <Badge tone={tunnel.config_src === 'cloudflare' ? 'green' : 'amber'}>{tunnel.config_src}</Badge>
                  </td>
                  <td className="td">
                    <Badge tone={statusTone(tunnel.status)} dot>
                      {tunnel.status}
                    </Badge>
                    {tunnel.last_seen_at && (
                      <p className="mt-1 text-[11px] text-slate-500">
                        seen {new Date(tunnel.last_seen_at).toLocaleTimeString()}
                      </p>
                    )}
                    {tunnel.status_message && (
                      <p className="mt-1 max-w-[220px] truncate text-[11px] text-rose-300/80">{tunnel.status_message}</p>
                    )}
                  </td>
                  <td className="td">
                    <div className="flex justify-end">
                      {tunnel.location === 'local' && tunnel.connector_mode === 'docker' ? (
                        <button
                          className="btn-ghost !px-2 !py-1 !text-xs"
                          disabled={busyId === tunnel.id}
                          onClick={() => void toggle(tunnel)}
                        >
                          {busyId === tunnel.id ? (
                            <Spinner className="h-3.5 w-3.5" />
                          ) : tunnel.status === 'running' ? (
                            <Square className="h-3.5 w-3.5" />
                          ) : (
                            <Play className="h-3.5 w-3.5" />
                          )}
                          {tunnel.status === 'running' ? 'Stop' : 'Start'}
                        </button>
                      ) : (
                        <Link to={`/tunnels/${tunnel.id}`} className="btn-ghost !px-2 !py-1 !text-xs">
                          Open
                        </Link>
                      )}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
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
      toast.push('success', `Tunnel ${tunnel.name} created`);
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
      title="Create tunnel"
      description="Created with remote configuration, so ingress and DNS are managed from here either way."
      footer={
        <>
          <button className="btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button className="btn-primary" disabled={busy || !accountId || !name} onClick={() => void submit()}>
            {busy && <Spinner className="h-4 w-4" />} Create
          </button>
        </>
      }
    >
      {error && <ErrorBox message={error} />}
      {accounts.length === 0 && (
        <div className="rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-xs text-amber-200">
          No API-token account available. Add one on the Accounts page, or import a tunnel token instead.
        </div>
      )}

      <Field label="Account" error={fields.account_id}>
        <select className="field" value={accountId} onChange={(e) => setAccountId(e.target.value)}>
          <option value="">Select an account</option>
          {accounts.map((a) => (
            <option key={a.id} value={a.id}>
              {a.name}
            </option>
          ))}
        </select>
      </Field>

      <Field label="Tunnel name" hint="Lowercase letters, digits and dashes." error={fields.name}>
        <input className="field" value={name} onChange={(e) => setName(e.target.value)} placeholder="my-app" />
      </Field>

      <div>
        <span className="label">Where will cloudflared run?</span>
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
              <HardDrive className="h-4 w-4" /> This server
            </div>
            <p className="mt-1 text-[11px] leading-relaxed text-slate-400">
              The panel runs one cloudflared container and supervises it: start, stop, logs, version.
            </p>
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
              <Laptop className="h-4 w-4" /> Another device
            </div>
            <p className="mt-1 text-[11px] leading-relaxed text-slate-400">
              You run cloudflared there with the token. The panel still manages ingress and DNS via the API.
            </p>
          </button>
        </div>
      </div>

      {location === 'remote' && (
        <Field
          label="Device label"
          hint="A name you will recognise, e.g. vps-jakarta or office-nas."
          error={fields.host_label}
        >
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
        {location === 'local'
          ? 'After creating, press Start on the tunnel row to launch the connector.'
          : 'After creating, open the tunnel and copy the token or config.yml to that device.'}
      </div>
    </Modal>
  );
}
