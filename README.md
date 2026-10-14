# Memo Meeting Glasses Client

专为 [OpenSource-Ai-Glasses](https://github.com/Iam5tillLearning/OpenSource-Ai-Glasses) 项目的 RV1106B 开源眼镜编写的 ARM Linux 会议记录应用，使用现有固件的音频镜像、相机和语音设备函数。原有物理按钮交互保留，安装无需重刷固件。

配套服务及网页在独立 [memo-meeting-server](https://github.com/elricyu/memo-meeting-server) 仓库维护。用户在网页扫码绑定账号，再通过“打开会议记录 / 关闭会议记录”控制录音。

## 默认线上服务

眼镜客户端默认连接 [https://memo-meeting.easymr.cn](https://memo-meeting.easymr.cn)。安装应用后，在网站注册账号，在「我的眼镜」生成二维码并完成扫码确认绑定，即可使用“打开会议记录 / 关闭会议记录”。使用此实例无需自行部署后端或填写 AI Key。

如需使用自己的服务，可以部署 [memo-meeting-server](https://github.com/elricyu/memo-meeting-server)，将设备 `meeting.env` 中的 `MEMO_SERVER` 改为自己的 HTTPS 服务地址，并配置对应可信 CA。账号与设备绑定在所连接的服务端管理，更换实例后需在新实例注册账号并绑定眼镜。

## 关联项目

- [OpenSource-Ai-Glasses](https://github.com/Iam5tillLearning/OpenSource-Ai-Glasses)：本应用运行的眼镜平台，提供固件、SDK 和硬件相关说明。
- [memo-meeting-glasses-client](https://github.com/elricyu/memo-meeting-glasses-client)（本仓库）：安装在眼镜上的会议记录应用。
- [memo-meeting-server](https://github.com/elricyu/memo-meeting-server)：配套后端与 Web，负责账号绑定、音频接收、实时转录和会议纪要。

眼镜平台的配置与固件资料请查阅上游项目；本应用的安装和使用请查阅本仓库文档。固件兼容性以实际接口和验收结果为准。

## 能力与要求

- 音频镜像采集、G711A 解码、16kHz 单声道 PCM16 上传、本地 WAV 保存。
- 扫码绑定、网页确认、凭证自动恢复、解绑失效。
- 录音就绪与停止保存确认；按键/播放状态过滤。

目标系统为 RV1106B / Buildroot / ARMv7 Linux，不是 Android APK。固件需提供音频镜像、相机抓拍、GPIO hub、播放状态和设备函数执行接口；仅芯片相同不能证明固件兼容。具体要求见 [兼容性说明](docs/compatibility.md)。

录音需要可达的 Memo Meeting 服务和固件原有语音 AI 链路；不支持离线转录或断网自动补传。AI Key 仅在后端，眼镜持有专属设备凭证。

## 安装和使用

预编译安装包将在 GitHub Releases 提供；当前目录中的 `dist/` 是本地构建产物，不提交源码仓库。安装步骤见 [安装说明](docs/install.md)，日常使用见 [用户指南](docs/user-guide.md)。

当前脚本需要 Linux/macOS 或具备 Bash 的电脑、ADB 和 Python 3。配置模板已使用上述线上实例地址；首次安装需安装服务配置与可信 CA，再部署程序和注册语音函数。暂未提供 Windows 原生安装器。

## 从源码构建

Go 1.24+ 用于主程序；C 辅助组件需要对应眼镜 SDK 和 RV1106 交叉编译器，Python 3 用于语音配置合并与测试。

```bash
make test
make arm
make camera notify media CAMERA_CC=/path/to/arm-rockchip831-linux-uclibcgnueabihf-gcc SDK_DIR=/path/to/sdk
```

主程序为 `dist/memo-glasses-armv7`，目标 `linux/arm`、`GOARM=7`、`CGO_ENABLED=0`。辅助程序为 `memo-capture-qr`、`memo-notify`、`memo-media-state`，发布时必须全部提供。

- [开发与测试](docs/development.md)
- [发布说明](docs/releasing.md)
- [贡献指南](CONTRIBUTING.md)、[安全说明](SECURITY.md)
- [第三方依赖与 SDK 许可](THIRD_PARTY_NOTICES.md)

## 许可

本项目原创代码采用 [Apache License 2.0](LICENSE)。眼镜固件、SDK 和工具链不包含在本仓库中，适用各自许可。预编译包包含链接 SDK 的组件，分发前需核实 SDK 的再分发条款。
