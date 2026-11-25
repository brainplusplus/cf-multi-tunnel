import { useCallback, useEffect, useState } from 'react';
import { ArrowUpCircle, CheckCircle2, Container, Globe2, RefreshCw, Trash2 } from 'lucide-react';
import { apiMessage } from '../api/client';
import { connectorApi, panelApi, tunnelsApi } from '../api/endpoints';
import { Badge, ErrorBox, Field, PageHeader, Spinner } from '../components/ui';
import { useToast } from '../components/Toast';
import type { ConnectorStatus, PublishedApp, RemoteTag, Tunnel } from '../types';

export default function SettingsPage() {
  return (
    <div className="space-y-8">
      <PageHeader title="Settings" description="Panel exposure and the connector runtime." />
      <PanelExposureCard />
      <ConnectorCard />
    </div>
  );
}

function PanelExposureCard() {
  const toast = useToast();
  const [tunnels, setTunnels] = useState<Tunnel[]>([]);
  const [exposure, setExposure] = useState<PublishedApp | null>(null);
  const [tunnelId, setTunnelId] = useState('');
  const [hostname, setHostname] = useState('');
  const [serviceUrl, setServiceUrl] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  const load = useCallback(async () => {
    try {
      const [tuns, exp] = await Promise.all([tunnelsApi.list(), panelApi.get()]);
      setTunnels(tuns);
      setExposure(exp);
      if (exp) {
        setTunnelId(exp.tunnel_id);
        setHostname(exp.hostname);
        setServiceUrl(exp.service_url);
      } else if (tuns.length > 0) {
        setTunnelId(tuns[0].id);
      }
    } catch (err) {
      setError(apiMessage(err));
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const publishable = tunnels.filter((t) => t.config_src === 'cloudflare');
  const selectedTunnel = tunnels.find((t) => t.id === tunnelId);
  const isRemote = selectedTunnel?.location === 'remote';

  const save = async () => {
    setBusy(true);
    setError('');
    try {
      const app = await panelApi.set({
        tunnel_id: tunnelId,
        hostname,
        ...(isRemote ? { service_url: serviceUrl } : {}),
      });
      toast.push(app.sync_status === 'synced' ? 'success' : 'info', app.last_error || 'Panel exposure saved');
      await load();
    } catch (err) {
      setError(apiMessage(err));
    } finally {
      setBusy(false);
    }
  };

  const remove = async () => {
    if (!window.confirm('Remove the public panel exposure? DNS and ingress are cleaned up.')) return;
    setBusy(true);
    try {
      await panelApi.remove();
      toast.push('success', 'Panel exposure removed');
      setExposure(null);
      setHostname('');
      await load();
    } catch (err) {
      setError(apiMessage(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="card p-5">
      <div className="mb-4 flex items-start gap-3">
        <div className="rounded-lg bg-violet-500/15 p-2 text-violet-300">
          <Globe2 className="h-5 w-5" />
        </div>
        <div>
          <h2 className="text-sm font-semibold text-white">Panel exposure</h2>
          <p className="mt-1 text-sm text-slate-400">
            Publish this dashboard through one of your tunnels so you can open it from a hostname instead of a port.
          </p>
        </div>
      </div>

      {error && <div className="mb-4"><ErrorBox message={error} /></div>}

      {exposure && (
        <div className="mb-4 flex flex-wrap items-center gap-2 rounded-lg border border-surface-700 bg-surface-950/50 px-3 py-2 text-xs">
          <CheckCircle2 className="h-4 w-4 text-emerald-400" />
          <span className="text-slate-300">{exposure.hostname}</span>
          <Badge tone={exposure.sync_status === 'synced' ? 'green' : 'amber'}>{exposure.sync_status}</Badge>
          <span className="font-mono text-slate-500">→ {exposure.service_url}</span>
          <button className="btn-danger ml-auto !px-2 !py-1 !text-xs" disabled={busy} onClick={() => void remove()}>
            <Trash2 className="h-3.5 w-3.5" /> Remove
          </button>
        </div>
      )}

      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="Tunnel" hint="Only tunnels with remote configuration can carry the panel.">
          <select className="field" value={tunnelId} onChange={(e) => setTunnelId(e.target.value)}>
            <option value="">Select a tunnel</option>
            {publishable.map((t) => (
              <option key={t.id} value={t.id}>
                {t.name} ({t.location === 'local' ? 'this server' : 'remote'})
              </option>
            ))}
          </select>
        </Field>
        <Field label="Hostname" hint="e.g. panel.example.com — the zone must be cached for that account.">
          <input className="field" value={hostname} onChange={(e) => setHostname(e.target.value)} placeholder="panel.example.com" />
        </Field>
      </div>

      {isRemote && (
        <div className="mt-3">
          <Field
            label="Panel address reachable from that device"
            hint="The remote connector proxies to this URL. Use the panel host's LAN/VPN address, not the hostname you are publishing."
          >
            <input
              className="field font-mono text-xs"
              value={serviceUrl}
              onChange={(e) => setServiceUrl(e.target.value)}
              placeholder="http://192.168.1.10:7777"
            />
          </Field>
        </div>
      )}

      <div className="mt-3 flex items-center justify-between gap-3">
        <p className="text-xs text-slate-500">
          {isRemote
            ? 'A remote connector cannot reach the panel over Docker; it needs a route to the address above.'
            : 'The local connector reaches the panel at PANEL_INTERNAL_URL on the shared Docker network.'}
        </p>
        <button
          className="btn-primary"
          disabled={busy || !tunnelId || !hostname || (isRemote && !serviceUrl)}
          onClick={() => void save()}
        >
          {busy && <Spinner className="h-4 w-4" />} {exposure ? 'Update exposure' : 'Publish panel'}
        </button>
      </div>
    </section>
  );
}

function ConnectorCard() {
  const toast = useToast();
  const [status, setStatus] = useState<ConnectorStatus | null>(null);
  const [tags, setTags] = useState<RemoteTag[]>([]);
  const [image, setImage] = useState('');
  const [pin, setPin] = useState(true);
  const [busy, setBusy] = useState('');
  const [error, setError] = useState('');

  const load = useCallback(async (checkRemote = true) => {
    setBusy('load');
    try {
      const s = await connectorApi.status(checkRemote);
      setStatus(s);
      setImage(s.image.image);
      setError('');
    } catch (err) {
      setError(apiMessage(err));
    } finally {
      setBusy('');
    }
  }, []);

  useEffect(() => {
    void load(true);
  }, [load]);

  const loadTags = async () => {
    setBusy('tags');
    try {
      setTags(await connectorApi.tags());
      toast.push('success', 'Fetched published tags from Docker Hub');
    } catch (err) {
      toast.push('error', apiMessage(err));
    } finally {
      setBusy('');
    }
  };

  const update = async () => {
    if (!window.confirm('Pull the image and recreate every managed connector? Tunnels reconnect automatically.')) return;
    setBusy('update');
    setError('');
    try {
      const result = await connectorApi.update(image, pin);
      // Defensive: an older backend returned null for empty lists.
      const recreated = result.recreated ?? [];
      const failed = result.failed ?? [];
      toast.push(
        failed.length === 0 ? 'success' : 'info',
        `Updated to ${result.image.version || image}. Recreated ${recreated.length} connector(s)` +
          (failed.length ? `, ${failed.length} failed` : ''),
      );
      await load(false);
    } catch (err) {
      setError(apiMessage(err));
    } finally {
      setBusy('');
    }
  };

  if (!status) {
    return (
      <section className="card flex justify-center p-10 text-slate-400">
        <Spinner className="h-6 w-6" />
      </section>
    );
  }

  const repo = status.image.image.includes(':')
    ? status.image.image.slice(0, status.image.image.lastIndexOf(':'))
    : status.image.image;

  return (
    <section className="card p-5">
      <div className="mb-4 flex items-start justify-between gap-3">
        <div className="flex items-start gap-3">
          <div className="rounded-lg bg-sky-500/15 p-2 text-sky-300">
            <Container className="h-5 w-5" />          </div>
          <div>
            <h2 className="text-sm font-semibold text-white">Connector runtime</h2>
            <p className="mt-1 text-sm text-slate-400">
              cloudflared runs as one container per tunnel. Updating the image recreates them without touching your data.
            </p>
          </div>
        </div>
        <button className="btn-ghost !px-2 !py-1.5 !text-xs" disabled={busy === 'load'} onClick={() => void load(true)}>
          {busy === 'load' ? <Spinner className="h-3.5 w-3.5" /> : <RefreshCw className="h-3.5 w-3.5" />} Check
        </button>
      </div>

      {error && <div className="mb-4"><ErrorBox message={error} /></div>}

      <div className="grid gap-3 sm:grid-cols-4">
        <Stat label="Mode" value={status.mode} />
        <Stat label="Docker" value={status.docker_available ? status.docker_version : 'unreachable'} tone={status.docker_available ? 'green' : 'red'} />
        <Stat label="Installed" value={status.image.version || 'unknown'} mono />
        <Stat
          label="Latest"
          value={status.latest_version || '—'}
          mono
          tone={status.update_available ? 'amber' : 'green'}
        />
      </div>

      <div className="mt-4 rounded-lg border border-surface-700 bg-surface-950/40 p-3">
        <p className="text-[11px] uppercase tracking-wider text-slate-500">Active image</p>
        <p className="mt-1 font-mono text-xs text-slate-300">{status.image.image || '—'}</p>
        {status.image.digest && <p className="mt-1 truncate font-mono text-[11px] text-slate-600">{status.image.digest}</p>}
        {status.image.error && <p className="mt-1 text-[11px] text-amber-300">{status.image.error}</p>}
      </div>

      <div className="mt-5 grid gap-3 sm:grid-cols-[1.6fr_auto]">
        <Field label="Target image" hint={`Any tag from ${repo}, e.g. ${repo}:latest or a pinned version.`}>
          <input className="field font-mono text-xs" value={image} onChange={(e) => setImage(e.target.value)} />
        </Field>
        <div className="flex items-end gap-2">
          <button className="btn-ghost" disabled={busy === 'tags'} onClick={() => void loadTags()}>
            {busy === 'tags' ? <Spinner className="h-4 w-4" /> : null} Browse tags
          </button>
          {status.update_available && (
            <button
              className="btn-ghost"
              onClick={() => setImage(`${repo}:${status.latest_version}`)}
              title="Use the latest published version"
            >
              <ArrowUpCircle className="h-4 w-4" /> Use {status.latest_version}
            </button>
          )}
        </div>
      </div>

      {tags.length > 0 && (
        <div className="mt-3 flex flex-wrap gap-1.5">
          {tags.slice(0, 18).map((tag) => (
            <button
              key={tag.name}
              onClick={() => setImage(`${repo}:${tag.name}`)}
              className={`rounded-md border px-2 py-1 font-mono text-[11px] transition-colors ${
                image.endsWith(`:${tag.name}`)
                  ? 'border-accent-500/60 bg-accent-500/10 text-accent-300'
                  : 'border-surface-600 text-slate-400 hover:border-surface-500 hover:text-slate-200'
              }`}
            >
              {tag.name}
              {tag.latest && <span className="ml-1 text-[9px] uppercase text-emerald-400">latest</span>}
            </button>
          ))}
        </div>
      )}

      <div className="mt-4 flex flex-wrap items-center justify-between gap-3">
        <label className="flex items-center gap-2 text-sm text-slate-300">
          <input type="checkbox" checked={pin} onChange={(e) => setPin(e.target.checked)} />
          Remember this image for future starts
        </label>
        <button className="btn-primary" disabled={busy === 'update' || !image} onClick={() => void update()}>
          {busy === 'update' ? <Spinner className="h-4 w-4" /> : <ArrowUpCircle className="h-4 w-4" />} Pull &amp; recreate
        </button>
      </div>

      {status.running_versions && status.running_versions.length > 0 && (
        <div className="mt-5 overflow-hidden rounded-lg border border-surface-700">
          <table className="w-full">
            <thead className="bg-surface-900/60">
              <tr>
                <th className="th">Tunnel</th>
                <th className="th">Running version</th>
                <th className="th">Image ref</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-surface-800">
              {status.running_versions.map((row) => (
                <tr key={row.tunnel_id}>
                  <td className="td">{row.name}</td>
                  <td className="td font-mono text-xs">{row.version || 'unknown'}</td>
                  <td className="td font-mono text-[11px] text-slate-500">{row.image_ref || '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

function Stat({
  label,
  value,
  tone = 'slate',
  mono = false,
}: {
  label: string;
  value: string;
  tone?: 'green' | 'red' | 'amber' | 'slate';
  mono?: boolean;
}) {
  const color =
    tone === 'green' ? 'text-emerald-300' : tone === 'red' ? 'text-rose-300' : tone === 'amber' ? 'text-amber-300' : 'text-slate-200';
  return (
    <div className="rounded-lg border border-surface-700 bg-surface-950/40 p-3">
      <p className="text-[11px] uppercase tracking-wider text-slate-500">{label}</p>
      <p className={`mt-1 text-sm ${color} ${mono ? 'font-mono' : ''}`}>{value}</p>
    </div>
  );
}
