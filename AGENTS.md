# AGENTS.md

Agent-facing notes for this repository. `README.md` is for humans; this file records
the invariants that are easy to break and the commands that actually verify a change.

## Layout

```
apps/backend        Go 1.26 + Echo + GORM + SQLite (module cf-multi-tunnel/backend)
apps/reverse-proxy  Go SPA + /api reverse proxy (module cf-multi-tunnel/reverse-proxy)
apps/frontend       React 19 + Vite 6 + Tailwind 4 (bun)
scripts             backup.ps1 / backup.sh (sqlite3 .backup, WAL-safe)
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

# stack
docker compose up -d --build
docker compose logs -f backend
```

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

**Account sync is an upsert that never deletes.**
`TunnelService.ReconcileAccount` (`services/account_reconcile.go`) creates or refreshes
remote tunnels and imports ingress hostnames as `published_apps`. It preserves
user-chosen placement, adopts DNS only when a CNAME to `<tunnel-id>.cfargotunnel.com`
already exists, skips hostnames owned by another tunnel, and counts missing remote
tunnels instead of deleting them. A token missing one permission must not destroy
working configuration. Do not add deletion to this path.

**Adding a token upgrades, it does not duplicate.**
`AccountService.CreateWithToken` reuses an existing `owner_id + account_id` row with
`credential=token_import` and flips it to `api`. Only a second `api` row is a 409.
`SetTunnelReconciler` wires `TunnelService` in after construction (avoids an import
cycle); a nil reconciler is valid.

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

## Gotchas

- Tailwind 4: component classes in `src/index.css` must repeat utilities; `@apply btn`
  fails with "unknown utility class".
- `lucide-react` here has no `Docker` icon — use `Container`.
- Cloudflare cannot edit tunnels with `config_src=local|unknown` via API. Show a
  `config.yml` example instead of pretending the write succeeded.
- Panel exposure on a remote tunnel requires an explicit `service_url` (LAN/VPN) and
  is validated by `checkNoLoop` (must not resolve back to the published hostname or
  `PUBLIC_URL`).
- Connector image is resolved at runtime: `settings` key `connector.image` overrides
  `CLOUDFLARED_IMAGE`.
- Dev ports: backend 3001, Vite 3002 (proxies `/api`), reverse proxy 7777 (reads `PORT`
  first, then `PROXY_PORT`).
- Docker builds: root `.dockerignore` keeps `node_modules` (120 MB) and `.env` out of
  the build context for `Dockerfile.proxy` (context is the repo root).
- The backend container runs as root because it needs `/var/run/docker.sock`.

## Deploy checklist

1. `.env` exists. `ADMIN_PASSWORD` may stay `admin123`; only an empty value fails.
2. `docker compose up -d --build` → both services healthy.
3. `curl http://localhost:7777/healthz` → `{"status":"ok"}`.
4. Login works; add an account; tunnels/hostnames appear.
5. `docker compose down && docker compose up -d` → data still there (volume
   `cfmt-data`; only `down -v` deletes it).
