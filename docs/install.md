# 安装与升级

## 准备

需要支持的 RV1106B 开源眼镜、USB 数据线、可用 ADB、Bash、Python 3，以及构建好的四个程序。确认固件原有网络和语音服务正常。

在仓库或安装包根目录执行 `adb devices`，明确选择目标：

```bash
export ADB_SERIAL=你的设备序列号
```

安装工具不会重刷固件。不要在正在录音时升级；脚本会拒绝已检测到的活动录音。

## 首次配置

复制 `device/meeting.env.example` 为本地 `meeting.env`。模板默认连接 [线上实例](https://memo-meeting.easymr.cn)，使用该实例无需修改服务地址；若自行部署 `memo-meeting-server`，将 `MEMO_SERVER` 改为自己的 HTTPS 服务地址。眼镜上的 localhost 指眼镜自身。配置文件是 shell 配置，仅加载可信内容，不放 AI Key。

`MEMO_CA_FILE` 指向设备上的 PEM 格式可信 CA 文件。没有系统证书库时，准备包含服务证书所需根 CA 的可信证书包 `ca-certificates.crt`；不要关闭 TLS 校验。设备时间需正确。

```bash
adb -s "$ADB_SERIAL" shell mkdir -p /userdata/apps/memo-meeting
adb -s "$ADB_SERIAL" push meeting.env /userdata/apps/memo-meeting/meeting.env
adb -s "$ADB_SERIAL" push ca-certificates.crt /userdata/apps/memo-meeting/ca-certificates.crt
bash scripts/deploy.sh
bash scripts/install-voice.sh
```

若使用设备已有的可信证书库，按实际情况调整配置，省略上述 CA 上传步骤。四个程序必须已经位于 `dist/`。

注册后需重新建立固件原有 AI 语音连接，使新函数清单上报；安装脚本不重启设备。然后按用户指南扫码绑定。

## 检查与升级

```bash
adb -s "$ADB_SERIAL" shell /userdata/apps/memo-meeting/memo-glasses status
```

日常开启脚本会按需启动服务，不自动恢复上次录音。新版构建完成后重复部署与注册，保留已有配置、凭证及录音。现有脚本仅保留首次更新前的主程序副本，不是完整的版本回退机制；正式安装包发布前需补齐完整备份与回退。

服务日志为设备上的 `service.log`，录音在 `audio/`。日志和录音可能含个人信息，请勿直接上传到公开 Issue。

当前没有自动卸载工具；不要直接删除整个目录，以免丢失录音。卸载需要先停止本应用、仅移除本应用三个语音函数，再按是否保留数据决定清理哪些文件。
