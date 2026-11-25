import { useCallback, useEffect, useState } from 'react';
import { ArrowUpCircle, CheckCircle2, Container, Globe2, RefreshCw, Trash2 } from 'lucide-react';
import { apiMessage } from '../api/client';
import { accountsApi, connectorApi, panelApi, tunnelsApi } from '../api/endpoints';
import { Badge, ErrorBox, Field, PageHeader, Spinner } from '../components/ui';
import { useToast } from '../components/Toast';
import type { CFAccount, CFZone, ConnectorStatus, PanelAddress, PublishedApp, RemoteTag, Tunnel } from '../types';

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
  const [accounts, setAccounts] = useState<CFAccount[]>([]);
  const [tunnels, setTunnels] = useState<Tunnel[]>([]);
  const [zones, setZones] = useState<CFZone[]>([]);
  const [address, setAddress] = useState<PanelAddress | null>(null);
  const [exposure, setExposure] = useState<PublishedApp | null>(null);
  const [accountFilter, setAccountFilter] = useState('');
  const [tunnelId, setTunnelId] = useState('');
  const [subdomain, setSubdomain] = useState('');
  const [domain, setDomain] = useState('');
  const [serviceUrl, setServiceUrl] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  const load = useCallback(async () => {
    try {
      const [accs, tuns, exp, addr] = await Promise.all([
        accountsApi.list(),
        tunnelsApi.list(),
        panelApi.get(),
        panelApi.address(),
      ]);
      setAccounts(accs);
      setTunnels(tuns);
      setExposure(exp);
      setAddress(addr);
      setTunnelId(exp?.tunnel_id ?? tuns.find((t) => t.config_src === 'cloudflare')?.id ?? '');
      if (exp) {
        // zone_name is the zone the panel resolved when the exposure was
        // created, so it is always the right split point.
        const zoneName = exp.zone_name;
        setDomain(zoneName);
        setSubdomain(
          zoneName && exp.hostname.endsWith(`.${zoneName}`)
            ? exp.hostname.slice(0, -(zoneName.length + 1))
            : exp.hostname,
        );
        setServiceUrl(exp.service_url);
      } else {
        setSubdomain('');
        setDomain('');
        setServiceUrl(addr.remote_service_url);
      }
      setError('');
    } catch (err) {
      setError(apiMessage(err));
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const selectedTunnel = tunnels.find((t) => t.id === tunnelId);
  const tunnelAccountId = selectedTunnel?.cf_account_id ?? '';
  const isRemote = selectedTunnel?.location === 'remote';

  // Zones follow the tunnel's account. Only tunnels with API access can carry
  // the panel, and their account is the one whose zones matter.
  useEffect(() => {
    let cancelled = false;
    if (!tunnelAccountId) {
      setZones([]);
      return;
    }
    accountsApi
      .zones(tunnelAccountId)
      .then((zs) => {
        if (!cancelled) setZones(zs);
      })
      .catch(() => {
        if (!cancelled) setZones([]);
      });
    return () => {
      cancelled = true;
    };
  }, [tunnelAccountId]);

  useEffect(() => {
    if (zones.length > 0 && !domain) setDomain(zones[0].name);
  }, [zones, domain]);

  const publishable = tunnels.filter(
    (t) => t.config_src === 'cloudflare' && (!accountFilter || t.cf_account_id === accountFilter),
  );
  const effectiveServiceUrl = isRemote ? serviceUrl : (address?.internal_url ?? '');

  const onTunnelChange = (id: string) => {
    setTunnelId(id);
    // Clear the zone list and domain: they belong to the previous account's
    // zones. The effects above repopulate from the new tunnel's account.
    setZones([]);
    setDomain('');
    setServiceUrl(address?.remote_service_url ?? '');
  };

  const save = async () => {
    setBusy(true);
    setError('');
    try {
      const app = await panelApi.set({
        tunnel_id: tunnelId,
        subdomain: subdomain.trim(),
        domain,
        ...(isRemote ? { service_url: serviceUrl.trim() } : {}),
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
      setSubdomain('');
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
        <Field label="Account" hint="Optional filter. The panel can only be published through a tunnel with an API token.">
          <select className="field" value={accountFilter} onChange={(e) => setAccountFilter(e.target.value)}>
            <option value="">All accounts</option>
            {accounts.map((a) => (
              <option key={a.id} value={a.id}>
                {a.name}
              </option>
            ))}
          </select>
        </Field>
        <Field label="Tunnel" error={publishable.length === 0 ? 'No tunnel with an API token yet.' : undefined}>
          <select className="field" value={tunnelId} onChange={(e) => onTunnelChange(e.target.value)}>
            <option value="">Select a tunnel</option>
            {publishable.map((t) => (
              <option key={t.id} value={t.id}>
                {t.name} ({t.location === 'local' ? 'this server' : 'remote'})
              </option>
            ))}
          </select>
        </Field>
      </div>

      <div className="mt-3 grid gap-3 sm:grid-cols-[1fr_1.4fr]">
        <Field label="Subdomain">
          <input
            className="field"
            value={subdomain}
            onChange={(e) => setSubdomain(e.target.value)}
            placeholder="panel"
            autoComplete="off"
          />
        </Field>
        <Field
          label="Domain"
          hint={zones.length === 0 ? 'No zone cached for this account; sync zones in Accounts first.' : undefined}
        >
          {zones.length > 0 && zones.some((z) => z.name === domain) ? (
            <select className="field" value={domain} onChange={(e) => setDomain(e.target.value)}>
              {zones.map((z) => (
                <option key={z.id} value={z.name}>
                  {z.name}
                </option>
              ))}
            </select>
          ) : (
            <input
              className="field"
              value={domain}
              onChange={(e) => setDomain(e.target.value)}
              placeholder="example.com"
              autoComplete="off"
            />
          )}
        </Field>
      </div>

      <div className="mt-3">
        {isRemote ? (
          <Field
            label="Panel address reachable from that device"
            hint="Filled in automatically from PUBLIC_URL. Change it only if that device reaches the panel another way."
          >
            <input
              className="field font-mono text-xs"
              value={serviceUrl}
              onChange={(e) => setServiceUrl(e.target.value)}
              placeholder="http://192.168.1.10:7777"
            />
          </Field>
        ) : (
          <Field
            label="Service URL"
            hint="Set automatically: the connector container reaches the panel on the Docker network."
          >
            <input className="field font-mono text-xs" value={effectiveServiceUrl} readOnly disabled />
          </Field>
        )}
      </div>

      {subdomain && domain && (
        <p className="mt-3 text-xs text-slate-500">
          Will be published as{' '}
          <span className="font-mono text-slate-300">
            {subdomain.trim()}.{domain}
          </span>{' '}
          → <span className="font-mono text-slate-400">{effectiveServiceUrl}</span>
        </p>
      )}

      <div className="mt-3 flex items-center justify-between gap-3">
        <p className="text-xs text-slate-500">
          Publishing here also adds this hostname to the tunnel&apos;s ingress and to{' '}
          <span className="text-slate-400">Published apps</span>
          {selectedTunnel ? ` (${selectedTunnel.name})` : ''}.
        </p>
        <button
          className="btn-primary"
          disabled={busy || !tunnelId || !subdomain.trim() || !domain || (isRemote && !serviceUrl.trim())}
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
