import { useCallback, useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { Globe2, RefreshCw } from 'lucide-react';
import { apiMessage } from '../api/client';
import { appsApi } from '../api/endpoints';
import { Badge, EmptyState, ErrorBox, PageHeader, Spinner } from '../components/ui';
import { useToast } from '../components/Toast';
import type { PublishedApp } from '../types';

export default function AppsPage() {
  const toast = useToast();
  const [apps, setApps] = useState<PublishedApp[] | null>(null);
  const [error, setError] = useState('');

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

  const retry = async (app: PublishedApp) => {
    try {
      const result = await appsApi.retry(app.id);
      toast.push(result.sync_status === 'synced' ? 'success' : 'error', result.last_error || 'Synced');
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

  return (
    <div>
      <PageHeader
        title="Published apps"
        description="Every hostname across your tunnels, with its Cloudflare sync state."
        actions={
          <button className="btn-ghost" onClick={() => void load()}>
            <RefreshCw className="h-4 w-4" /> Refresh
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
            title="No published apps"
            description="Open a tunnel and add a hostname to publish it."
          />
        </div>
      ) : (
        <div className="card overflow-hidden">
          <table className="w-full">
            <thead className="border-b border-surface-700/70 bg-surface-900/60">
              <tr>
                <th className="th">Hostname</th>
                <th className="th">Service</th>
                <th className="th">Kind</th>
                <th className="th">Sync</th>
                <th className="th">Enabled</th>
                <th className="th text-right">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-surface-800">
              {apps.map((app) => (
                <tr key={app.id} className="hover:bg-surface-900/40">
                  <td className="td">
                    <Link to={`/tunnels/${app.tunnel_id}`} className="font-medium text-slate-100 hover:text-accent-400">
                      {app.hostname}
                    </Link>
                    {app.zone_name && <p className="text-[11px] text-slate-500">{app.zone_name}</p>}
                  </td>
                  <td className="td font-mono text-xs">{app.service_url}</td>
                  <td className="td">
                    <Badge tone={app.kind === 'panel' ? 'violet' : 'slate'}>{app.kind}</Badge>
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
                    {app.last_error && <p className="mt-1 max-w-xs truncate text-[11px] text-rose-300/80">{app.last_error}</p>}
                  </td>
                  <td className="td">
                    <button
                      role="switch"
                      aria-checked={app.enabled}
                      onClick={() => void toggle(app)}
                      className={`relative h-5 w-9 rounded-full transition-colors ${app.enabled ? 'bg-emerald-500/70' : 'bg-surface-600'}`}
                    >
                      <span
                        className={`absolute top-0.5 h-4 w-4 rounded-full bg-white transition-all ${app.enabled ? 'left-[18px]' : 'left-0.5'}`}
                      />
                    </button>
                  </td>
                  <td className="td text-right">
                    {app.sync_status !== 'synced' && (
                      <button className="btn-ghost !px-2 !py-1 !text-xs" onClick={() => void retry(app)}>
                        <RefreshCw className="h-3.5 w-3.5" /> Retry
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
