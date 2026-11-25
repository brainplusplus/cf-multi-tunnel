import { NavLink, useNavigate } from 'react-router-dom';
import {
  Cloud,
  Globe2,
  LayoutDashboard,
  LogOut,
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

const navItems = [
  { to: '/', label: 'Dashboard', icon: LayoutDashboard, admin: false },
  { to: '/accounts', label: 'Accounts', icon: Cloud, admin: false },
  { to: '/tunnels', label: 'Tunnels', icon: Waypoints, admin: false },
  { to: '/apps', label: 'Published apps', icon: Globe2, admin: false },
  { to: '/users', label: 'Users', icon: Users, admin: true },
  { to: '/settings', label: 'Settings', icon: Settings, admin: true },
  { to: '/audit', label: 'Audit log', icon: ScrollText, admin: true },
];

export default function AppShell({ children }: { children: ReactNode }) {
  const { user, logout } = useAuth();
  const navigate = useNavigate();

  const handleLogout = async () => {
    await logout();
    navigate('/login', { replace: true });
  };

  return (
    <div className="min-h-screen lg:grid lg:grid-cols-[248px_1fr]">
      <aside className="border-b border-surface-700/70 bg-surface-900/60 lg:sticky lg:top-0 lg:h-screen lg:border-b-0 lg:border-r">
        <div className="flex items-center gap-3 px-5 py-5">
          <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-accent-600/20 text-accent-400">
            <Wifi className="h-5 w-5" />
          </div>
          <div>
            <p className="text-sm font-semibold text-white">CF Tunnel</p>
            <p className="text-[11px] uppercase tracking-wider text-slate-500">Control panel</p>
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
                {label}
              </NavLink>
            ))}
        </nav>

        <div className="hidden border-t border-surface-700/60 p-4 lg:block">
          <p className="truncate text-sm text-slate-200">{user?.name}</p>
          <p className="truncate text-xs text-slate-500">{user?.email}</p>
          <div className="mt-2 flex items-center justify-between">
            <Badge tone={user?.role === 'admin' ? 'violet' : 'slate'}>{user?.role}</Badge>
            <div className="flex items-center gap-1">
              <NavLink
                to="/account"
                className="rounded-md p-1.5 text-slate-400 hover:bg-surface-800 hover:text-white"
                title="Account settings"
              >
                <UserCog className="h-4 w-4" />
              </NavLink>
              <button onClick={handleLogout} className="rounded-md p-1.5 text-slate-400 hover:bg-surface-800 hover:text-white" title="Sign out">
                <LogOut className="h-4 w-4" />
              </button>
            </div>
          </div>
        </div>
      </aside>

      <main className="min-w-0">
        <div className="mx-auto max-w-6xl px-4 py-6 sm:px-6 lg:py-8">{children}</div>
      </main>
    </div>
  );
}
