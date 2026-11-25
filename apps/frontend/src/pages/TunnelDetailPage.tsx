import { useCallback, useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import {
  ArrowLeft,
  FileCode2,
  Globe2,
  HardDrive,
  KeyRound,
  Laptop,
  Pencil,
  Play,
  Plus,
  Square,
  Terminal,
  Trash2,
} from 'lucide-react';
import { apiFields, apiMessage } from '../api/client';
import { accountsApi, appsApi, tunnelsApi, type AppPayload } from '../api/endpoints';
import { Badge, CopyButton, EmptyState, ErrorBox, Field, Modal, PageHeader, Spinner } from '../components/ui';
import { useToast } from '../components/Toast';
import type { CFAccount, CFZone, PublishedApp, Tunnel } from '../types';

export default function TunnelDetailPage() {
  const { id = '' } = useParams();
  const toast = useToast();
  const navigate = useNavigate();
  const [tunnel, setTunnel] = useState<Tunnel | null>(null);
  const [account, setAccount] = useState<CFAccount | null>(null);
  const [zones, setZones] = useState<CFZone[]>([]);
  const [apps, setApps] = useState<PublishedApp[]>([]);
  const [logs, setLogs] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState('');
  const [showLogs, setShowLogs] = useState(false);
  const [showConfig, setShowConfig] = useState(false);
  const [showToken, setShowToken] = useState(false);
  const [showAppModal, setShowAppModal] = useState(false);
  const [showPlacement, setShowPlacement] = useState(false);
  const [configYml, setConfigYml] = useState('');
  const [token, setToken] = useState('');

  const load = useCallback(async () => {
    try {
      const t = await tunnelsApi.get(id);
      setTunnel(t);
      const [acc, appList] = await Promise.all([
        accountsApi.get(t.cf_account_id).catch(() => null),
        tunnelsApi.apps(id),
      ]);
      setAccount(acc);
      setApps(appList);
      if (acc) setZones(await accountsApi.zones(acc.id).catch(() => []));
      setError('');
    } catch (err) {
      setError(apiMessage(err));
    }
  }, [id]);

  useEffect(() => {
    void load();
  }, [load]);

  const startStop = async () => {
    if (!tunnel) return;
    setBusy('lifecycle');
    try {
      if (tunnel.status === 'running') await tunnelsApi.stop(tunnel.id);
      else await tunnelsApi.start(tunnel.id);
      await load();
    } catch (err) {
      toast.push('error', apiMessage(err));
    } finally {
      setBusy('');
    }
  };

  const openLogs = async () => {
    setShowLogs(true);
    setBusy('logs');
    try {
      setLogs(await tunnelsApi.logs(tunnel!.id, 200));
    } catch (err) {
      setLogs(apiMessage(err));
    } finally {
      setBusy('');
    }
  };

  const openConfig = async () => {
    setShowConfig(true);
    try {
      setConfigYml(await tunnelsApi.configYml(id));
    } catch (err) {
      toast.push('error', apiMessage(err));
    }
  };

  const openToken = async () => {
    setShowToken(true);
    try {
      setToken(await tunnelsApi.token(id));
    } catch (err) {
      toast.push('error', apiMessage(err));
    }
  };

  const remove = async () => {
    if (!tunnel) return;
    if (!window.confirm(`Delete tunnel ${tunnel.name}? It is also removed from Cloudflare.`)) return;
    try {
      await tunnelsApi.remove(tunnel.id);
      toast.push('success', 'Tunnel deleted');
      navigate('/tunnels');
    } catch (err) {
      toast.push('error', apiMessage(err));
    }
  };

  const removeApp = async (app: PublishedApp) => {
    if (!window.confirm(`Delete ${app.hostname}?`)) return;
    try {
      await appsApi.remove(app.id);
      toast.push('success', 'Published app removed');
      void load();
    } catch (err) {
      toast.push('error', apiMessage(err));
    }
  };

  if (error) return <ErrorBox message={error} />;
  if (!tunnel) {
    return (
      <div className="flex justify-center py-20 text-slate-400">
        <Spinner className="h-6 w-6" />
      </div>
    );
  }

  const isLocal = tunnel.location === 'local';
  const managed = isLocal && tunnel.connector_mode === 'docker';

  return (
    <div>
      <Link to="/tunnels" className="mb-4 inline-flex items-center gap-1.5 text-sm text-slate-400 hover:text-slate-200">
        <ArrowLeft className="h-4 w-4" /> Tunnels
      </Link>

      <PageHeader
        title={tunnel.name}
        description={`${account?.name ?? 'account'} · ${tunnel.tunnel_id}`}
        actions={
          <>
            <button className="btn-ghost" onClick={() => void openConfig()}>
              <FileCode2 className="h-4 w-4" /> config.yml
            </button>
            <button className="btn-ghost" onClick={() => void openToken()}>
              <KeyRound className="h-4 w-4" /> Token
            </button>
            {managed && (
              <button className="btn-ghost" onClick={() => void openLogs()}>
                <Terminal className="h-4 w-4" /> Logs
              </button>
            )}
            {managed && (
              <button className="btn-ghost" disabled={busy === 'lifecycle'} onClick={() => void startStop()}>
                {busy === 'lifecycle' ? (
                  <Spinner className="h-4 w-4" />
                ) : tunnel.status === 'running' ? (
                  <Square className="h-4 w-4" />
                ) : (
                  <Play className="h-4 w-4" />
                )}
                {tunnel.status === 'running' ? 'Stop' : 'Start'}
              </button>
            )}
            <button className="btn-primary" onClick={() => setShowAppModal(true)}>
              <Plus className="h-4 w-4" /> Publish app
            </button>
            <button className="btn-danger" onClick={() => void remove()}>
              <Trash2 className="h-4 w-4" />
            </button>
          </>
        }
      />

      <div className="mb-6 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <InfoTile label="Placement">
          <div className="flex items-center gap-2">
            <Badge tone={isLocal ? 'blue' : 'violet'} dot>
              {isLocal ? 'this server' : 'remote device'}
            </Badge>
            <button onClick={() => setShowPlacement(true)} className="text-slate-500 hover:text-slate-300" title="Change placement">
              <Pencil className="h-3 w-3" />
            </button>
          </div>
          <p className="mt-1 truncate text-[11px] text-slate-500">{tunnel.host_label || '—'}</p>
        </InfoTile>
        <InfoTile label="Status">
          <Badge tone={tunnel.status === 'running' ? 'green' : tunnel.status === 'error' ? 'red' : 'slate'} dot>
            {tunnel.status}
          </Badge>
          {tunnel.last_seen_at && (
            <p className="mt-1 text-[11px] text-slate-500">seen {new Date(tunnel.last_seen_at).toLocaleString()}</p>
          )}
        </InfoTile>
        <InfoTile label="Config source">
          <Badge tone={tunnel.config_src === 'cloudflare' ? 'green' : 'amber'}>{tunnel.config_src}</Badge>
        </InfoTile>
        <InfoTile label="cloudflared">
          <span className="font-mono text-sm text-slate-200">{managed ? tunnel.image_version || '—' : 'n/a here'}</span>
        </InfoTile>
      </div>

      {!isLocal && (
        <div className="mb-4 flex items-start gap-2 rounded-lg border border-violet-500/30 bg-violet-500/10 px-4 py-3 text-xs text-violet-200">
          <Laptop className="mt-0.5 h-4 w-4 shrink-0" />
          <div>
            This tunnel runs on <span className="font-medium">{tunnel.host_label || 'another device'}</span>. The panel manages its
            ingress and DNS through the Cloudflare API but cannot start, stop or read logs of that connector — use the Token
            button to run cloudflared there.
          </div>
        </div>
      )}

      {tunnel.status_message && (
        <div className="mb-4 rounded-lg border border-rose-500/30 bg-rose-950/30 px-4 py-3 text-xs text-rose-200">
          {tunnel.status_message}
        </div>
      )}

      <div className="card overflow-hidden">
        <div className="flex items-center justify-between border-b border-surface-700/70 px-4 py-3">
          <h2 className="text-sm font-semibold text-white">Published apps</h2>
          <span className="text-xs text-slate-500">{apps.length} hostname(s)</span>
        </div>
        {apps.length === 0 ? (
          <EmptyState
            icon={<Globe2 className="h-7 w-7" />}
            title="Nothing published yet"
            description="Add a hostname and the panel writes the ingress rule and the DNS CNAME for you."
            action={
              <button className="btn-primary" onClick={() => setShowAppModal(true)}>
                <Plus className="h-4 w-4" /> Publish app
              </button>
            }
          />
        ) : (
          <table className="w-full">
            <thead className="border-b border-surface-700/70 bg-surface-900/60">
              <tr>
                <th className="th">Hostname</th>
                <th className="th">Service</th>
                <th className="th">DNS</th>
                <th className="th">Sync</th>
                <th className="th text-right">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-surface-800">
              {apps.map((app) => (
                <tr key={app.id} className="hover:bg-surface-900/40">
                  <td className="td">
                    <a
                      href={`https://${app.hostname}`}
                      target="_blank"
                      rel="noreferrer"
                      className="font-medium text-slate-100 hover:text-accent-400"
                    >
                      {app.hostname}
                    </a>
                    {app.kind === 'panel' && (
                      <span className="ml-2 align-middle">
                        <Badge tone="violet">panel</Badge>
                      </span>
                    )}
                    <p className="font-mono text-[11px] text-slate-500">{app.path}</p>
                  </td>
                  <td className="td font-mono text-xs">{app.service_url}</td>
                  <td className="td">
                    {app.manage_dns ? (
                      <Badge tone={app.dns_record_id ? 'green' : 'amber'}>{app.dns_record_id ? 'managed' : 'pending'}</Badge>
                    ) : (
                      <Badge tone="slate">manual</Badge>
                    )}
                  </td>
                  <td className="td">
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
                    {app.last_error && <p className="mt-1 max-w-xs text-[11px] text-rose-300/80">{app.last_error}</p>}
                  </td>
                  <td className="td">
                    <div className="flex justify-end gap-1.5">
                      {app.kind === 'panel' ? (
                        <Link to="/settings" className="text-[11px] text-slate-500 hover:text-accent-400">
                          managed in Settings
                        </Link>
                      ) : (
                        <button className="btn-ghost !px-2 !py-1 !text-xs" onClick={() => void removeApp(app)}>
                          <Trash2 className="h-3.5 w-3.5" />
                        </button>
                      )}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      <Modal open={showLogs} onClose={() => setShowLogs(false)} title="Connector logs" wide>
        {busy === 'logs' ? (
          <div className="flex justify-center py-10 text-slate-400">
            <Spinner className="h-6 w-6" />
          </div>
        ) : (
          <div className="relative">
            <div className="absolute right-2 top-2">
              <CopyButton value={logs} />
            </div>
            <pre className="max-h-[50vh] overflow-auto rounded-lg bg-black/60 p-3 font-mono text-[11px] leading-relaxed text-slate-300">
              {logs || 'No output'}
            </pre>
          </div>
        )}
      </Modal>

      <Modal
        open={showConfig}
        onClose={() => setShowConfig(false)}
        title="cloudflared config.yml"
        description="What to place on the connector host when it runs outside this panel."
        wide
      >
        <div className="relative">
          <div className="absolute right-2 top-2">
            <CopyButton value={configYml} />
          </div>
          <pre className="max-h-[50vh] overflow-auto rounded-lg bg-black/60 p-3 font-mono text-[11px] leading-relaxed text-slate-300">
            {configYml}
          </pre>
        </div>
      </Modal>

      <Modal
        open={showToken}
        onClose={() => setShowToken(false)}
        title="Tunnel token"
        description="Run this on the connector host: cloudflared tunnel --no-autoupdate run --token <token>"
        wide
      >
        <div className="relative">
          <div className="absolute right-2 top-2">
            <CopyButton value={token} />
          </div>
          <pre className="max-h-40 overflow-auto rounded-lg bg-black/60 p-3 font-mono text-[11px] leading-relaxed text-slate-300">
            {token || 'loading...'}
          </pre>
        </div>
        <p className="text-xs text-slate-500">
          Anyone holding this token can connect a connector to the tunnel. It is shown only to the tunnel owner and admins.
        </p>
      </Modal>

      <PlacementModal
        open={showPlacement}
        tunnel={tunnel}
        onClose={() => setShowPlacement(false)}
        onDone={() => {
          setShowPlacement(false);
          void load();
        }}
      />

      <PublishAppModal
        open={showAppModal}
        tunnel={tunnel}
        zones={zones}
        onClose={() => setShowAppModal(false)}
        onDone={() => {
          setShowAppModal(false);
          void load();
        }}
      />
    </div>
  );
}

function InfoTile({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="card p-3">
      <p className="text-[11px] uppercase tracking-wider text-slate-500">{label}</p>
      <div className="mt-1.5">{children}</div>
    </div>
  );
}

function PlacementModal({
  open,
  tunnel,
  onClose,
  onDone,
}: {
  open: boolean;
  tunnel: Tunnel;
  onClose: () => void;
  onDone: () => void;
}) {
  const toast = useToast();
  const [location, setLocation] = useState<'local' | 'remote'>(tunnel.location);
  const [hostLabel, setHostLabel] = useState(tunnel.host_label);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!open) return;
    setLocation(tunnel.location);
    setHostLabel(tunnel.host_label);
    setError('');
  }, [open, tunnel]);

  const submit = async () => {
    setBusy(true);
    setError('');
    try {
      await tunnelsApi.update(tunnel.id, {
        location,
        host_label: hostLabel,
        connector_mode: location === 'local' ? 'docker' : 'external',
      });
      toast.push('success', 'Placement updated');
      onDone();
    } catch (err) {
      setError(apiMessage(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Tunnel placement"
      description="Tells the panel whether it may control the connector on this host."
      footer={
        <>
          <button className="btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button className="btn-primary" disabled={busy} onClick={() => void submit()}>
            {busy && <Spinner className="h-4 w-4" />} Save
          </button>
        </>
      }
    >
      {error && <ErrorBox message={error} />}
      <div className="grid gap-2 sm:grid-cols-2">
        <button
          type="button"
          onClick={() => setLocation('local')}
          className={`rounded-lg border p-3 text-left transition-colors ${
            location === 'local' ? 'border-accent-500/60 bg-accent-500/10' : 'border-surface-600 hover:border-surface-500'
          }`}
        >
          <div className="flex items-center gap-2 text-sm font-medium text-slate-100">
            <HardDrive className="h-4 w-4" /> This server
          </div>
          <p className="mt-1 text-[11px] text-slate-400">Panel runs the container and can start/stop it.</p>
        </button>
        <button
          type="button"
          onClick={() => setLocation('remote')}
          className={`rounded-lg border p-3 text-left transition-colors ${
            location === 'remote' ? 'border-accent-500/60 bg-accent-500/10' : 'border-surface-600 hover:border-surface-500'
          }`}
        >
          <div className="flex items-center gap-2 text-sm font-medium text-slate-100">
            <Laptop className="h-4 w-4" /> Another device
          </div>
          <p className="mt-1 text-[11px] text-slate-400">Panel only manages ingress and DNS through the API.</p>
        </button>
      </div>

      <Field label="Device label" hint="Shown in the tunnel list so you can tell devices apart.">
        <input className="field" value={hostLabel} onChange={(e) => setHostLabel(e.target.value)} />
      </Field>

      {location === 'remote' && tunnel.desired_running && (
        <div className="rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-xs text-amber-200">
          Saving stops and removes the container currently running on this host. Make sure cloudflared is running on the new
          device before the old connector disappears, or the tunnel will be offline.
        </div>
      )}
    </Modal>
  );
}

function PublishAppModal({
  open,
  tunnel,
  zones,
  onClose,
  onDone,
}: {
  open: boolean;
  tunnel: Tunnel;
  zones: CFZone[];
  onClose: () => void;
  onDone: () => void;
}) {
  const toast = useToast();
  const [subdomain, setSubdomain] = useState('');
  const [domain, setDomain] = useState(zones[0]?.name ?? '');
  // A connector runs in its own container, so "localhost" is the container, not
  // the host. host.docker.internal is injected by the panel (--add-host
  // host-gateway) to reach services on the Docker host.
  const [serviceUrl, setServiceUrl] = useState(
    tunnel.location === 'local' ? 'http://host.docker.internal:8080' : 'http://localhost:8080',
  );
  const [path, setPath] = useState('/*');
  const [manageDns, setManageDns] = useState(tunnel.config_src === 'cloudflare');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [fields, setFields] = useState<Record<string, string>>({});

  useEffect(() => {
    if (zones.length > 0 && !domain) setDomain(zones[0].name);
  }, [zones, domain]);

  const submit = async () => {
    setBusy(true);
    setError('');
    setFields({});
    try {
      const payload: AppPayload = {
        subdomain,
        domain,
        service_url: serviceUrl,
        path,
        manage_dns: manageDns,
        proxied: true,
      };
      const app = await tunnelsApi.createApp(tunnel.id, payload);
      toast.push(
        app.sync_status === 'synced' ? 'success' : 'info',
        app.sync_status === 'synced' ? `${app.hostname} is live` : `${app.hostname} saved: ${app.last_error}`,
      );
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
      title="Publish an app"
      description="Pick a hostname and the origin the tunnel should reach."
      footer={
        <>
          <button className="btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button className="btn-primary" disabled={busy || !subdomain || !domain} onClick={() => void submit()}>
            {busy && <Spinner className="h-4 w-4" />} Publish
          </button>
        </>
      }
    >
      {error && <ErrorBox message={error} />}
      <div className="grid grid-cols-[1fr_1.4fr] gap-3">
        <Field label="Subdomain" error={fields.hostname}>
          <input className="field" value={subdomain} onChange={(e) => setSubdomain(e.target.value)} placeholder="app" />
        </Field>
        <Field label="Domain" hint={zones.length === 0 ? 'No zone cached; sync the account first.' : undefined}>
          {zones.length > 0 ? (
            <select className="field" value={domain} onChange={(e) => setDomain(e.target.value)}>
              {zones.map((z) => (
                <option key={z.id} value={z.name}>
                  {z.name}
                </option>
              ))}
            </select>
          ) : (
            <input className="field" value={domain} onChange={(e) => setDomain(e.target.value)} placeholder="example.com" />
          )}
        </Field>
      </div>
      <Field
        label="Service URL"
        hint={
          tunnel.location === 'local'
            ? 'Reachable from the connector container. host.docker.internal = this Docker host; a container on the panel network is reachable by its name.'
            : 'Reachable from the device running the connector, not from this panel.'
        }
        error={fields.service_url}
      >
        <input className="field font-mono text-xs" value={serviceUrl} onChange={(e) => setServiceUrl(e.target.value)} />
      </Field>
      <Field label="Path" hint="Match only a sub-path, e.g. /api/*">
        <input className="field font-mono text-xs" value={path} onChange={(e) => setPath(e.target.value)} />
      </Field>
      <label className="flex items-center gap-2 text-sm text-slate-300">
        <input
          type="checkbox"
          checked={manageDns}
          disabled={tunnel.config_src !== 'cloudflare'}
          onChange={(e) => setManageDns(e.target.checked)}
        />
        Create the DNS CNAME automatically
      </label>
      {tunnel.config_src !== 'cloudflare' && (
        <div className="rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-xs text-amber-200">
          This tunnel has no remote configuration. The app is saved and written into config.yml, but you must create the
          DNS record yourself.
        </div>
      )}
    </Modal>
  );
}
