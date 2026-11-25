import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { authApi } from '../api/endpoints';
import type { ImpersonationSnapshot, User } from '../types';

const SNAPSHOT_KEY = 'impersonation';

interface AuthContextValue {
  user: User | null;
  loading: boolean;
  /** The admin whose session is parked while an impersonation is active. */
  impersonating: ImpersonationSnapshot | null;
  login: (email: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  refreshUser: () => Promise<void>;
  startImpersonation: (userId: string) => Promise<void>;
  stopImpersonation: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | null>(null);

function readSnapshot(): ImpersonationSnapshot | null {
  try {
    const raw = localStorage.getItem(SNAPSHOT_KEY);
    return raw ? (JSON.parse(raw) as ImpersonationSnapshot) : null;
  } catch {
    return null;
  }
}

function clearLocalSession() {
  localStorage.removeItem('token');
  localStorage.removeItem('refresh_token');
  localStorage.removeItem('user');
  localStorage.removeItem(SNAPSHOT_KEY);
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(() => {
    const raw = localStorage.getItem('user');
    return raw ? (JSON.parse(raw) as User) : null;
  });
  const [impersonating, setImpersonating] = useState<ImpersonationSnapshot | null>(readSnapshot);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const token = localStorage.getItem('token');
    if (!token) {
      setLoading(false);
      return;
    }
    authApi
      .me()
      .then((me) => {
        setUser(me);
        localStorage.setItem('user', JSON.stringify(me));
      })
      .catch(() => {
        // An expired impersonation token must return to the admin, not sign the
        // operator out of a session they still hold.
        const snapshot = readSnapshot();
        if (snapshot) {
          localStorage.setItem('token', snapshot.access_token);
          localStorage.setItem('refresh_token', snapshot.refresh_token);
          localStorage.setItem('user', JSON.stringify(snapshot.user));
          localStorage.removeItem(SNAPSHOT_KEY);
          setImpersonating(null);
          setUser(snapshot.user);
          return;
        }
        clearLocalSession();
        setUser(null);
      })
      .finally(() => setLoading(false));
  }, []);

  const login = useCallback(async (email: string, password: string) => {
    const result = await authApi.login(email, password);
    localStorage.removeItem(SNAPSHOT_KEY);
    setImpersonating(null);
    localStorage.setItem('token', result.access_token);
    localStorage.setItem('refresh_token', result.refresh_token);
    localStorage.setItem('user', JSON.stringify(result.user));
    setUser(result.user);
  }, []);

  const logout = useCallback(async () => {
    const refreshToken = localStorage.getItem('refresh_token');
    if (refreshToken) {
      await authApi.logout(refreshToken).catch(() => undefined);
    }
    clearLocalSession();
    setImpersonating(null);
    setUser(null);
  }, []);

  const refreshUser = useCallback(async () => {
    const me = await authApi.me();
    setUser(me);
    localStorage.setItem('user', JSON.stringify(me));
  }, []);

  // The admin's own tokens are parked in localStorage rather than kept in
  // memory: a page reload while impersonating must not strand the session.
  const startImpersonation = useCallback(async (userId: string) => {
    const currentToken = localStorage.getItem('token');
    const currentRefresh = localStorage.getItem('refresh_token');
    const currentUser = localStorage.getItem('user');
    if (!currentToken || !currentRefresh || !currentUser) {
      throw new Error('no active session to return to');
    }

    const result = await authApi.impersonate(userId);
    localStorage.setItem(
      SNAPSHOT_KEY,
      JSON.stringify({
        access_token: currentToken,
        refresh_token: currentRefresh,
        user: JSON.parse(currentUser) as User,
      } satisfies ImpersonationSnapshot),
    );
    localStorage.setItem('token', result.access_token);
    // An impersonation token has no refresh pair; keeping the admin's refresh
    // token here would let a silent refresh escalate back to admin.
    localStorage.removeItem('refresh_token');
    localStorage.setItem('user', JSON.stringify(result.user));
    setImpersonating(readSnapshot());
    setUser(result.user);
  }, []);

  const stopImpersonation = useCallback(async () => {
    const snapshot = readSnapshot();
    await authApi.exitImpersonation().catch(() => undefined);
    if (!snapshot) {
      clearLocalSession();
      setUser(null);
      setImpersonating(null);
      return;
    }
    localStorage.setItem('token', snapshot.access_token);
    localStorage.setItem('refresh_token', snapshot.refresh_token);
    localStorage.setItem('user', JSON.stringify(snapshot.user));
    localStorage.removeItem(SNAPSHOT_KEY);
    setImpersonating(null);
    setUser(snapshot.user);
  }, []);

  const value = useMemo(
    () => ({ user, loading, impersonating, login, logout, refreshUser, startImpersonation, stopImpersonation }),
    [user, loading, impersonating, login, logout, refreshUser, startImpersonation, stopImpersonation],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth must be used inside AuthProvider');
  return ctx;
}
