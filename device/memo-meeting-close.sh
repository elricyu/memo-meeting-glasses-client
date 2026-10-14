#!/bin/sh
set -eu
APP=/userdata/apps/memo-meeting
export GOMEMLIMIT=20MiB
export GOGC=50
# Exit 0 only after the backend confirms the final transcript and local WAV is closed.
exec "$APP/memo-glasses" stop
