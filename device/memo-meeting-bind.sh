#!/bin/sh
set -eu
[ ! -d /userdata/apps/memo-meeting/.manage.lock ] || { echo "应用正在更新，请稍后再试" >&2; exit 1; }
/userdata/apps/memo-meeting/memo-meeting-daemon.sh
exec /userdata/apps/memo-meeting/memo-glasses scan-binding
