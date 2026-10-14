# 固件兼容性与构建来源

本项目针对 [OpenSource-Ai-Glasses](https://github.com/Iam5tillLearning/OpenSource-Ai-Glasses) 的 RV1106B 开源眼镜。使用以下固件接口：

- `/tmp/ai-core_audio_stream`：AASF v1 音频镜像，AF_UNIX / SOCK_SEQPACKET。
- 现有相机 JPEG 抓拍 IPC。
- SDK `ai_audio_play_toast_text`、GPIO hub 客户端。
- `/tmp/ai-core_audio_ctrl` 的 `PLAYBACK_STATUS`。
- `/usr/share/ai_core_device_functions.conf` 语音函数注册与执行。
- BusyBox、可写 `/userdata/apps/`、ADB、可信 CA 和正确设备时间。

Beta 构建使用上游提交 `dbc981216b8ef5525d8ce7d0cc5914db606ab6e0` 中的 SDK（版本 0.7.2），工具链为 `arm-rockchip831-linux-uclibcgnueabihf`。完整校验值与编译器版本记录于安装包 `BUILD.json`。

真机系统标识为 `Buildroot 2025.08-git`、ARMv7、`/lib/ld-uClibc.so.0`。当前设备未提供可核实的固件 Release 版本号，因此此 Beta 不宣称已验证所有上游固件版本；安装器会检查运行接口并在失败时恢复旧程序。

C 辅助组件动态依赖固件自带 uClibc，安装包不替换系统库。SDK 的静态库按上游 Apache-2.0 许可链接，随包保留许可和版权声明。上游固件更新后仍需重新验收音频镜像、GPIO hub、相机和播放状态兼容性。
