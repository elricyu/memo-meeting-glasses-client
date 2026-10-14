# Third-party notices

The original application code is Apache-2.0. This does not relicense third-party components.

- Go runtime and standard library: Go BSD-style license, `licenses/Go.txt`.
- gorilla/websocket v1.5.3: BSD-2-Clause, `licenses/gorilla-websocket.txt`.
- OpenSource-Ai-Glasses SDK 0.7.2, upstream commit `dbc981216b8ef5525d8ce7d0cc5914db606ab6e0`: Apache-2.0, `licenses/OpenSource-Ai-Glasses.txt`. Copyright 2025 Zhejiang Shengyi Intelligent Optical Perception Technology Co., Ltd.; Copyright 2025 Hangzhou EasyXR Advanced Technology Co.,Ltd. Source: https://github.com/Iam5tillLearning/OpenSource-Ai-Glasses . `memo-notify` and `memo-media-state` link the upstream static SDK; the SDK archive and firmware are not bundled.
- Public trusted root certificates: sourced from the Debian ca-certificates trust store, `licenses/ca-certificates.txt`; selection described in `certs/README.md`.
- uClibc: dynamically supplied by the existing firmware, not distributed or replaced by this installer; license text retained as `licenses/uClibc.txt` for reference.

See `BUILD.json` in the installation ZIP for the exact source and build inputs. No cloud service credentials or user data are included. SDK helper programs are original application integrations; upstream SDK code itself has not been modified by this project.
