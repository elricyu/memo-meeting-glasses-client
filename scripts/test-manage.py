#!/usr/bin/env python3
"""No-device regression: preserve unrelated voice functions and user data on failure."""
import importlib.util
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('manage', Path(__file__).with_name('manage.py'))
manage = importlib.util.module_from_spec(spec)
spec.loader.exec_module(manage)


class ManageTests(unittest.TestCase):
    def test_registry_ownership(self):
        source = '[device_function]\nname=video\ncustom=preserve\n\n[device_function]\nname=open_meeting_recording\nold=1\n'
        merged = manage.merge_registry(source, '[device_function]\nname=open_meeting_recording\nnew=1')
        self.assertIn('custom=preserve', merged)
        self.assertNotIn('old=1', merged)
        self.assertEqual(merged.count('name=open_meeting_recording'), 1)
        removed = manage.merge_registry(merged)
        self.assertIn('custom=preserve', removed)
        self.assertNotIn('name=open_meeting_recording', removed)

    def test_manifest_rejects_tampering_and_traversal(self):
        with tempfile.TemporaryDirectory() as t:
            root = Path(t)
            (root / 'test').write_text('modified')
            (root / 'SHA256SUMS').write_text('0' * 64 + '  test\n')
            with self.assertRaises(RuntimeError):
                manage.verify_package(root)
            (root / 'SHA256SUMS').write_text('0' * 64 + '  ../test\n')
            with self.assertRaises(RuntimeError):
                manage.verify_package(root)

    def test_device_transaction(self):
        # Test the real transaction shell in a temporary filesystem with test binaries.
        with tempfile.TemporaryDirectory() as t:
            root = Path(t); app = root / 'app'; app.mkdir()
            registry = root / 'registry'; registry.write_text('original-registry')
            bins = root / 'bin'; bins.mkdir()
            busybox = bins / 'busybox'; busybox.write_text('#!/bin/sh\nexec "$@"\n'); busybox.chmod(0o755)
            script = (manage.ROOT / 'device/manage-device.sh').read_text().replace('APP=/userdata/apps/memo-meeting', 'APP=' + str(app)).replace('REGISTRY=/usr/share/ai_core_device_functions.conf', 'REGISTRY=' + str(registry))
            script_path = root / 'transaction.sh'; script_path.write_text(script)
            credentials = app / 'scan-credentials.enc'; credentials.write_text('never-touch')
            audio = app / 'audio'; audio.mkdir(); (audio / 'record.wav').write_text('never-touch-audio')
            def stage(version, fail=False):
                folder = app / ('.stage-' + version); folder.mkdir()
                (folder / 'registry.conf').write_text('registry-' + version)
                (folder / 'VERSION').write_text(version)
                (folder / 'meeting.env').write_text('MEMO_SERVER=https://example.com\n')
                (folder / 'ca-certificates.crt').write_text('certificate')
                for name in ['memo-capture-qr', 'memo-notify', 'memo-media-state', 'memo-meeting-open.sh', 'memo-meeting-close.sh', 'memo-meeting-bind.sh']:
                    (folder / name).write_text('#!/bin/sh\nexit 0\n')
                (folder / 'memo-meeting-daemon.sh').write_text('#!/bin/sh\ntouch "' + str(app / '.ready') + '"\n')
                (folder / 'memo-glasses').write_text('#!/bin/sh\ncase "$1" in check) exit ' + ('1' if fail else '0') + ';; status) test -f "' + str(app / '.ready') + '" || exit 1; echo \'{"state":"idle"}\';; esac\n')
                return folder
            def run(action, folder, expected=0):
                (app / '.ready').unlink(missing_ok=True)
                import os
                result = subprocess.run(['sh', str(script_path), action, str(folder)], env={**os.environ, 'PATH': str(bins) + ':' + os.environ['PATH']}, capture_output=True, text=True)
                self.assertEqual(result.returncode, expected, result.stdout + result.stderr)
                self.assertEqual(credentials.read_text(), 'never-touch')
                self.assertEqual((audio / 'record.wav').read_text(), 'never-touch-audio')
                self.assertFalse((app / '.manage.lock').exists())
            run('install', stage('v1'))
            self.assertEqual((app / 'VERSION').read_text(), 'v1')
            run('upgrade', stage('bad', True), 1)
            self.assertEqual((app / 'VERSION').read_text(), 'v1')
            self.assertEqual(registry.read_text(), 'registry-v1')
            (app / 'meeting.env').write_text('MEMO_SERVER=https://custom.example.com\n')
            run('upgrade', stage('v2'))
            self.assertEqual((app / 'meeting.env').read_text(), 'MEMO_SERVER=https://custom.example.com\n')
            rollback = stage('rollback'); (rollback / 'registry.conf').write_text('registry-v1')
            run('rollback', rollback)
            self.assertEqual((app / 'VERSION').read_text(), 'v1')
            run('uninstall', stage('uninstall'))
            self.assertFalse((app / 'memo-glasses').exists())
            restore = stage('restore'); (restore / 'registry.conf').write_text('registry-v1')
            run('rollback', restore)
            self.assertEqual((app / 'VERSION').read_text(), 'v1')


if __name__ == '__main__':
    unittest.main()
