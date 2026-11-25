import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { Cloud, KeyRound, Plus, RefreshCw, Trash2 } from 'lucide-react';
import { apiFields, apiMessage } from '../api/client';
import { accountsApi } from '../api/endpoints';
import { DataTable, type Column } from '../components/DataTable';
import { FilterToggle, QueryBuilder, filterCount } from '../components/QueryBuilder';
import { createGroup, matchesQuery, type FieldDefinition, type QueryGroup } from '../components/query';
import { Badge, EmptyState, ErrorBox, Field, Modal, PageHeader, Spinner } from '../components/ui';
import { ApiTokenGuide } from '../components/ApiTokenGuide';
import { useToast } from '../components/Toast';
import { useTranslation } from '../i18n/useTranslation';
import type { AccountChoice, CFAccount } from '../types';

type AccountRow = CFAccount & { credential_label: string };

export default function AccountsPage() {
  const { t } = useTranslation('accounts');
  const { t: tc } = useTranslation('common');
  const toast = useToast();
  const [accounts, setAccounts] = useState<CFAccount[] | null>(null);
  const [error, setError] = useState('');
  const [modal, setModal] = useState<'api' | 'import' | null>(null);
  const [attachFor, setAttachFor] = useState<CFAccount | null>(null);
  const [busyId, setBusyId] = useState('');
  const [search, setSearch] = useState('');
  const [showFilters, setShowFilters] = useState(false);
  const [filters, setFilters] = useState<QueryGroup>(() => createGroup());

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
      toast.push('success', t('actions.verified', { name: account.name }));
    } catch (err) {
      toast.push('error', apiMessage(err));
    } finally {
      setBusyId('');
      void load();
    }
  };

  const remove = async (account: CFAccount) => {
    if (!window.confirm(t('actions.delete_confirm', { name: account.name }))) return;
    try {
      await accountsApi.remove(account.id);
      toast.push('success', t('actions.deleted'));
      void load();
    } catch (err) {
      toast.push('error', apiMessage(err));
    }
  };

  const fields: FieldDefinition[] = useMemo(
    () => [
      { name: 'name', label: t('filter.name'), type: 'string' },
      {
        name: 'credential_label',
        label: t('filter.credential'),
        type: 'select',
        options: [
          { value: t('credential.api'), label: t('credential.api') },
          { value: t('credential.token_import'), label: t('credential.token_import') },
        ],
      },
      {
        name: 'status',
        label: t('filter.status'),
        type: 'select',
        options: ['active', 'invalid', 'error'].map((s) => ({ value: s, label: s })),
      },
    ],
    [t],
  );

  const rows: AccountRow[] = useMemo(() => {
    const term = search.trim().toLowerCase();
    return (accounts ?? [])
      .map((account) => ({
        ...account,
        credential_label: account.credential === 'api' ? t('credential.api') : t('credential.token_import'),
      }))
      .filter((account) => {
        if (term && !`${account.name} ${account.account_id} ${account.account_name}`.toLowerCase().includes(term)) {
          return false;
        }
        return matchesQuery(account as unknown as Record<string, unknown>, filters);
      });
  }, [accounts, search, filters, t]);

  const columns: Column<AccountRow>[] = [
    {
      key: 'name',
      label: t('columns.name'),
      sortable: true,
      minWidth: 200,
      render: (account) => (
        <div>
          <p className="font-medium text-slate-100">{account.name}</p>
          <p className="font-mono text-[11px] text-slate-500">{account.account_id}</p>
          {account.status_message && (
            <p className="mt-1 max-w-md text-[11px] text-amber-300/80">{account.status_message}</p>
          )}
        </div>
      ),
    },
    {
      key: 'credential_label',
      label: t('columns.credential'),
      sortable: true,
      width: 170,
      render: (account) => (
        <div>
          <Badge tone={account.credential === 'api' ? 'green' : 'amber'}>{account.credential_label}</Badge>
          <p className="mt-1 font-mono text-[11px] text-slate-500">{account.token_hint}</p>
        </div>
      ),
    },
    {
      key: 'status',
      label: t('columns.status'),
      sortable: true,
      width: 120,
      render: (account) => (
        <Badge tone={account.status === 'active' ? 'green' : 'red'} dot>
          {account.status}
        </Badge>
      ),
    },
    {
      key: 'tunnels',
      label: t('columns.tunnels'),
      width: 120,
      render: (account) => (
        <Link to={`/tunnels?account=${account.id}`} className="text-xs text-accent-400 hover:underline">
          {t('actions.view')}
        </Link>
      ),
    },
    {
      key: 'actions',
      label: t('columns.actions'),
      width: 280,
      render: (account) => (
        <div className="flex justify-end gap-1.5">
          {account.credential === 'token_import' && (
            <button className="btn-primary !px-2 !py-1 !text-xs" onClick={() => setAttachFor(account)}>
              <KeyRound className="h-3.5 w-3.5" /> {t('credential.attach')}
            </button>
          )}
          <button
            className="btn-ghost !px-2 !py-1 !text-xs"
            disabled={busyId === account.id}
            onClick={() => void verify(account)}
          >
            {busyId === account.id ? <Spinner className="h-3.5 w-3.5" /> : <RefreshCw className="h-3.5 w-3.5" />}
            {t('actions.verify')}
          </button>
          <button className="btn-danger !px-2 !py-1 !text-xs" onClick={() => void remove(account)}>
            <Trash2 className="h-3.5 w-3.5" />
          </button>
        </div>
      ),
    },
  ];

  return (
    <div>
      <PageHeader
        title={t('title')}
        description={t('description')}
        actions={
          <>
            <button className="btn-ghost" onClick={() => void load()}>
              <RefreshCw className="h-4 w-4" /> {t('refresh')}
            </button>
            <button className="btn-ghost" onClick={() => setModal('import')}>
              <KeyRound className="h-4 w-4" /> {t('import_tunnel')}
            </button>
            <button className="btn-primary" onClick={() => setModal('api')}>
              <Plus className="h-4 w-4" /> {t('new_account')}
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
            title={t('empty')}
            description={t('empty_hint')}
            action={
              <button className="btn-primary" onClick={() => setModal('api')}>
                <Plus className="h-4 w-4" /> {t('empty_action')}
              </button>
            }
          />
        </div>
      ) : (
        <div className="card">
          <div className="mb-3 flex flex-wrap items-center gap-2">
            <input
              className="field !w-64 !py-1.5 text-xs"
              placeholder={t('search')}
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
            <FilterToggle
              open={showFilters}
              count={filterCount(filters)}
              onClick={() => setShowFilters((v) => !v)}
            />
            <span className="text-[11px] text-slate-500">{tc('rows_found', { count: rows.length })}</span>
          </div>

          {showFilters && (
            <div className="mb-3">
              <QueryBuilder fields={fields} value={filters} onChange={setFilters} />
            </div>
          )}

          <DataTable
            tableId="accounts"
            data={rows}
            columns={columns}
            keyField="id"
            emptyMessage={tc('no_match')}
            initialSort={{ field: 'name', direction: 'asc' }}
          />
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
  const { t } = useTranslation('accounts');
  const { t: tc } = useTranslation('common');
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
      toast.push('success', t('attach.success'));
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
      title={t('attach.title')}
      description={t('attach.description', { name: account?.name ?? '—' })}
      wide
      footer={
        <>
          <button className="btn-ghost" onClick={onClose}>
            {tc('cancel')}
          </button>
          <button className="btn-primary" disabled={busy || !token} onClick={() => void submit()}>
            {busy && <Spinner className="h-4 w-4" />} {t('attach.submit')}
          </button>
        </>
      }
    >
      {error && <ErrorBox message={error} />}
      <Field label={t('fields.api_token')} hint={t('fields.api_token_hint')} error={fields.api_token}>
        <input
          className="field font-mono text-xs"
          value={token}
          onChange={(e) => setToken(e.target.value)}
          placeholder={t('fields.token_placeholder')}
          autoComplete="off"
        />
      </Field>
      <div className="rounded-lg border border-surface-700 bg-surface-950/50 p-3 text-xs text-slate-400">
        <p className="font-medium text-slate-300">{t('permissions.title')}</p>
        <ul className="mt-1 list-inside list-disc space-y-0.5">
          <li>
            <span className="text-slate-300">{t('permissions.account_policy')}</span> → {t('permissions.tunnel_edit')}
          </li>
          <li>
            <span className="text-slate-300">{t('permissions.account_policy')}</span> → {t('permissions.settings_read')}
          </li>
          <li>
            <span className="text-slate-300">{t('permissions.zone_policy')}</span> → {t('permissions.zone_read')}
          </li>
          <li>
            <span className="text-slate-300">{t('permissions.zone_policy')}</span> → {t('permissions.dns_edit')}
          </li>
        </ul>
      </div>
      <div className="rounded-lg border border-sky-500/30 bg-sky-500/10 p-3 text-xs text-sky-200">{t('attach.after')}</div>
      <ApiTokenGuide />
    </Modal>
  );
}

function AddAccountModal({ open, onClose, onDone }: { open: boolean; onClose: () => void; onDone: () => void }) {
  const { t } = useTranslation('accounts');
  const { t: tc } = useTranslation('common');
  const toast = useToast();
  const [name, setName] = useState('');
  const [token, setToken] = useState('');
  const [accountId, setAccountId] = useState('');
  const [choices, setChoices] = useState<AccountChoice[] | null>(null);
  const [probing, setProbing] = useState(false);
  const [probeError, setProbeError] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [fields, setFields] = useState<Record<string, string>>({});
  const probeSeq = useRef(0);

  // Probe the token as soon as it looks complete so the account list comes from
  // Cloudflare instead of the user copying an id out of an error message.
  useEffect(() => {
    const trimmed = token.trim();
    setChoices(null);
    setProbeError('');
    setAccountId('');
    if (trimmed.length < 20) {
      setProbing(false);
      return;
    }

    const seq = ++probeSeq.current;
    setProbing(true);
    const timer = setTimeout(() => {
      accountsApi
        .probe(trimmed)
        .then((list) => {
          if (probeSeq.current !== seq) return;
          setChoices(list);
          // A single account is not a choice; preselect it.
          if (list.length === 1) setAccountId(list[0].id);
        })
        .catch((err) => {
          if (probeSeq.current === seq) setProbeError(apiMessage(err));
        })
        .finally(() => {
          if (probeSeq.current === seq) setProbing(false);
        });
    }, 600);
    return () => clearTimeout(timer);
  }, [token]);

  const multiple = (choices?.length ?? 0) > 1;

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
      toast.push('success', t('add.success', { name: account.name }));
      setName('');
      setToken('');
      setAccountId('');
      setChoices(null);
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
      title={t('add.title')}
      description={t('add.description')}
      wide
      footer={
        <>
          <button className="btn-ghost" onClick={onClose}>
            {tc('cancel')}
          </button>
          <button
            className="btn-primary"
            disabled={busy || !token || (multiple && !accountId)}
            onClick={() => void submit()}
          >
            {busy && <Spinner className="h-4 w-4" />} {t('add.submit')}
          </button>
        </>
      }
    >
      {error && <ErrorBox message={error} />}
      <Field label={t('fields.display_name')} hint={t('fields.display_name_hint')}>
        <input className="field" value={name} onChange={(e) => setName(e.target.value)} placeholder="Production" />
      </Field>
      <Field label={t('fields.api_token')} error={fields.api_token}>
        <input
          className="field font-mono text-xs"
          value={token}
          onChange={(e) => setToken(e.target.value)}
          placeholder={t('fields.token_placeholder')}
          autoComplete="off"
        />
      </Field>

      {multiple ? (
        <Field label={t('fields.account')} hint={t('fields.account_multiple_hint')} error={fields.account_id}>
          <select className="field" value={accountId} onChange={(e) => setAccountId(e.target.value)}>
            <option value="">{t('fields.account_select')}</option>
            {choices?.map((c) => (
              <option key={c.id} value={c.id} disabled={c.already_added}>
                {c.name}
                {c.already_added ? ` — ${t('fields.account_already_added')}` : ` · ${c.id}`}
              </option>
            ))}
          </select>
        </Field>
      ) : choices?.length === 1 ? (
        <Field label={t('fields.account')} hint={t('fields.account_detected')}>
          <div className="flex flex-wrap items-center gap-x-2 gap-y-1 rounded-lg border border-surface-700 bg-surface-950/50 px-3 py-2 text-xs">
            <span className="font-medium text-slate-200">{choices[0].name}</span>
            <code className="font-mono text-[11px] text-slate-500">{choices[0].id}</code>
          </div>
        </Field>
      ) : (
        <Field
          label={t('fields.account_id')}
          hint={probing ? t('fields.account_id_checking') : probeError || t('fields.account_id_hint')}
          error={fields.account_id}
        >
          <div className="flex items-center gap-2">
            <input
              className="field font-mono text-xs"
              value={accountId}
              onChange={(e) => setAccountId(e.target.value)}
              placeholder={t('fields.account_id_placeholder')}
            />
            {probing && <Spinner className="h-4 w-4 shrink-0 text-slate-500" />}
          </div>
        </Field>
      )}

      <div className="rounded-lg border border-surface-700 bg-surface-950/50 p-3 text-xs text-slate-400">
        <p className="font-medium text-slate-300">{t('permissions.title')}</p>
        <ul className="mt-1 list-inside list-disc space-y-0.5">
          <li>
            <span className="text-slate-300">{t('permissions.account_policy')}</span> → {t('permissions.tunnel_edit')}
          </li>
          <li>
            <span className="text-slate-300">{t('permissions.account_policy')}</span> → {t('permissions.settings_read')}
          </li>
          <li>
            <span className="text-slate-300">{t('permissions.zone_policy')}</span> → {t('permissions.zone_read')}
          </li>
          <li>
            <span className="text-slate-300">{t('permissions.zone_policy')}</span> → {t('permissions.dns_edit')}
          </li>
        </ul>
      </div>
      <ApiTokenGuide />
    </Modal>
  );
}

function ImportTunnelModal({ open, onClose, onDone }: { open: boolean; onClose: () => void; onDone: () => void }) {
  const { t } = useTranslation('accounts');
  const { t: tc } = useTranslation('common');
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
      toast.push('success', t('import.success', { name: result.tunnel.name }));
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
      title={t('import.title')}
      description={t('import.description')}
      footer={
        <>
          <button className="btn-ghost" onClick={onClose}>
            {tc('cancel')}
          </button>
          <button className="btn-primary" disabled={busy || !token} onClick={() => void submit()}>
            {busy && <Spinner className="h-4 w-4" />} {t('import.submit')}
          </button>
        </>
      }
    >
      {error && <ErrorBox message={error} />}
      <Field label={t('fields.tunnel_name')} hint={t('fields.tunnel_name_hint')}>
        <input className="field" value={name} onChange={(e) => setName(e.target.value)} placeholder="laptop-tunnel" />
      </Field>
      <Field label={t('fields.tunnel_token')} hint={t('fields.tunnel_token_hint')}>
        <textarea className="field h-24 font-mono text-xs" value={token} onChange={(e) => setToken(e.target.value)} />
      </Field>
      <Field label={t('fields.device_label')} hint={t('fields.device_label_hint')}>
        <input
          className="field"
          value={hostLabel}
          onChange={(e) => setHostLabel(e.target.value)}
          placeholder="office-nas"
        />
      </Field>
      <div className="rounded-lg border border-surface-700 bg-surface-950/50 p-3 text-xs text-slate-400">
        <p className="font-medium text-slate-300">{t('import.where_title')}</p>
        <p className="mt-1.5">{t('import.where_body')}</p>
        <p className="mt-1.5">{t('import.where_body_existing')}</p>
      </div>
      <div className="rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-xs text-amber-200">
        {t('import.remote_warning')}
      </div>
    </Modal>
  );
}
