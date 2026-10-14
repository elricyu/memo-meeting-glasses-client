GO ?= go
CAMERA_CC ?= arm-rockchip831-linux-uclibcgnueabihf-gcc
SDK_DIR ?=
export GOCACHE ?= /tmp/memo-glasses-go-cache
export GOMODCACHE ?= /tmp/memo-glasses-go-mod
.PHONY: test build arm camera notify media

test:
	$(GO) test ./...
	python3 scripts/test-register-voice.py
	sh -n device/memo-meeting-open.sh device/memo-meeting-close.sh device/memo-meeting-daemon.sh device/memo-meeting-bind.sh
	bash -n scripts/deploy.sh scripts/install-voice.sh
build:
	mkdir -p dist
	CGO_ENABLED=0 $(GO) build -buildvcs=false -trimpath -ldflags="-s -w" -o dist/memo-glasses ./cmd/memo-glasses
arm:
	mkdir -p dist
	CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 $(GO) build -buildvcs=false -trimpath -ldflags="-s -w" -o dist/memo-glasses-armv7 ./cmd/memo-glasses
camera:
	test -f "$(SDK_DIR)/include/ai_camera.h"
	mkdir -p dist
	$(CAMERA_CC) -Os -Wall -Wextra -I"$(SDK_DIR)/include" device/capture-qr.c -lrt -o dist/memo-capture-qr
notify:
	test -f "$(SDK_DIR)/lib/libai_glass_sdk.a"
	mkdir -p dist
	$(CAMERA_CC) -Os -Wall -Wextra -I"$(SDK_DIR)/include" device/notify.c "$(SDK_DIR)/lib/libai_glass_sdk.a" -pthread -lrt -lm -o dist/memo-notify
media:
	test -f "$(SDK_DIR)/lib/libai_glass_sdk.a"
	mkdir -p dist
	$(CAMERA_CC) -Os -Wall -Wextra -I"$(SDK_DIR)/include" device/media-state.c "$(SDK_DIR)/lib/libai_glass_sdk.a" -pthread -lrt -lm -o dist/memo-media-state
