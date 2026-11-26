<#
.SYNOPSIS
  CF Multi Tunnel — one-line installer for Windows.

.DESCRIPTION
  irm https://raw.githubusercontent.com/brainplusplus/cf-multi-tunnel/main/scripts/install.ps1 | iex

  With options (PowerShell cannot pass arguments through a pipe, so use the
  scriptblock form):

    & ([scriptblock]::Create((irm .../install.ps1))) -Port 8080 -Mode native

  Modes:
    docker  (default when Docker Desktop is running) downloads the compose
            project and runs it. The panel container still needs the Docker
            socket to supervise cloudflared containers, which Docker Desktop
            provides at /var/run/docker.sock inside Linux containers.
    native  downloads the prebuilt release for this machine and starts the two
            binaries from a Scheduled Task at logon. No Docker needed, but the
            connector can only run cloudflared containers when Docker is
            installed (CONNECTOR_MODE=docker) — otherwise tunnels are
            "external" and you run cloudflared yourself.

  Re-running is safe: an existing .env and the data directory are kept.
#>
[CmdletBinding()]
param(
  [ValidateSet('auto', 'docker', 'native')][string]$Mode = 'auto',
  [string]$Dir = '',
  [int]$Port = 0,
  [string]$Email = '',
  [string]$Password = '',
  [string]$Version = '',
  [string]$Ref = '',
  [switch]$NoStart
)

$ErrorActionPreference = 'Stop'

$Repo = if ($env:CFMT_REPO) { $env:CFMT_REPO } else { 'brainplusplus/cf-multi-tunnel' }
if (-not $Ref) { $Ref = if ($env:CFMT_REF) { $env:CFMT_REF } else { 'main' } }
if (-not $Version) { $Version = if ($env:CFMT_VERSION) { $env:CFMT_VERSION } else { 'latest' } }
if ($Port -le 0) { $Port = if ($env:CFMT_PORT) { [int]$env:CFMT_PORT } else { 7777 } }
if (-not $Email) { $Email = $env:CFMT_EMAIL }
if (-not $Password) { $Password = $env:CFMT_PASSWORD }
if (-not $Dir) {
  $Dir = if ($env:CFMT_DIR) { $env:CFMT_DIR } else { Join-Path $env:LOCALAPPDATA 'cf-multi-tunnel' }
}
$Dir = [System.IO.Path]::GetFullPath($Dir)

function Write-Step($message) { Write-Host "==> $message" }
function Write-Warn($message) { Write-Host "warning: $message" -ForegroundColor Yellow }
function Fail($message) { Write-Host "error: $message" -ForegroundColor Red; exit 1 }

function Test-Docker {
  $docker = Get-Command docker -ErrorAction SilentlyContinue
  if (-not $docker) { return $false }
  & docker info *> $null
  return ($LASTEXITCODE -eq 0)
}

function Get-HostIp {
  try {
    $address = [System.Net.Dns]::GetHostAddresses([System.Net.Dns]::GetHostName()) |
      Where-Object { $_.AddressFamily -eq 'InterNetwork' -and -not $_.ToString().StartsWith('127.') } |
      Select-Object -First 1
    if ($address) { return $address.ToString() }
  } catch { }
  return 'localhost'
}

function Set-EnvValue($file, $key, $value) {
  $lines = @()
  if (Test-Path $file) { $lines = @(Get-Content -LiteralPath $file) }
  $found = $false
  $result = foreach ($line in $lines) {
    if ($line -match "^\s*$([regex]::Escape($key))\s*=") {
      $found = $true
      "$key=$value"
    } else {
      $line
    }
  }
  if (-not $found) { $result = @($result) + "$key=$value" }
  # ASCII keeps the file free of a BOM; godotenv reads the first key literally.
  Set-Content -LiteralPath $file -Value $result -Encoding Ascii
}

$mode = $Mode
$hasDocker = Test-Docker
if ($mode -eq 'auto') {
  if ($hasDocker) { $mode = 'docker' } else { $mode = 'native' }
}
if ($mode -eq 'docker' -and -not $hasDocker) {
  Write-Warn 'Docker is not running; falling back to native mode'
  $mode = 'native'
}

Write-Step "installing to $Dir (mode=$mode, port=$Port)"

$Temp = Join-Path ([System.IO.Path]::GetTempPath()) ("cfmt-install-" + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Force -Path $Temp | Out-Null

try {
  if ($mode -eq 'docker') {
    $url = if ($env:CFMT_SOURCE_URL) {
      $env:CFMT_SOURCE_URL
    } elseif ($Version -ne 'latest') {
      "https://codeload.github.com/$Repo/zip/refs/tags/$Version"
    } else {
      "https://codeload.github.com/$Repo/zip/refs/heads/$Ref"
    }

    Write-Step "downloading $url"
    $zip = Join-Path $Temp 'source.zip'
    Invoke-WebRequest -Uri $url -OutFile $zip -UseBasicParsing

    Write-Step "extracting into $Dir"
    $stage = Join-Path $Temp 'stage'
    Expand-Archive -LiteralPath $zip -DestinationPath $stage -Force
    $inner = Get-ChildItem -LiteralPath $stage | Select-Object -First 1
    New-Item -ItemType Directory -Force -Path $Dir | Out-Null
    # Copy-Item merges, so an existing .env survives an upgrade.
    Copy-Item -Path (Join-Path $inner.FullName '*') -Destination $Dir -Recurse -Force

    $envFile = Join-Path $Dir '.env'
    if (-not (Test-Path $envFile)) {
      Write-Step 'creating .env'
      Copy-Item -LiteralPath (Join-Path $Dir '.env.example') -Destination $envFile
      Set-EnvValue $envFile 'PANEL_PORT' $Port
      Set-EnvValue $envFile 'PUBLIC_URL' "http://$(Get-HostIp):$Port"
      if ($Email) { Set-EnvValue $envFile 'ADMIN_EMAIL' $Email }
      if ($Password) { Set-EnvValue $envFile 'ADMIN_PASSWORD' $Password }
    } else {
      Write-Step 'keeping existing .env'
    }

    if (-not $NoStart) {
      Write-Step 'starting containers'
      Push-Location $Dir
      try {
        & docker compose pull --quiet
        if ($LASTEXITCODE -ne 0) {
          Write-Warn 'could not pull images; building locally'
          & docker compose up -d --build
        } else {
          & docker compose up -d --no-build
        }
        if ($LASTEXITCODE -ne 0) { Fail 'docker compose up failed' }
      } finally {
        Pop-Location
      }
    } else {
      Write-Step "files installed; start with: cd $Dir; docker compose up -d"
    }
  } else {
    $asset = 'cfmt-windows-amd64.zip'
    $base = if ($env:CFMT_RELEASE_BASE) {
      $env:CFMT_RELEASE_BASE
    } elseif ($Version -eq 'latest') {
      "https://github.com/$Repo/releases/latest/download"
    } else {
      "https://github.com/$Repo/releases/download/$Version"
    }

    Write-Step "downloading $base/$asset"
    $zip = Join-Path $Temp $asset
    try {
      Invoke-WebRequest -Uri "$base/$asset" -OutFile $zip -UseBasicParsing
    } catch {
      Fail "could not download $base/$asset`n       native mode needs a published release (git tag v1.0.0 && git push origin v1.0.0),`n       or use -Mode docker, or -Version <tag> for an existing release."
    }

    Write-Step "extracting into $Dir"
    New-Item -ItemType Directory -Force -Path $Dir | Out-Null
    Expand-Archive -LiteralPath $zip -DestinationPath $Dir -Force

    $envFile = Join-Path $Dir '.env'
    if (-not (Test-Path $envFile)) {
      Write-Step 'creating .env'
      $connector = if ($hasDocker) { 'docker' } else { 'external' }
      $lines = @(
        '# Written by install.ps1 - edit and restart the task afterwards.'
        "PANEL_PORT=$Port"
        "DATA_DIR=$Dir\data"
        "DB_PATH=$Dir\data\app.db"
        "STATIC_DIR=$Dir\static"
        'BACKEND_URL=http://127.0.0.1:3001'
        "PUBLIC_URL=http://$(Get-HostIp):$Port"
        "CONNECTOR_MODE=$connector"
        'CLOUDFLARED_IMAGE=cloudflare/cloudflared:latest'
        'DOCKER_BIN=docker'
        'CONNECTOR_DOCKER_NETWORK='
        "PANEL_INTERNAL_URL=http://host.docker.internal:$Port"
        'JWT_SECRET='
        'ENCRYPTION_KEY='
        "ADMIN_EMAIL=$(if ($Email) { $Email } else { 'admin@example.com' })"
        "ADMIN_PASSWORD=$(if ($Password) { $Password } else { 'admin123' })"
        'LOG_LEVEL=info'
      )
      Set-Content -LiteralPath $envFile -Value $lines -Encoding Ascii
    } else {
      Write-Step 'keeping existing .env'
    }

    $runner = Join-Path $Dir 'run.ps1'
    $runnerBody = @'
# Starts the backend (3001) and the reverse proxy (public port) from .env.
$ErrorActionPreference = 'Stop'
$dir = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $dir

$port = '7777'
$envFile = Join-Path $dir '.env'
if (Test-Path $envFile) {
  foreach ($line in Get-Content -LiteralPath $envFile) {
    if ($line -match '^\s*PANEL_PORT\s*=\s*(\d+)') { $port = $Matches[1] }
  }
}

$env:PORT = '3001'
$backend = Start-Process -FilePath (Join-Path $dir 'bin\backend.exe') -NoNewWindow -PassThru
# The proxy must not inherit PORT=3001, or it binds the backend's port.
$env:PROXY_PORT = $port
$env:BACKEND_URL = 'http://127.0.0.1:3001'
$proxy = Start-Process -FilePath (Join-Path $dir 'bin\reverse-proxy.exe') -NoNewWindow -PassThru

try {
  Wait-Process -Id $proxy.Id
} finally {
  Stop-Process -Id $backend.Id -Force -ErrorAction SilentlyContinue
}
'@
    Set-Content -LiteralPath $runner -Value $runnerBody -Encoding Ascii

    if ($NoStart) {
      Write-Step "files installed; start with: powershell -ExecutionPolicy Bypass -File `"$runner`""
    } else {
      $taskName = 'CF Multi Tunnel Panel'
      $action = New-ScheduledTaskAction -Execute 'powershell.exe' `
        -Argument "-NoProfile -WindowStyle Hidden -ExecutionPolicy Bypass -File `"$runner`""
      $trigger = New-ScheduledTaskTrigger -AtLogOn
      $settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries `
        -DontStopIfGoingOnBatteries -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1)
      Write-Step "registering scheduled task '$taskName'"
      Register-ScheduledTask -TaskName $taskName -Action $action -Trigger $trigger `
        -Settings $settings -Description 'CF Multi Tunnel panel' -Force | Out-Null
      Start-ScheduledTask -TaskName $taskName
    }
  }
} finally {
  Remove-Item -LiteralPath $Temp -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Host ''
Write-Step 'done'

# Report what the .env actually says: a re-run keeps the existing file, so
# -Port / -Email may have been ignored.
$effPort = $Port
$effEmail = if ($Email) { $Email } else { 'admin@example.com' }
$envFile = Join-Path $Dir '.env'
if (Test-Path $envFile) {
  foreach ($line in Get-Content -LiteralPath $envFile) {
    if ($line -match '^\s*PANEL_PORT\s*=\s*(\d+)') { $effPort = $Matches[1] }
    if ($line -match '^\s*ADMIN_EMAIL\s*=\s*(.+)$') { $effEmail = $Matches[1].Trim() }
  }
}

Write-Host "    panel : http://$(Get-HostIp):$effPort"
Write-Host "    login : $effEmail (first start only; password from ADMIN_PASSWORD in .env)"
if ($mode -eq 'docker') {
  Write-Host "    manage: cd `"$Dir`"; docker compose logs -f backend"
  Write-Host "    data  : docker volume cfmt_data (app.db, .jwt_secret, encryption.key)"
} else {
  Write-Host "    start : powershell -ExecutionPolicy Bypass -File `"$Dir\run.ps1`""
  Write-Host "    manage: Start-ScheduledTask -TaskName 'CF Multi Tunnel Panel'"
  Write-Host "    data  : $Dir\data (app.db, .jwt_secret, encryption.key) - back this up"
}
