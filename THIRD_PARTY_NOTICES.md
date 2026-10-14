# 第三方组件与外部 SDK

主程序依赖 `github.com/gorilla/websocket v1.5.3`（BSD-2-Clause）及 Go 标准库（Go 项目 BSD 风格许可证）。构建产物需附适用的版权与许可文本；依赖精确版本见 `go.mod`、`go.sum`。

`memo-notify` 和 `memo-media-state` 静态链接外部眼镜 SDK 的 `libai_glass_sdk.a`；`memo-capture-qr` 使用 SDK 头文件及相机 IPC。眼镜 SDK、固件、交叉编译器和其运行库不包含在本仓库，也不由本仓库的 Apache-2.0 重新授权。

当前尚未核实 SDK 的明确二进制再分发条款。源码可要求开发者自行取得 SDK；公开预编译包前需确认 SDK/运行库分发许可，保留需要的版权、许可和 NOTICE，记录其来源与版本。不能仅依据眼镜项目名称中的“开源”推定所有 SDK 文件可自由分发。

完整发布依赖需按实际链接与构建结果核对，本文件不是完整 SBOM。
