# build-deploy.py — assemble a complete deployment folder (deploy/pack)
# Run on the dev machine:  python build-deploy.py
# Result: deploy\pack\  (copy the whole folder to a fresh server, then run
#         runtime\python\python.exe install.py inside it)

import os
import shutil
import subprocess
import sys
from pathlib import Path

# ---------------- CONFIG ----------------
UvPythonName = 'cpython-3.14.0-windows-x86_64-none'   # portable python runtime under %APPDATA%\uv\python
PgSetupName  = 'postgresql-18.6-1-windows-x64.exe'    # PostgreSQL installer at repo root
# ----------------------------------------

DEPLOY_ROOT = Path(__file__).resolve().parent
REPO_ROOT   = DEPLOY_ROOT.parent
PACK_DIR    = DEPLOY_ROOT / 'pack'
VUE_CAD_DIR = REPO_ROOT / 'vue' / 'cad'
GO_DIR      = REPO_ROOT / 'go'


def ok(msg):
    print(f'  OK {msg}')


def die(msg):
    print(f'  FAIL {msg}')
    sys.exit(1)


def copy_tree(src: Path, dst: Path, skip_dirs=None, skip_files=None):
    """Robocopy-like recursive copy with path-relative dir and name exclusions."""
    skip_dirs = {d.lower() for d in (skip_dirs or [])}
    skip_files = {f.lower() for f in (skip_files or [])}

    def ignore(directory, names):
        rel = os.path.relpath(directory, src).lower()
        out = set()
        if rel == '.' or any(rel == d or rel.startswith(d + os.sep) for d in skip_dirs):
            return set(names)
        for name in names:
            if name.lower() in skip_files:
                out.add(name)
        return out

    shutil.copytree(src, dst, ignore=ignore, dirs_exist_ok=True)


def main():
    print('==============================================')
    print(' cadguanliq deploy builder (python)')
    print(f' repo root: {REPO_ROOT}')
    print(f' output:    {PACK_DIR}')
    print('==============================================')

    pg_setup    = REPO_ROOT / PgSetupName
    go_venv     = GO_DIR / '.venv'
    uv_python   = Path(os.environ['APPDATA']) / 'uv' / 'python' / UvPythonName

    if not pg_setup.exists():
        die(f'PostgreSQL installer not found: {pg_setup}')
    if not (go_venv / 'Scripts' / 'python.exe').exists():
        die(f'go\\.venv missing, create it first: {go_venv}')
    if not uv_python.exists():
        die(f'uv python runtime missing: {uv_python}')

    if PACK_DIR.exists():
        shutil.rmtree(PACK_DIR)
    PACK_DIR.mkdir(parents=True)

    print('==> Building backend exe')
    r = subprocess.run(['go', 'build', '-trimpath', '-o', str(PACK_DIR / 'cadguanliq.exe'), './cmd/server'],
                       cwd=GO_DIR)
    if r.returncode != 0:
        die('go build failed')
    ok('cadguanliq.exe')

    print('==> Copying tools (exb_probe, wheels, CAXA plugin)')
    copy_tree(REPO_ROOT / 'tools', PACK_DIR / 'tools',
              skip_dirs=[r'tools\exb2dxf\plugin', r'tools\exb2dxf\ok\examples'],
              skip_files=['cad2x.exe', 'cad2x_real.exe', 'sample-output.dwg', 'sample-test.dxf'])
    ok('tools')

    print('==> Copying migrations')
    copy_tree(GO_DIR / 'database' / 'migrations', PACK_DIR / 'database' / 'migrations')
    ok('database\\migrations')

    print('==> Copying venv')
    copy_tree(go_venv, PACK_DIR / '.venv')
    ok('.venv')

    print('==> Copying portable python runtime')
    copy_tree(uv_python, PACK_DIR / 'runtime' / 'python')
    ok('runtime\\python')

    print('==> Copying PostgreSQL installer')
    shutil.copy2(pg_setup, PACK_DIR / PgSetupName)
    ok(PgSetupName)

    print('==> Copying install scripts')
    for name in ('install.py', 'add-admin.py', 'add-fonts.py'):
        shutil.copy2(DEPLOY_ROOT / name, PACK_DIR / name)
    ok('install.py / add-admin.py / add-fonts.py')

    print('==> Copying CAD fonts (server-side hosting)')
    copy_tree(VUE_CAD_DIR / 'public' / 'cad-data', PACK_DIR / 'cad-data')
    ok('cad-data\\fonts')

    size_mb = sum(f.stat().st_size for f in PACK_DIR.rglob('*') if f.is_file()) / (1024 * 1024)
    print()
    print('==============================================')
    print(f' DONE: {PACK_DIR}  ({size_mb:.0f} MB)')
    print(' Target machine: copy this whole folder, then run')
    print('   runtime\\python\\python.exe install.py')
    print('===============================================')


if __name__ == '__main__':
    main()
