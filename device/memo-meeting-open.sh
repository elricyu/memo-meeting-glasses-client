#!/bin/sh
set -eu
/userdata/apps/memo-meeting/memo-meeting-daemon.sh
exec /userdata/apps/memo-meeting/memo-glasses open --wait=true --title "眼镜会议记录"
