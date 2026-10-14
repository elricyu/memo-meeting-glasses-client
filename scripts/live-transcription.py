#!/usr/bin/env python3
"""Observe production glasses transcription over a temporary USB loopback forward.
No passwords or cloud keys are accepted. By default wait for native voice commands.
--start starts/stops only this tool's recording; --check performs read-only preflight.
"""
import argparse
import datetime
import json
import os
from pathlib import Path
import subprocess
import time
import urllib.error
import urllib.request


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--serial', default=os.environ.get('ADB_SERIAL'))
    parser.add_argument('--check', action='store_true')
    parser.add_argument('--start', action='store_true')
    parser.add_argument('--seconds', type=int, default=30)
    parser.add_argument('--wait-seconds', type=int, default=180)
    parser.add_argument('--evidence', type=Path)
    args = parser.parse_args()
    if not args.serial:
        parser.error("请通过 --serial 或 ADB_SERIAL 指定目标设备")
    if not 1 <= args.seconds <= 300 or not 1 <= args.wait_seconds <= 600:
        parser.error('durations must be positive, recording <=300s and wait <=600s')
    def adb(*parts):
        return subprocess.run(['adb', '-s', args.serial, *parts], check=True,
                              capture_output=True, text=True, timeout=15).stdout.strip()
    port = int(adb('forward', 'tcp:0', 'tcp:18765'))
    started = False
    snapshots = []
    final = None
    def api(path, body=None):
        req = urllib.request.Request(f'http://127.0.0.1:{port}{path}',
            data=json.dumps(body).encode() if body is not None else None,
            headers={'Content-Type': 'application/json'})
        try:
            with urllib.request.urlopen(req, timeout=45) as response:
                return json.load(response)
        except urllib.error.HTTPError as error:
            # Only the local daemon's bounded, sanitized control response is shown.
            raise RuntimeError(error.read(1024).decode(errors='replace').strip()) from None
    try:
        binding, status = api('/binding'), api('/status')
        print(json.dumps({'bindingState': binding.get('state'), 'status': status}, ensure_ascii=False))
        if args.check:
            return
        if binding.get('state') != 'BOUND':
            raise RuntimeError('眼镜未绑定或凭证未解锁；请先完成绑定/解锁，不要在聊天中发送口令。')
        old_id = status.get('meetingId')
        if args.start:
            if status.get('state') not in ('idle', 'completed', 'interrupted'):
                raise RuntimeError('眼镜已有录音，本工具不会打断它。')
            api('/start', {'title': '眼镜真实云端转录验收', 'context': ''})
            started = True
            print('录音已启动，请对眼镜说一段可核对的中文。', flush=True)
        else:
            print('等待按住物理键说“打开会议记录”，结束时说“关闭会议记录”。', flush=True)
        deadline = time.monotonic() + args.wait_seconds
        recording_at = None
        first_text_at = None
        last_text = ''
        active_id = None
        while time.monotonic() < deadline:
            status = api('/status')
            if status.get('meetingId') and (status['meetingId'] != old_id or started):
                active_id = status['meetingId']
            if active_id:
                detail = api('/transcript')
                now = time.monotonic()
                if recording_at is None and status['state'] == 'recording':
                    recording_at = now
                text = json.dumps({'draft': detail.get('draft'), 'segments': detail.get('segments')}, ensure_ascii=False)
                if text != last_text:
                    entry = {'at': datetime.datetime.now(datetime.timezone.utc).isoformat(),
                             'elapsedSeconds': round(now-recording_at, 2) if recording_at else None,
                             'state': status['state'], 'draft': detail.get('draft'), 'segments': detail.get('segments')}
                    snapshots.append(entry)
                    print(json.dumps(entry, ensure_ascii=False), flush=True)
                    last_text = text
                    if first_text_at is None and (detail.get('draft') or detail.get('segments')):
                        first_text_at = now
                if started and recording_at and now-recording_at >= args.seconds and status['state'] == 'recording':
                    api('/stop', {})
                    started = False
                    continue
                if status['state'] in ('completed', 'interrupted'):
                    final = {'status': status, 'detail': detail,
                             'firstTextAfterReadySeconds': round(first_text_at-recording_at, 2) if first_text_at and recording_at else None}
                    print(json.dumps(final, ensure_ascii=False), flush=True)
                    if status['state'] != 'completed' or detail['meeting']['status'] != 'COMPLETED':
                        raise RuntimeError('录音中断，请检查失败原因；这次验收未通过。')
                    if status['bytes'] <= 0 or status['bytes'] != status['committedBytes'] or status['droppedFrames']:
                        raise RuntimeError('音频保存字节或丢帧检查失败。')
                    if not detail.get('segments'):
                        raise RuntimeError('没有实际确认文字，不能认定转录验收通过。')
                    print('真实音频与末句已保存；准确率请与实际说话内容逐句核对。')
                    return
            time.sleep(1)
        raise RuntimeError('等待超时，未完成真实转录验收。')
    finally:
        if started:
            try:
                api('/stop', {})
            except Exception:
                print('本工具停止录音未确认，请查看眼镜与网页状态。')
        if args.evidence and snapshots:
            args.evidence.parent.mkdir(parents=True, exist_ok=True)
            fd = os.open(args.evidence, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
            os.fchmod(fd, 0o600)
            with os.fdopen(fd, 'w') as out:
                json.dump({'snapshots': snapshots, 'final': final}, out, ensure_ascii=False, indent=2)
            print(f'证据保存：{args.evidence}')
        adb('forward', '--remove', f'tcp:{port}')


if __name__ == '__main__':
    try:
        main()
    except (RuntimeError, subprocess.SubprocessError, urllib.error.URLError) as error:
        raise SystemExit(str(error))
