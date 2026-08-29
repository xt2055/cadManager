# ============================================================
# cadguanliq offline one-click installer (Windows)
# Steps: elevate -> silent PostgreSQL -> create db -> migrations
#        -> admin account -> fix venv -> write .env -> start app
# Edit the CONFIG section below before running.
# ============================================================

# ---------------- CONFIG ----------------
$DbPort        = 5432
$DbName        = 'cadguanliq'
$DbUser        = 'postgres'
$DbPassword    = 'cadguanliq2026'        # PostgreSQL superuser password (set by silent install)
$PgHome        = Join-Path $env:ProgramFiles 'PostgreSQL\18'
$AdminAccount  = 'admin'
$AdminName     = 'Administrator'
$AdminPassword = 'admin123456'           # login password, change after first login
$OpenFirewall  = $true                   # allow LAN access on $AppPort
$AppPort       = 8080
# ----------------------------------------

$ErrorActionPreference = 'Stop'
$DeployRoot = $PSScriptRoot

function Write-Step($message) { Write-Host "==> $message" -ForegroundColor Cyan }
function Write-Ok($message)   { Write-Host "  OK $message" -ForegroundColor Green }
function Die($message)        { Write-Host "  FAIL $message" -ForegroundColor Red; Read-Host 'Press Enter to exit'; exit 1 }

# --- 1. elevate to administrator (PostgreSQL install needs it) ---
$identity = [Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()
if (-not $identity.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Step 'Requesting administrator privileges'
    Start-Process powershell -Verb RunAs -ArgumentList @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', "`"$PSCommandPath`"")
    exit
}

Write-Host '==============================================' -ForegroundColor Yellow
Write-Host ' cadguanliq offline installer' -ForegroundColor Yellow
Write-Host " deploy root: $DeployRoot" -ForegroundColor Yellow
Write-Host '==============================================' -ForegroundColor Yellow

# --- 2. silent install PostgreSQL 18 ---
$pgSetup = Join-Path $DeployRoot 'postgresql-18.6-1-windows-x64.exe'
$psql = Join-Path $PgHome 'bin\psql.exe'
if (Test-Path $psql) {
    Write-Ok "PostgreSQL already installed at $PgHome (skip install, make sure `$DbPassword matches)"
} else {
    if (-not (Test-Path $pgSetup)) { Die "installer not found: $pgSetup" }
    Write-Step 'Silent installing PostgreSQL 18 (1-3 minutes, please wait)'
    $args = @(
        '--mode', 'unattended',
        '--unattendedmodeui', 'none',
        '--superpassword', $DbPassword,
        '--serverport', "$DbPort",
        '--prefix', $PgHome,
        '--enable-components', 'server,commandlinetools',
        '--disable-components', 'pgAdmin,stackbuilder'
    )
    $process = Start-Process -FilePath $pgSetup -ArgumentList $args -Wait -PassThru
    if ($process.ExitCode -ne 0) { Die "PostgreSQL installer exited with code $($process.ExitCode)" }
    if (-not (Test-Path $psql)) { Die "psql not found after install: $psql" }
    Write-Ok "PostgreSQL installed at $PgHome"
}

# --- 3. wait for database ready ---
Write-Step 'Waiting for PostgreSQL service'
$pgReady = Join-Path $PgHome 'bin\pg_isready.exe'
$env:PGPASSWORD = $DbPassword
$env:PGCLIENTENCODING = 'UTF8'
$ready = $false
for ($i = 0; $i -lt 60; $i++) {
    & $pgReady -h 127.0.0.1 -p $DbPort -U $DbUser 2>$null | Out-Null
    if ($LASTEXITCODE -eq 0) { $ready = $true; break }
    Start-Sleep -Seconds 2
}
if (-not $ready) { Die 'PostgreSQL service did not become ready in 120s' }
Write-Ok 'PostgreSQL is accepting connections'

function Invoke-Psql([string]$database, [string]$sql) {
    & $psql -h 127.0.0.1 -p $DbPort -U $DbUser -d $database -v ON_ERROR_STOP=1 -c $sql 2>&1 | Out-Null
    if ($LASTEXITCODE -ne 0) { return $false }
    return $true
}

# --- 4. create database ---
Write-Step 'Creating database (if missing)'
$exists = & $psql -h 127.0.0.1 -p $DbPort -U $DbUser -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname='$DbName'"
if ("$exists".Trim() -eq '1') {
    Write-Ok "database $DbName already exists"
} else {
    if (-not (Invoke-Psql 'postgres' "CREATE DATABASE `"$DbName`";")) { Die "failed to create database $DbName" }
    Write-Ok "database $DbName created"
}

# --- 5. run migrations (skip if schema already present) ---
Write-Step 'Running database migrations'
$hasSchema = & $psql -h 127.0.0.1 -p $DbPort -U $DbUser -d $DbName -tAc "SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name='users')"
if ("$hasSchema".Trim() -eq 't') {
    Write-Ok 'schema already present (skip migrations)'
} else {
    $migrations = Get-ChildItem -Path (Join-Path $DeployRoot 'database\migrations') -Filter '*.sql' | Sort-Object Name
    if (-not $migrations) { Die 'no migration files found in database\migrations' }
    foreach ($file in $migrations) {
        & $psql -h 127.0.0.1 -p $DbPort -U $DbUser -d $DbName -v ON_ERROR_STOP=1 -f $file.FullName *> $null
        if ($LASTEXITCODE -ne 0) { Die "migration failed: $($file.Name)" }
        Write-Ok "applied $($file.Name)"
    }
}

# --- 6. create admin account ---
Write-Step 'Creating admin account'
$safePassword = $AdminPassword.Replace("'", "''")
$safeAccount = $AdminAccount.Replace("'", "''")
$sqlAdmin = "INSERT INTO users (account, display_name, password_hash, status) VALUES ('$safeAccount', '$AdminName', crypt('$safePassword', gen_salt('bf')), 'active') ON CONFLICT (account) DO NOTHING; INSERT INTO user_roles (user_id, role) SELECT id, 'admin' FROM users WHERE account = '$safeAccount' ON CONFLICT DO NOTHING;"
if (-not (Invoke-Psql $DbName $sqlAdmin)) { Die 'failed to create admin account' }
Write-Ok "admin account '$AdminAccount' ready (password: $AdminPassword - change it after first login)"

# --- 7. repair venv for this machine (portable python runtime) ---
Write-Step 'Repairing Python venv'
$venvCfg = Join-Path $DeployRoot '.venv\pyvenv.cfg'
$runtimePython = Join-Path $DeployRoot 'runtime\python\python.exe'
if (-not (Test-Path $venvCfg)) { Write-Ok 'no venv in package (skip)' }
elseif (-not (Test-Path $runtimePython)) { Die "portable python missing: $runtimePython" }
else {
    $cfg = Get-Content -Path $venvCfg -Raw
    $cfg = $cfg -replace 'home = .*', "home = $(Join-Path $DeployRoot 'runtime\python')"
    [IO.File]::WriteAllText($venvCfg, $cfg)
    $venvPython = Join-Path $DeployRoot '.venv\Scripts\python.exe'
    & $venvPython -c "import olefile, sys; print('  OK python', sys.version.split()[0], 'olefile', olefile.__version__)"
    if ($LASTEXITCODE -ne 0) { Die 'venv python check failed' }
}

# --- 8. write .env ---
Write-Step 'Writing .env config'
$envLines = @(
    '# generated by offline installer',
    "CAD_SERVER_ADDR=:$AppPort",
    'CAD_ALLOWED_ORIGINS=*',
    "CAD_DB_HOST=127.0.0.1",
    "CAD_DB_PORT=$DbPort",
    "CAD_DB_NAME=$DbName",
    "CAD_DB_USER=$DbUser",
    "CAD_DB_PASSWORD=$DbPassword",
    'CAD_DB_SSL_MODE=disable',
    'CAD_STORAGE_ROOT=./storage/attachments',
    'CAD_LOG_DIR=./logs',
    'CAD_UPDATES_DIR=./updates',
    'CAD_SMB_ENABLED=false'
)
[IO.File]::WriteAllText((Join-Path $DeployRoot '.env'), ($envLines -join "`r`n") + "`r`n")
Write-Ok '.env written'

# --- 9. firewall for LAN clients ---
if ($OpenFirewall) {
    Write-Step 'Opening firewall port for LAN clients'
    netsh advfirewall firewall delete rule name="cadguanliq-app" *> $null
    netsh advfirewall firewall add rule name="cadguanliq-app" dir=in action=allow protocol=TCP localport=$AppPort *> $null
    if ($LASTEXITCODE -eq 0) { Write-Ok "port $AppPort opened" } else { Write-Host '  WARN firewall rule failed (LAN access may be blocked)' -ForegroundColor Yellow }
}

# --- 10. start application ---
Write-Step 'Starting cadguanliq'
$running = Get-Process -Name 'cadguanliq' -ErrorAction SilentlyContinue
if ($running) {
    Write-Ok 'already running'
} else {
    Start-Process -FilePath (Join-Path $DeployRoot 'cadguanliq.exe') -WorkingDirectory $DeployRoot
    Start-Sleep -Seconds 3
    Write-Ok 'started'
}
Start-Process "http://127.0.0.1:$AppPort"

Write-Host ''
Write-Host '==============================================' -ForegroundColor Yellow
Write-Host ' INSTALL COMPLETE' -ForegroundColor Yellow
Write-Host " url:      http://127.0.0.1:$AppPort" -ForegroundColor Yellow
Write-Host " account:  $AdminAccount / $AdminPassword" -ForegroundColor Yellow
Write-Host " logs:     $(Join-Path $DeployRoot 'logs')" -ForegroundColor Yellow
Write-Host ' remember: change admin password after first login' -ForegroundColor Yellow
Write-Host '==============================================' -ForegroundColor Yellow
Read-Host 'Press Enter to exit'
