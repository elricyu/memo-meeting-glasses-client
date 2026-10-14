# 固件兼容性

本项目只针对 RV1106B 开源眼镜，不提供其他设备适配承诺。依赖当前固件接口：

- `/tmp/ai-core_audio_stream`：AASF v1 音频镜像，AF_UNIX / SOCK_SEQPACKET。
- 现有相机 JPEG 抓拍 IPC。
- SDK `ai_audio_play_toast_text`、GPIO hub 客户端。
- `/tmp/ai-core_audio_ctrl` 的 `PLAYBACK_STATUS` 查询。
- `/usr/share/ai_core_device_functions.conf` 的语音函数配置及脚本执行。
- 可写的 `/userdata/apps/`、BusyBox、ADB 和可用 HTTPS/设备时钟。

具体接口实现见 `device/*.c` 和 `internal/client/`。主程序不依赖 Go 运行时安装；C 程序需要与固件 ABI 匹配的交叉工具链。

正式二进制发布前，须记录原眼镜项目 URL、固件版本或提交号、SDK 版本/校验值及工具链版本。目前仓库未获得可核实的上游版本标识，因此不宣称任意该型号固件均已验证。不要提交整个 SDK 或固件树。
