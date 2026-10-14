# 安装、升级、回退与卸载

下载 GitHub Release 中的 `memo-meeting-glasses-client-v0.1.0-beta.1.zip` 并完整解压。不要下载 Source code 代替安装包。包内四个程序已编译，无需 Go、SDK 或交叉编译器。

## 电脑与设备准备

- 安装 Python 3.9+ 和 Android SDK Platform Tools（ADB），确保它们在 PATH。
- Windows 使用 PowerShell；Linux/macOS 使用 Bash。
- 眼镜已联网，固件原有语音 AI 服务正常，设备时间正确。
- USB 连接后执行 `adb devices`。只有一台可用设备时自动选择；有多台时传入 `--serial 设备序列号`。

推荐先使用 Release 附带的 `SHA256SUMS` 校验 ZIP：Linux 使用 `sha256sum -c SHA256SUMS`，macOS 使用 `shasum -a 256 -c SHA256SUMS`，Windows 使用 `Get-FileHash 文件.zip -Algorithm SHA256` 并比对。安装工具还会检查解压包内的文件清单。

## 安装

在解压目录执行：

```bash
# Linux / macOS
bash install.sh install
```

```powershell
# Windows
powershell -ExecutionPolicy Bypass -File .\install.ps1 install
```

默认连接 [线上实例](https://memo-meeting.easymr.cn)，随包提供服务配置和可信 CA。安装会检查硬件接口、可用空间、交互状态及设备到服务器的 HTTPS 连接；合并会议语音函数，保留原功能。不会重刷固件、修改原固件源码或重启设备，也不会创建会议或开启录音。

首次安装也会启动本应用后台服务。出现 `MEMO_MANAGE_OK install` 表示完成；随后重新建立固件原有 AI 语音连接，让新函数清单上报。注册网站账号并扫码绑定，见 [用户指南](user-guide.md)。

自部署只在首次安装时指定：

```bash
bash install.sh install --server https://meeting.example.com --ca /path/to/trusted-ca.crt
```

CA 必须是可信 PEM 证书包，不是私钥。已有应用的升级保留原 `meeting.env` 和 CA，不用安装参数覆盖；需换服务时由设备维护者另行修改配置并重新绑定。

## 管理

```bash
bash install.sh check       # 查看本机应用状态
bash install.sh upgrade     # 升级并保留账号凭证、服务配置和录音
bash install.sh rollback    # 恢复最近一次操作前的程序、配置和会议语音函数
bash install.sh uninstall   # 停止并移除程序与会议语音函数，保留录音和凭证
```

Windows 使用同样的操作参数替换 `install.sh` 为 `install.ps1`。正在录音、绑定或控制接口异常时拒绝更新；多台设备必须指定 `--serial`。

应用备份位于 `/userdata/apps/memo-meeting/.rollback/`，另保留一层 `.rollback.old/`。回退只恢复本应用拥有的语音函数，保留其他应用的当前条目；首次安装前没有旧版本时不能回退到不存在的程序。卸载后可用回退恢复最近版本。卸载不删除 WAV、凭证或备份，彻底清除敏感数据需由用户单独处理。

失败时安装工具尝试恢复本次操作前的程序和语音配置，并保留 `.stage-*` 事务目录用于排查。不要在失败原因未明确前删除备份。服务日志为 `service.log`，本地录音位于 `audio/`；公开反馈前需脱敏。

GitHub 安装包是 Beta 版本；真实拾音与语音意图效果受原固件和云端服务影响。断网不会自动补传。
