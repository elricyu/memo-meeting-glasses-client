# 发布

本版本：`v0.1.0-beta.1`。源码标签与预编译安装包一起发布，二进制、SDK、运行数据不进入源码 Git。

```bash
make test arm camera notify media GO=/path/to/go CAMERA_CC=/path/to/compiler SDK_DIR=/path/to/sdk
python3 scripts/package-release.py --sdk-dir /path/to/sdk --compiler /path/to/compiler --go /path/to/go
```

打包工具仅选择明确的文件清单，生成 ZIP、包内校验清单、外部 SHA256SUMS 和 BUILD.json，包含来源提交、SDK 版本与库校验值、编译器版本、依赖许可和 CA。不会打包本地凭证、配置、日志或录音。

发布前运行管理工具测试、真机只读组件/HTTPS检查、打包后管理工具校验及兼容固件验收。GitHub Release 标记 Pre-release，附 ZIP 和外部 SHA256SUMS；Release 描述提供使用、升级与回退步骤和实际验证边界。

本仓库不包含 Android APK；Windows/Mac/Linux 是电脑安装工具平台，不是眼镜二进制的目标平台。眼镜程序目标为 ARMv7 / RV1106B。
