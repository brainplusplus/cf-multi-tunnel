import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { ArrowUpRight, Cloud, Globe2, HardDrive, Laptop, TriangleAlert, Waypoints } from 'lucide-react';
import { apiMessage } from '../api/client';
import { connectorApi, dashboardApi } from '../api/endpoints';
import { Badge, ErrorBox, PageHeader, Spinner } from '../components/ui';
import { useAuth } from '../store/auth-context';
import type { ConnectorStatus, DashboardStats } from '../types';

export default function DashboardPage() {
  const { user } = useAuth();
  const [stats, setStats] = useState<DashboardStats | null>(null);
  const [connector, setConnector] = useState<ConnectorStatus | null>(null);
  const [error, setError] = useState('');

  useEffect(() => {
    dashboardApi
      .stats()
      .then(setStats)
      .catch((err) => setError(apiMessage(err)));
    if (user?.role === 'admin') {
      connectorApi.status(true).then(setConnector).catch(() => undefined);
    }
  }, [user?.role]);

  if (error) return <ErrorBox message={error} />;
  if (!stats) {
    return (
      <div className="flex justify-center py-20 text-slate-400">
        <Spinner className="h-6 w-6" />
      </div>
    );
  }

  const cards = [
    { label: 'Cloudflare accounts', value: stats.accounts, to: '/accounts', icon: Cloud },
    { label: 'Tunnels', value: stats.tunnels, to: '/tunnels', icon: Waypoints, hint: `${stats.tunnels_running} connected` },
    { label: 'Published apps', value: stats.apps, to: '/apps', icon: Globe2, hint: `${stats.apps_synced} synced` },
  ];

  return (
    <div>
      <PageHeader
        title={`Welcome, ${user?.name?.split(' ')[0] ?? 'there'}`}
        description="Every tunnel, account and hostname you can reach from this panel."
      />

      <div className="grid gap-4 sm:grid-cols-3">
        {cards.map(({ label, value, to, icon: Icon, hint }) => (
          <Link key={label} to={to} className="card group p-5 transition-colors hover:border-accent-500/40">
            <div className="flex items-start justify-between">
              <div>
                <p className="text-xs uppercase tracking-wider text-slate-500">{label}</p>
                <p className="mt-2 text-3xl font-semibold text-white">{value}</p>
                {hint && <p className="mt-1 text-xs text-slate-500">{hint}</p>}
              </div>
              <div className="rounded-lg bg-surface-800 p-2 text-slate-400 group-hover:text-accent-400">
                <Icon className="h-5 w-5" />
              </div>
            </div>
          </Link>
        ))}
      </div>

      <div className="mt-4 grid gap-4 sm:grid-cols-2">
        <Link to="/tunnels?location=local" className="card group p-4 transition-colors hover:border-sky-500/40">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2 text-sm text-slate-200">
              <HardDrive className="h-4 w-4 text-sky-400" /> Hosted by this panel
            </div>
            <Badge tone={stats.local_running > 0 ? 'green' : 'slate'}>
              {stats.local_running}/{stats.local_tunnels} running
            </Badge>
          </div>
          <p className="mt-1.5 text-xs text-slate-500">
            Containers the panel starts, stops, logs and updates.
          </p>
        </Link>
        <Link to="/tunnels?location=remote" className="card group p-4 transition-colors hover:border-violet-500/40">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2 text-sm text-slate-200">
              <Laptop className="h-4 w-4 text-violet-400" /> Running on other devices
            </div>
            <Badge tone={stats.remote_connected > 0 ? 'green' : 'slate'}>
              {stats.remote_connected}/{stats.remote_tunnels} connected
            </Badge>
          </div>
          <p className="mt-1.5 text-xs text-slate-500">
            Ingress and DNS managed here; the connector lives elsewhere.
          </p>
        </Link>
      </div>

      {stats.tunnels_error > 0 && (
        <div className="mt-4 flex items-center gap-2 rounded-lg border border-amber-500/30 bg-amber-500/10 px-4 py-3 text-sm text-amber-200">
          <TriangleAlert className="h-4 w-4" />
          {stats.tunnels_error} tunnel(s) report an error. Open the tunnel to see the message.
        </div>
      )}

      {connector && (
        <div className="mt-6 card p-5">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div>
              <h2 className="text-sm font-semibold text-white">Connector runtime</h2>
              <p className="mt-1 text-xs text-slate-400">
                Mode <span className="text-slate-200">{connector.mode}</span> ·{' '}
                {connector.docker_available ? (
                  <>Docker {connector.docker_version}</>
                ) : (
                  <span className="text-amber-300">Docker not reachable</span>
                )}
              </p>
            </div>
            <Link to="/settings" className="btn-ghost !py-1.5 !text-xs">
              Manage <ArrowUpRight className="h-3.5 w-3.5" />
            </Link>
          </div>

          <div className="mt-4 grid gap-3 sm:grid-cols-3">
            <div className="rounded-lg border border-surface-700 bg-surface-950/40 p-3">
              <p className="text-[11px] uppercase tracking-wider text-slate-500">Image</p>
              <p className="mt-1 truncate font-mono text-xs text-slate-300">{connector.image.image || '—'}</p>
            </div>
            <div className="rounded-lg border border-surface-700 bg-surface-950/40 p-3">
              <p className="text-[11px] uppercase tracking-wider text-slate-500">cloudflared</p>
              <p className="mt-1 font-mono text-xs text-slate-300">{connector.image.version || 'unknown'}</p>
            </div>
            <div className="rounded-lg border border-surface-700 bg-surface-950/40 p-3">
              <p className="text-[11px] uppercase tracking-wider text-slate-500">Latest release</p>
              <div className="mt-1 flex items-center gap-2">
                <p className="font-mono text-xs text-slate-300">{connector.latest_version || '—'}</p>
                {connector.update_available && <Badge tone="amber">update</Badge>}
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
