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
  RefreshCw,
  Square,
  Terminal,
  Trash2,
} from 'lucide-react';
import { apiFields, apiMessage } from '../api/client';
import { accountsApi, appsApi, tunnelsApi, type AppPayload } from '../api/endpoints';
import { Badge, CopyButton, EmptyState, ErrorBox, Field, Modal, PageHeader, Spinner } from '../components/ui';
import { useToast } from '../components/Toast';
import { useTranslation } from '../i18n/useTranslation';
import type { CFAccount, CFZone, PublishedApp, Tunnel } from '../types';

export default function TunnelDetailPage() {
  const { t } = useTranslation('tunnel_detail');
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
  // null means "create", otherwise the modal edits that row.
  const [editingApp, setEditingApp] = useState<PublishedApp | null>(null);
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

  // Pull this account's remote ingress into the panel, then reload. Without this
  // the page can only ever show what the panel already knows, so a tunnel whose
  // routes live only in Cloudflare looks empty forever.
  const syncFromCloudflare = async () => {
    if (!tunnel) return;
    setBusy('sync');
    try {
      const res = await tunnelsApi.sync(tunnel.cf_account_id);
      await load();
      const { apps_created: created, apps_updated: updated } = res.result;
      toast.push(
        created + updated === 0 ? 'info' : 'success',
        created + updated === 0 ? t('sync_none') : t('sync_done', { created, updated }),
      );
    } catch (err) {
      toast.push('error', apiMessage(err));
    } finally {
      setBusy('');
    }
  };

  const remove = async () => {
    if (!tunnel) return;
    if (!window.confirm(t('delete_confirm', { name: tunnel.name }))) return;
    try {
      await tunnelsApi.remove(tunnel.id);
      toast.push('success', t('deleted'));
      navigate('/tunnels');
    } catch (err) {
      toast.push('error', apiMessage(err));
    }
  };

  const removeApp = async (app: PublishedApp) => {
    if (!window.confirm(t('app_delete_confirm', { hostname: app.hostname }))) return;
    try {
      await appsApi.remove(app.id);
      toast.push('success', t('app_deleted'));
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
        <ArrowLeft className="h-4 w-4" /> {t('back')}
      </Link>

      <PageHeader
        title={tunnel.name}
        description={`${account?.name ?? 'account'} · ${tunnel.tunnel_id}`}
        actions={
          <>
            <button className="btn-ghost" disabled={busy === 'sync'} onClick={() => void syncFromCloudflare()}>
              {busy === 'sync' ? <Spinner className="h-4 w-4" /> : <RefreshCw className="h-4 w-4" />} {t('sync')}
            </button>
            <button className="btn-ghost" onClick={() => void openConfig()}>
              <FileCode2 className="h-4 w-4" /> config.yml
            </button>
            <button className="btn-ghost" onClick={() => void openToken()}>
              <KeyRound className="h-4 w-4" /> {t('token')}
            </button>
            {managed && (
              <button className="btn-ghost" onClick={() => void openLogs()}>
                <Terminal className="h-4 w-4" /> {t('logs')}
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
                {tunnel.status === 'running' ? t('stop') : t('start')}
              </button>
            )}
            <button className="btn-primary" onClick={() => { setEditingApp(null); setShowAppModal(true); }}>
              <Plus className="h-4 w-4" /> {t('publish_app')}
            </button>
            <button className="btn-danger" onClick={() => void remove()}>
              <Trash2 className="h-4 w-4" />
            </button>
          </>
        }
      />

      <div className="mb-6 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <InfoTile label={t('tiles.placement')}>
          <div className="flex items-center gap-2">
            <Badge tone={isLocal ? 'blue' : 'violet'} dot>
              {isLocal ? t('placement.this_server') : t('placement.remote_device')}
            </Badge>
            <button
              onClick={() => setShowPlacement(true)}
              className="text-slate-500 hover:text-slate-300"
              title={t('placement.change')}
            >
              <Pencil className="h-3 w-3" />
            </button>
          </div>
          <p className="mt-1 truncate text-[11px] text-slate-500">{tunnel.host_label || '—'}</p>
        </InfoTile>
        <InfoTile label={t('tiles.status')}>
          <Badge tone={tunnel.status === 'running' ? 'green' : tunnel.status === 'error' ? 'red' : 'slate'} dot>
            {tunnel.status}
          </Badge>
          {tunnel.last_seen_at && (
            <p className="mt-1 text-[11px] text-slate-500">
              {t('seen', { when: new Date(tunnel.last_seen_at).toLocaleString() })}
            </p>
          )}
        </InfoTile>
        <InfoTile label={t('tiles.config_source')}>
          <Badge tone={tunnel.config_src === 'cloudflare' ? 'green' : 'amber'}>{tunnel.config_src}</Badge>
        </InfoTile>
        <InfoTile label={t('tiles.cloudflared')}>
          <span className="font-mono text-sm text-slate-200">
            {managed ? tunnel.image_version || '—' : t('na_here')}
          </span>
        </InfoTile>
      </div>

      {!isLocal && (
        <div className="mb-4 flex items-start gap-2 rounded-lg border border-violet-500/30 bg-violet-500/10 px-4 py-3 text-xs text-violet-200">
          <Laptop className="mt-0.5 h-4 w-4 shrink-0" />
          <div>{t('remote_notice', { host: tunnel.host_label || t('placement.remote_device') })}</div>
        </div>
      )}

      {tunnel.status_message && (
        <div className="mb-4 rounded-lg border border-rose-500/30 bg-rose-950/30 px-4 py-3 text-xs text-rose-200">
          {tunnel.status_message}
        </div>
      )}

      <div className="card overflow-hidden">
        <div className="flex items-center justify-between border-b border-surface-700/70 px-4 py-3">
          <h2 className="text-sm font-semibold text-white">{t('apps.title')}</h2>
          <span className="text-xs text-slate-500">{t('apps.count', { count: apps.length })}</span>
        </div>
        {apps.length === 0 ? (
          <EmptyState
            icon={<Globe2 className="h-7 w-7" />}
            title={t('apps.empty_title')}
            description={t('apps.empty_desc')}
            action={
              <button className="btn-primary" onClick={() => { setEditingApp(null); setShowAppModal(true); }}>
                <Plus className="h-4 w-4" /> {t('publish_app')}
              </button>
            }
          />
        ) : (
          <table className="w-full">
            <thead className="border-b border-surface-700/70 bg-surface-900/60">
              <tr>
                <th className="th">{t('columns.hostname')}</th>
                <th className="th">{t('columns.service')}</th>
                <th className="th">{t('columns.dns')}</th>
                <th className="th">{t('columns.sync')}</th>
                <th className="th text-right">{t('columns.actions')}</th>
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
                      <Badge tone={app.dns_record_id ? 'green' : 'amber'}>
                        {app.dns_record_id ? t('dns.managed') : t('dns.pending')}
                      </Badge>
                    ) : (
                      <Badge tone="slate">{t('dns.manual')}</Badge>
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
                          {t('apps.managed_in_settings')}
                        </Link>
                      ) : (
                        <>
                          <button
                            className="btn-ghost !px-2 !py-1 !text-xs"
                            title={t('edit_hint')}
                            onClick={() => {
                              setEditingApp(app);
                              setShowAppModal(true);
                            }}
                          >
                            <Pencil className="h-3.5 w-3.5" />
                          </button>
                          <button
                            className="btn-ghost !px-2 !py-1 !text-xs"
                            title={t('delete_hint')}
                            onClick={() => void removeApp(app)}
                          >
                            <Trash2 className="h-3.5 w-3.5" />
                          </button>
                        </>
                      )}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      <Modal open={showLogs} onClose={() => setShowLogs(false)} title={t('modals.logs_title')} wide>
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
              {logs || t('modals.no_output')}
            </pre>
          </div>
        )}
      </Modal>

      <Modal
        open={showConfig}
        onClose={() => setShowConfig(false)}
        title={t('modals.config_title')}
        description={t('modals.config_desc')}
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
        title={t('modals.token_title')}
        description={t('modals.token_desc')}
        wide
      >
        <div className="relative">
          <div className="absolute right-2 top-2">
            <CopyButton value={token} />
          </div>
          <pre className="max-h-40 overflow-auto rounded-lg bg-black/60 p-3 font-mono text-[11px] leading-relaxed text-slate-300">
            {token || t('modals.loading')}
          </pre>
        </div>
        <p className="text-xs text-slate-500">{t('modals.token_warning')}</p>
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
        app={editingApp}
        onClose={() => {
          setShowAppModal(false);
          setEditingApp(null);
        }}
        onDone={() => {
          setShowAppModal(false);
          setEditingApp(null);
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
  const { t } = useTranslation('tunnel_detail');
  const { t: tc } = useTranslation('common');
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
      toast.push('success', t('placement_modal.saved'));
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
      title={t('placement_modal.title')}
      description={t('placement_modal.description')}
      footer={
        <>
          <button className="btn-ghost" onClick={onClose}>
            {tc('cancel')}
          </button>
          <button className="btn-primary" disabled={busy} onClick={() => void submit()}>
            {busy && <Spinner className="h-4 w-4" />} {tc('save')}
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
            <HardDrive className="h-4 w-4" /> {t('placement.this_server')}
          </div>
          <p className="mt-1 text-[11px] text-slate-400">{t('placement_modal.this_server_hint')}</p>
        </button>
        <button
          type="button"
          onClick={() => setLocation('remote')}
          className={`rounded-lg border p-3 text-left transition-colors ${
            location === 'remote' ? 'border-accent-500/60 bg-accent-500/10' : 'border-surface-600 hover:border-surface-500'
          }`}
        >
          <div className="flex items-center gap-2 text-sm font-medium text-slate-100">
            <Laptop className="h-4 w-4" /> {t('placement.remote_device')}
          </div>
          <p className="mt-1 text-[11px] text-slate-400">{t('placement_modal.another_device_hint')}</p>
        </button>
      </div>

      <Field label={t('placement_modal.device_label')} hint={t('placement_modal.device_label_hint')}>
        <input className="field" value={hostLabel} onChange={(e) => setHostLabel(e.target.value)} />
      </Field>

      {location === 'remote' && tunnel.desired_running && (
        <div className="rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-xs text-amber-200">
          {t('placement_modal.remote_warning')}
        </div>
      )}
    </Modal>
  );
}

function PublishAppModal({
  open,
  tunnel,
  zones,
  app,
  onClose,
  onDone,
}: {
  open: boolean;
  tunnel: Tunnel;
  zones: CFZone[];
  // When set, the modal edits that app instead of creating a new one.
  app?: PublishedApp | null;
  onClose: () => void;
  onDone: () => void;
}) {
  const { t } = useTranslation('tunnel_detail');
  const { t: tc } = useTranslation('common');
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
  // A LAN service served over HTTPS usually has a certificate for its own name,
  // not for the public hostname, so cloudflared must be told to skip the check.
  // Without this, pointing at https://192.168.0.103 fails with a TLS error.
  const [skipTLSVerify, setSkipTLSVerify] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [fields, setFields] = useState<Record<string, string>>({});

  useEffect(() => {
    if (!open) return;
    setError('');
    setFields({});
    if (app) {
      // Split the stored hostname on the longest matching zone so the two
      // fields show what was saved rather than an empty form. A hostname equal
      // to the zone itself is the apex, which has no subdomain.
      const zone = [...zones].sort((a, b) => b.name.length - a.name.length).find((z) => app.hostname === z.name || app.hostname.endsWith('.' + z.name));
      setSubdomain(zone && app.hostname !== zone.name ? app.hostname.slice(0, -(zone.name.length + 1)) : '');
      setDomain(zone ? zone.name : app.zone_name || '');
      setServiceUrl(app.service_url);
      setPath(app.path);
      setManageDns(app.manage_dns);
      setSkipTLSVerify(app.origin_request?.noTLSVerify === true);
      return;
    }
    setSubdomain('');
    setDomain(zones[0]?.name ?? '');
    setServiceUrl(tunnel.location === 'local' ? 'http://host.docker.internal:8080' : 'http://localhost:8080');
    setPath('/*');
    setManageDns(tunnel.config_src === 'cloudflare');
    setSkipTLSVerify(false);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, app]);

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
        // An explicit empty object clears a previously saved option, which is
        // what unchecking the box has to do.
        ...(skipTLSVerify ? { origin_request: { noTLSVerify: true } } : { origin_request: {} }),
      };
      const saved = app ? await appsApi.update(app.id, payload) : await tunnelsApi.createApp(tunnel.id, payload);
      toast.push(
        saved.sync_status === 'synced' ? 'success' : 'info',
        saved.sync_status === 'synced'
          ? t('app_modal.saved_live', { hostname: saved.hostname })
          : t('app_modal.saved_pending', { hostname: saved.hostname, error: saved.last_error ?? '' }),
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
      title={app ? t('app_modal.edit_title') : t('app_modal.create_title')}
      description={app ? t('app_modal.edit_desc') : t('app_modal.create_desc')}
      footer={
        <>
          <button className="btn-ghost" onClick={onClose}>
            {tc('cancel')}
          </button>
          <button className="btn-primary" disabled={busy || !domain} onClick={() => void submit()}>
            {busy && <Spinner className="h-4 w-4" />} {app ? t('app_modal.save') : t('app_modal.submit')}
          </button>
        </>
      }
    >
      {error && <ErrorBox message={error} />}
      <div className="grid grid-cols-[1fr_1.4fr] gap-3">
        <Field label={t('app_modal.subdomain')} hint={t('app_modal.subdomain_hint')} error={fields.hostname}>
          <input className="field" value={subdomain} onChange={(e) => setSubdomain(e.target.value)} placeholder="app" />
        </Field>
        <Field label={t('app_modal.domain')} hint={zones.length === 0 ? t('app_modal.no_zone') : undefined}>
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
        label={t('app_modal.service_url')}
        hint={tunnel.location === 'local' ? t('app_modal.service_local_hint') : t('app_modal.service_remote_hint')}
        error={fields.service_url}
      >
        <input className="field font-mono text-xs" value={serviceUrl} onChange={(e) => setServiceUrl(e.target.value)} />
      </Field>
      {tunnel.location === 'local' && (
        <div className="rounded-lg border border-surface-700 bg-surface-950/50 p-3 text-xs text-slate-400">
          <p className="font-medium text-slate-300">{t('app_modal.what_title')}</p>
          <ul className="mt-1 space-y-0.5">
            <li>
              <code className="font-mono text-slate-300">http://host.docker.internal:8080</code> {t('app_modal.what_1')}
            </li>
            <li>
              <code className="font-mono text-slate-300">http://my-container:3000</code> {t('app_modal.what_2')}
            </li>
            <li>
              <code className="font-mono text-slate-300">http://192.168.0.103</code> {t('app_modal.what_3')}
            </li>
            <li>
              <code className="font-mono text-slate-300">https://localhost:8006</code> {t('app_modal.what_4')}
            </li>
          </ul>
          <p className="mt-1.5 text-slate-500">{t('app_modal.what_note')}</p>
        </div>
      )}
      {serviceUrl.trim().toLowerCase().startsWith('https://') && (
        <label className="flex items-start gap-2 text-sm text-slate-300">
          <input
            type="checkbox"
            className="mt-1"
            checked={skipTLSVerify}
            onChange={(e) => setSkipTLSVerify(e.target.checked)}
          />
          <span>
            {t('app_modal.skip_tls')}
            <span className="block text-xs text-slate-500">{t('app_modal.skip_tls_hint')}</span>
          </span>
        </label>
      )}
      <Field label={t('app_modal.path')} hint={t('app_modal.path_hint')}>
        <input className="field font-mono text-xs" value={path} onChange={(e) => setPath(e.target.value)} />
      </Field>
      <label className="flex items-center gap-2 text-sm text-slate-300">
        <input
          type="checkbox"
          checked={manageDns}
          disabled={tunnel.config_src !== 'cloudflare'}
          onChange={(e) => setManageDns(e.target.checked)}
        />
        {t('app_modal.dns_auto')}
      </label>
      {tunnel.config_src !== 'cloudflare' && (
        <div className="rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-xs text-amber-200">
          {t('app_modal.dns_manual_warning')}
        </div>
      )}
    </Modal>
  );
}
