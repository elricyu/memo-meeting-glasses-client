package client

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
)

// BeginCode keeps the old control endpoint usable without activating the camera.
func (r *Recorder) BeginCode() (Binding, error) {
	cid, mac, err := deviceHardware()
	if err != nil {
		return Binding{}, err
	}
	return r.beginCode(cid, mac)
}
func (r *Recorder) beginCode(cid, mac string) (Binding, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil || r.scanning {
		return Binding{}, errors.New("会议记录或旧绑定正在运行")
	}
	if r.credential != nil {
		previousCode := r.credential.PairingCode
		binding, err := r.refreshBindingLocked()
		if err != nil {
			return Binding{}, err
		}
		if binding.State == "BOUND" {
			return binding, nil
		}
		if r.credential.CodeBinding && binding.State == "PENDING" {
			r.bindingNotice = true
			if previousCode == binding.PairingCode {
				r.notifyCode(false)
			}
			return binding, nil
		}
		if binding.State == "PENDING" {
			return Binding{}, errors.New("请先完成或取消原绑定申请")
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
	value := credential{Server: r.options.Server, ClientID: cid, MAC: mac, DisplayName: "Memo Meeting 眼镜", DeviceToken: hex.EncodeToString(token), RequestID: id, CodeBinding: true}
	r.scanMode = true
	r.unlockPassword = key
	if err = r.saveCredential(value, key, false); err != nil {
		return Binding{}, err
	}
	r.bindingNotice = true
	r.credential = &value
	r.options.DeviceToken = ""
	return r.initiatePairingLocked()
}
func (r *Recorder) notifyCode(renewed bool) {
	kind := "binding-code"
	if renewed {
		kind = "binding-code-renewed"
	}
	r.notify(kind, r.binding.PairingCode)
}
