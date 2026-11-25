# Hot backup of the SQLite database while the panel keeps running.
# Uses SQLite's .backup, which is WAL-safe, instead of copying the file.
#
#   .\scripts\backup.ps1
#
$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $PSScriptRoot
$backupDir = Join-Path $root 'backups'
New-Item -ItemType Directory -Force -Path $backupDir | Out-Null

$stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
$name = "cfmt-$stamp.db"

Write-Host "Creating $name ..."
docker compose exec -T backend sqlite3 /app/data/app.db ".backup '/app/data/_backup.db'"
docker compose cp "backend:/app/data/_backup.db" (Join-Path $backupDir $name)
docker compose exec -T backend rm -f /app/data/_backup.db

Write-Host "Backup written to backups\$name"
Write-Host "Secrets (.jwt_secret, encryption.key) live in the volume; back them up separately if you want a full restore."
