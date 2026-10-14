package client

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// QR binding uses device-local random key protection and OS file permissions.
// The legacy password-protected vault is preserved and never overwritten.
func (r *Recorder) autoKeyPath() string {
	return filepath.Join(filepath.Dir(r.options.DataDir), "scan-key")
}
func (r *Recorder) loadAutomaticCredential() error {
	path := filepath.Join(filepath.Dir(r.options.DataDir), "scan-credentials.enc")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	key, err := os.ReadFile(r.autoKeyPath())
	if err != nil || len(key) != 64 {
		return errors.New("扫码凭证的本地密钥缺失，原凭证未覆盖")
	}
	value, err := openCredential(data, string(key))
	if err != nil {
		return err
	}
	if value.Server != r.options.Server {
		return errors.New("扫码凭证属于其他会议服务，不能自动迁移")
	}
	r.scanMode = true
	r.credential = &value
	r.unlockPassword = string(key)
	r.binding = Binding{State: "PENDING"}
	return nil
}
func (r *Recorder) autoKeyLocked() (string, error) {
	if err := os.MkdirAll(filepath.Dir(r.options.DataDir), 0700); err != nil {
		return "", err
	}
	data, err := os.ReadFile(r.autoKeyPath())
	if err == nil {
		if len(data) != 64 {
			return "", errors.New("扫码密钥格式错误")
		}
		return string(data), nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	key := make([]byte, 32)
	if _, err = rand.Read(key); err != nil {
		return "", err
	}
	value := hex.EncodeToString(key)
	f, err := os.OpenFile(r.autoKeyPath(), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	_, writeErr := f.Write([]byte(value))
	syncErr := f.Sync()
	closeErr := f.Close()
	return value, errors.Join(writeErr, syncErr, closeErr)
}
func deviceHardware() (string, string, error) {
	raw, err := os.ReadFile("/sys/class/net/wlan0/address")
	if err != nil {
		return "", "", errors.New("无法读取眼镜Wi-Fi MAC")
	}
	mac, err := net.ParseMAC(strings.TrimSpace(string(raw)))
	if err != nil || len(mac) != 6 {
		return "", "", errors.New("眼镜MAC格式无效")
	}
	var cid string
	for _, path := range []string{"/sys/block/mmcblk0/device/cid", "/sys/class/mmc_host/mmc0/mmc0:0001/cid"} {
		if value, e := os.ReadFile(path); e == nil {
			cid = strings.TrimSpace(string(value))
			break
		}
	}
	if len(cid) != 32 {
		return "", "", errors.New("无法读取稳定eMMC客户端标识")
	}
	if _, err = hex.DecodeString(cid); err != nil {
		return "", "", errors.New("eMMC标识格式无效")
	}
	return "RV1106B-" + strings.ToLower(cid), strings.ToUpper(mac.String()), nil
}
func (r *Recorder) BeginScan() (Binding, error) {
	cid, mac, err := deviceHardware()
	if err != nil {
		return Binding{}, err
	}
	return r.beginScan(cid, mac)
}
func (r *Recorder) beginScan(cid, mac string) (Binding, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil || r.scanning {
		return Binding{}, errors.New("当前正在录音或扫码，请等待结束")
	}
	if r.credential != nil {
		value, err := r.refreshBindingLocked()
		if err != nil {
			return Binding{}, err
		}
		if value.State == "BOUND" {
			return Binding{}, errors.New("这副眼镜已绑定，请先在原账号下解绑")
		}
	}
	key, err := r.autoKeyLocked()
	if err != nil {
		return Binding{}, err
	}
	token := make([]byte, 32)
	if _, err = rand.Read(token); err != nil {
		return Binding{}, err
	}
	id, err := randomUUID()
	if err != nil {
		return Binding{}, err
	}
	value := credential{Server: r.options.Server, ClientID: cid, MAC: mac, DisplayName: "Memo Meeting 眼镜", DeviceToken: hex.EncodeToString(token), RequestID: id}
	r.scanMode = true
	r.unlockPassword = key
	if err = r.saveCredential(value, key, false); err != nil {
		return Binding{}, err
	}
	r.credential = &value
	r.options.DeviceToken = ""
	r.scanning = true
	r.binding = Binding{State: "SCANNING"}
	go r.scanScreens(value)
	return r.binding, nil
}
func (r *Recorder) scanScreens(value credential) {
	defer func() { r.mu.Lock(); r.scanning = false; r.mu.Unlock() }()
	helper := r.options.CaptureHelper
	if helper == "" {
		helper = "/userdata/apps/memo-meeting/memo-capture-qr"
	}
	r.notify("scan-start")
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	var last error
	failureNotice := "scan-failed"
	for attempt := 0; attempt < 50 && ctx.Err() == nil; attempt++ {
		file, err := os.CreateTemp(filepath.Dir(r.options.DataDir), ".binding-*.jpg")
		if err != nil {
			last = err
			break
		}
		path := file.Name()
		file.Close()
		captureCtx, stop := context.WithTimeout(ctx, 8*time.Second)
		err = exec.CommandContext(captureCtx, helper, path).Run()
		stop()
		if err != nil {
			os.Remove(path)
			last = errors.New("相机抓拍失败，请确认眼镜相机可用")
			failureNotice = "camera-failed"
			break
		}
		f, e := os.Open(path)
		if e != nil {
			os.Remove(path)
			last = e
			break
		}
		jpeg, e := io.ReadAll(io.LimitReader(f, 2097153))
		f.Close()
		os.Remove(path)
		if e != nil || len(jpeg) > 2097152 {
			last = errors.New("扫码图片过大或不可读取")
			break
		}
		payload := map[string]any{"device": map[string]string{"clientId": value.ClientID, "mac": value.MAC, "displayName": value.DisplayName, "platform": "RV1106B", "deviceToken": value.DeviceToken, "clientRequestId": value.RequestID}, "jpegBase64": base64.StdEncoding.EncodeToString(jpeg)}
		body, _ := json.Marshal(payload)
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, r.options.Server+"/api/v1/devices/binding-claims/scan", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		response, e := r.http.Do(req)
		if e != nil {
			last = errors.New("会议服务不可达，请检查网络")
			failureNotice = "network-failed"
			break
		}
		var result struct{ PairingID, DeviceID, ExpiresAt string }
		decodeErr := json.NewDecoder(io.LimitReader(response.Body, 16384)).Decode(&result)
		response.Body.Close()
		if response.StatusCode == 400 {
			last = errors.New("未识别到二维码，请正对屏幕，调整距离后重试")
			if attempt == 3 || attempt == 15 || attempt == 30 {
				r.notify("scan-adjust")
			}
			if !scanPause(ctx, 3*time.Second) {
				break
			}
			continue
		}
		if response.StatusCode == 409 {
			last = errors.New("此眼镜已有绑定或申请，请在原账号解绑旧设备或取消旧申请")
			if attempt == 0 || attempt == 15 {
				r.notify("rebind-required")
			}
			if !scanPause(ctx, 3*time.Second) {
				break
			}
			continue
		}
		if response.StatusCode == 429 {
			last = errors.New("扫码服务繁忙，请稍后重试")
			if !scanPause(ctx, 5*time.Second) {
				break
			}
			continue
		}
		if response.StatusCode == 404 {
			failureNotice = "qr-expired"
		}
		if response.StatusCode != 200 || decodeErr != nil || result.PairingID == "" {
			last = fmt.Errorf("扫码申请未完成（HTTP %d），请检查二维码是否过期、设备是否已绑定", response.StatusCode)
			break
		}
		r.mu.Lock()
		value.PairingID = result.PairingID
		err = r.saveCredential(value, r.unlockPassword, false)
		r.mu.Unlock()
		if err != nil {
			last = err
			break
		}
		r.notify("scan-recognized")
		// Keep polling in SCANNING until the recognition prompt has been queued.
		// A fast web confirmation must not announce success before this prompt.
		r.mu.Lock()
		r.credential = &value
		r.binding = Binding{State: "PENDING", DeviceID: result.DeviceID, ExpiresAt: result.ExpiresAt}
		r.mu.Unlock()
		return
	}
	r.mu.Lock()
	message := "扫码已超时，请重新生成二维码并重试"
	if last != nil {
		message = last.Error()
	}
	r.binding = Binding{State: "SCAN_FAILED", Error: message}
	r.mu.Unlock()
	r.notify(failureNotice)
}
func scanPause(ctx context.Context, duration time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(duration):
		return true
	}
}
func (r *Recorder) recoverScanLocked() (Binding, error) {
	if r.scanning {
		return r.binding, nil
	}
	body, _ := json.Marshal(map[string]string{"clientRequestId": r.credential.RequestID, "deviceToken": r.credential.DeviceToken})
	req, _ := http.NewRequest(http.MethodPost, r.options.Server+"/api/v1/devices/binding-claims/recover", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	response, err := r.http.Do(req)
	if err != nil {
		return Binding{}, errors.New("扫码凭证恢复查询失败")
	}
	defer response.Body.Close()
	if response.StatusCode == 404 {
		if r.binding.State == "SCAN_FAILED" {
			return r.binding, nil
		}
		return Binding{State: "UNPAIRED"}, nil
	}
	if response.StatusCode != 200 {
		return Binding{}, errors.New("扫码凭证恢复被拒绝")
	}
	var result struct{ PairingID string }
	if err = json.NewDecoder(io.LimitReader(response.Body, 16384)).Decode(&result); err != nil || result.PairingID == "" {
		return Binding{}, errors.New("扫码凭证恢复返回无效数据")
	}
	r.credential.PairingID = result.PairingID
	if err = r.saveCredential(*r.credential, r.unlockPassword, false); err != nil {
		return Binding{}, err
	}
	return r.refreshBindingLocked()
}
