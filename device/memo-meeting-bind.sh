#!/bin/sh
set -eu
/userdata/apps/memo-meeting/memo-meeting-daemon.sh
exec /userdata/apps/memo-meeting/memo-glasses scan-binding
