import { useEffect, useState } from 'react';
import { ScrollText } from 'lucide-react';
import { apiMessage } from '../api/client';
import { auditApi } from '../api/endpoints';
import { Badge, EmptyState, ErrorBox, PageHeader, Spinner } from '../components/ui';
import type { AuditLog } from '../types';

export default function AuditPage() {
  const [logs, setLogs] = useState<AuditLog[] | null>(null);
  const [error, setError] = useState('');

  useEffect(() => {
    auditApi
      .list(1, 100)
      .then((result) => setLogs(result.items))
      .catch((err) => setError(apiMessage(err)));
  }, []);

  return (
    <div>
      <PageHeader title="Audit log" description="Every mutation performed through the panel, newest first." />
      {error && <ErrorBox message={error} />}
      {!logs ? (
        <div className="flex justify-center py-16 text-slate-400">
          <Spinner className="h-6 w-6" />
        </div>
      ) : logs.length === 0 ? (
        <div className="card">
          <EmptyState icon={<ScrollText className="h-8 w-8" />} title="Nothing logged yet" />
        </div>
      ) : (
        <div className="card overflow-hidden">
          <table className="w-full">
            <thead className="border-b border-surface-700/70 bg-surface-900/60">
              <tr>
                <th className="th">When</th>
                <th className="th">Actor</th>
                <th className="th">Action</th>
                <th className="th">Entity</th>
                <th className="th">Detail</th>
                <th className="th">IP</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-surface-800">
              {logs.map((log) => (
                <tr key={log.id} className="hover:bg-surface-900/40">
                  <td className="td whitespace-nowrap text-xs text-slate-400">
                    {new Date(log.created_at).toLocaleString()}
                  </td>
                  <td className="td text-xs">{log.actor_email || '—'}</td>
                  <td className="td">
                    <Badge tone="blue">{log.action}</Badge>
                  </td>
                  <td className="td text-xs text-slate-400">
                    {log.entity}
                    <span className="ml-1 font-mono text-[10px] text-slate-600">{log.entity_id.slice(0, 8)}</span>
                  </td>
                  <td className="td max-w-xs truncate text-xs text-slate-400">{log.detail || '—'}</td>
                  <td className="td font-mono text-[11px] text-slate-500">{log.ip || '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
