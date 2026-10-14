#!/usr/bin/env python3
"""Whitelist-only release packaging; no device access, credentials or runtime data."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[1]


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--sdk-dir', type=Path, required=True)
    parser.add_argument('--compiler', required=True)
    parser.add_argument('--go', default='go')
    args = parser.parse_args()
    version = (ROOT / 'VERSION').read_text().strip()
    import re
    if not re.fullmatch(r'v\d+\.\d+\.\d+(?:-[a-z0-9.]+)?', version):
        raise SystemExit('Invalid release version')
    name = 'memo-meeting-glasses-client-' + version
    out = ROOT / 'releases'; out.mkdir(exist_ok=True)
    paths = ['README.md', 'LICENSE', 'THIRD_PARTY_NOTICES.md', 'CHANGELOG.md', 'VERSION', 'install.sh', 'install.ps1',
             'scripts/manage.py', 'device/manage-device.sh', 'device/voice-functions.conf', 'certs/ca-certificates.crt', 'certs/README.md',
             'docs/install.md', 'docs/user-guide.md', 'docs/compatibility.md', 'docs/development.md', 'docs/releasing.md',
             'CONTRIBUTING.md', 'SECURITY.md']
    paths += ['device/memo-meeting-' + p + '.sh' for p in ('open', 'close', 'daemon', 'bind')]
    paths += ['dist/' + p for p in ('memo-glasses-armv7', 'memo-capture-qr', 'memo-notify', 'memo-media-state')]
    paths += [str(p.relative_to(ROOT)) for p in sorted((ROOT / 'licenses').glob('*.txt'))]
    # Each included binary must be a little-endian 32-bit ARM ELF, not the host tool.
    for rel in paths:
        if rel.startswith('dist/'):
            h = (ROOT / rel).read_bytes()[:20]
            if h[:6] != b'\x7fELF\x01\x01' or h[18:20] != b'\x28\x00':
                raise SystemExit('Not an ARM ELF: ' + rel)
    build = {
        'version': version, 'sourceCommit': subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip(),
        'sdkSource': 'https://github.com/Iam5tillLearning/OpenSource-Ai-Glasses',
        'sdkCommit': subprocess.check_output(['git', '-C', str(args.sdk_dir), 'rev-parse', 'HEAD'], text=True).strip(),
        'sdkVersion': subprocess.check_output(['git', '-C', str(args.sdk_dir), 'show', 'HEAD:SDK/ai_glass_sdk/VERSION'], text=True).strip(),
        'sdkLibrarySha256': sha(args.sdk_dir / 'lib/libai_glass_sdk.a'),
        'compiler': subprocess.check_output([args.compiler, '--version'], text=True).splitlines()[0],
        'go': subprocess.check_output([args.go, 'version'], text=True).strip(),
        'target': 'linux/arm GOARM=7; C helpers require firmware uClibc',
        'defaultServer': 'https://memo-meeting.easymr.cn',
        'firmwareObserved': 'Buildroot 2025.08-git; exact firmware release version unavailable',
    }
    payload = {p: (ROOT / p).read_bytes() for p in paths}
    payload['BUILD.json'] = (json.dumps(build, ensure_ascii=False, indent=2) + '\n').encode()
    payload['SHA256SUMS'] = ''.join(hashlib.sha256(data).hexdigest() + '  ' + p + '\n' for p, data in sorted(payload.items())).encode()
    archive = out / (name + '.zip')
    with zipfile.ZipFile(archive, 'w', zipfile.ZIP_DEFLATED, compresslevel=9) as z:
        for rel, data in sorted(payload.items()):
            info = zipfile.ZipInfo(name + '/' + rel, (2026, 1, 1, 0, 0, 0))
            info.compress_type = zipfile.ZIP_DEFLATED
            info.external_attr = (0o100755 if rel.startswith('dist/') or rel.endswith('.sh') else 0o100644) << 16
            z.writestr(info, data)
    (out / 'SHA256SUMS').write_text(sha(archive) + '  ' + archive.name + '\n')
    print(str(archive))
    print('Files:', len(payload), 'Bytes:', archive.stat().st_size)


if __name__ == '__main__':
    main()
