#!/usr/bin/env sh
# Hot backup of the SQLite database while the panel keeps running.
# Uses SQLite's .backup, which is WAL-safe, instead of copying the file.
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BACKUP_DIR="$ROOT/backups"
mkdir -p "$BACKUP_DIR"

STAMP="$(date +%Y%m%d-%H%M%S)"
NAME="cfmt-$STAMP.db"

echo "Creating $NAME ..."
docker compose exec -T backend sqlite3 /app/data/app.db ".backup '/app/data/_backup.db'"
docker compose cp "backend:/app/data/_backup.db" "$BACKUP_DIR/$NAME"
docker compose exec -T backend rm -f /app/data/_backup.db

echo "Backup written to backups/$NAME"
echo "Secrets (.jwt_secret, encryption.key) live in the volume; back them up separately for a full restore."
