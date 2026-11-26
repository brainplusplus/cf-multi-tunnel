import { useCallback, useEffect, useState } from 'react';
import { ArrowUpCircle, CheckCircle2, Container, Globe2, RefreshCw, Trash2, TriangleAlert } from 'lucide-react';
import { apiMessage } from '../api/client';
import { accountsApi, connectorApi, panelApi, tunnelsApi } from '../api/endpoints';
import { Badge, ErrorBox, Field, PageHeader, Spinner } from '../components/ui';
import { useToast } from '../components/Toast';
import { useTranslation } from '../i18n/useTranslation';
import type { CFAccount, CFZone, ConnectorStatus, PanelAddress, PublishedApp, RemoteTag, Tunnel } from '../types';

export default function SettingsPage() {
  const { t } = useTranslation('settings');
  return (
    <div className="space-y-8">
      <PageHeader title={t('title')} description={t('description')} />
      <PanelExposureCard />
      <ConnectorCard />
    </div>
  );
}

function PanelExposureCard() {
  const { t } = useTranslation('settings');
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
      // Only a tunnel installed on this server can carry the panel, so the
      // preselection ignores anything else. An exposure left on a remote tunnel
      // keeps its warning below instead of being silently rewritten.
      const local = tuns.filter((tun) => tun.location === 'local' && tun.config_src === 'cloudflare');
      const exposureTunnel = exp ? tuns.find((tun) => tun.id === exp.tunnel_id) : undefined;
      setTunnelId(
        exposureTunnel?.location === 'local' ? exposureTunnel.id : (local[0]?.id ?? ''),
      );
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
      } else {
        setSubdomain('');
        setDomain('');
      }
      setError('');
    } catch (err) {
      setError(apiMessage(err));
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const selectedTunnel = tunnels.find((tun) => tun.id === tunnelId);
  const tunnelAccountId = selectedTunnel?.cf_account_id ?? '';
  const effectiveServiceUrl = address?.internal_url ?? '';

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
    (tun) =>
      tun.location === 'local' &&
      tun.config_src === 'cloudflare' &&
      (!accountFilter || tun.cf_account_id === accountFilter),
  );

  // An existing exposure can still point at a remote tunnel (created before the
  // rule changed). Say so instead of pretending the dropdown can edit it.
  const exposureTunnel = exposure ? tunnels.find((tun) => tun.id === exposure.tunnel_id) : undefined;
  const exposureIsRemote = exposureTunnel ? exposureTunnel.location !== 'local' : false;

  const onTunnelChange = (id: string) => {
    setTunnelId(id);
    // Clear the zone list and domain: they belong to the previous account's
    // zones. The effects above repopulate from the new tunnel's account.
    setZones([]);
    setDomain('');
  };

  const save = async () => {
    setBusy(true);
    setError('');
    try {
      const app = await panelApi.set({
        tunnel_id: tunnelId,
        subdomain: subdomain.trim(),
        domain,
      });
      toast.push(app.sync_status === 'synced' ? 'success' : 'info', app.last_error || t('exposure.saved'));
      await load();
    } catch (err) {
      setError(apiMessage(err));
    } finally {
      setBusy(false);
    }
  };

  const remove = async () => {
    if (!window.confirm(t('exposure.remove_confirm'))) return;
    setBusy(true);
    try {
      await panelApi.remove();
      toast.push('success', t('exposure.removed'));
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
          <h2 className="text-sm font-semibold text-white">{t('exposure.title')}</h2>
          <p className="mt-1 text-sm text-slate-400">{t('exposure.description')}</p>
        </div>
      </div>

      {error && <div className="mb-4"><ErrorBox message={error} /></div>}

      {exposure && (
        <div className="mb-4 space-y-2">
          <div className="flex flex-wrap items-center gap-2 rounded-lg border border-surface-700 bg-surface-950/50 px-3 py-2 text-xs">
            <CheckCircle2 className="h-4 w-4 text-emerald-400" />
            <span className="text-slate-300">{exposure.hostname}</span>
            <Badge tone={exposure.sync_status === 'synced' ? 'green' : 'amber'}>{exposure.sync_status}</Badge>
            <span className="font-mono text-slate-500">→ {exposure.service_url}</span>
            <button className="btn-danger ml-auto !px-2 !py-1 !text-xs" disabled={busy} onClick={() => void remove()}>
              <Trash2 className="h-3.5 w-3.5" /> {t('exposure.remove')}
            </button>
          </div>
          {exposureIsRemote && (
            <div className="flex items-start gap-2 rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-200">
              <TriangleAlert className="mt-0.5 h-4 w-4 shrink-0" />
              <span>
                {t('exposure.remote_warning', {
                  name: exposureTunnel?.name ?? '—',
                  service: exposure.service_url,
                })}
              </span>
            </div>
          )}
        </div>
      )}

      <div className="grid gap-3 sm:grid-cols-2">
        <Field label={t('exposure.account')} hint={t('exposure.account_hint')}>
          <select className="field" value={accountFilter} onChange={(e) => setAccountFilter(e.target.value)}>
            <option value="">{t('exposure.all_accounts')}</option>
            {accounts.map((a) => (
              <option key={a.id} value={a.id}>
                {a.name}
              </option>
            ))}
          </select>
        </Field>
        <Field label={t('exposure.tunnel')} error={publishable.length === 0 ? t('exposure.no_local_tunnel') : undefined}>
          <select className="field" value={tunnelId} onChange={(e) => onTunnelChange(e.target.value)}>
            <option value="">{t('exposure.select_tunnel')}</option>
            {publishable.map((tun) => (
              <option key={tun.id} value={tun.id}>
                {tun.name}
              </option>
            ))}
          </select>
        </Field>
      </div>
      <p className="mt-1.5 text-[11px] text-slate-500">{t('exposure.tunnel_hint')}</p>

      <div className="mt-3 grid gap-3 sm:grid-cols-[1fr_1.4fr]">
        <Field label={t('exposure.subdomain')} hint={t('exposure.subdomain_hint')}>
          <input
            className="field"
            value={subdomain}
            onChange={(e) => setSubdomain(e.target.value)}
            placeholder="panel"
            autoComplete="off"
          />
        </Field>
        <Field
          label={t('exposure.domain')}
          hint={zones.length === 0 ? t('exposure.no_zones') : undefined}
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
        <Field label={t('exposure.service_url')} hint={t('exposure.service_auto')}>
          <input className="field font-mono text-xs" value={effectiveServiceUrl} readOnly disabled />
        </Field>
      </div>

      {domain && (
        <p className="mt-3 text-xs text-slate-500">
          {t('exposure.will_publish')}{' '}
          <span className="font-mono text-slate-300">
            {subdomain.trim() ? `${subdomain.trim()}.${domain}` : domain}
          </span>{' '}
          → <span className="font-mono text-slate-400">{effectiveServiceUrl}</span>
        </p>
      )}

      <div className="mt-3 flex items-center justify-end gap-3">
        <button
          className="btn-primary"
          disabled={busy || !tunnelId || !domain}
          onClick={() => void save()}
        >
          {busy && <Spinner className="h-4 w-4" />} {exposure ? t('exposure.update') : t('exposure.publish')}
        </button>
      </div>
    </section>
  );
}

function ConnectorCard() {
  const { t } = useTranslation('settings');
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
      toast.push('success', t('connector.tags_fetched'));
    } catch (err) {
      toast.push('error', apiMessage(err));
    } finally {
      setBusy('');
    }
  };

  const update = async () => {
    if (!window.confirm(t('connector.update_confirm'))) return;
    setBusy('update');
    setError('');
    try {
      const result = await connectorApi.update(image, pin);
      // Defensive: an older backend returned null for empty lists.
      const recreated = result.recreated ?? [];
      const failed = result.failed ?? [];
      toast.push(
        failed.length === 0 ? 'success' : 'info',
        t('connector.update_done', { version: result.image.version || image, count: recreated.length }) +
          (failed.length ? t('connector.update_failed_count', { count: failed.length }) : ''),
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
            <Container className="h-5 w-5" />
          </div>
          <div>
            <h2 className="text-sm font-semibold text-white">{t('connector.title')}</h2>
            <p className="mt-1 text-sm text-slate-400">{t('connector.description')}</p>
          </div>
        </div>
        <button className="btn-ghost !px-2 !py-1.5 !text-xs" disabled={busy === 'load'} onClick={() => void load(true)}>
          {busy === 'load' ? <Spinner className="h-3.5 w-3.5" /> : <RefreshCw className="h-3.5 w-3.5" />}
          {t('connector.check')}
        </button>
      </div>

      {error && <div className="mb-4"><ErrorBox message={error} /></div>}

      <div className="mb-4 grid gap-2 sm:grid-cols-4">
        <Stat label={t('connector.mode')} value={t('connector.docker')} />
        <Stat
          label={t('connector.installed')}
          value={status.image.version || t('connector.unknown')}
          mono
        />
        <Stat label={t('connector.latest')} value={status.latest_version || t('connector.unknown')} mono />
        <Stat
          label={t('connector.active_image')}
          value={status.image.image}
          mono
        />
      </div>

      <div className="grid gap-3 sm:grid-cols-[1.4fr_1fr]">
        <Field label={t('connector.target_image')} hint={t('connector.target_hint')}>
          <input className="field font-mono text-xs" value={image} onChange={(e) => setImage(e.target.value)} />
        </Field>
        <div className="flex items-end gap-2">
          <button className="btn-ghost" disabled={busy === 'tags'} onClick={() => void loadTags()}>
            {busy === 'tags' ? <Spinner className="h-4 w-4" /> : null} {t('connector.browse_tags')}
          </button>
          {status.update_available && (
            <button
              className="btn-ghost"
              onClick={() => setImage(`${repo}:${status.latest_version}`)}
              title={t('connector.use_latest', { version: status.latest_version })}
            >
              <ArrowUpCircle className="h-4 w-4" /> {t('connector.use_latest', { version: status.latest_version })}
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
          {t('connector.remember')}
        </label>
        <button className="btn-primary" disabled={busy === 'update' || !image} onClick={() => void update()}>
          {busy === 'update' ? <Spinner className="h-4 w-4" /> : <ArrowUpCircle className="h-4 w-4" />}{' '}
          {t('connector.pull_recreate')}
        </button>
      </div>

      {status.running_versions && status.running_versions.length > 0 && (
        <div className="mt-5 overflow-hidden rounded-lg border border-surface-700">
          <table className="w-full">
            <thead className="bg-surface-900/60">
              <tr>
                <th className="th">{t('connector.tunnel')}</th>
                <th className="th">{t('connector.running_version')}</th>
                <th className="th">{t('connector.image_ref')}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-surface-800">
              {status.running_versions.map((row) => (
                <tr key={row.tunnel_id}>
                  <td className="td">{row.name}</td>
                  <td className="td font-mono text-xs">{row.version || t('connector.unknown')}</td>
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
