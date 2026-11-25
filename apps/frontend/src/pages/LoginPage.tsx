import { useState } from 'react';
import { Navigate, useNavigate } from 'react-router-dom';
import { Wifi } from 'lucide-react';
import { apiMessage } from '../api/client';
import { ErrorBox, Field, Spinner } from '../components/ui';
import { useTranslation } from '../i18n/useTranslation';
import { useAuth } from '../store/auth-context';

export default function LoginPage() {
  const { t } = useTranslation('login');
  const { user, login } = useAuth();
  const navigate = useNavigate();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  if (user) return <Navigate to="/" replace />;

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    setBusy(true);
    try {
      await login(email, password);
      navigate('/', { replace: true });
    } catch (err) {
      setError(apiMessage(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="flex min-h-screen items-center justify-center px-4">
      <div className="w-full max-w-sm">
        <div className="mb-6 flex flex-col items-center gap-3 text-center">
          <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-accent-600/20 text-accent-400">
            <Wifi className="h-6 w-6" />
          </div>
          <div>
            <h1 className="text-lg font-semibold text-white">{t('panel_title')}</h1>
            <p className="text-sm text-slate-400">{t('panel_subtitle')}</p>
          </div>
        </div>

        <form onSubmit={submit} className="card space-y-4 p-6">
          {error && <ErrorBox message={error} />}
          <Field label={t('email')}>
            <input
              className="field"
              type="email"
              autoComplete="username"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              required
            />
          </Field>
          <Field label={t('password')}>
            <input
              className="field"
              type="password"
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
            />
          </Field>
          <button type="submit" className="btn-primary w-full" disabled={busy}>
            {busy ? <Spinner className="h-4 w-4" /> : null}
            {t('submit')}
          </button>
        </form>
      </div>
    </div>
  );
}
