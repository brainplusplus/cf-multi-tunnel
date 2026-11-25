import { useCallback, useEffect, useState } from 'react';
import { Plus, Trash2, UserPlus } from 'lucide-react';
import { apiFields, apiMessage } from '../api/client';
import { usersApi } from '../api/endpoints';
import { Badge, EmptyState, ErrorBox, Field, Modal, PageHeader, Spinner } from '../components/ui';
import { useToast } from '../components/Toast';
import { useAuth } from '../store/auth-context';
import type { User } from '../types';

export default function UsersPage() {
  const toast = useToast();
  const { user: me } = useAuth();
  const [users, setUsers] = useState<User[] | null>(null);
  const [error, setError] = useState('');
  const [search, setSearch] = useState('');
  const [modal, setModal] = useState(false);
  const [editing, setEditing] = useState<User | null>(null);

  const load = useCallback(async () => {
    try {
      const result = await usersApi.list(search);
      setUsers(result.items);
      setError('');
    } catch (err) {
      setError(apiMessage(err));
    }
  }, [search]);

  useEffect(() => {
    const timer = window.setTimeout(() => void load(), 250);
    return () => window.clearTimeout(timer);
  }, [load]);

  const toggleActive = async (user: User) => {
    try {
      await usersApi.update(user.id, { is_active: !user.is_active });
      toast.push('success', user.is_active ? 'User disabled' : 'User enabled');
      void load();
    } catch (err) {
      toast.push('error', apiMessage(err));
    }
  };

  const remove = async (user: User) => {
    if (!window.confirm(`Delete ${user.email}? This cannot be undone.`)) return;
    try {
      await usersApi.remove(user.id);
      toast.push('success', 'User deleted');
      void load();
    } catch (err) {
      toast.push('error', apiMessage(err));
    }
  };

  return (
    <div>
      <PageHeader
        title="Users"
        description="Admins manage everything. Users only ever see accounts and tunnels they own."
        actions={
          <>
            <input
              className="field !w-56 !py-1.5 text-xs"
              placeholder="Search email or name"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
            <button
              className="btn-primary"
              onClick={() => {
                setEditing(null);
                setModal(true);
              }}
            >
              <Plus className="h-4 w-4" /> New user
            </button>
          </>
        }
      />

      {error && <ErrorBox message={error} />}

      {!users ? (
        <div className="flex justify-center py-16 text-slate-400">
          <Spinner className="h-6 w-6" />
        </div>
      ) : users.length === 0 ? (
        <div className="card">
          <EmptyState icon={<UserPlus className="h-8 w-8" />} title="No users found" />
        </div>
      ) : (
        <div className="card overflow-hidden">
          <table className="w-full">
            <thead className="border-b border-surface-700/70 bg-surface-900/60">
              <tr>
                <th className="th">User</th>
                <th className="th">Role</th>
                <th className="th">Status</th>
                <th className="th">Created</th>
                <th className="th text-right">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-surface-800">
              {users.map((user) => (
                <tr key={user.id} className="hover:bg-surface-900/40">
                  <td className="td">
                    <p className="font-medium text-slate-100">
                      {user.name}
                      {user.id === me?.id && <span className="ml-2 text-[11px] text-slate-500">you</span>}
                    </p>
                    <p className="text-[11px] text-slate-500">{user.email}</p>
                  </td>
                  <td className="td">
                    <Badge tone={user.role === 'admin' ? 'violet' : 'slate'}>{user.role}</Badge>
                  </td>
                  <td className="td">
                    <button onClick={() => void toggleActive(user)} disabled={user.id === me?.id}>
                      <Badge tone={user.is_active ? 'green' : 'red'} dot>
                        {user.is_active ? 'active' : 'disabled'}
                      </Badge>
                    </button>
                  </td>
                  <td className="td text-xs text-slate-500">{new Date(user.created_at).toLocaleDateString()}</td>
                  <td className="td">
                    <div className="flex justify-end gap-1.5">
                      <button
                        className="btn-ghost !px-2 !py-1 !text-xs"
                        onClick={() => {
                          setEditing(user);
                          setModal(true);
                        }}
                      >
                        Edit
                      </button>
                      <button
                        className="btn-danger !px-2 !py-1 !text-xs"
                        disabled={user.id === me?.id}
                        onClick={() => void remove(user)}
                      >
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
        toast.push('success', 'User updated');
      } else {
        await usersApi.create({ name, email, password, role });
        toast.push('success', 'User created');
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
      title={user ? 'Edit user' : 'Create user'}
      footer={
        <>
          <button className="btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button className="btn-primary" disabled={busy} onClick={() => void submit()}>
            {busy && <Spinner className="h-4 w-4" />} {user ? 'Save' : 'Create'}
          </button>
        </>
      }
    >
      {error && <ErrorBox message={error} />}
      <Field label="Name" error={fields.name}>
        <input className="field" value={name} onChange={(e) => setName(e.target.value)} />
      </Field>
      <Field label="Email" error={fields.email}>
        <input className="field" type="email" value={email} disabled={!!user} onChange={(e) => setEmail(e.target.value)} />
      </Field>
      <Field
        label={user ? 'New password' : 'Password'}
        hint={user ? 'Leave empty to keep the current password.' : 'At least 8 characters.'}
        error={fields.password}
      >
        <input className="field" type="password" value={password} onChange={(e) => setPassword(e.target.value)} />
      </Field>
      <Field label="Role" error={fields.role}>
        <select className="field" value={role} onChange={(e) => setRole(e.target.value)}>
          <option value="user">user — manages own accounts only</option>
          <option value="admin">admin — manages everything</option>
        </select>
      </Field>
    </Modal>
  );
}
