import client from './client';
import type {
  AccountChoice,
  AuditLog,
  AuthResult,
  CFAccount,
  CFZone,
  ConnectorStatus,
  DashboardStats,
  PanelAddress,
  PublishedApp,
  ReconcileResult,
  RemoteTag,
  Tunnel,
  User,
} from '../types';

// ── Auth ────────────────────────────────────────────────────────────
export const authApi = {
  login: (email: string, password: string) =>
    client.post<AuthResult>('/api/auth/login', { email, password }).then((r) => r.data),
  logout: (refreshToken: string) => client.post('/api/auth/logout', { refresh_token: refreshToken }),
  me: () => client.get<User>('/api/auth/me').then((r) => r.data),
  changePassword: (current: string, next: string) =>
    client.post('/api/auth/change-password', { current_password: current, new_password: next }),
};

// ── Dashboard ───────────────────────────────────────────────────────
export const dashboardApi = {
  stats: () => client.get<DashboardStats>('/api/dashboard/stats').then((r) => r.data),
};

// ── Accounts ────────────────────────────────────────────────────────
export const accountsApi = {
  list: () => client.get<CFAccount[]>('/api/cf-accounts').then((r) => r.data),
  get: (id: string) => client.get<CFAccount>(`/api/cf-accounts/${id}`).then((r) => r.data),
  create: (payload: { name?: string; api_token: string; account_id?: string }) =>
    client.post<CFAccount>('/api/cf-accounts', payload).then((r) => r.data),
  // Dry run: verifies the token and lists the accounts it can reach, so the form
  // can offer a combobox instead of a raw account id.
  probe: (apiToken: string) =>
    client
      .post<{ accounts: AccountChoice[] }>('/api/cf-accounts/probe', { api_token: apiToken })
      .then((r) => r.data.accounts),
  importTunnel: (payload: {
    name?: string;
    tunnel_token: string;
    location?: string;
    host_label?: string;
    connector_mode?: string;
  }) =>
    client
      .post<{ account: CFAccount; tunnel: Tunnel }>('/api/cf-accounts/import-tunnel', payload)
      .then((r) => r.data),
  rename: (id: string, name: string) =>
    client.patch<CFAccount>(`/api/cf-accounts/${id}`, { name }).then((r) => r.data),
  attachToken: (id: string, apiToken: string) =>
    client.post<CFAccount>(`/api/cf-accounts/${id}/attach-token`, { api_token: apiToken }).then((r) => r.data),
  verify: (id: string) => client.post<CFAccount>(`/api/cf-accounts/${id}/verify`).then((r) => r.data),
  remove: (id: string) => client.delete(`/api/cf-accounts/${id}`),
  zones: (id: string) => client.get<CFZone[]>(`/api/cf-accounts/${id}/zones`).then((r) => r.data),
  syncZones: (id: string) => client.post<CFZone[]>(`/api/cf-accounts/${id}/sync-zones`).then((r) => r.data),
};

// ── Tunnels ─────────────────────────────────────────────────────────
export const tunnelsApi = {
  list: (accountId?: string, location?: string) =>
    client
      .get<Tunnel[]>('/api/tunnels', {
        params: {
          ...(accountId ? { account_id: accountId } : {}),
          ...(location ? { location } : {}),
        },
      })
      .then((r) => r.data),
  get: (id: string) => client.get<Tunnel>(`/api/tunnels/${id}`).then((r) => r.data),
  create: (payload: {
    account_id: string;
    name: string;
    location?: string;
    host_label?: string;
    connector_mode?: string;
  }) => client.post<Tunnel>('/api/tunnels', payload).then((r) => r.data),
  update: (id: string, payload: { location?: string; host_label?: string; connector_mode?: string }) =>
    client.patch<Tunnel>(`/api/tunnels/${id}`, payload).then((r) => r.data),
  sync: (accountId: string) =>
    client
      .post<{ result: ReconcileResult; tunnels: Tunnel[] }>('/api/tunnels/sync', { account_id: accountId })
      .then((r) => r.data),
  remove: (id: string) => client.delete(`/api/tunnels/${id}`),
  start: (id: string) => client.post<Tunnel>(`/api/tunnels/${id}/start`).then((r) => r.data),
  stop: (id: string) => client.post<Tunnel>(`/api/tunnels/${id}/stop`).then((r) => r.data),
  logs: (id: string, lines = 200) =>
    client.get<{ logs: string }>(`/api/tunnels/${id}/logs`, { params: { lines } }).then((r) => r.data.logs),
  token: (id: string) => client.get<{ token: string }>(`/api/tunnels/${id}/token`).then((r) => r.data.token),
  configYml: (id: string) =>
    client.get<{ config: string }>(`/api/tunnels/${id}/config.yml`).then((r) => r.data.config),
  credentialsJson: (id: string) =>
    client.get<{ credentials: string }>(`/api/tunnels/${id}/credentials.json`).then((r) => r.data.credentials),
  apps: (id: string) => client.get<PublishedApp[]>(`/api/tunnels/${id}/apps`).then((r) => r.data),
  createApp: (id: string, payload: AppPayload) =>
    client.post<PublishedApp>(`/api/tunnels/${id}/apps`, payload).then((r) => r.data),
};

export interface AppPayload {
  hostname?: string;
  subdomain?: string;
  domain?: string;
  path?: string;
  service_url: string;
  manage_dns?: boolean;
  proxied?: boolean;
  origin_request?: unknown;
}

// ── Apps ────────────────────────────────────────────────────────────
export const appsApi = {
  list: () => client.get<PublishedApp[]>('/api/apps').then((r) => r.data),
  get: (id: string) => client.get<PublishedApp>(`/api/apps/${id}`).then((r) => r.data),
  update: (id: string, payload: AppPayload) =>
    client.put<PublishedApp>(`/api/apps/${id}`, payload).then((r) => r.data),
  remove: (id: string) => client.delete(`/api/apps/${id}`),
  setEnabled: (id: string, enabled: boolean) =>
    client.patch<PublishedApp>(`/api/apps/${id}/enabled`, { enabled }).then((r) => r.data),
  retry: (id: string) => client.post<PublishedApp>(`/api/apps/${id}/retry`).then((r) => r.data),
};

// ── Panel exposure ──────────────────────────────────────────────────
export const panelApi = {
  get: () => client.get<PublishedApp | null>('/api/settings/panel-exposure').then((r) => r.data),
  address: () => client.get<PanelAddress>('/api/settings/panel-address').then((r) => r.data),
  set: (payload: { tunnel_id: string; hostname?: string; subdomain?: string; domain?: string; service_url?: string }) =>
    client.put<PublishedApp>('/api/settings/panel-exposure', payload).then((r) => r.data),
  remove: () => client.delete('/api/settings/panel-exposure'),
};

// ── Connector ───────────────────────────────────────────────────────
export const connectorApi = {
  status: (checkRemote = false) =>
    client
      .get<ConnectorStatus>('/api/admin/connector', { params: { check_remote: checkRemote } })
      .then((r) => r.data),
  tags: () => client.get<RemoteTag[]>('/api/admin/connector/tags').then((r) => r.data),
  update: (image: string, pin: boolean) =>
    client
      .post<{ image: { version: string }; recreated: string[]; failed: string[] }>('/api/admin/connector/update', {
        image,
        pin,
      })
      .then((r) => r.data),
};

// ── Admin ───────────────────────────────────────────────────────────
export const usersApi = {
  list: (search = '', page = 1, pageSize = 50) =>
    client
      .get<{ items: User[]; total: number }>('/api/admin/users', { params: { search, page, page_size: pageSize } })
      .then((r) => r.data),
  create: (payload: { email: string; password: string; name: string; role: string }) =>
    client.post<User>('/api/admin/users', payload).then((r) => r.data),
  update: (id: string, payload: Partial<{ name: string; role: string; is_active: boolean; password: string }>) =>
    client.put<User>(`/api/admin/users/${id}`, payload).then((r) => r.data),
  remove: (id: string) => client.delete(`/api/admin/users/${id}`),
};

export const auditApi = {
  list: (page = 1, pageSize = 50) =>
    client
      .get<{ items: AuditLog[]; total: number }>('/api/admin/audit-logs', { params: { page, page_size: pageSize } })
      .then((r) => r.data),
};
