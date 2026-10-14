package client

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// No credential or unlock password is written as plaintext on the glasses.
type credential struct {
	Server      string `json:"server"`
	ClientID    string `json:"clientId"`
	MAC         string `json:"mac"`
	DisplayName string `json:"displayName"`
	DeviceToken string `json:"deviceToken"`
	RequestID   string `json:"clientRequestId"`
	PairingID   string `json:"pairingId"`
}
type Binding struct {
	State       string `json:"state"`
	PairingCode string `json:"pairingCode,omitempty"`
	ExpiresAt   string `json:"expiresAt,omitempty"`
	DeviceID    string `json:"deviceId,omitempty"`
	Error       string `json:"error,omitempty"`
}

const vaultIterations = 600000

func sealCredential(value credential, password string) ([]byte, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, vaultIterations, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, err
	}
	header := append([]byte("MGC1"), salt...)
	header = append(header, nonce...)
	plain, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return gcm.Seal(header, nonce, plain, header), nil
}
func openCredential(data []byte, password string) (credential, error) {
	var value credential
	if len(data) < 48 || len(data) > 16384 || string(data[:4]) != "MGC1" {
		return value, errors.New("设备凭证文件格式错误")
	}
	key, err := pbkdf2.Key(sha256.New, password, data[4:20], vaultIterations, 32)
	if err != nil {
		return value, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return value, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return value, err
	}
	plain, err := gcm.Open(nil, data[20:32], data[32:], data[:32])
	if err != nil {
		return value, errors.New("凭证解锁失败：口令错误或文件损坏")
	}
	if err = json.Unmarshal(plain, &value); err != nil {
		return value, errors.New("设备凭证文件内容错误")
	}
	return value, nil
}
func (r *Recorder) vaultPath() string {
	if r.scanMode {
		return filepath.Join(filepath.Dir(r.options.DataDir), "scan-credentials.enc")
	}
	return filepath.Join(filepath.Dir(r.options.DataDir), "credentials.enc")
}
func (r *Recorder) saveCredential(value credential, password string, initial bool) error {
	path := r.vaultPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := sealCredential(value, password)
	if err != nil {
		return err
	}
	if initial {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return errors.New("凭证文件已存在或不可写，请先解锁原设备")
		}
		_, writeErr := f.Write(data)
		syncErr := f.Sync()
		closeErr := f.Close()
		return errors.Join(writeErr, syncErr, closeErr)
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".credentials-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	_, writeErr := f.Write(data)
	syncErr := f.Sync()
	closeErr := f.Close()
	if err = errors.Join(writeErr, syncErr, closeErr); err != nil {
		return err
	}
	return os.Rename(name, path)
}
func (r *Recorder) Pair(clientID, mac, displayName, password string) (Binding, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil {
		return Binding{}, errors.New("录音中不能重新绑定设备")
	}
	if len(password) < 12 || len(password) > 1024 {
		return Binding{}, errors.New("解锁口令需要 12–1024 字节")
	}
	hw, err := net.ParseMAC(mac)
	if err != nil || len(hw) != 6 || strings.TrimSpace(clientID) == "" || len(clientID) > 128 || strings.TrimSpace(displayName) == "" || len([]rune(displayName)) > 64 {
		return Binding{}, errors.New("设备身份参数无效")
	}
	token := make([]byte, 32)
	if _, err = rand.Read(token); err != nil {
		return Binding{}, err
	}
	id, err := randomUUID()
	if err != nil {
		return Binding{}, err
	}
	value := credential{Server: r.options.Server, ClientID: clientID, MAC: strings.ToUpper(hw.String()), DisplayName: displayName, DeviceToken: hex.EncodeToString(token), RequestID: id}
	// Save before HTTP: retry the same request after a lost response, without creating another credential.
	if err = r.saveCredential(value, password, true); err != nil {
		return Binding{}, err
	}
	r.credential = &value
	r.unlockPassword = password
	return r.initiatePairingLocked()
}
func (r *Recorder) Unlock(password string) (Binding, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil {
		return Binding{}, errors.New("录音中不能更换设备凭证")
	}
	f, err := os.Open(r.vaultPath())
	if err != nil {
		return Binding{}, errors.New("没有可解锁的设备凭证，请先发起绑定")
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 16385))
	if err != nil {
		return Binding{}, err
	}
	value, err := openCredential(data, password)
	if err != nil {
		return Binding{}, err
	}
	if value.Server != r.options.Server {
		return Binding{}, errors.New("凭证属于其他后端，请检查服务地址")
	}
	r.credential = &value
	r.unlockPassword = password
	if value.PairingID == "" {
		return r.initiatePairingLocked()
	}
	return r.refreshBindingLocked()
}
func (r *Recorder) initiatePairingLocked() (Binding, error) {
	value := r.credential
	payload := map[string]string{"clientId": value.ClientID, "mac": value.MAC, "displayName": value.DisplayName, "platform": "RV1106B", "deviceToken": value.DeviceToken, "clientRequestId": value.RequestID}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, r.options.Server+"/api/v1/devices/pairings", bytes.NewReader(body))
	if err != nil {
		return Binding{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := r.http.Do(req)
	if err != nil {
		return Binding{}, errors.New("绑定服务不可达；凭证已加密保存，可解锁后重试")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return Binding{}, fmt.Errorf("绑定申请失败：HTTP %d", response.StatusCode)
	}
	var result struct{ PairingID, DeviceID, PairingCode, ExpiresAt string }
	if err = json.NewDecoder(io.LimitReader(response.Body, 16384)).Decode(&result); err != nil || result.PairingID == "" || result.PairingCode == "" {
		return Binding{}, errors.New("绑定服务返回无效数据")
	}
	value.PairingID = result.PairingID
	if err = r.saveCredential(*value, r.unlockPassword, false); err != nil {
		return Binding{}, err
	}
	r.binding = Binding{State: "PENDING", PairingCode: result.PairingCode, ExpiresAt: result.ExpiresAt, DeviceID: result.DeviceID}
	return r.binding, nil
}
func (r *Recorder) RefreshBinding() (Binding, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.refreshBindingLocked()
}
func (r *Recorder) refreshBindingLocked() (Binding, error) {
	if r.credential == nil {
		if r.scanning {
			return r.binding, nil
		}
		return Binding{State: "LOCKED"}, nil
	}
	if r.credential.PairingID == "" {
		if r.scanMode {
			return r.recoverScanLocked()
		}
		return r.initiatePairingLocked()
	}
	req, err := http.NewRequest(http.MethodGet, r.options.Server+"/api/v1/devices/pairings/"+r.credential.PairingID, nil)
	if err != nil {
		return Binding{}, err
	}
	req.Header.Set("Authorization", "Bearer "+r.credential.DeviceToken)
	response, err := r.http.Do(req)
	if err != nil {
		return Binding{}, errors.New("绑定状态查询失败")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return Binding{}, fmt.Errorf("绑定状态查询失败：HTTP %d", response.StatusCode)
	}
	var result struct{ Status, DeviceID string }
	if err = json.NewDecoder(io.LimitReader(response.Body, 16384)).Decode(&result); err != nil {
		return Binding{}, errors.New("绑定状态格式错误")
	}
	r.binding.State = result.Status
	r.binding.DeviceID = result.DeviceID
	if r.cancel == nil {
		if result.Status == "BOUND" {
			r.options.DeviceToken = r.credential.DeviceToken
		} else {
			r.options.DeviceToken = ""
		}
	}
	return r.binding, nil
}
