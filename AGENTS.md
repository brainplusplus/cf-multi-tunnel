# AGENTS.md

Agent-facing notes for this repository. `README.md` is for humans; this file records
the invariants that are easy to break and the commands that actually verify a change.

## Layout

```
apps/backend        Go 1.26 + Echo + GORM + SQLite (module cf-multi-tunnel/backend)
apps/reverse-proxy  Go SPA + /api reverse proxy (module cf-multi-tunnel/reverse-proxy)
apps/frontend       React 19 + Vite 6 + Tailwind 4 (bun)
scripts             build.mjs / run.mjs (cross-OS), install.sh / install.ps1 (one-line
                    installer), backup.ps1 / backup.sh (sqlite3 .backup, WAL-safe)
.github/workflows   release.yml (tag v* → binaries + multi-arch ghcr images + release)
docker-compose.yml  backend + reverse-proxy, network cfmt-panel, volume cfmt-data
```

Backend packages: `cloudflare` (API client), `config`, `connector` (docker CLI wrapper +
supervisor), `handlers` (Echo), `infrastructure` (DB init/close), `middleware`, `models`,
`pkg/cryptox|logger`, `repositories/db`, `routes` (dependency graph), `services`.

## Commands

```bash
# backend
cd apps/backend && gofmt -w ./internal ./main.go && go build ./... && go vet ./... && go test ./...

# frontend (tsc is strict: noUnusedLocals, noUnusedParameters)
cd apps/frontend && bun run build

# cross-OS build/run (adds .exe only on Windows; output in apps/bin)
bun run build            # = bun scripts/build.mjs all
bun run prod

# stack
docker compose up -d --build
docker compose logs -f backend

# syntax check the installers without running them
bash -n scripts/install.sh
```

Cross-compiling is a real check here, not a formality — the backend is CGO-free
(`glebarez/sqlite`), so `GOOS=windows|darwin go build ./...` must stay green.

Tests exist only in `internal/services` and use `httptest` as a fake Cloudflare API
(`account_reconcile_test.go`) plus a temp SQLite file. No network or token needed.
Run the narrowest test first when debugging:
`go test ./internal/services/ -run TestUpgradeKeepsImportedTunnelAndImportsIngress -v`.

## Invariants (do not break these)

**Placement is two independent fields.**
`Tunnel.Location` (`local|remote`) is where cloudflared runs; `Tunnel.ConnectorMode`
(`docker|external`) is how it runs there. `Tunnel.ManagedLocally()` =
`location==local && connector_mode==docker` is the ONLY gate for Docker operations.
Never infer placement from `credential`. A tunnel on another device must stay
`remote/external` so the panel never claims to supervise a container it cannot see.

**Ingress is a whole-list PUT.**
`AppService` rebuilds the complete ingress list from `published_apps` and appends
`{"service":"http_status:404"}`. Writes must hold the per-tunnel mutex
(`TunnelService.ingressLocks`); two concurrent publishes on one tunnel would otherwise
lose a rule. Never patch a single rule in place.

**Reading ingress is `result.config.ingress`, never `result.ingress`.**
`cloudflare.GetTunnelConfig` decodes the `{"config": {...}}` wrapper. Decoding `result`
directly yields an empty ingress with **no error**, which is indistinguishable from a
tunnel with no hostnames — that shipped once and made every account import zero apps.
Test fakes must return the nested shape too, or the bug passes green.
`PutTunnelConfig` keeps sending `{"config": cfg}`.

**Origin options travel as `origin_request` on the app API.**
`PublishedApp.OriginRequestJSON` is the stored column (`json:"-"`);
`PublishedApp.OriginRequest` is the same bytes exposed as an object, filled by
`AfterFind`. The edit form must be able to see `noTLSVerify`, otherwise saving a
hostname change silently turns TLS verification back on. A nil field means "leave
alone"; `{}` means "clear" and empties the column. A service URL typed as
`192.168.0.103:80` is read as `http://…` by `ValidateServiceURL` — Cloudflare needs a
scheme and the raw parse error is useless to an operator.

**Account sync is an upsert that never deletes.**
`TunnelService.ReconcileAccount` (`services/account_reconcile.go`) creates or refreshes
remote tunnels and imports ingress hostnames as `published_apps`. It preserves
user-chosen placement, adopts DNS only when a CNAME to `<tunnel-id>.cfargotunnel.com`
already exists, skips hostnames owned by another tunnel, and counts missing remote
tunnels instead of deleting them. A token missing one permission must not destroy
working configuration. Do not add deletion to this path.

**Panel exposure is a `published_app` with `kind=panel`.**
One row drives both sides: `ListEnabledByTunnel` does not filter by `kind`, so the
exposure becomes an ingress rule on its tunnel and shows up in **Published apps**
automatically. `kind` is the flag that keeps that consistent — do not add a second
one. The generic app API refuses `update`/`enable`/`delete` on a panel row (403,
"managed from Settings"), and deleting a tunnel or account that carries the panel
is refused (409) because the cascade would silently unpublish the control plane.
`SetPanelExposure` reuses the existing row and refuses a hostname already owned by
another app.

**Panel service URL is derived, and the tunnel must be on this server.**
`resolvePanelServiceURL` accepts only `location=local` and returns
`PANEL_INTERNAL_URL`; anything else is a 422 naming `tunnel_id`. The connector
container reaches the dashboard over the Docker network, while a connector on
another device cannot resolve that name and pointing it at `PUBLIC_URL` loops
back through the tunnel being created. Do not re-add a "remote exposure" branch
or a typed service URL: `PanelExposureInput` has none, `PanelAddress` exposes only
`public_url`/`internal_url`, and `checkNoLoop`/`remotePanelServiceURL` are gone.
An exposure left on a remote tunnel from before this rule is still listed in
Settings with a warning so it is not lost silently.

**Adding a token upgrades, it does not duplicate.**
`AccountService.CreateWithToken` reuses an existing `owner_id + account_id` row with
`credential=token_import` and flips it to `api`. Only a second `api` row is a 409.
`SetTunnelReconciler` wires `TunnelService` in after construction (avoids an import
cycle); a nil reconciler is valid.

**Account selection comes from the token, not from the user's memory.**
`AccountService.ProbeToken` (`POST /api/cf-accounts/probe`) verifies the token and
returns every account it can reach, flagging rows the caller already manages with an
API token (`already_added`). It writes nothing. The add-account form debounces on the
token and renders a combobox from that list, preselecting the account when there is
exactly one, so a multi-account token never turns into a 409 listing raw ids. A
`token_import` row is NOT flagged — it is the upgrade path. Keep `ProbeToken` and
`CreateWithToken` in agreement: probe decides what the form offers, create decides
what is stored.

**Secrets never leave the server.**
API tokens and tunnel secrets are AES-256-GCM encrypted (`pkg/cryptox`) with
`ENCRYPTION_KEY` or a generated `encryption.key` in the data dir. API responses expose
`token_hint` only. Never log a decrypted token or add a field that returns one.

**Owner scoping lives in the repository.**
Use `db.Actor{UserID, Admin}.Scope(db, ownerColumn)`. Services accept
`services.Caller{ID, Email, Admin}` and must never take `echo.Context`, so the same
logic can be reused by CLI/MCP later. Admin-only routes also get
`middleware.AdminOnly`; services still re-check ownership.

**SQLite defaults + GORM zero values.**
`published_apps.manage_dns` has a SQL default of `true`, and GORM omits zero-value
fields on INSERT *and* writes the default back into the struct. `AppRepository.Create`
captures the intended value first and writes it back. Any new boolean column with a
`default:true` tag needs the same treatment, or `false` silently becomes `true`.

**First-admin bootstrap does not block weak passwords.**
`config.Load` defaults `ADMIN_PASSWORD` to `admin123`; an empty env value therefore
becomes `admin123`. `seedAdmin` creates the user and logs a warning when the password
is `admin123` or shorter than 10 characters. Only a literally empty string (possible
when `config.Config` is constructed directly, e.g. in tests) is rejected. Changing a
password later requires 8+ characters. The operator explicitly asked for simple
passwords — do not turn this back into a hard failure.

**Login throttle is per email, in memory.**
5 failures / 15 min locks the email for 10 min (`auth_service.go`). Per-email (not
per-IP) on purpose: the panel sits behind one reverse-proxy IP or a tunnel. Counters
reset on restart, which is also how to clear a lockout while testing.

**Impersonation is a short-lived token, never a refreshed session.**
`AuthService.Impersonate` (admin-only, `POST /api/auth/impersonate`) mints an
access token that carries the **target's** role and `imp=admin-id`. Middleware
re-reads the user, so the session really loses admin endpoints instead of only
looking like it. There is no refresh token: `POST /api/auth/exit-impersonation`
only writes the audit entry, and the browser restores the parked admin tokens
from `localStorage.impersonation`. `api/client.ts` must never refresh out of an
impersonated session — it restores the admin and reloads instead. Audit both
`user.impersonate_start` (naming the admin) and `user.impersonate_end`.

**UI text is never a literal. i18n is mandatory for new pages.**
Every page/component calls `useTranslation('<module>')` and reads text from
`src/locales/<module>.json`, whose leaves are `{ "id": "...", "en": "..." }`.
Adding a page means adding the json file and registering it in
`src/i18n/useTranslation.ts` — nothing else. `t('key', {name})` interpolates
`{{name}}`; a missing key renders the key itself and warns in dev. The default
language is `id`; the sidebar switcher persists it in `localStorage.language`.
`ApiTokenGuide.tsx` is the one deliberate exception: its permission names are
Cloudflare's own English labels and stay untranslated, while its surroundings
are translated.

**List pages use the shared `DataTable` + `QueryBuilder`.**
`components/DataTable.tsx` owns sorting, pagination, page size and per-table
column visibility (`localStorage.table:<tableId>:hidden`), and
`components/QueryBuilder.tsx` + `components/query.ts` own the nested AND/OR
filter model (`matchesQuery`). Every list page must use both with a stable
`tableId`, a client-side search box and `emptyMessage={tc('no_match')}`. Do not
hand-roll another `<table>` with its own pager; the tunnel detail page is the
only exception because it shows one tunnel's handful of routes.

**DNS records are live, and a tunnel-owned CNAME is locked.**
`services/dns_service.go` reads every record straight from Cloudflare on each
request (`cloudflare.ListAllDNSRecords`); only the zone list is cached, as the
directory of what can be opened. That cache must not go stale silently:
`AccountService.ListZones` refreshes it best-effort when it is empty or older
than `zoneCacheTTL` (10 min), because the publish-app DOMAIN dropdown reads it
and a zone created after the last Sync used to stay invisible until someone
pressed Sync (`zone_cache_test.go`). Do not add a local DNS record cache — a stale
row the operator deletes is a silent outage. A row is **managed** when a
`published_app` owns that record id, or when its CNAME target ends in
`.cfargotunnel.com` and the hostname is published on that zone; managed rows are
edited/deleted from the tunnel page and the service returns 409 for
`dns.update`/`dns.delete` on them. `POST /api/dns/zones/sync` refreshes zones per
API-token account and never deletes a cached zone; `dns.zones_sync` is audited.
Record type drives validation (`buildDNSRecordInput`): A/AAAA must match the right
IP family, only A/AAAA/CNAME may carry `proxied`, MX/SRV need a priority, SRV's
owner is `_service._proto`, CAA uses `data.flags/tag/value`. A relative name
(`www`) is expanded to `www.<zone>`; an absolute name is kept.

## Gotchas

- **The SPA needs its provider tree.** `src/main.tsx` must wrap `<App />` in
  `<BrowserRouter>` → `<LanguageProvider>` → `<ToastProvider>` → `<AuthProvider>`.
  Without `BrowserRouter`, `<Routes>` throws "useRoutes() may be used only in the
  context of a `<Router>` component"; without the other three every page throws on
  `useAuth()`/`useToast()`/`useTranslation()`. The failure mode is a **blank page
  with one console error** while `/api` keeps working, so always open the built UI
  in a browser, not just curl.
- Tailwind 4: component classes in `src/index.css` must repeat utilities; `@apply btn`
  fails with "unknown utility class".
- `lucide-react` here has no `Docker` icon — use `Container`.
- The publish form's subdomain is optional: an empty subdomain means the apex
  domain (`resolveHostname` in `app_service.go`). Cloudflare accepts a CNAME at
  the apex only because it flattens it, so the apex must not already carry A/AAAA
  records (Cloudflare answers 81053) — remove those or uncheck automatic DNS.
- Cloudflare cannot edit tunnels with `config_src=local|unknown` via API. Show a
  `config.yml` example instead of pretending the write succeeded.
- Panel exposure is local-tunnel-only; the Settings dropdown filters
  `location === 'local' && config_src === 'cloudflare'` and the backend rejects
  anything else with a 422 on `tunnel_id`. A pre-existing exposure on a remote
  tunnel stays visible with a warning.
- Connector image is resolved at runtime: `settings` key `connector.image` overrides
  `CLOUDFLARED_IMAGE`.
- Dev ports: backend 3001, Vite 3002 (proxies `/api`), reverse proxy 7777 (reads
  `PROXY_PORT` first, then `PORT` — a native run shares one `.env` with the
  backend, so `PORT=3001` must not make the proxy bind the backend's port).
- Docker builds: root `.dockerignore` keeps `node_modules` (120 MB) and `.env` out of
  the build context for `Dockerfile.proxy` (context is the repo root).
- The backend container runs as root because it needs `/var/run/docker.sock`.
- **The compose socket path is a variable.** `docker-compose.yml` mounts
  `${DOCKER_SOCKET:-/var/run/docker.sock}`; do not hard-code it again. `image:` sits
  next to `build:` on purpose — `compose pull` gets the published ghcr image and
  `--build` still works offline with the same tag name.
- **No path may assume Linux.** Config uses `filepath.Join` and `DATA_DIR` defaults to
  `./data`; `scripts/build.mjs` adds `.exe` only on `win32`. If you add a new file
  operation, keep it `filepath`-based so `GOOS=windows|darwin go build ./...` stays
  green.
- **The installers are shipped as-is by `curl | bash` / `irm | iex`.** `install.sh`
  must stay POSIX-bash-only with no repo-relative paths (it runs from a pipe, so
  `$0` is not the script path), must not overwrite an existing `.env` or `data/`,
  and must fall back `docker → native` when Docker is unavailable. Release assets
  are named `cfmt-<os>-<arch>.tar.gz` (`.zip` on Windows) with `bin/` + `static/`
  inside — renaming them breaks both installers.

## Deploy checklist

1. `.env` exists. `ADMIN_PASSWORD` may stay `admin123`; only an empty value fails.
2. `docker compose up -d --build` → both services healthy.
3. `curl http://localhost:7777/healthz` → `{"status":"ok"}`.
4. Login works; add an account; tunnels/hostnames appear.
5. `docker compose down && docker compose up -d` → data still there (volume
   `cfmt-data`; only `down -v` deletes it).
