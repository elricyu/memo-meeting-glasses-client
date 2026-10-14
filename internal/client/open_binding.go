package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"
)

// Opening is an explicit user intent. No meeting starts before code binding succeeds.
type OpenPreparation struct {
	State    string `json:"state"`
	Action   string `json:"action"`
	Guidance string `json:"guidance,omitempty"`
}

func (r *Recorder) checkServerBinding(cid, mac, token string) (string, error) {
	input := map[string]string{"clientId": cid, "mac": mac}
	if token != "" {
		input["deviceToken"] = token
	}
	body, _ := json.Marshal(input)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.options.Server+"/api/v1/devices/binding-claims/check", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := r.http.Do(req)
	if err != nil {
		return "", errors.New("无法连接会议服务器，请检查网络，尚未开始录音")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return "", errors.New("服务器绑定状态检查失败，尚未开始录音")
	}
	var value struct{ State string }
	if json.NewDecoder(io.LimitReader(response.Body, 8192)).Decode(&value) != nil {
		return "", errors.New("服务器绑定状态返回无效")
	}
	switch value.State {
	case "BOUND", "PENDING", "UNBOUND", "REBIND_REQUIRED":
		return value.State, nil
	}
	return "", errors.New("服务器绑定状态无法识别")
}
func (r *Recorder) PrepareOpen() (OpenPreparation, error) {
	r.mu.Lock()
	if r.cancel != nil {
		r.mu.Unlock()
		return OpenPreparation{}, errors.New("会议记录正在运行")
	}
	if r.scanning {
		r.mu.Unlock()
		return OpenPreparation{State: "SCANNING", Action: "SCAN", Guidance: "bind-required"}, nil
	}
	var cid, mac, token string
	if r.credential != nil {
		cid, mac, token = r.credential.ClientID, r.credential.MAC, r.credential.DeviceToken
	}
	r.mu.Unlock()
	if cid == "" {
		var err error
		cid, mac, err = deviceHardware()
		if err != nil {
			return OpenPreparation{}, err
		}
	}
	return r.prepareOpen(cid, mac, token)
}
func (r *Recorder) prepareOpen(cid, mac, token string) (OpenPreparation, error) {
	state, err := r.checkServerBinding(cid, mac, token)
	if err != nil {
		return OpenPreparation{}, err
	}
	if state == "BOUND" {
		return OpenPreparation{State: state, Action: "RECORD"}, nil
	}
	if state == "REBIND_REQUIRED" {
		return OpenPreparation{}, errors.New("设备凭证不可用，请先在原账号解绑设备，尚未开始录音")
	}
	binding, err := r.beginCode(cid, mac)
	if err != nil {
		return OpenPreparation{}, err
	}
	if binding.State == "BOUND" {
		return OpenPreparation{State: "BOUND", Action: "RECORD"}, nil
	}
	return OpenPreparation{State: "PENDING", Action: "CODE"}, nil
}
