#!/bin/sh
set -eu
APP=/userdata/apps/memo-meeting
BIN="$APP/memo-glasses"
export GOMEMLIMIT=20MiB GOGC=50
[ -x "$BIN" ] || { echo "memo client is not installed" >&2; exit 1; }
if [ -f "$APP/meeting.env" ]; then . "$APP/meeting.env"; fi
: "${MEMO_SERVER:?configure MEMO_SERVER in /userdata/apps/memo-meeting/meeting.env}"
if "$BIN" status >/dev/null 2>&1; then exit 0; fi
if [ -n "${MEMO_CA_FILE:-}" ]; then
  nohup "$BIN" serve --server "$MEMO_SERVER" --ca "$MEMO_CA_FILE" --data "$APP/audio" >"$APP/service.log" 2>&1 </dev/null &
else
  nohup "$BIN" serve --server "$MEMO_SERVER" --data "$APP/audio" >"$APP/service.log" 2>&1 </dev/null &
fi
echo $! >"$APP/service.pid"
count=0
while ! "$BIN" status >/dev/null 2>&1; do
  # RV1106B needs additional time to decrypt its credential vault on cold start.
  count=$((count+1)); if [ "$count" -ge 20 ]; then echo "memo daemon did not start" >&2; exit 1; fi
  sleep 1
done
