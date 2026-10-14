#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
serial="${ADB_SERIAL:?Set ADB_SERIAL to the target shown by adb devices}"
work="$(mktemp -d /tmp/memo-voice-register.XXXXXX)"
adb -s "$serial" pull /usr/share/ai_core_device_functions.conf "$work/current.conf"
python3 "$root/scripts/register-voice.py" --current "$work/current.conf" --output "$work/merged.conf"
adb -s "$serial" shell 'mkdir -p /userdata/apps/memo-meeting; if [ ! -f /userdata/apps/memo-meeting/voice-functions.original.conf ]; then cp -p /usr/share/ai_core_device_functions.conf /userdata/apps/memo-meeting/voice-functions.original.conf; fi'
adb -s "$serial" push "$work/merged.conf" /usr/share/ai_core_device_functions.conf.new
adb -s "$serial" shell 'chmod 664 /usr/share/ai_core_device_functions.conf.new; mv /usr/share/ai_core_device_functions.conf.new /usr/share/ai_core_device_functions.conf'
echo 'Voice functions installed; reconnect the existing AI session to announce the new registry. No device reboot was performed.'
