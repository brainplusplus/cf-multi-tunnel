import { NavLink, useNavigate } from 'react-router-dom';
import {
  Cloud,
  Globe2,
  LayoutDashboard,
  LogOut,
  Network,
  ScrollText,
  Settings,
  UserCog,
  Users,
  Waypoints,
  Wifi,
} from 'lucide-react';
import type { ReactNode } from 'react';
import { useAuth } from '../store/auth-context';
import { Badge } from './ui';
import { useTranslation } from '../i18n/useTranslation';
import { useLanguage, type Language } from '../i18n/LanguageContext';

// `label` is a key in locales/nav.json, not a literal, so the sidebar follows
// the language switcher without every item carrying its own t() call.
const navItems = [
  { to: '/', label: 'dashboard', icon: LayoutDashboard, admin: false },
  { to: '/accounts', label: 'accounts', icon: Cloud, admin: false },
  { to: '/tunnels', label: 'tunnels', icon: Waypoints, admin: false },
  { to: '/apps', label: 'apps', icon: Globe2, admin: false },
  { to: '/dns', label: 'dns', icon: Network, admin: false },
  { to: '/users', label: 'users', icon: Users, admin: true },
  { to: '/settings', label: 'settings', icon: Settings, admin: true },
  { to: '/audit', label: 'audit', icon: ScrollText, admin: true },
];

const LANGUAGES: { value: Language; label: string }[] = [
  { value: 'id', label: 'ID' },
  { value: 'en', label: 'EN' },
];

export default function AppShell({ children }: { children: ReactNode }) {
  const { user, logout, impersonating, stopImpersonation } = useAuth();
  const { t } = useTranslation('nav');
  const { t: tc } = useTranslation('common');
  const { language, setLanguage } = useLanguage();
  const navigate = useNavigate();

  const handleLogout = async () => {
    await logout();
    navigate('/login', { replace: true });
  };

  const handleExitImpersonation = async () => {
    await stopImpersonation();
    navigate('/users', { replace: true });
  };

  return (
    <div className="min-h-screen lg:grid lg:grid-cols-[248px_1fr]">
      <aside className="border-b border-surface-700/70 bg-surface-900/60 lg:sticky lg:top-0 lg:h-screen lg:border-b-0 lg:border-r">
        <div className="flex items-center gap-3 px-5 py-5">
          <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-accent-600/20 text-accent-400">
            <Wifi className="h-5 w-5" />
          </div>
          <div>
            <p className="text-sm font-semibold text-white">{t('panel_title')}</p>
            <p className="text-[11px] uppercase tracking-wider text-slate-500">{t('panel_subtitle')}</p>
          </div>
        </div>

        <nav className="flex gap-1 overflow-x-auto px-3 pb-3 lg:flex-col lg:overflow-visible">
          {navItems
            .filter((item) => !item.admin || user?.role === 'admin')
            .map(({ to, label, icon: Icon }) => (
              <NavLink
                key={to}
                to={to}
                end={to === '/'}
                className={({ isActive }) =>
                  `flex shrink-0 items-center gap-2.5 rounded-lg px-3 py-2 text-sm transition-colors ${
                    isActive
                      ? 'bg-surface-700/80 text-white'
                      : 'text-slate-400 hover:bg-surface-800 hover:text-slate-200'
                  }`
                }
              >
                <Icon className="h-4 w-4" />
                {t(label)}
              </NavLink>
            ))}
        </nav>

        <div className="border-t border-surface-700/60 px-4 py-3">
          <div className="flex items-center justify-between">
            <span className="text-[11px] uppercase tracking-wider text-slate-500">{tc('language')}</span>
            <div className="inline-flex overflow-hidden rounded-md border border-surface-700">
              {LANGUAGES.map(({ value, label }) => (
                <button
                  key={value}
                  onClick={() => setLanguage(value)}
                  className={`px-2 py-0.5 text-[11px] ${
                    language === value ? 'bg-accent-600/25 text-accent-300' : 'text-slate-400 hover:text-slate-200'
                  }`}
                >
                  {label}
                </button>
              ))}
            </div>
          </div>
        </div>

        <div className="hidden border-t border-surface-700/60 p-4 lg:block">
          <p className="truncate text-sm text-slate-200">{user?.name}</p>
          <p className="truncate text-xs text-slate-500">{user?.email}</p>
          <div className="mt-2 flex items-center justify-between">
            <Badge tone={user?.role === 'admin' ? 'violet' : 'slate'}>{user?.role}</Badge>
            <div className="flex items-center gap-1">
              <NavLink
                to="/account"
                className="rounded-md p-1.5 text-slate-400 hover:bg-surface-800 hover:text-white"
                title={t('account')}
              >
                <UserCog className="h-4 w-4" />
              </NavLink>
              <button
                onClick={handleLogout}
                className="rounded-md p-1.5 text-slate-400 hover:bg-surface-800 hover:text-white"
                title={tc('sign_out')}
              >
                <LogOut className="h-4 w-4" />
              </button>
            </div>
          </div>
        </div>
      </aside>

      <main className="min-w-0">
        {/* The banner is the only thing that tells a page reload it is still
            impersonating; the token alone looks like a normal login. */}
        {impersonating && (
          <div className="border-b border-amber-500/30 bg-amber-500/10 px-4 py-2.5 text-amber-100 sm:px-6">
            <div className="mx-auto flex max-w-6xl flex-wrap items-center gap-3 text-xs">
              <Badge tone="amber">{t('viewing_as')}</Badge>
              <span className="font-medium">{user?.name || user?.email}</span>
              <span className="text-amber-200/80">{t('impersonation_hint')}</span>
              <button
                className="btn-ghost ml-auto !border-amber-500/40 !py-1 !text-xs !text-amber-200"
                onClick={() => void handleExitImpersonation()}
              >
                {t('exit_impersonation')}
              </button>
            </div>
          </div>
        )}
        <div className="mx-auto max-w-6xl px-4 py-6 sm:px-6 lg:py-8">{children}</div>
      </main>
    </div>
  );
}
