"""Package a verified Cazy Lyre Windows release for GitHub Releases."""
import hashlib
import json
import os
import sys
import zipfile
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
META = json.loads((ROOT / 'release.json').read_text(encoding='utf-8'))
VERSION = META['version']
if META.get('application') != 'Cazy Lyre' or not all(p.isdigit() for p in VERSION.split('.')):
    raise SystemExit('Invalid release.json')
if len(sys.argv) > 1 and VERSION != sys.argv[1]:
    raise SystemExit(f'release.json {VERSION} differs from requested version {sys.argv[1]}')

must_have = ['release.json', 'Cazy_Lyre.exe']
extra = [
    'Cazy_Pink_Icon.ico', 'Cazy_Pink_Icon.png', 'MacroDeck_Icon.ico',
    'Create_Desktop_Shortcut.cmd', 'Create_Cazy_Lyre_Desktop_Shortcut.ps1',
    'INSTALL_CAPTURE_ENGINE.cmd', 'INSTALL_VIDEO_ENGINE.cmd',
    'capture_lyre.py', 'sampled_preview.py', 'video_notes.py', 'Lyre_Demo.mid',
    'README.txt', 'README_v3_7_TEMPO_LAB.md', 'README_v3_9_GITHUB_UPDATER.md',
]
missing = [n for n in must_have if not (ROOT / n).is_file()]
if missing:
    raise SystemExit('Missing required files: '+str(missing))
# Ship the bootstrap and full source as well, so the very first install ZIP
# can initialize its own GitHub repo without a second download.
source = sorted(n.name for n in ROOT.glob('*.go'))
source += sorted(n.name for n in ROOT.glob('*.py'))
source += ['build_windows.cmd', 'SETUP_GITHUB_UPDATES.cmd',
           'SETUP_GITHUB_UPDATES.ps1', 'Update-Apply.ps1', '.gitignore',
           '.github/workflows/release-windows.yml',
           'scripts/package_release.py']
files = list(dict.fromkeys(must_have + [n for n in extra + source if (ROOT / n).is_file()]))
out = ROOT / 'dist'
out.mkdir(exist_ok=True)
archive = out / 'Cazy_Lyre_Windows.zip'
with zipfile.ZipFile(archive, 'w', zipfile.ZIP_DEFLATED, compresslevel=8) as z:
    for name in files:
        z.write(ROOT / name, arcname=f'Cazy_Lyre_v{VERSION.replace(".","_")}/{name}')
digest = hashlib.sha256(archive.read_bytes()).hexdigest()
(out / 'Cazy_Lyre_Windows.zip.sha256').write_text(digest+'  Cazy_Lyre_Windows.zip\n', encoding='ascii')
print(f'Created {archive}: {archive.stat().st_size} bytes, SHA256 {digest}')
