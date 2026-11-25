import { useCallback, useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Plus, Trash2, UserPlus, UserRoundCog } from 'lucide-react';
import { apiFields, apiMessage } from '../api/client';
import { usersApi } from '../api/endpoints';
import { DataTable, type Column } from '../components/DataTable';
import { FilterToggle, QueryBuilder, filterCount } from '../components/QueryBuilder';
import { createGroup, matchesQuery, type FieldDefinition, type QueryGroup } from '../components/query';
import { Badge, ErrorBox, Field, Modal, PageHeader, Spinner } from '../components/ui';
import { useToast } from '../components/Toast';
import { useTranslation } from '../i18n/useTranslation';
import { useAuth } from '../store/auth-context';
import type { User } from '../types';

export default function UsersPage() {
  const { t } = useTranslation('users');
  const { t: tc } = useTranslation('common');
  const toast = useToast();
  const navigate = useNavigate();
  const { user: me, startImpersonation } = useAuth();
  const [users, setUsers] = useState<User[] | null>(null);
  const [total, setTotal] = useState(0);
  const [error, setError] = useState('');
  const [search, setSearch] = useState('');
  const [showFilters, setShowFilters] = useState(false);
  const [filters, setFilters] = useState<QueryGroup>(() => createGroup());
  const [modal, setModal] = useState(false);
  const [editing, setEditing] = useState<User | null>(null);
  const [impersonatingId, setImpersonatingId] = useState('');

  const load = useCallback(async () => {
    try {
      // The table pages and filters on the client, and the API caps a page at
      // 100. Ask for that cap and say so when there are more, instead of
      // silently hiding the rest.
      const result = await usersApi.list('', 1, 100);
      setUsers(result.items);
      setTotal(result.total);
      setError('');
    } catch (err) {
      setError(apiMessage(err));
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const toggleActive = async (user: User) => {
    try {
      await usersApi.update(user.id, { is_active: !user.is_active });
      toast.push('success', t(user.is_active ? 'actions.disabled_toast' : 'actions.enabled_toast'));
      void load();
    } catch (err) {
      toast.push('error', apiMessage(err));
    }
  };

  const remove = async (user: User) => {
    if (!window.confirm(t('actions.delete_confirm', { email: user.email }))) return;
    try {
      await usersApi.remove(user.id);
      toast.push('success', t('actions.deleted_toast'));
      void load();
    } catch (err) {
      toast.push('error', apiMessage(err));
    }
  };

  // An impersonated session is no longer an admin, so it cannot stay on this
  // page. Navigating to the dashboard also proves the role really changed.
  const impersonate = async (user: User) => {
    if (!window.confirm(t('impersonate.confirm', { name: user.name || user.email }))) return;
    setImpersonatingId(user.id);
    try {
      await startImpersonation(user.id);
      toast.push('success', t('impersonate.success', { name: user.name || user.email }));
      navigate('/');
    } catch (err) {
      toast.push('error', `${t('impersonate.failed')}: ${apiMessage(err)}`);
    } finally {
      setImpersonatingId('');
    }
  };

  const fields: FieldDefinition[] = useMemo(
    () => [
      { name: 'name', label: t('filter.name'), type: 'string' },
      { name: 'email', label: t('filter.email'), type: 'string' },
      {
        name: 'role',
        label: t('filter.role'),
        type: 'select',
        options: [
          { value: 'admin', label: 'admin' },
          { value: 'user', label: 'user' },
        ],
      },
      {
        name: 'status',
        label: t('filter.active'),
        type: 'select',
        options: [
          { value: t('filter.active_yes'), label: t('filter.active_yes') },
          { value: t('filter.active_no'), label: t('filter.active_no') },
        ],
      },
    ],
    [t],
  );

  // The table renders a derived `status` string so a filter can select it; the
  // raw is_active boolean stays the source of truth everywhere else.
  const rows = useMemo(() => {
    const term = search.trim().toLowerCase();
    return (users ?? [])
      .map((user) => ({ ...user, status: user.is_active ? t('filter.active_yes') : t('filter.active_no') }))
      .filter((user) => {
        if (term && !`${user.name} ${user.email}`.toLowerCase().includes(term)) return false;
        return matchesQuery(user as unknown as Record<string, unknown>, filters);
      });
  }, [users, search, filters, t]);

  const columns: Column<User & { status: string }>[] = [
    {
      key: 'name',
      label: t('columns.name'),
      sortable: true,
      minWidth: 180,
      render: (user) => (
        <div>
          <p className="font-medium text-slate-100">
            {user.name}
            {user.id === me?.id && <span className="ml-2 text-[11px] text-slate-500">{t('you')}</span>}
          </p>
          <p className="text-[11px] text-slate-500">{user.email}</p>
        </div>
      ),
    },
    {
      key: 'email',
      label: t('columns.email'),
      sortable: true,
      hiddenByDefault: true,
    },
    {
      key: 'role',
      label: t('columns.role'),
      sortable: true,
      width: 110,
      render: (user) => <Badge tone={user.role === 'admin' ? 'violet' : 'slate'}>{user.role}</Badge>,
    },
    {
      key: 'status',
      label: t('columns.status'),
      sortable: true,
      width: 120,
      render: (user) => (
        <button onClick={() => void toggleActive(user)} disabled={user.id === me?.id}>
          <Badge tone={user.is_active ? 'green' : 'red'} dot>
            {user.is_active ? t('status.active') : t('status.inactive')}
          </Badge>
        </button>
      ),
    },
    {
      key: 'created_at',
      label: t('columns.created'),
      sortable: true,
      width: 130,
      render: (user) => (
        <span className="text-xs text-slate-500">{new Date(user.created_at).toLocaleDateString()}</span>
      ),
    },
    {
      key: 'actions',
      label: t('columns.impersonate'),
      width: 240,
      render: (user) => (
        <div className="flex justify-end gap-1.5">
          <button
            className="btn-ghost !px-2 !py-1 !text-xs"
            disabled={user.id === me?.id || !user.is_active || impersonatingId === user.id}
            title={
              user.id === me?.id
                ? t('impersonate.self')
                : t('impersonate.label')
            }
            onClick={() => void impersonate(user)}
          >
            {impersonatingId === user.id ? (
              <Spinner className="h-3.5 w-3.5" />
            ) : (
              <UserRoundCog className="h-3.5 w-3.5" />
            )}
            {t('impersonate.label')}
          </button>
          <button
            className="btn-ghost !px-2 !py-1 !text-xs"
            onClick={() => {
              setEditing(user);
              setModal(true);
            }}
          >
            {tc('edit')}
          </button>
          <button
            className="btn-danger !px-2 !py-1 !text-xs"
            disabled={user.id === me?.id}
            onClick={() => void remove(user)}
          >
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
          <button
            className="btn-primary"
            onClick={() => {
              setEditing(null);
              setModal(true);
            }}
          >
            <Plus className="h-4 w-4" /> {t('new_user')}
          </button>
        }
      />

      {error && <ErrorBox message={error} />}

      <div className="card">
        <div className="mb-3 flex flex-wrap items-center gap-2">
          <input
            className="field !w-64 !py-1.5 text-xs"
            placeholder={t('search_placeholder')}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
          <FilterToggle
            open={showFilters}
            count={filterCount(filters)}
            onClick={() => setShowFilters((v) => !v)}
          />
          <span className="text-[11px] text-slate-500">{tc('rows_found', { count: rows.length })}</span>
          {total > (users?.length ?? 0) && (
            <span className="text-[11px] text-amber-300/80">{t('truncated', { shown: users?.length ?? 0, total })}</span>
          )}
        </div>

        {showFilters && (
          <div className="mb-3">
            <QueryBuilder fields={fields} value={filters} onChange={setFilters} />
          </div>
        )}

        {!users ? (
          <div className="flex justify-center py-16 text-slate-400">
            <Spinner className="h-6 w-6" />
          </div>
        ) : (
          <DataTable
            tableId="users"
            data={rows}
            columns={columns}
            keyField="id"
            emptyState={
              <div className="py-10 text-center">
                <UserPlus className="mx-auto mb-2 h-7 w-7 text-slate-600" />
                <p className="text-sm text-slate-500">{t('no_users')}</p>
              </div>
            }
            initialSort={{ field: 'created_at', direction: 'desc' }}
          />
        )}
      </div>

      <UserModal
        open={modal}
        user={editing}
        onClose={() => setModal(false)}
        onDone={() => {
          setModal(false);
          void load();
        }}
      />
    </div>
  );
}

function UserModal({
  open,
  user,
  onClose,
  onDone,
}: {
  open: boolean;
  user: User | null;
  onClose: () => void;
  onDone: () => void;
}) {
  const { t } = useTranslation('users');
  const { t: tc } = useTranslation('common');
  const toast = useToast();
  const [name, setName] = useState('');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [role, setRole] = useState('user');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [fields, setFields] = useState<Record<string, string>>({});

  useEffect(() => {
    if (!open) return;
    setName(user?.name ?? '');
    setEmail(user?.email ?? '');
    setRole(user?.role ?? 'user');
    setPassword('');
    setError('');
    setFields({});
  }, [open, user]);

  const submit = async () => {
    setBusy(true);
    setError('');
    setFields({});
    try {
      if (user) {
        await usersApi.update(user.id, {
          name,
          role,
          ...(password ? { password } : {}),
        });
        toast.push('success', t('user_updated'));
      } else {
        await usersApi.create({ name, email, password, role });
        toast.push('success', t('user_created'));
      }
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
      title={user ? t('edit_user') : t('create_user')}
      footer={
        <>
          <button className="btn-ghost" onClick={onClose}>
            {tc('cancel')}
          </button>
          <button className="btn-primary" disabled={busy} onClick={() => void submit()}>
            {busy && <Spinner className="h-4 w-4" />} {user ? tc('save') : t('create')}
          </button>
        </>
      }
    >
      {error && <ErrorBox message={error} />}
      <Field label={t('fields.name')} error={fields.name}>
        <input className="field" value={name} onChange={(e) => setName(e.target.value)} />
      </Field>
      <Field label={t('fields.email')} error={fields.email}>
        <input className="field" type="email" value={email} disabled={!!user} onChange={(e) => setEmail(e.target.value)} />
      </Field>
      <Field
        label={user ? t('fields.new_password') : t('fields.password')}
        hint={user ? t('fields.keep_password') : t('fields.password_hint')}
        error={fields.password}
      >
        <input className="field" type="password" value={password} onChange={(e) => setPassword(e.target.value)} />
      </Field>
      <Field label={t('fields.role')} error={fields.role}>
        <select className="field" value={role} onChange={(e) => setRole(e.target.value)}>
          <option value="user">{t('fields.role_user')}</option>
          <option value="admin">{t('fields.role_admin')}</option>
        </select>
      </Field>
    </Modal>
  );
}
