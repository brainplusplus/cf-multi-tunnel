export type Role = 'admin' | 'user';

export interface User {
  id: string;
  email: string;
  name: string;
  role: Role;
  is_active: boolean;
  created_at: string;
  // Set by /api/auth/me while an admin is impersonating this account.
  impersonated_by?: string;
}

export interface AuthResult {
  access_token: string;
  refresh_token: string;
  expires_in: number;
  user: User;
}

export interface ImpersonationResult {
  access_token: string;
  expires_in: number;
  user: User;
  original_user: User;
}

/** The admin session a browser must return to when impersonation ends. */
export interface ImpersonationSnapshot {
  access_token: string;
  refresh_token: string;
  user: User;
}

export type Credential = 'api' | 'token_import';

export interface CFAccount {
  id: string;
  owner_id: string;
  name: string;
  account_id: string;
  account_name: string;
  credential: Credential;
  token_hint: string;
  status: 'active' | 'invalid' | 'error';
  status_message: string;
  last_checked_at: string | null;
  created_at: string;
}

// One Cloudflare account a token can reach, returned by the dry-run probe that
// powers the account combobox in the add-account form.
export interface AccountChoice {
  id: string;
  name: string;
  already_added: boolean;
}

export interface CFZone {
  id: string;
  cf_account_id: string;
  zone_id: string;
  name: string;
  status: string;
  synced_at: string;
}

// A zone plus its account, returned by the DNS page so one dropdown can group
// domains per account without a request per account.
export interface DNSZone {
  id: string;
  zone_id: string;
  name: string;
  status: string;
  cf_account_id: string;
  account_name: string;
  has_api: boolean;
  synced_at: string;
}

// A live Cloudflare record. display_type is "Tunnel" when a CNAME points at
// <tunnel-id>.cfargotunnel.com, which is how the Cloudflare dashboard labels it.
export interface DNSRecord {
  id: string;
  type: string;
  name: string;
  content: string;
  proxied: boolean;
  proxiable: boolean;
  ttl: number;
  priority?: number | null;
  comment: string;
  tags?: string[] | null;
  created_on: string;
  modified_on: string;
  data?: {
    priority?: number | null;
    weight?: number | null;
    port?: number | null;
    target?: string;
    flags?: number | null;
    tag?: string;
    value?: string;
  } | null;
  zone_id: string;
  zone_name: string;
  display_type: string;
  managed: boolean;
  tunnel_id?: string;
  tunnel_name?: string;
  app_id?: string;
  hostname?: string;
}

export interface DNSRecordPayload {
  type: string;
  name?: string;
  content?: string;
  ttl: number;
  proxied?: boolean;
  priority?: number;
  comment?: string;
  service?: string;
  protocol?: string;
  weight?: number;
  port?: number;
  target?: string;
  flags?: number;
  tag?: string;
  value?: string;
}

export interface DNSSyncResult {
  accounts: number;
  zones: number;
  failed: string[];
}

export type ConnectorMode = 'docker' | 'external';
export type TunnelStatus = 'running' | 'stopped' | 'unknown' | 'error';
export type TunnelLocation = 'local' | 'remote';

export interface Tunnel {
  id: string;
  owner_id: string;
  cf_account_id: string;
  tunnel_id: string;
  name: string;
  credential: Credential;
  config_src: string;
  location: TunnelLocation;
  host_label: string;
  connector_mode: ConnectorMode;
  desired_running: boolean;
  status: TunnelStatus;
  status_message: string;
  image_ref: string;
  image_version: string;
  last_seen_at: string | null;
  created_at: string;
  // Number of published apps routed through this tunnel, filled by the list
  // endpoint so the table can show it without one request per row.
  apps_count: number;
}

export type SyncStatus = 'synced' | 'pending' | 'error' | 'manual';

export interface PublishedApp {
  id: string;
  owner_id: string;
  tunnel_id: string;
  kind: 'app' | 'panel';
  hostname: string;
  path: string;
  service_url: string;
  zone_id: string;
  zone_name: string;
  dns_record_id: string;
  manage_dns: boolean;
  proxied: boolean;
  // Origin options stored for this app (e.g. { noTLSVerify: true }). The edit
  // form reads it so saving never silently drops what was configured.
  origin_request?: Record<string, unknown>;
  enabled: boolean;
  sync_status: SyncStatus;
  last_error: string;
  created_at: string;
}

export interface PanelAddress {
  public_url: string;
  internal_url: string;
}

export interface DashboardStats {
  accounts: number;
  tunnels: number;
  tunnels_running: number;
  tunnels_error: number;
  local_tunnels: number;
  local_running: number;
  remote_tunnels: number;
  remote_connected: number;
  apps: number;
  apps_synced: number;
  apps_error: number;
  users: number;
  connector_mode: string;
  docker_available: boolean;
}

export interface ImageInfo {
  image: string;
  version: string;
  digest: string;
  checked_at: string;
  available: boolean;
  error?: string;
}

export interface ConnectorStatus {
  mode: ConnectorMode;
  docker_available: boolean;
  docker_version: string;
  image: ImageInfo;
  latest_version: string;
  update_available: boolean;
  managed_tunnels: number;
  running_tunnels: number;
  running_versions: { tunnel_id: string; name: string; version: string; image_ref: string }[];
  last_checked_at: string;
}

// What one account sync changed. Used to tell the user their existing tunnels
// and hostnames came along instead of leaving them to find out by hand.
export interface ReconcileResult {
  tunnels: number;
  tunnels_created: number;
  tunnels_updated: number;
  tunnels_missing: number;
  apps: number;
  apps_created: number;
  apps_updated: number;
  skipped?: string[];
}

export interface RemoteTag {
  name: string;
  updated_at: string;
  latest: boolean;
}

export interface AuditLog {
  id: string;
  actor_id: string;
  actor_email: string;
  action: string;
  entity: string;
  entity_id: string;
  detail: string;
  ip: string;
  created_at: string;
}

export interface ApiError {
  code: string;
  message: string;
  fields?: Record<string, string>;
}
