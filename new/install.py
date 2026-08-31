# cadguanliq offline one-click installer (Windows)
# Steps: elevate -> silent PostgreSQL -> create db -> migrations
#        -> admin account -> fix venv -> write .env -> start app
# Run via install.bat (uses the bundled portable python runtime).
# Edit the CONFIG section below before running.

import ctypes
import os
import re
import shutil
import socket
import subprocess
import sys
import time
import webbrowser
from pathlib import Path

# ---------------- CONFIG ----------------
DB_PORT        = 5432
DB_NAME        = 'cadguanliq'
DB_USER        = 'postgres'
DB_PASSWORD    = 'cadguanliq2026'        # PostgreSQL superuser password (set by silent install)
ADMIN_ACCOUNT  = 'admin'
ADMIN_NAME     = 'Administrator'
ADMIN_PASSWORD = 'admin123456'           # login password, change after first login
OPEN_FIREWALL  = True                    # allow LAN access on APP_PORT
APP_PORT       = 8080
# ----------------------------------------

DEPLOY_ROOT = Path(__file__).resolve().parent
PG_HOME = Path(os.environ.get('ProgramFiles', r'C:\Program Files')) / 'PostgreSQL' / '18'


def step(msg):
    print(f'==> {msg}')


def ok(msg):
    print(f'  OK {msg}')


def die(msg):
    print(f'  FAIL {msg}')
    try:
        input('Press Enter to exit')
    except EOFError:
        pass
    sys.exit(1)


def is_admin():
    try:
        return bool(ctypes.windll.shell32.IsUserAnAdmin())
    except Exception:
        return False


def elevate():
    # PostgreSQL install needs administrator; re-launch self via UAC
    if is_admin():
        return
    step('Requesting administrator privileges')
    ret = ctypes.windll.shell32.ShellExecuteW(
        None, 'runas', sys.executable, f'"{DEPLOY_ROOT / "install.py"}"', str(DEPLOY_ROOT), 1)
    if ret <= 32:
        die('administrator privileges were cancelled')
    sys.exit(0)


def run(cmd, encoding='utf-8', timeout=None):
    """Run a command, capture output, decode with the given encoding."""
    return subprocess.run(
        [str(c) for c in cmd],
        capture_output=True, text=True, encoding=encoding, errors='replace',
        timeout=timeout)


def detect_local_ip():
    """Detect the LAN IP of this machine (for the SMB UNC display)."""
    for probe in (('8.8.8.8', 80), ('192.168.0.1', 80), ('192.168.1.1', 80)):
        try:
            s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
            s.settimeout(1)
            s.connect(probe)
            ip = s.getsockname()[0]
            s.close()
            if ip and not ip.startswith('127.'):
                return ip
        except Exception:
            continue
    try:
        return socket.gethostbyname(socket.gethostname())
    except Exception:
        return '127.0.0.1'


def main():
    elevate()

    print('==============================================')
    print(' cadguanliq offline installer')
    print(f' deploy root: {DEPLOY_ROOT}')
    print('==============================================')

    # --- 1. silent install PostgreSQL 18 ---
    pg_setup = DEPLOY_ROOT / 'postgresql-18.6-1-windows-x64.exe'
    psql = PG_HOME / 'bin' / 'psql.exe'
    if psql.exists():
        ok(f'PostgreSQL already installed at {PG_HOME} (skip install, make sure DB password matches: {DB_PASSWORD})')
    else:
        if not pg_setup.exists():
            die(f'installer not found: {pg_setup}')
        step('Silent installing PostgreSQL 18 (1-3 minutes, please wait)')
        args = [
            '--mode', 'unattended',
            '--unattendedmodeui', 'none',
            '--superpassword', DB_PASSWORD,
            '--serverport', str(DB_PORT),
            '--prefix', str(PG_HOME),
            '--enable-components', 'server,commandlinetools',
            '--disable-components', 'pgAdmin,stackbuilder',
        ]
        proc = run([pg_setup] + args, timeout=600)
        if proc.returncode != 0:
            die(f'PostgreSQL installer exited with code {proc.returncode}')
        if not psql.exists():
            die(f'psql not found after install: {psql}')
        ok(f'PostgreSQL installed at {PG_HOME}')

    # --- 2. wait for database ready ---
    step('Waiting for PostgreSQL service')
    os.environ['PGPASSWORD'] = DB_PASSWORD
    os.environ['PGCLIENTENCODING'] = 'UTF8'
    pg_ready = PG_HOME / 'bin' / 'pg_isready.exe'
    ready = False
    for _ in range(60):
        if run([pg_ready, '-h', '127.0.0.1', '-p', DB_PORT, '-U', DB_USER]).returncode == 0:
            ready = True
            break
        time.sleep(2)
    if not ready:
        die('PostgreSQL service did not become ready in 120s')
    ok('PostgreSQL is accepting connections')

    def run_psql(database, *extra):
        return run([psql, '-h', '127.0.0.1', '-p', DB_PORT, '-U', DB_USER, '-d', database] + list(extra))

    # --- 3. create database ---
    step('Creating database (if missing)')
    exists = run_psql('postgres', '-tAc', f"SELECT 1 FROM pg_database WHERE datname='{DB_NAME}'").stdout.strip()
    if exists == '1':
        ok(f'database {DB_NAME} already exists')
    else:
        if run_psql('postgres', '-v', 'ON_ERROR_STOP=1', '-c', f'CREATE DATABASE "{DB_NAME}";').returncode != 0:
            die(f'failed to create database {DB_NAME}')
        ok(f'database {DB_NAME} created')

    # --- 4. run migrations (tracked in schema_migrations; SQL is idempotent) ---
    step('Running database migrations')
    role_r = run_psql('postgres', '-v', 'ON_ERROR_STOP=1', '-c',
                      "DO $$ BEGIN "
                      "IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'cadguanliq_app') THEN "
                      "CREATE ROLE cadguanliq_app NOLOGIN; END IF; END $$;")
    if role_r.returncode != 0:
        die('failed to ensure database role cadguanliq_app')
    mig_dir = DEPLOY_ROOT / 'database' / 'migrations'
    migrations = sorted(mig_dir.glob('*.sql')) if mig_dir.exists() else []
    if not migrations:
        die(r'no migration files found in database\migrations')
    if run_psql(DB_NAME, '-v', 'ON_ERROR_STOP=1', '-c',
                'CREATE TABLE IF NOT EXISTS schema_migrations ('
                'filename text PRIMARY KEY, applied_at timestamptz DEFAULT now());').returncode != 0:
        die('failed to create schema_migrations table')
    listing = run_psql(DB_NAME, '-tAc', 'SELECT filename FROM schema_migrations')
    applied = {line.strip() for line in listing.stdout.splitlines() if line.strip()} if listing.returncode == 0 else set()
    pending = [f for f in migrations if f.name not in applied]
    if not pending:
        ok('all migrations already applied')
    for file in pending:
        r = run_psql(DB_NAME, '-v', 'ON_ERROR_STOP=1', '-f', file)
        if r.returncode != 0:
            if r.stdout:
                print(r.stdout, end='')
            if r.stderr:
                print(r.stderr, file=sys.stderr, end='')
            die(f'migration failed: {file.name}')
        if run_psql(DB_NAME, '-v', 'ON_ERROR_STOP=1', '-c',
                    f"INSERT INTO schema_migrations (filename) VALUES ('{file.name}');").returncode != 0:
            die(f'failed to record migration: {file.name}')
        ok(f'applied {file.name}')

    # --- 5. create admin account ---
    step('Creating admin account')
    safe_password = ADMIN_PASSWORD.replace("'", "''")
    safe_account = ADMIN_ACCOUNT.replace("'", "''")
    sql_admin = (
        f"INSERT INTO users (account, display_name, password_hash, status) "
        f"VALUES ('{safe_account}', '{ADMIN_NAME}', crypt('{safe_password}', gen_salt('bf')), 'active') "
        f"ON CONFLICT (account) DO NOTHING; "
        f"INSERT INTO user_roles (user_id, role) SELECT id, 'admin' FROM users WHERE account = '{safe_account}' "
        f"ON CONFLICT DO NOTHING;"
    )
    if run_psql(DB_NAME, '-v', 'ON_ERROR_STOP=1', '-c', sql_admin).returncode != 0:
        die('failed to create admin account')
    ok(f"admin account '{ADMIN_ACCOUNT}' ready (password: {ADMIN_PASSWORD} - change it after first login)")

    # --- 6. repair venv for this machine (portable python runtime) ---
    step('Repairing Python venv')
    venv_cfg = DEPLOY_ROOT / '.venv' / 'pyvenv.cfg'
    runtime_python = DEPLOY_ROOT / 'runtime' / 'python' / 'python.exe'
    if not venv_cfg.exists():
        ok('no venv in package (skip)')
    elif not runtime_python.exists():
        die(f'portable python missing: {runtime_python}')
    else:
        cfg = venv_cfg.read_text(encoding='utf-8-sig')
        cfg = re.sub(r'home = .*', lambda m: f'home = {runtime_python.parent}', cfg)
        venv_cfg.write_text(cfg, encoding='utf-8')
        venv_python = DEPLOY_ROOT / '.venv' / 'Scripts' / 'python.exe'
        r = run([venv_python, '-c',
                 "import olefile, sys; print('  OK python', sys.version.split()[0], 'olefile', olefile.__version__)"])
        if r.returncode != 0:
            if r.stderr:
                print(r.stderr, file=sys.stderr, end='')
            die('venv python check failed')
        print(r.stdout, end='')

    # --- 7. write .env ---
    step('Writing .env config')
    env_lines = [
        '# generated by offline installer',
        f'CAD_SERVER_ADDR=:{APP_PORT}',
        'CAD_ALLOWED_ORIGINS=*',
        'CAD_DB_HOST=127.0.0.1',
        f'CAD_DB_PORT={DB_PORT}',
        f'CAD_DB_NAME={DB_NAME}',
        f'CAD_DB_USER={DB_USER}',
        f'CAD_DB_PASSWORD={DB_PASSWORD}',
        'CAD_DB_SSL_MODE=disable',
        'CAD_STORAGE_ROOT=./storage/attachments',
        'CAD_LOG_DIR=./logs',
        'CAD_UPDATES_DIR=./updates',
        'CAD_SMB_ENABLED=true',
        f'CAD_SMB_HOST={detect_local_ip()}',
        'CAD_SMB_SHARE=CadWorking',
        'CAD_SMB_LOCAL_ROOT=./storage/smb',
        'CAD_SMB_USERNAME=',
        'CAD_SMB_PASSWORD=',
    ]
    (DEPLOY_ROOT / '.env').write_text('\r\n'.join(env_lines) + '\r\n', encoding='utf-8')
    ok('.env written')

    # --- 8. CAXA plugin (copy exb2dwg.crx into every CAXA install's Bin64) ---
    step('Installing CAXA plugin (exb2dwg.crx)')
    plugin_src = DEPLOY_ROOT / 'tools' / 'exb2dxf' / 'exb2dwg.crx'
    plugin_done = False
    bin_dirs = []
    for root_name in ('ProgramFiles', 'ProgramW6432', 'ProgramFiles(x86)'):
        root = os.environ.get(root_name)
        if not root:
            continue
        caxa_root = Path(root) / 'CAXA'
        if not caxa_root.exists():
            continue
        for depth in (1, 2, 3):
            for exe in caxa_root.glob('*/' * depth + 'Bin64/CDRAFT_M.exe'):
                if exe.parent not in bin_dirs:
                    bin_dirs.append(exe.parent)
    if plugin_src.exists() and bin_dirs:
        for bin64 in bin_dirs:
            try:
                shutil.copy2(plugin_src, bin64 / 'exb2dwg.crx')
                ok(f'plugin installed to {bin64}')
                plugin_done = True
            except OSError as exc:
                print(f'  WARN copy failed for {bin64}: {exc}')
    if not plugin_done:
        print(r'  WARN CAXA CAD not found - install CAXA CAD, then copy '
              r'tools\exb2dxf\exb2dwg.crx into <CAXA>\Bin64\ (or rerun install.bat)')

    # --- 8. firewall for LAN clients ---
    if OPEN_FIREWALL:
        step('Opening firewall port for LAN clients')
        run(['netsh', 'advfirewall', 'firewall', 'delete', 'rule', 'name=cadguanliq-app'], encoding='gbk')
        r = run(['netsh', 'advfirewall', 'firewall', 'add', 'rule', 'name=cadguanliq-app',
                 'dir=in', 'action=allow', 'protocol=TCP', f'localport={APP_PORT}'], encoding='gbk')
        if r.returncode == 0:
            ok(f'port {APP_PORT} opened')
        else:
            print('  WARN firewall rule failed (LAN access may be blocked)')

    # --- 9. start application ---
    step('Starting cadguanliq')
    r = run(['tasklist', '/FI', 'IMAGENAME eq cadguanliq.exe', '/NH'], encoding='gbk')
    if 'cadguanliq.exe' in (r.stdout or '').lower():
        ok('already running')
    else:
        subprocess.Popen([DEPLOY_ROOT / 'cadguanliq.exe'], cwd=str(DEPLOY_ROOT))
        time.sleep(3)
        ok('started')
    webbrowser.open(f'http://127.0.0.1:{APP_PORT}')

    print()
    print('==============================================')
    print(' INSTALL COMPLETE')
    print(f' url:      http://127.0.0.1:{APP_PORT}')
    print(f' account:  {ADMIN_ACCOUNT} / {ADMIN_PASSWORD}')
    print(f' logs:     {DEPLOY_ROOT / "logs"}')
    print(' remember: change admin password after first login')
    print('==============================================')
    input('Press Enter to exit')


if __name__ == '__main__':
    main()
