import { useCallback, useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { Cloud, KeyRound, Plus, RefreshCw, Trash2 } from 'lucide-react';
import { apiFields, apiMessage } from '../api/client';
import { accountsApi } from '../api/endpoints';
import { Badge, EmptyState, ErrorBox, Field, Modal, PageHeader, Spinner } from '../components/ui';
import { ApiTokenGuide } from '../components/ApiTokenGuide';
import { useToast } from '../components/Toast';
import type { CFAccount } from '../types';

export default function AccountsPage() {
  const toast = useToast();
  const [accounts, setAccounts] = useState<CFAccount[] | null>(null);
  const [error, setError] = useState('');
  const [modal, setModal] = useState<'api' | 'import' | null>(null);
  const [attachFor, setAttachFor] = useState<CFAccount | null>(null);
  const [busyId, setBusyId] = useState('');

  const load = useCallback(async () => {
    try {
      setAccounts(await accountsApi.list());
      setError('');
    } catch (err) {
      setError(apiMessage(err));
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const verify = async (account: CFAccount) => {
    setBusyId(account.id);
    try {
      await accountsApi.verify(account.id);
      toast.push('success', `${account.name} verified`);
    } catch (err) {
      toast.push('error', apiMessage(err));
    } finally {
      setBusyId('');
      void load();
    }
  };

  const remove = async (account: CFAccount) => {
    if (!window.confirm(`Delete account ${account.name}? Its tunnels and published apps are removed from the panel.`)) return;
    try {
      await accountsApi.remove(account.id);
      toast.push('success', 'Account deleted');
      void load();
    } catch (err) {
      toast.push('error', apiMessage(err));
    }
  };

  return (
    <div>
      <PageHeader
        title="Cloudflare accounts"
        description="Each account is a credential set. Users only see their own; admins see everything."
        actions={
          <>
            <button className="btn-ghost" onClick={() => void load()}>
              <RefreshCw className="h-4 w-4" /> Refresh
            </button>
            <button className="btn-ghost" onClick={() => setModal('import')}>
              <KeyRound className="h-4 w-4" /> Import tunnel token
            </button>
            <button className="btn-primary" onClick={() => setModal('api')}>
              <Plus className="h-4 w-4" /> Add API token
            </button>
          </>
        }
      />

      {error && <ErrorBox message={error} />}

      {!accounts ? (
        <div className="flex justify-center py-16 text-slate-400">
          <Spinner className="h-6 w-6" />
        </div>
      ) : accounts.length === 0 ? (
        <div className="card">
          <EmptyState
            icon={<Cloud className="h-8 w-8" />}
            title="No Cloudflare account yet"
            description="Add a scoped API token so the panel can create tunnels, push ingress and write DNS for you."
            action={
              <button className="btn-primary" onClick={() => setModal('api')}>
                <Plus className="h-4 w-4" /> Add your first account
              </button>
            }
          />
        </div>
      ) : (
        <div className="card overflow-hidden">
          <table className="w-full">
            <thead className="border-b border-surface-700/70 bg-surface-900/60">
              <tr>
                <th className="th">Account</th>
                <th className="th">Credential</th>
                <th className="th">Status</th>
                <th className="th">Tunnels</th>
                <th className="th text-right">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-surface-800">
              {accounts.map((account) => (
                <tr key={account.id} className="hover:bg-surface-900/40">
                  <td className="td">
                    <p className="font-medium text-slate-100">{account.name}</p>
                    <p className="font-mono text-[11px] text-slate-500">{account.account_id}</p>
                    {account.status_message && (
                      <p className="mt-1 max-w-md text-[11px] text-amber-300/80">{account.status_message}</p>
                    )}
                  </td>
                  <td className="td">
                    {account.credential === 'api' ? (
                      <Badge tone="green">API token</Badge>
                    ) : (
                      <Badge tone="amber">tunnel token</Badge>
                    )}
                    <p className="mt-1 font-mono text-[11px] text-slate-500">{account.token_hint}</p>
                  </td>
                  <td className="td">
                    <Badge tone={account.status === 'active' ? 'green' : 'red'} dot>
                      {account.status}
                    </Badge>
                  </td>
                  <td className="td">
                    <Link to={`/tunnels?account=${account.id}`} className="text-accent-400 hover:underline">
                      View
                    </Link>
                  </td>
                  <td className="td">
                    <div className="flex justify-end gap-1.5">
                      {account.credential === 'token_import' && (
                        <button
                          className="btn-primary !px-2 !py-1 !text-xs"
                          onClick={() => setAttachFor(account)}
                        >
                          <KeyRound className="h-3.5 w-3.5" /> Attach API token
                        </button>
                      )}
                      <button className="btn-ghost !px-2 !py-1 !text-xs" disabled={busyId === account.id} onClick={() => void verify(account)}>
                        {busyId === account.id ? <Spinner className="h-3.5 w-3.5" /> : <RefreshCw className="h-3.5 w-3.5" />}
                        Verify
                      </button>
                      <button className="btn-danger !px-2 !py-1 !text-xs" onClick={() => void remove(account)}>
                        <Trash2 className="h-3.5 w-3.5" />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <AddAccountModal
        open={modal === 'api'}
        onClose={() => setModal(null)}
        onDone={() => {
          setModal(null);
          void load();
        }}
      />
      <ImportTunnelModal
        open={modal === 'import'}
        onClose={() => setModal(null)}
        onDone={() => {
          setModal(null);
          void load();
        }}
      />
      <AttachTokenModal
        account={attachFor}
        onClose={() => setAttachFor(null)}
        onDone={() => {
          setAttachFor(null);
          void load();
        }}
      />
    </div>
  );
}

// AttachTokenModal upgrades an account that was created from a tunnel token.
// The same row is reused, so its imported tunnels and their hostnames stay.
function AttachTokenModal({
  account,
  onClose,
  onDone,
}: {
  account: CFAccount | null;
  onClose: () => void;
  onDone: () => void;
}) {
  const toast = useToast();
  const [token, setToken] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [fields, setFields] = useState<Record<string, string>>({});

  useEffect(() => {
    if (!account) return;
    setToken('');
    setError('');
    setFields({});
  }, [account]);

  const submit = async () => {
    if (!account) return;
    setBusy(true);
    setError('');
    setFields({});
    try {
      await accountsApi.attachToken(account.id, token);
      toast.push('success', 'Token attached. Existing tunnels and hostnames were imported.');
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
      open={!!account}
      onClose={onClose}
      title="Attach an API token"
      description={`Upgrade ${account?.name ?? 'this account'} in place. Its imported tunnels are kept; ingress and DNS become manageable from here.`}
      wide
      footer={
        <>
          <button className="btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button className="btn-primary" disabled={busy || !token} onClick={() => void submit()}>
            {busy && <Spinner className="h-4 w-4" />} Attach token
          </button>
        </>
      }
    >
      {error && <ErrorBox message={error} />}
      <Field label="API token" hint="Must be able to read this Cloudflare account and its tunnels." error={fields.api_token}>
        <input
          className="field font-mono text-xs"
          value={token}
          onChange={(e) => setToken(e.target.value)}
          placeholder="Paste the token"
          autoComplete="off"
        />
      </Field>
      <div className="rounded-lg border border-surface-700 bg-surface-950/50 p-3 text-xs text-slate-400">
        <p className="font-medium text-slate-300">Required permissions — two policies</p>
        <ul className="mt-1 list-inside list-disc space-y-0.5">
          <li>
            <span className="text-slate-300">Account policy</span> → Cloudflare Tunnel · Edit
          </li>
          <li>
            <span className="text-slate-300">Account policy</span> → Account Settings · Read
          </li>
          <li>
            <span className="text-slate-300">Zone policy</span> → Zone · Read
          </li>
          <li>
            <span className="text-slate-300">Zone policy</span> → DNS · Edit
          </li>
        </ul>
      </div>
      <div className="rounded-lg border border-sky-500/30 bg-sky-500/10 p-3 text-xs text-sky-200">
        After attaching, the panel lists every tunnel the token can see and imports the hostnames already present in each
        tunnel&apos;s ingress. Nothing is deleted.
      </div>
      <ApiTokenGuide />
    </Modal>
  );
}

function AddAccountModal({ open, onClose, onDone }: { open: boolean; onClose: () => void; onDone: () => void }) {
  const toast = useToast();
  const [name, setName] = useState('');
  const [token, setToken] = useState('');
  const [accountId, setAccountId] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [fields, setFields] = useState<Record<string, string>>({});

  const submit = async () => {
    setBusy(true);
    setError('');
    setFields({});
    try {
      const account = await accountsApi.create({
        name: name || undefined,
        api_token: token,
        account_id: accountId || undefined,
      });
      toast.push(
        'success',
        `Account ${account.name} saved. Existing tunnels and their hostnames were imported.`,
      );
      setName('');
      setToken('');
      setAccountId('');
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
      title="Add Cloudflare account"
      description="Use a scoped API token. The panel verifies it before saving and stores it encrypted."
      wide
      footer={
        <>
          <button className="btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button className="btn-primary" disabled={busy || !token} onClick={() => void submit()}>
            {busy && <Spinner className="h-4 w-4" />} Add account
          </button>
        </>
      }
    >
      {error && <ErrorBox message={error} />}
      <Field label="Display name" hint="Optional. Defaults to the Cloudflare account name.">
        <input className="field" value={name} onChange={(e) => setName(e.target.value)} placeholder="Production" />
      </Field>
      <Field label="API token" error={fields.api_token}>
        <input
          className="field font-mono text-xs"
          value={token}
          onChange={(e) => setToken(e.target.value)}
          placeholder="Paste the token"
          autoComplete="off"
        />
      </Field>
      <Field label="Account ID" hint="Only needed when the token can access more than one account." error={fields.account_id}>
        <input className="field font-mono text-xs" value={accountId} onChange={(e) => setAccountId(e.target.value)} />
      </Field>
      <div className="rounded-lg border border-surface-700 bg-surface-950/50 p-3 text-xs text-slate-400">
        <p className="font-medium text-slate-300">Required permissions — two policies</p>
        <ul className="mt-1 list-inside list-disc space-y-0.5">
          <li>
            <span className="text-slate-300">Account policy</span> → Cloudflare Tunnel · Edit
          </li>
          <li>
            <span className="text-slate-300">Account policy</span> → Account Settings · Read
          </li>
          <li>
            <span className="text-slate-300">Zone policy</span> → Zone · Read
          </li>
          <li>
            <span className="text-slate-300">Zone policy</span> → DNS · Edit
          </li>
        </ul>
      </div>
      <ApiTokenGuide />
    </Modal>
  );
}

function ImportTunnelModal({ open, onClose, onDone }: { open: boolean; onClose: () => void; onDone: () => void }) {
  const toast = useToast();
  const [name, setName] = useState('');
  const [token, setToken] = useState('');
  const [hostLabel, setHostLabel] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  const submit = async () => {
    setBusy(true);
    setError('');
    try {
      const result = await accountsApi.importTunnel({
        name: name || undefined,
        tunnel_token: token,
        location: 'remote',
        host_label: hostLabel || undefined,
      });
      toast.push('success', `Imported tunnel ${result.tunnel.name}`);
      setName('');
      setToken('');
      setHostLabel('');
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
      title="Import an existing tunnel"
      description="Paste the token printed by `cloudflared tunnel token <name>`. No API token required."
      footer={
        <>
          <button className="btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button className="btn-primary" disabled={busy || !token} onClick={() => void submit()}>
            {busy && <Spinner className="h-4 w-4" />} Import
          </button>
        </>
      }
    >
      {error && <ErrorBox message={error} />}
      <Field label="Tunnel name" hint="Optional label inside the panel.">
        <input className="field" value={name} onChange={(e) => setName(e.target.value)} placeholder="laptop-tunnel" />
      </Field>
      <Field label="Tunnel token" hint="The long eyJ... string, not the API token.">
        <textarea className="field h-24 font-mono text-xs" value={token} onChange={(e) => setToken(e.target.value)} />
      </Field>
      <Field label="Device label" hint="Where this connector runs, so you can tell it apart later.">
        <input
          className="field"
          value={hostLabel}
          onChange={(e) => setHostLabel(e.target.value)}
          placeholder="office-nas"
        />
      </Field>
      <div className="rounded-lg border border-surface-700 bg-surface-950/50 p-3 text-xs text-slate-400">
        <p className="font-medium text-slate-300">Where to get this token</p>
        <p className="mt-1">
          Cloudflare Zero Trust → <span className="text-slate-300">Networks → Tunnels</span> → open the tunnel (or
          create one) → the install command for the <span className="text-slate-300">cloudflared</span> connector
          contains it:{' '}
          <code className="rounded bg-surface-800 px-1 py-0.5 font-mono text-[11px] text-slate-300">
            --token eyJ...
          </code>
          . Copy only the part after <span className="text-slate-300">--token</span>.
        </p>
        <p className="mt-1.5">
          On an existing host you can also run{' '}
          <code className="rounded bg-surface-800 px-1 py-0.5 font-mono text-[11px] text-slate-300">
            cloudflared tunnel token &lt;name&gt;
          </code>
          , or read it from that device&apos;s <code className="font-mono text-[11px] text-slate-400">config.yml</code>{' '}
          / <code className="font-mono text-[11px] text-slate-400">credentials.json</code>.
        </p>
      </div>
      <div className="rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-xs text-amber-200">
        Imported tunnels are treated as <span className="font-medium">remote</span>: you run cloudflared on that device. A
        tunnel-token account cannot push ingress or DNS through the API, so the panel generates config.yml and shows the
        CNAME you must create. Attach an API token later to unlock full automation.
      </div>
    </Modal>
  );
}
