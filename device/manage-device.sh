#!/bin/sh
# Transactional application-only operations. Never edit firmware or user audio/vaults.
set -eu
APP=/userdata/apps/memo-meeting
REGISTRY=/usr/share/ai_core_device_functions.conf
ACTION=${1:?}
STAGE=${2:?}
case "$ACTION" in install|upgrade|rollback|uninstall) ;; *) exit 2;; esac
case "$STAGE" in "$APP"/.stage-*) ;; *) exit 2;; esac
FILES='memo-glasses memo-capture-qr memo-notify memo-media-state memo-meeting-open.sh memo-meeting-close.sh memo-meeting-daemon.sh memo-meeting-bind.sh VERSION meeting.env ca-certificates.crt'
LOCK="$APP/.manage.lock"
mkdir "$LOCK" || { echo '已有安装操作，未修改应用'; exit 1; }
changed=0
committed=0
was_running=0
cleanup() {
 rc=$?
 trap - EXIT HUP INT TERM
 if [ "$changed" = 1 ] && [ "$committed" = 0 ]; then
  echo '操作未完成，恢复原应用和语音函数' >&2
  # Only this transaction may have started a replacement daemon.
  if [ -f "$APP/service.pid" ]; then
   read -r pid < "$APP/service.pid"
   case "$pid" in ''|*[!0-9]*) ;; *)
    if [ -r "/proc/$pid/cmdline" ] && busybox tr '\000' ' ' < "/proc/$pid/cmdline" | busybox grep -F "$APP/memo-glasses serve" >/dev/null; then
     kill -TERM "$pid" 2>/dev/null || true
     count=0
     while kill -0 "$pid" 2>/dev/null && [ "$count" -lt 20 ]; do count=$((count+1)); sleep 1; done
     if kill -0 "$pid" 2>/dev/null; then
      echo '新进程未停止；保留事务备份，未覆盖运行文件' >&2
      rm -rf "$LOCK"; exit 1
     fi
    fi;;
   esac
  fi
  for name in $FILES; do
   if [ -f "$STAGE/previous/$name" ]; then cp -p "$STAGE/previous/$name" "$APP/$name"; else rm -f "$APP/$name"; fi
  done
  cp -p "$STAGE/previous/registry.conf" "$REGISTRY"
  if [ "$was_running" = 1 ]; then "$APP/memo-meeting-daemon.sh" || echo '原服务启动失败，请检查配置' >&2; fi
 fi
 rm -rf "$LOCK" || true
 exit "$rc"
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM
if [ "$ACTION" = upgrade ] || [ "$ACTION" = uninstall ]; then test -x "$APP/memo-glasses"; fi
if [ "$ACTION" = rollback ]; then
 test -f "$APP/.rollback/installed"
 test -f "$APP/.rollback/memo-glasses"
fi
if [ "$ACTION" = install ] || [ "$ACTION" = upgrade ]; then
 free_kb=$(df -Pk "$APP" | busybox tail -n 1 | busybox awk '{print $4}')
 [ "$free_kb" -ge 32768 ] || { echo '可用空间不足 32 MiB，未修改应用'; exit 1; }
fi
# Refuse active recording and unknown processes, including a crashed control endpoint.
if [ -x "$APP/memo-glasses" ] && "$APP/memo-glasses" status >"$STAGE/status" 2>/dev/null; then
 busybox grep -Eq '"state":"(idle|completed|interrupted)"' "$STAGE/status" || { echo '正在录音或绑定，请稍后再操作'; exit 1; }
 was_running=1
fi
stop_pid=
if [ -f "$APP/service.pid" ]; then
 read -r pid < "$APP/service.pid"
 case "$pid" in ''|*[!0-9]*) echo '服务 PID 无效，未修改应用'; exit 1;; esac
 if kill -0 "$pid" 2>/dev/null; then
  busybox tr '\000' ' ' < "/proc/$pid/cmdline" | busybox grep -F "$APP/memo-glasses serve" >/dev/null || { echo 'PID 不属于本应用'; exit 1; }
  [ "$was_running" = 1 ] || { echo '控制接口无响应，未覆盖运行程序'; exit 1; }
  stop_pid=$pid
 fi
elif [ "$was_running" = 1 ]; then
 echo '缺少服务 PID，未修改应用'; exit 1
fi
if [ "$was_running" = 1 ]; then
 "$APP/memo-glasses" binding >"$STAGE/binding" 2>/dev/null || { echo '无法确认绑定状态'; exit 1; }
 if busybox grep -Eq '"state":"(SCANNING|PENDING)"' "$STAGE/binding"; then echo '正在绑定，请稍后再操作'; exit 1; fi
fi
if [ -f "$STAGE/registry.original" ]; then
 [ "$(cat "$REGISTRY")" = "$(cat "$STAGE/registry.original")" ] || { echo '语音清单已被其他应用修改，请重试'; exit 1; }
fi
# TLS validity requires a sane clock. USB installer supplies the trusted computer's UTC.
# Perform only after refusing recording/binding/unknown processes, before code changes.
if [ "$ACTION" != uninstall ] && [ -f "$STAGE/host-epoch" ]; then
 read -r host_epoch < "$STAGE/host-epoch"
 case "$host_epoch" in ''|*[!0-9]*) echo '电脑时间参数无效，未修改应用'; exit 1;; esac
 [ "$host_epoch" -ge 1735689600 ] && [ "$host_epoch" -lt 4102444800 ] || { echo '电脑时间异常，未修改应用'; exit 1; }
 device_epoch=$(date -u +%s)
 offset=$((device_epoch - host_epoch))
 if [ "$offset" -lt -300 ] || [ "$offset" -gt 300 ]; then
  echo '眼镜时间与电脑相差超过5分钟，正在通过 USB 校正 UTC 时间'
  date -u -s "@$host_epoch" >/dev/null || { echo '眼镜校时失败，请检查权限；未修改应用'; exit 1; }
  device_epoch=$(date -u +%s)
  offset=$((device_epoch - host_epoch))
  [ "$offset" -ge -30 ] && [ "$offset" -le 30 ] || { echo '眼镜校时未生效，未修改应用'; exit 1; }
 else
  echo '眼镜 UTC 时间检查通过'
 fi
fi
mkdir "$STAGE/previous"
for name in $FILES; do
 [ ! -f "$APP/$name" ] || cp -p "$APP/$name" "$STAGE/previous/$name"
done
cp -p "$REGISTRY" "$STAGE/previous/registry.conf"
[ ! -x "$APP/memo-glasses" ] || touch "$STAGE/previous/installed"
changed=1
if [ -n "$stop_pid" ]; then
 kill -TERM "$stop_pid"
 count=0
 while kill -0 "$stop_pid" 2>/dev/null; do count=$((count+1)); [ "$count" -lt 20 ] || { echo '服务未停止'; exit 1; }; sleep 1; done
fi
if [ "$ACTION" = uninstall ]; then
 for name in $FILES; do rm -f "$APP/$name"; done
else
 if [ "$ACTION" = rollback ]; then SOURCE="$APP/.rollback"; else SOURCE="$STAGE"; fi
 for name in $FILES; do
  # Upgrades keep server/CA settings; rollback restores the previous settings.
  if [ "$ACTION" != rollback ] && { [ "$name" = meeting.env ] || [ "$name" = ca-certificates.crt ]; } && [ -f "$APP/$name" ]; then continue; fi
  if [ -f "$SOURCE/$name" ]; then
   cp -p "$SOURCE/$name" "$APP/$name.new"
   case "$name" in memo-* ) chmod 755 "$APP/$name.new";; *) chmod 644 "$APP/$name.new";; esac
   mv "$APP/$name.new" "$APP/$name"
  else
   rm -f "$APP/$name"
  fi
 done
 # Read-only compatibility and device-to-server HTTPS checks; no meeting is created.
 "$APP/memo-media-state" --once
 . "$APP/meeting.env"
 "$APP/memo-glasses" check --server "$MEMO_SERVER" --ca "${MEMO_CA_FILE:-}" --data "$STAGE/health-audio"
fi
cp -p "$STAGE/registry.conf" "$REGISTRY.new"
chmod 664 "$REGISTRY.new"
mv "$REGISTRY.new" "$REGISTRY"
if [ "$ACTION" != uninstall ]; then
 "$APP/memo-meeting-daemon.sh"
 "$APP/memo-glasses" status
fi
# Rotate complete code/config snapshots after success; keep audio and credentials intact.
rm -rf "$APP/.rollback.old"
[ ! -d "$APP/.rollback" ] || mv "$APP/.rollback" "$APP/.rollback.old"
mv "$STAGE/previous" "$APP/.rollback"
committed=1
rm -f "$APP/service.pid.new"
echo "MEMO_MANAGE_OK $ACTION"
