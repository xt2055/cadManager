# cadguanliq 测试数据清理脚本（Windows 离线部署环境使用，仅依赖标准库）
# 用法（在 new 目录下）：
#   python cleanup_db.py                 # 清理数据库 + 磁盘文件（需交互确认）
#   python cleanup_db.py --keep-files    # 只清数据库，保留磁盘文件
#   python cleanup_db.py --yes           # 跳过确认直接执行
# 保留：用户账号、角色、登录会话、审核流程模板、更新配置。
# 清理：图纸、结构、附件、文件版本、编辑会话、借用记录、BOM、审核实例、操作日志、业务文档缓存，
#       以及附件存储目录与 SMB 工作目录（drawings）下的磁盘文件。

import os
import shutil
import subprocess
import sys
from pathlib import Path

DEPLOY_ROOT = Path(__file__).resolve().parent
ENV_PATH = DEPLOY_ROOT / '.env'

# 与 migrations 对应的业务表（引用链整体 TRUNCATE，保留系统配置表）
BUSINESS_TABLES = [
    'edit_session_tickets',
    'edit_sessions',
    'file_versions',
    'attachments',
    'review_actions',
    'review_case_nodes',
    'review_cases',
    'borrow_records',
    'bom_items',
    'drawing_versions',
    'drawing_signers',
    'structure_parts',
    'drawings',
    'audit_logs',
    'data_documents',
]


def load_env():
    values = {}
    if ENV_PATH.exists():
        for line in ENV_PATH.read_text(encoding='utf-8', errors='ignore').splitlines():
            line = line.strip()
            if not line or line.startswith('#') or '=' not in line:
                continue
            key, _, value = line.partition('=')
            values[key.strip()] = value.strip()
    return values


def find_psql():
    program_files = os.environ.get('ProgramFiles', r'C:\Program Files')
    pg_root = Path(program_files) / 'PostgreSQL'
    if pg_root.is_dir():
        # 版本号倒序，优先最新安装（如 PostgreSQL/18）
        for version_dir in sorted(pg_root.iterdir(), reverse=True):
            candidate = version_dir / 'bin' / 'psql.exe'
            if candidate.is_file():
                return candidate
    return 'psql'  # 回退到 PATH


def clear_directory(directory: Path):
    if not directory.is_dir():
        print(f'  跳过（目录不存在）：{directory}')
        return
    for child in directory.iterdir():
        try:
            if child.is_dir():
                shutil.rmtree(child, ignore_errors=True)
            else:
                child.unlink()
        except Exception as exc:
            print(f'  跳过 {child.name}: {exc}')


def main():
    args = set(sys.argv[1:])
    keep_files = '--keep-files' in args
    assume_yes = '--yes' in args

    env = load_env()
    db_host = env.get('CAD_DB_HOST', '127.0.0.1')
    db_port = env.get('CAD_DB_PORT', '5432')
    db_name = env.get('CAD_DB_NAME', 'cadguanliq')
    db_user = env.get('CAD_DB_USER', 'postgres')
    db_password = env.get('CAD_DB_PASSWORD', '')
    storage_root = (DEPLOY_ROOT / env.get('CAD_STORAGE_ROOT', './storage/attachments')).resolve()
    smb_root = (DEPLOY_ROOT / env.get('CAD_SMB_LOCAL_ROOT', './storage/smb')).resolve()

    print('=== 图枢测试数据清理 ===')
    print(f'数据库: {db_user}@{db_host}:{db_port}/{db_name}')
    print(f'附件目录: {storage_root}')
    print(f'SMB 工作目录: {smb_root / "drawings"}')
    print('保留: 用户账号、角色、登录会话、审核流程模板、更新配置')

    if not assume_yes:
        answer = input('确认清理？(yes/no): ').strip().lower()
        if answer not in ('y', 'yes'):
            print('已取消')
            return

    sql = 'TRUNCATE TABLE ' + ', '.join(BUSINESS_TABLES) + ' CASCADE;'
    run_env = os.environ.copy()
    if db_password:
        run_env['PGPASSWORD'] = db_password
    psql = find_psql()
    print(f'==> 清理数据库（psql: {psql}）')
    result = subprocess.run(
        [str(psql), '-h', db_host, '-p', db_port, '-U', db_user, '-d', db_name, '-v', 'ON_ERROR_STOP=1', '-c', sql],
        env=run_env,
    )
    if result.returncode != 0:
        print('  FAIL 数据库清理失败（请确认后端服务已停止、psql 可用）')
        sys.exit(1)
    print('  OK 数据库业务数据已清空')

    if keep_files:
        print('==> 按参数跳过磁盘文件清理')
        print('=== 完成（仅数据库）===')
        return

    print('==> 清理附件存储目录')
    clear_directory(storage_root)
    print('  OK 附件存储已清空')

    print('==> 清理 SMB 工作目录（drawings）')
    clear_directory(smb_root / 'drawings')
    print('  OK SMB 工作文件已清空')

    print('=== 清理完成，启动后端服务即可重新测试 ===')


if __name__ == '__main__':
    main()
