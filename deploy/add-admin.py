# add-admin.py — create or reset an admin account for the deployed database
# Run via add-admin.bat (uses the bundled portable python runtime).
# Usage: add-admin.py [account] [password] [display_name]

import getpass
import os
import subprocess
import sys
from pathlib import Path

DEPLOY_ROOT = Path(__file__).resolve().parent
PG_HOME = Path(os.environ.get('ProgramFiles', r'C:\Program Files')) / 'PostgreSQL' / '18'
ADMIN_ROLE = 'admin'


def ok(msg):
    print(f'  OK {msg}')


def die(msg):
    print(f'  FAIL {msg}')
    try:
        input('Press Enter to exit')
    except EOFError:
        pass
    sys.exit(1)


def load_db_config():
    """Read database settings from the .env written by install.py."""
    env_path = DEPLOY_ROOT / '.env'
    if not env_path.exists():
        die(f'.env not found: {env_path} (run install.bat first)')
    cfg = {}
    for line in env_path.read_text(encoding='utf-8-sig').splitlines():
        line = line.strip()
        if line and not line.startswith('#') and '=' in line:
            key, value = line.split('=', 1)
            cfg[key.strip()] = value.strip()
    for key in ('CAD_DB_HOST', 'CAD_DB_PORT', 'CAD_DB_NAME', 'CAD_DB_USER', 'CAD_DB_PASSWORD'):
        if key not in cfg:
            die(f'.env is missing {key}')
    return cfg


def run_psql(cfg, database, *extra):
    psql = PG_HOME / 'bin' / 'psql.exe'
    if not psql.exists():
        die(f'psql not found: {psql} (install PostgreSQL first)')
    env = os.environ.copy()
    env['PGPASSWORD'] = cfg['CAD_DB_PASSWORD']
    env['PGCLIENTENCODING'] = 'UTF8'
    return subprocess.run(
        [str(psql), '-h', cfg['CAD_DB_HOST'], '-p', cfg['CAD_DB_PORT'], '-U', cfg['CAD_DB_USER'],
         '-d', database] + [str(a) for a in extra],
        capture_output=True, text=True, encoding='utf-8', errors='replace', env=env)


def quote(text):
    return text.replace("'", "''")


def ask(prompt, secret=False):
    try:
        value = getpass.getpass(prompt) if secret else input(prompt)
    except EOFError:
        die('input cancelled')
    return value.strip()


def main():
    print('==============================================')
    print(' cadguanliq add admin account')
    print(f' deploy root: {DEPLOY_ROOT}')
    print('==============================================')

    cfg = load_db_config()
    db_name = cfg['CAD_DB_NAME']

    account = sys.argv[1] if len(sys.argv) > 1 else ask('账号 (account): ')
    if not account:
        die('账号不能为空')
    if any(ch in account for ch in "';\\ "):
        die('账号包含非法字符（禁止空格和引号）')

    display_name = (sys.argv[3] if len(sys.argv) > 3 else None) \
        or ask(f'显示名 (回车默认 {account}): ') or account

    password = sys.argv[2] if len(sys.argv) > 2 else ask('密码 (至少 6 位): ', secret=True)
    if len(password) < 6:
        die('密码至少 6 位')
    if len(sys.argv) <= 2:
        confirm = ask('再次输入密码确认: ', secret=True)
        if confirm != password:
            die('两次输入的密码不一致')

    check = run_psql(cfg, 'postgres', '-tAc', 'SELECT 1')
    if check.returncode != 0:
        if check.stderr:
            print(check.stderr.strip())
        die('数据库连接失败，请确认 PostgreSQL 已安装并启动')

    exists = run_psql(cfg, db_name, '-tAc',
                      f"SELECT 1 FROM users WHERE account='{quote(account)}'").stdout.strip()
    if exists == '1':
        confirm = ask(f"账号 {account} 已存在，是否重置其密码? (y/N): ").lower()
        if confirm != 'y':
            die('已取消')
        sql = (f"UPDATE users SET password_hash = crypt('{quote(password)}', gen_salt('bf')) "
               f"WHERE account = '{quote(account)}';")
        action = '密码已重置'
    else:
        sql = ("INSERT INTO users (account, display_name, password_hash, status) "
               f"VALUES ('{quote(account)}', '{quote(display_name)}', "
               f"crypt('{quote(password)}', gen_salt('bf')), 'active');")
        action = '账号已创建'

    r = run_psql(cfg, db_name, '-v', 'ON_ERROR_STOP=1', '-c', sql)
    if r.returncode != 0:
        if r.stderr:
            print(r.stderr.strip())
        die(f'{action}失败')
    ok(action)

    sql_role = (f"INSERT INTO user_roles (user_id, role) SELECT id, '{ADMIN_ROLE}' FROM users "
                f"WHERE account = '{quote(account)}' ON CONFLICT DO NOTHING;")
    if run_psql(cfg, db_name, '-v', 'ON_ERROR_STOP=1', '-c', sql_role).returncode != 0:
        die(f"赋予 '{ADMIN_ROLE}' 角色失败")
    ok(f"已赋予 '{ADMIN_ROLE}' 角色")

    print()
    print('==============================================')
    print(' DONE')
    print(f' 账号: {account}')
    print(f' 密码: {password}')
    print(' 请提醒用户首次登录后修改密码')
    print('==============================================')
    input('Press Enter to exit')


if __name__ == '__main__':
    main()
