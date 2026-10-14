#!/usr/bin/env python3
"""USB install/upgrade/rollback/uninstall; never upload account credentials."""
import argparse
import hashlib
import os
from pathlib import Path
import re
import shlex
import subprocess
import tempfile
from urllib.parse import urlsplit

ROOT = Path(__file__).resolve().parents[1]
APP = '/userdata/apps/memo-meeting'
REGISTRY = '/usr/share/ai_core_device_functions.conf'
OWNED = {'open_meeting_recording', 'close_meeting_recording', 'bind_meeting_account'}
PROGRAMS = {'memo-glasses-armv7': 'memo-glasses', 'memo-capture-qr': 'memo-capture-qr',
            'memo-notify': 'memo-notify', 'memo-media-state': 'memo-media-state'}


def merge_registry(source, additions=''):
    parts = re.split(r'(?=^\[device_function\]\s*$)', source, flags=re.MULTILINE)
    keep = []
    for part in parts:
        match = re.search(r'^name\s*=\s*(\S+)\s*$', part, re.MULTILINE)
        if not match or match.group(1) not in OWNED:
            keep.append(part.rstrip())
    return '\n\n'.join(keep).rstrip() + '\n\n' + additions.strip() + '\n'


def owned_registry(source):
    result = []
    for part in re.split(r'(?=^\[device_function\]\s*$)', source, flags=re.MULTILINE):
        match = re.search(r'^name\s*=\s*(\S+)\s*$', part, re.MULTILINE)
        if match and match.group(1) in OWNED:
            result.append(part)
    return '\n'.join(result)


def verify_package(root):
    manifest = root / 'SHA256SUMS'
    if not manifest.is_file():
        raise RuntimeError('缺少 SHA256SUMS，请使用完整 Release 安装包')
    listed = set()
    for line in manifest.read_text().splitlines():
        digest, rel = line.split('  ', 1)
        path = root / rel
        if Path(rel).is_absolute() or '..' in Path(rel).parts or not re.fullmatch('[0-9a-f]{64}', digest):
            raise RuntimeError('校验清单格式错误')
        if hashlib.sha256(path.read_bytes()).hexdigest() != digest:
            raise RuntimeError('文件校验失败：' + rel)
        listed.add(rel)
    required = {'dist/' + p for p in PROGRAMS} | {
        'VERSION', 'certs/ca-certificates.crt', 'device/manage-device.sh',
        'device/voice-functions.conf'} | {'device/memo-meeting-' + p + '.sh' for p in ('open', 'close', 'daemon', 'bind')}
    if not required <= listed:
        raise RuntimeError('校验清单缺少安装必需文件')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['install', 'upgrade', 'rollback', 'uninstall', 'check'])
    parser.add_argument('--serial', default=os.environ.get('ADB_SERIAL'))
    parser.add_argument('--server', help='首次安装的 HTTPS 地址；升级保留原配置')
    parser.add_argument('--ca', type=Path, help='首次安装的可信 PEM CA 文件')
    args = parser.parse_args()
    if (args.server or args.ca) and args.action != 'install':
        parser.error('--server/--ca 只用于首次安装')
    if args.server:
        url = urlsplit(args.server)
        if url.scheme != 'https' or not url.hostname or url.username or url.password or url.query or url.fragment or url.path not in ('', '/'):
            parser.error('--server 必须为无凭据的 HTTPS origin')
    devices = subprocess.check_output(['adb', 'devices'], text=True)
    ready = [line.split()[0] for line in devices.splitlines()[1:] if len(line.split()) == 2 and line.split()[1] == 'device']
    serial = args.serial
    if not serial:
        if len(ready) != 1:
            parser.error('请用 --serial 明确选择 adb devices 中的一台可用设备')
        serial = ready[0]
    if serial not in ready:
        parser.error('目标设备未连接或未授权')

    def adb(*parts, timeout=180):
        return subprocess.run(['adb', '-s', serial, *parts], check=True, capture_output=True,
                              text=True, timeout=timeout).stdout

    def shell(command):
        # Legacy firmware does not forward remote exit status: inspect our marker.
        output = adb('shell', command + '; memo_rc=$?; printf "\\n__MEMO_RC__%s\\n" "$memo_rc"')
        body, marker, status = output.rpartition('__MEMO_RC__')
        if not marker or status.strip() != '0':
            raise RuntimeError(body.strip() or '设备未确认执行成功')
        return body.strip()

    if args.action == 'check':
        print(shell('sh -c ' + shlex.quote('busybox head -n 10 /proc/cpuinfo; test -x ' + APP + '/memo-glasses && ' + APP + '/memo-glasses status')))
        return
    if args.action in ('install', 'upgrade'):
        verify_package(ROOT)
    # Only query read-only firmware interfaces; no recording or camera capture.
    shell('busybox grep -q ARMv7 /proc/cpuinfo && test -f ' + REGISTRY)
    if args.action in ('install', 'upgrade'):
        shell('test -S /tmp/ai-core_audio_stream && test -S /tmp/ai-core_audio_ctrl')
    if args.action == 'install' and (args.server or args.ca):
        shell('test ! -e ' + APP + '/memo-glasses')
    with tempfile.TemporaryDirectory(prefix='memo-install-') as temp:
        work = Path(temp)
        adb('pull', REGISTRY, str(work / 'registry.original'))
        current = (work / 'registry.original').read_text()
        if args.action == 'rollback':
            previous = shell('cat ' + APP + '/.rollback/registry.conf')
            additions = owned_registry(previous)
        elif args.action == 'uninstall':
            additions = ''
        else:
            additions = (ROOT / 'device/voice-functions.conf').read_text()
        (work / 'registry.conf').write_text(merge_registry(current, additions))
        token = os.urandom(8).hex()
        stage = APP + '/.stage-' + token
        shell('mkdir -p ' + stage)
        try:
            adb('push', str(work / 'registry.original'), stage + '/registry.original')
            adb('push', str(work / 'registry.conf'), stage + '/registry.conf')
            if args.action in ('install', 'upgrade'):
                for src, dst in PROGRAMS.items():
                    adb('push', str(ROOT / 'dist' / src), stage + '/' + dst)
                for name in ('open', 'close', 'daemon', 'bind'):
                    filename = 'memo-meeting-' + name + '.sh'
                    adb('push', str(ROOT / 'device' / filename), stage + '/' + filename)
                adb('push', str(ROOT / 'VERSION'), stage + '/VERSION')
                server = (args.server or 'https://memo-meeting.easymr.cn').rstrip('/')
                (work / 'meeting.env').write_text('MEMO_SERVER=' + shlex.quote(server) + '\nMEMO_CA_FILE=' + APP + '/ca-certificates.crt\n')
                adb('push', str(work / 'meeting.env'), stage + '/meeting.env')
                adb('push', str(args.ca or ROOT / 'certs/ca-certificates.crt'), stage + '/ca-certificates.crt')
            adb('push', str(ROOT / 'device/manage-device.sh'), stage + '/manage-device.sh')
            print(shell('sh ' + stage + '/manage-device.sh ' + args.action + ' ' + stage))
        except BaseException:
            print('设备事务目录已保留用于恢复：' + stage)
            raise
        else:
            shell('rm -rf ' + stage)
    print('请重新建立固件原有 AI 语音连接以更新函数清单；无需重刷固件。')


if __name__ == '__main__':
    try:
        main()
    except (OSError, ValueError, subprocess.SubprocessError, RuntimeError) as error:
        raise SystemExit('操作失败：' + str(error))
