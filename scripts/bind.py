#!/usr/bin/env python3
"""USB 绑定或解锁眼镜；口令仅经 stdin 传输，不写文件或命令行。"""
import datetime
import getpass
import json
import os
import subprocess
import sys
import urllib.request

serial = os.environ.get("ADB_SERIAL")
if not serial:
    raise SystemExit("请设置 ADB_SERIAL 为 adb devices 显示的目标设备序列号")
app = "/userdata/apps/memo-meeting"

def adb(command, allow_failure=False):
    # This firmware lacks shell_v2: adb may return 0 even if the remote command failed.
    wrapped = command + "; memo_rc=$?; printf '\\n__MEMO_EXIT__%s\\n' \"$memo_rc\""
    result = subprocess.run(["adb", "-s", serial, "shell", wrapped],
                            text=True, capture_output=True)
    if "__MEMO_EXIT__" in result.stdout:
        output, _, status = result.stdout.rpartition("__MEMO_EXIT__")
        result.stdout = output.strip()
        result.returncode = int(status.strip())
    elif result.returncode == 0:
        raise RuntimeError("设备没有返回执行确认")
    if result.returncode and not allow_failure:
        # Commands and outputs never include the unlock password or device token.
        raise RuntimeError(result.stderr.strip() or result.stdout.strip() or "ADB 操作失败")
    return result

def main():
    now = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    adb("date -u -s '" + now + "'")
    adb(app + "/memo-glasses check --ca " + app + "/ca-certificates.crt")
    adb(app + "/memo-meeting-daemon.sh")
    existing = adb("test -f " + app + "/credentials.enc", allow_failure=True).returncode == 0
    password = getpass.getpass("眼镜凭证解锁口令（首次设置至少 12 字节）：")
    if not existing:
        if len(password.encode()) < 12:
            raise RuntimeError("口令至少需要 12 字节")
        if getpass.getpass("再次输入口令：") != password:
            raise RuntimeError("两次输入不一致")
        mac = adb("cat /sys/class/net/wlan0/address").stdout.strip()
        # Validate before constructing the device shell command.
        import re
        if not re.fullmatch(r"[0-9a-fA-F]{2}(:[0-9a-fA-F]{2}){5}", mac):
            raise RuntimeError("无法读取设备 MAC")
        if not re.fullmatch(r"[A-Za-z0-9_-]{1,64}", serial):
            raise RuntimeError("ADB serial 格式错误")
        action="pair"
        body={"clientId":"RV1106B-"+serial,"mac":mac,"displayName":"Memo Meeting 眼镜","password":password}
    else:
        action="unlock"
        body={"password":password}
    # shell_v2 is unavailable. Forward only the device's loopback control socket;
    # never send the password through a PTY where it could be echoed.
    forward=subprocess.run(["adb","-s",serial,"forward","tcp:0","tcp:18765"],text=True,capture_output=True,check=True)
    port=int(forward.stdout.strip())
    try:
        request=urllib.request.Request("http://127.0.0.1:"+str(port)+"/"+action,
                                       data=json.dumps(body).encode(),headers={"Content-Type":"application/json"})
        opener=urllib.request.build_opener(urllib.request.ProxyHandler({}))
        with opener.open(request,timeout=60) as response:
            result=json.load(response)
    finally:
        subprocess.run(["adb","-s",serial,"forward","--remove","tcp:"+str(port)],capture_output=True)
    del password
    if result["state"] == "BOUND":
        print("设备已解锁，可以按住按钮说“打开会议记录”。")
    elif result["state"] == "PENDING":
        code = result.get("pairingCode", "")
        if code:
            print("绑定码：" + code + "（有效期 5 分钟）")
            print("登录 https://memo-meeting.easymr.cn ，在“我的眼镜”中确认绑定。")
        else:
            print("绑定尚未完成；请使用此前的绑定码在网页确认。")
    else:
        raise RuntimeError("绑定状态：" + result["state"] + "；请先在网页处理解绑，再重新绑定。")

if __name__ == "__main__":
    try:
        main()
    except (RuntimeError, ValueError, OSError, subprocess.SubprocessError) as error:
        print(str(error), file=sys.stderr)
        sys.exit(1)
