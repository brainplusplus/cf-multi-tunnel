import { useState } from 'react';
import { KeyRound } from 'lucide-react';
import { apiFields, apiMessage } from '../api/client';
import { authApi } from '../api/endpoints';
import { ErrorBox, Field, PageHeader, Spinner } from '../components/ui';
import { useToast } from '../components/Toast';
import { useAuth } from '../store/auth-context';

export default function ChangePasswordPage() {
  const { user } = useAuth();
  const toast = useToast();
  const [current, setCurrent] = useState('');
  const [next, setNext] = useState('');
  const [confirm, setConfirm] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [fields, setFields] = useState<Record<string, string>>({});

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    setFields({});
    if (next !== confirm) {
      setFields({ confirm_password: 'does not match' });
      return;
    }
    setBusy(true);
    try {
      await authApi.changePassword(current, next);
      toast.push('success', 'Password changed. Other sessions keep working until their token expires.');
      setCurrent('');
      setNext('');
      setConfirm('');
    } catch (err) {
      setError(apiMessage(err));
      setFields(apiFields(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="max-w-lg">
      <PageHeader title="Account" description="Your sign-in details for this panel." />

      <div className="card mb-4 p-4">
        <p className="text-sm text-slate-200">{user?.name}</p>
        <p className="text-xs text-slate-500">{user?.email}</p>
        <p className="mt-1 text-[11px] uppercase tracking-wider text-slate-500">role: {user?.role}</p>
      </div>

      <form onSubmit={submit} className="card space-y-4 p-5">
        <div className="flex items-center gap-2 text-sm font-semibold text-white">
          <KeyRound className="h-4 w-4" /> Change password
        </div>
        {error && <ErrorBox message={error} />}
        <Field label="Current password" error={fields.current_password}>
          <input
            className="field"
            type="password"
            autoComplete="current-password"
            value={current}
            onChange={(e) => setCurrent(e.target.value)}
            required
          />
        </Field>
        <Field label="New password" hint="At least 8 characters." error={fields.new_password}>
          <input
            className="field"
            type="password"
            autoComplete="new-password"
            value={next}
            onChange={(e) => setNext(e.target.value)}
            required
            minLength={8}
          />
        </Field>
        <Field label="Repeat new password" error={fields.confirm_password}>
          <input
            className="field"
            type="password"
            autoComplete="new-password"
            value={confirm}
            onChange={(e) => setConfirm(e.target.value)}
            required
          />
        </Field>
        <button type="submit" className="btn-primary" disabled={busy}>
          {busy && <Spinner className="h-4 w-4" />} Update password
        </button>
      </form>
    </div>
  );
}
