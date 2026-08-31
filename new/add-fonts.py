# add-fonts.py — install CAD fonts for both rendering ends:
#   1. CAXA CAD Font folder   (server-side conversion, CAXA opens DWG)
#   2. frontend font library  (public/cad-data/fonts + fonts.json, then rebuild client)
# Run: add-fonts.py [font_dir]   (dir containing .shx/.ttf/.otf/.woff)

import json
import os
import shutil
import sys
from pathlib import Path

DEPLOY_ROOT = Path(__file__).resolve().parent
# repo layout: deploy/add-fonts.py -> vue/cad/public/cad-data/fonts
FRONTEND_FONTS = DEPLOY_ROOT.parent / 'vue' / 'cad' / 'public' / 'cad-data' / 'fonts'
FONT_EXTS = {'.shx', '.ttf', '.otf', '.woff'}


def ok(msg):
    print(f'  OK {msg}')


def die(msg):
    print(f'  FAIL {msg}')
    try:
        input('Press Enter to exit')
    except EOFError:
        pass
    sys.exit(1)


def caxa_font_dirs():
    """All CAXA CAD Font folders found under Program Files."""
    out = []
    for env in ('ProgramFiles', 'ProgramW6432', 'ProgramFiles(x86)'):
        root = os.environ.get(env)
        if not root:
            continue
        caxa = Path(root) / 'CAXA'
        if not caxa.exists():
            continue
        for depth in (1, 2, 3):
            for d in caxa.glob('*/' * depth + 'Font'):
                if d.is_dir() and d not in out:
                    out.append(d)
    return out


def load_manifest(fonts_dir):
    mf = fonts_dir / 'fonts.json'
    if mf.exists():
        return json.loads(mf.read_text(encoding='utf-8-sig')), mf
    return [], mf


def main():
    print('==============================================')
    print(' cadguanliq add CAD fonts')
    print('==============================================')

    src_dir = Path(sys.argv[1]) if len(sys.argv) > 1 else Path(input('字体所在目录: ').strip().strip('"'))
    if not src_dir.exists():
        die(f'目录不存在: {src_dir}')
    fonts = [f for f in sorted(src_dir.iterdir()) if f.suffix.lower() in FONT_EXTS]
    if not fonts:
        die(f'目录中没有字体文件 (shx/ttf/otf/woff): {src_dir}')

    installed_caxa = 0
    for caxa_dir in caxa_font_dirs():
        for f in fonts:
            shutil.copy2(f, caxa_dir / f.name)
            installed_caxa += 1
        ok(f'installed {len(fonts)} fonts to CAXA: {caxa_dir}')
    if not caxa_font_dirs():
        print('  WARN 未找到 CAXA CAD Font 目录（跳过 CAXA 端）')

    if FRONTEND_FONTS.exists():
        manifest, mf_path = load_manifest(FRONTEND_FONTS)
        known_files = {entry.get('file', '').lower() for entry in manifest}
        added = 0
        for f in fonts:
            if f.name.lower() in known_files:
                shutil.copy2(f, FRONTEND_FONTS / f.name)
                continue
            entry = {
                'file': f.name,
                'name': [f.stem],
                'type': 'shx' if f.suffix.lower() == '.shx' else 'mesh',
            }
            manifest.append(entry)
            shutil.copy2(f, FRONTEND_FONTS / f.name)
            added += 1
        mf_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2), encoding='utf-8')
        ok(f'frontend font library: {added} new / {len(fonts)} synced -> {FRONTEND_FONTS}')
        print('  NOTE 前端字体已更新，需要重新打包客户端: npm run tauri build')
    else:
        print(f'  WARN 前端字体目录不存在（跳过）: {FRONTEND_FONTS}')

    print()
    print(f'共处理 {len(fonts)} 个字体文件, CAXA 端安装 {installed_caxa} 个副本')
    input('Press Enter to exit')


if __name__ == '__main__':
    main()
