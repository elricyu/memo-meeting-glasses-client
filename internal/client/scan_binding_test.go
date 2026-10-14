package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestScanBindingNeedsConfirmationAndSurvivesRestart(t *testing.T) {
	var confirmed atomic.Bool
	var token atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/devices/binding-claims/scan":
			var body struct {
				Device     struct{ ClientID, MAC, DeviceToken, ClientRequestID string }
				JPEGBase64 string
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil || body.Device.ClientID != "qr-test-client" || body.Device.MAC != "02:11:22:33:44:55" || body.JPEGBase64 == "" {
				t.Error("invalid QR request")
			}
			token.Store(body.Device.DeviceToken)
			json.NewEncoder(w).Encode(map[string]string{"pairingId": "test-pairing", "deviceId": "test-device", "expiresAt": "2099-01-01T00:00:00Z"})
		case "/api/v1/devices/pairings/test-pairing":
			if r.Header.Get("Authorization") != "Bearer "+token.Load().(string) {
				t.Error("device auth missing")
			}
			state := "PENDING"
			if confirmed.Load() {
				state = "BOUND"
			}
			json.NewEncoder(w).Encode(map[string]string{"status": state, "deviceId": "test-device"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	root := t.TempDir()
	helper := filepath.Join(root, "capture-test")
	os.WriteFile(helper, []byte("#!/bin/sh\nprintf '\\377\\330test-qr-fixture\\377\\331' > \"$1\"\n"), 0700)
	options := Options{Server: server.URL, DataDir: filepath.Join(root, "audio"), CaptureHelper: helper}
	recorder, err := NewRecorder(options)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = recorder.beginScan("qr-test-client", "02:11:22:33:44:55"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		recorder.mu.Lock()
		done := !recorder.scanning
		recorder.mu.Unlock()
		if done {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	binding, err := recorder.RefreshBinding()
	if err != nil || binding.State != "PENDING" {
		t.Fatalf("claim not pending: %v %v", binding, err)
	}
	if err = recorder.Start("test", ""); err == nil || !strings.Contains(err.Error(), "请先在网页确认绑定") {
		t.Fatal("unconfirmed device can start")
	}
	confirmed.Store(true)
	binding, err = recorder.RefreshBinding()
	if err != nil || binding.State != "BOUND" {
		t.Fatal("confirmation not observed")
	}
	encrypted, _ := os.ReadFile(filepath.Join(root, "scan-credentials.enc"))
	if strings.Contains(string(encrypted), token.Load().(string)) {
		t.Fatal("plaintext token stored")
	}
	restarted, err := NewRecorder(options)
	if err != nil {
		t.Fatal(err)
	}
	binding, err = restarted.RefreshBinding()
	if err != nil || binding.State != "BOUND" || restarted.options.DeviceToken != token.Load().(string) {
		t.Fatalf("automatic credential restoration failed: %v %v", binding, err)
	}
}
