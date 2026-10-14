#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
serial="${ADB_SERIAL:?Set ADB_SERIAL to the target shown by adb devices}"
app=/userdata/apps/memo-meeting
# Never overwrite a running executable or delete encrypted credentials/history.
adb -s "$serial" shell mkdir -p "$app"
adb -s "$serial" push dist/memo-glasses-armv7 "$app/memo-glasses.new"
adb -s "$serial" push dist/memo-capture-qr "$app/memo-capture-qr.new"
adb -s "$serial" push dist/memo-notify "$app/memo-notify.new"
adb -s "$serial" push dist/memo-media-state "$app/memo-media-state.new"
for name in open close daemon bind; do
  adb -s "$serial" push "device/memo-meeting-$name.sh" "$app/memo-meeting-$name.sh.new"
done
# This firmware uses legacy shell protocol; require an explicit success marker.
result=$(adb -s "$serial" shell '
set -eu
app=/userdata/apps/memo-meeting
if [ -x "$app/memo-glasses" ] && "$app/memo-glasses" status >/dev/null 2>&1; then
 "$app/memo-glasses" status | busybox grep -Eq "\"state\":\"(idle|completed|interrupted)\"" || { echo "Recording is active; upgrade deferred"; exit 1; }
 [ -f "$app/service.pid" ] || exit 1
 read -r pid < "$app/service.pid"
 case "$pid" in ""|*[!0-9]*) exit 1;; esac
 busybox tr "\000" " " < "/proc/$pid/cmdline" | busybox grep -F "$app/memo-glasses serve" >/dev/null || exit 1
 kill -TERM "$pid"
 count=0
 while kill -0 "$pid" 2>/dev/null; do
  count=$((count+1)); [ "$count" -lt 10 ] || exit 1; sleep 1
 done
fi
if [ -f "$app/memo-glasses" ] && [ ! -f "$app/memo-glasses.pre-qr" ]; then
 cp -p "$app/memo-glasses" "$app/memo-glasses.pre-qr"
fi
chmod 755 "$app/"*.new
mv "$app/memo-glasses.new" "$app/memo-glasses"
mv "$app/memo-capture-qr.new" "$app/memo-capture-qr"
mv "$app/memo-notify.new" "$app/memo-notify"
mv "$app/memo-media-state.new" "$app/memo-media-state"
for name in open close daemon bind; do
 mv "$app/memo-meeting-$name.sh.new" "$app/memo-meeting-$name.sh"
done
"$app/memo-meeting-daemon.sh"
"$app/memo-glasses" status
echo MEMO_DEPLOY_OK
')
printf '%s\n' "$result"
[[ "$result" == *MEMO_DEPLOY_OK* ]] || exit 1
