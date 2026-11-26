#!/usr/bin/env bash
# CF Multi Tunnel — one-line installer for Linux and macOS.
#
#   curl -fsSL https://raw.githubusercontent.com/brainplusplus/cf-multi-tunnel/main/scripts/install.sh | bash
#   curl -fsSL .../install.sh | bash -s -- --port 8080 --email me@example.com
#
# Two modes:
#   docker  (default when Docker is installed) downloads the compose project and
#           runs it. Works on Linux, macOS and Windows via WSL.
#   native  downloads the prebuilt release for this OS/arch and runs it under
#           systemd (Linux) or launchd (macOS). Needs no Docker, but a connector
#           can only supervise cloudflared containers when Docker is present.
#
# Everything is idempotent: an existing .env and an existing data directory are
# never overwritten, so re-running upgrades the code and keeps the credentials.
set -euo pipefail

REPO="${CFMT_REPO:-brainplusplus/cf-multi-tunnel}"
REF="${CFMT_REF:-main}"
VERSION="${CFMT_VERSION:-latest}"
MODE="auto"
PORT="${CFMT_PORT:-7777}"
DIR=""
ADMIN_EMAIL=""
ADMIN_PASSWORD=""
START=1

log() { printf '==> %s\n' "$*"; }
warn() { printf 'warning: %s\n' "$*" >&2; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

usage() {
  cat <<'USAGE'
Usage: install.sh [options]

  --mode docker|native|auto   Install with Docker Compose (default when Docker
                              exists) or as native binaries. Default: auto.
  --dir PATH                  Install directory. Default: /opt/cf-multi-tunnel
                              when writable as root/sudo, else
                              $HOME/.local/share/cf-multi-tunnel.
  --port N                    Public port for the panel. Default: 7777.
  --email ADDR                Initial admin email. Default: admin@example.com.
  --password VALUE            Initial admin password. Default: admin123.
  --version TAG               Release tag for native mode. Default: latest.
  --ref BRANCH                Branch to install in docker mode. Default: main.
  --no-start                  Install files only; do not start the service.
  -h, --help                  This text.

Environment overrides: CFMT_REPO, CFMT_SOURCE_URL, CFMT_RELEASE_BASE,
CFMT_DIR, CFMT_PORT, CFMT_REF, CFMT_VERSION, DOCKER_SOCKET.
USAGE
}

while [ $# -gt 0 ]; do
  case "$1" in
    --mode) MODE="${2:?--mode needs a value}"; shift 2 ;;
    --dir) DIR="${2:?--dir needs a value}"; shift 2 ;;
    --port) PORT="${2:?--port needs a value}"; shift 2 ;;
    --email) ADMIN_EMAIL="${2:?--email needs a value}"; shift 2 ;;
    --password) ADMIN_PASSWORD="${2:?--password needs a value}"; shift 2 ;;
    --version) VERSION="${2:?--version needs a value}"; shift 2 ;;
    --ref) REF="${2:?--ref needs a value}"; shift 2 ;;
    --no-start) START=0; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown option $1 (see --help)" ;;
  esac
done

case "$MODE" in
  auto|docker|native) ;;
  *) die "--mode must be auto, docker or native" ;;
esac
case "$PORT" in
  ''|*[!0-9]*) die "--port must be a number" ;;
esac

# ── host detection ───────────────────────────────────────────────────
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
  linux|darwin) ;;
  *) die "unsupported OS $OS; use scripts/install.ps1 on Windows" ;;
esac

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) die "unsupported architecture $ARCH" ;;
esac

# ── install dir + whether root is needed ─────────────────────────────
# Sudo is only used when the target directory is not writable by the current
# user. Installing into $HOME must not prompt for a password, and a piped
# `curl | bash` has no tty to answer one.
if [ -z "$DIR" ]; then
  if [ "$(id -u)" -eq 0 ] || [ -w /opt ]; then
    DIR="/opt/cf-multi-tunnel"
  else
    DIR="$HOME/.local/share/cf-multi-tunnel"
  fi
fi

NEED_ROOT=0
if [ "$(id -u)" -ne 0 ]; then
  probe="$DIR"
  while [ ! -e "$probe" ] && [ "$probe" != "/" ]; do probe="$(dirname "$probe")"; done
  [ -w "$probe" ] || NEED_ROOT=1
fi

if [ "$(id -u)" -eq 0 ]; then
  SUDO=""
elif [ "$NEED_ROOT" -eq 1 ]; then
  if command -v sudo >/dev/null 2>&1; then
    SUDO="sudo"
  else
    die "$DIR is not writable and sudo is unavailable; pass --dir with a writable path"
  fi
else
  SUDO=""
fi

# ── helpers ──────────────────────────────────────────────────────────
have() { command -v "$1" >/dev/null 2>&1; }

fetch() { # fetch <url> <dest>
  if have curl; then
    curl -fsSL --retry 3 --connect-timeout 15 -o "$2" "$1"
  elif have wget; then
    wget -q -O "$2" "$1"
  else
    die "curl or wget is required"
  fi
}

as_root() { # as_root <cmd...>
  if [ -n "$SUDO" ]; then "$SUDO" "$@"; else "$@"; fi
}

# Docker often needs sudo on a fresh host (the user is not in the docker group
# yet). Detect that once so compose, pull and run all use the same prefix.
DOCKER="docker"
compose() { # compose <args...> — runs from the install dir
  if [ "$DOCKER" = "docker" ] && docker compose version >/dev/null 2>&1; then
    (cd "$DIR" && docker compose "$@")
  elif [ "$DOCKER" != "docker" ] && $DOCKER compose version >/dev/null 2>&1; then
    (cd "$DIR" && $DOCKER compose "$@")
  elif have docker-compose; then
    (cd "$DIR" && docker-compose "$@")
  else
    die "Docker Compose v2 is required (https://docs.docker.com/compose/install/)"
  fi
}

docker_ready() {
  if have docker && docker info >/dev/null 2>&1; then
    DOCKER="docker"
    return 0
  fi
  if [ -n "$SUDO" ] && $SUDO docker info >/dev/null 2>&1; then
    DOCKER="$SUDO docker"
    return 0
  fi
  return 1
}

host_ip() {
  if [ "$OS" = "linux" ] && have hostname; then
    ip="$(hostname -I 2>/dev/null | awk '{print $1}')"
    if [ -n "${ip:-}" ]; then printf '%s' "$ip"; return; fi
  fi
  if have ipconfig && ipconfig getifaddr en0 >/dev/null 2>&1; then
    ipconfig getifaddr en0; return
  fi
  printf 'localhost'
}

# set_env_sudo <file> <key> <value> — replaces the key in place, appends when
# absent. Honours SUDO so a root-owned .env stays root-owned.
set_env_sudo() {
  as_root awk -v k="$2" -v v="$3" '
    $0 ~ "^"k"=" { print k"="v; found=1; next }
    { print }
    END { if (!found) print k"="v }
  ' "$1" > "$TMP/env.tmp"
  as_root cp "$TMP/env.tmp" "$1"
}

# ── decide the mode ──────────────────────────────────────────────────
if [ "$MODE" = "auto" ]; then
  if docker_ready; then MODE="docker"; else MODE="native"; fi
fi
if [ "$MODE" = "docker" ] && ! docker_ready; then
  warn "Docker is not running; falling back to native mode"
  MODE="native"
fi
if [ "$MODE" = "docker" ] && ! $DOCKER compose version >/dev/null 2>&1 && ! have docker-compose; then
  warn "Docker Compose v2 is missing; falling back to native mode"
  MODE="native"
fi

log "installing to $DIR (mode=$MODE, os=$OS/$ARCH, port=$PORT)"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# ── docker mode ──────────────────────────────────────────────────────
install_docker() {
  have tar || die "tar is required"
  local url
  if [ -n "${CFMT_SOURCE_URL:-}" ]; then
    url="$CFMT_SOURCE_URL"
  elif [ "$VERSION" != "latest" ]; then
    url="https://codeload.github.com/$REPO/tar.gz/refs/tags/$VERSION"
  else
    url="https://codeload.github.com/$REPO/tar.gz/refs/heads/$REF"
  fi

  log "downloading $url"
  if ! fetch "$url" "$TMP/source.tar.gz"; then
    die "could not download $url
       check --ref/--version, or use --mode native when the release has binaries."
  fi

  as_root mkdir -p "$DIR"
  log "extracting into $DIR"
  as_root tar -xzf "$TMP/source.tar.gz" -C "$DIR" --strip-components=1

  if [ ! -f "$DIR/.env" ]; then
    log "creating .env"
    as_root cp "$DIR/.env.example" "$DIR/.env"
    as_root chmod 600 "$DIR/.env"
    set_env_sudo "$DIR/.env" PANEL_PORT "$PORT"
    set_env_sudo "$DIR/.env" PUBLIC_URL "http://$(host_ip):$PORT"
    if [ -n "$ADMIN_EMAIL" ]; then set_env_sudo "$DIR/.env" ADMIN_EMAIL "$ADMIN_EMAIL"; fi
    if [ -n "$ADMIN_PASSWORD" ]; then set_env_sudo "$DIR/.env" ADMIN_PASSWORD "$ADMIN_PASSWORD"; fi
  else
    log "keeping existing .env"
  fi

  if [ "$START" -eq 1 ]; then
    # Prefer the published images (fast, no toolchain needed) and fall back to a
    # local build when the registry is unreachable or the tag does not exist.
    log "starting containers"
    if ! compose pull --quiet 2>/dev/null; then
      warn "could not pull images; building locally"
      compose up -d --build
    else
      compose up -d --no-build
    fi
  else
    log "files installed; start with: cd $DIR && docker compose up -d"
  fi
}

# ── native mode ──────────────────────────────────────────────────────
install_native() {
  have tar || die "tar is required"
  local asset base url
  asset="cfmt-$OS-$ARCH.tar.gz"
  if [ -n "${CFMT_RELEASE_BASE:-}" ]; then
    base="$CFMT_RELEASE_BASE"
  elif [ "$VERSION" = "latest" ]; then
    base="https://github.com/$REPO/releases/latest/download"
  else
    base="https://github.com/$REPO/releases/download/$VERSION"
  fi
  url="$base/$asset"

  log "downloading $url"
  if ! fetch "$url" "$TMP/$asset"; then
    die "could not download $url
       native mode needs a published release. Create one with:
         git tag v1.0.0 && git push origin v1.0.0
       (the release workflow then attaches $asset), or use --mode docker,
       or pass --version <tag> for an existing release."
  fi

  as_root mkdir -p "$DIR"
  log "extracting into $DIR"
  as_root tar -xzf "$TMP/$asset" -C "$DIR"

  if [ ! -f "$DIR/.env" ]; then
    log "creating .env"
    write_native_env
  else
    log "keeping existing .env"
  fi

  write_runner

  if [ "$START" -eq 1 ]; then
    install_service
  else
    log "files installed; start with: $DIR/run.sh"
  fi
}

write_native_env() {
  local mode="external"
  if docker_ready; then mode="docker"; fi
  {
    echo "# Written by install.sh — edit and restart the service afterwards."
    echo "PANEL_PORT=$PORT"
    echo "DATA_DIR=$DIR/data"
    echo "DB_PATH=$DIR/data/app.db"
    echo "STATIC_DIR=$DIR/static"
    echo "BACKEND_URL=http://127.0.0.1:3001"
    echo "PUBLIC_URL=http://$(host_ip):$PORT"
    echo "CONNECTOR_MODE=$mode"
    echo "CLOUDFLARED_IMAGE=cloudflare/cloudflared:latest"
    echo "DOCKER_BIN=docker"
    echo "CONNECTOR_DOCKER_NETWORK="
    echo "PANEL_INTERNAL_URL=http://host.docker.internal:$PORT"
    echo "JWT_SECRET="
    echo "ENCRYPTION_KEY="
    echo "ADMIN_EMAIL=${ADMIN_EMAIL:-admin@example.com}"
    echo "ADMIN_PASSWORD=${ADMIN_PASSWORD:-admin123}"
    echo "LOG_LEVEL=info"
  } > "$TMP/env"
  as_root cp "$TMP/env" "$DIR/.env"
  as_root chmod 600 "$DIR/.env"
}

write_runner() {
  cat > "$TMP/run.sh" <<'RUNNER'
#!/bin/sh
# Starts the backend on 3001 and the reverse proxy on PANEL_PORT. Both binaries
# read the same .env, so the ports are overridden per process: the backend uses
# PORT, the proxy uses PROXY_PORT (which wins over PORT).
set -eu
DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$DIR"

PUBLIC_PORT=7777
if [ -f "$DIR/.env" ]; then
  from_env=$(sed -n 's/^PANEL_PORT=//p' "$DIR/.env" | tail -n 1)
  if [ -n "$from_env" ]; then PUBLIC_PORT=$from_env; fi
fi

PORT="${BACKEND_PORT:-3001}" "$DIR/bin/backend" &
BACK=$!
PROXY_PORT="$PUBLIC_PORT" BACKEND_URL="http://127.0.0.1:${BACKEND_PORT:-3001}" "$DIR/bin/reverse-proxy" &
PROXY=$!
trap 'kill $BACK $PROXY 2>/dev/null || true' INT TERM
wait $PROXY || true
kill $BACK 2>/dev/null || true
RUNNER
  as_root cp "$TMP/run.sh" "$DIR/run.sh"
  as_root chmod 755 "$DIR/run.sh"
}

install_service() {
  if [ "$(id -u)" -ne 0 ] && [ -z "$SUDO" ]; then
    warn "not running as root; skipping the service file — start it with $DIR/run.sh"
    warn "or re-run with sudo to install autostart"
    return
  fi
  if [ "$OS" = "linux" ]; then
    local unit=/etc/systemd/system/cfmt-panel.service
    if ! have systemctl; then
      warn "systemctl not found; start manually with $DIR/run.sh"
      return
    fi
    cat > "$TMP/cfmt-panel.service" <<UNIT
[Unit]
Description=CF Multi Tunnel panel
After=network-online.target docker.service
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=$DIR
EnvironmentFile=-$DIR/.env
ExecStart=$DIR/run.sh
Restart=always
RestartSec=5
KillMode=control-group

[Install]
WantedBy=multi-user.target
UNIT
    log "installing systemd unit"
    as_root cp "$TMP/cfmt-panel.service" "$unit"
    as_root systemctl daemon-reload
    as_root systemctl enable cfmt-panel >/dev/null 2>&1 || true
    as_root systemctl restart cfmt-panel
  else
    local plist_label="com.cfmt.panel"
    local plist_dir="/Library/LaunchDaemons"
    local plist="$plist_dir/$plist_label.plist"
    as_root mkdir -p "$DIR/logs"
    cat > "$TMP/$plist_label.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>$plist_label</string>
  <key>ProgramArguments</key><array><string>$DIR/run.sh</string></array>
  <key>WorkingDirectory</key><string>$DIR</string>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>$DIR/logs/panel.log</string>
  <key>StandardErrorPath</key><string>$DIR/logs/panel.err</string>
</dict>
</plist>
PLIST
    log "installing launchd job"
    as_root cp "$TMP/$plist_label.plist" "$plist"
    as_root launchctl bootout "system/$plist_label" >/dev/null 2>&1 || true
    as_root launchctl bootstrap system "$plist"
  fi
}

case "$MODE" in
  docker) install_docker ;;
  native) install_native ;;
esac

# Report what is actually configured, not what was passed on the command line:
# a re-run keeps the existing .env, so --port/--email may have been ignored.
read_env() { # read_env <key>
  [ -f "$DIR/.env" ] || return 0
  as_root awk -F= -v k="$1" '$1 == k { sub("^"k"=", ""); print; exit }' "$DIR/.env" 2>/dev/null
}
EFF_PORT="$(read_env PANEL_PORT)"
EFF_EMAIL="$(read_env ADMIN_EMAIL)"

echo
log "done"
echo "    panel : http://$(host_ip):${EFF_PORT:-$PORT}"
echo "    login : ${EFF_EMAIL:-admin@example.com} (first start only; password from ADMIN_PASSWORD in .env)"
if [ "$MODE" = "docker" ]; then
  echo "    manage: cd $DIR && docker compose logs -f backend"
  echo "    data  : docker volume cfmt_data (app.db, .jwt_secret, encryption.key)"
else
  echo "    start : $DIR/run.sh   (autostart: systemctl status cfmt-panel | launchctl print system/com.cfmt.panel)"
  echo "    data  : $DIR/data (app.db, .jwt_secret, encryption.key) — back this up"
fi
