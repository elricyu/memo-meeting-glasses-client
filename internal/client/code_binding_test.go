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
)

func TestCodeBindingRenewsAndRestoresSameCredential(t *testing.T) {
	var expired, bound atomic.Bool
	var allocations atomic.Int32
	var first struct{ DeviceToken, ClientRequestID string }
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/devices/pairings":
			var input struct{ DeviceToken, ClientRequestID string }
			json.NewDecoder(r.Body).Decode(&input)
			if allocations.Add(1) == 1 {
				first = input
			} else if input != first {
				t.Error("renewal replaced durable credential or request")
			}
			code := "0382"
			if expired.Load() {
				code = "9021"
				expired.Store(false)
			}
			json.NewEncoder(w).Encode(map[string]string{"pairingId": "pair", "pairingCode": code, "deviceId": "device", "expiresAt": "2099-01-01T00:00:00Z"})
		case "/api/v1/devices/pairings/pair":
			if r.Header.Get("Authorization") != "Bearer "+first.DeviceToken {
				t.Error("poll lacks credential")
			}
			state := "PENDING"
			if expired.Load() {
				state = "EXPIRED"
			}
			if bound.Load() {
				state = "BOUND"
			}
			json.NewEncoder(w).Encode(map[string]string{"status": state, "deviceId": "device"})
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	root := t.TempDir()
	events := filepath.Join(root, "events")
	helper := filepath.Join(root, "notify")
	os.WriteFile(helper, []byte("#!/bin/sh\nprintf '%s %s\\n' \"$1\" \"$2\" >> '"+events+"'\n"), 0700)
	options := Options{Server: server.URL, DataDir: filepath.Join(root, "audio"), NotifyHelper: helper}
	recorder, err := NewRecorder(options)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := recorder.beginCode("client", "02:11:22:33:44:55")
	if err != nil || binding.PairingCode != "0382" {
		t.Fatalf("initial code: %+v %v", binding, err)
	}
	// A repeated intent replays the valid code without issuing another one.
	if _, err = recorder.beginCode("client", "02:11:22:33:44:55"); err != nil {
		t.Fatal(err)
	}
	if allocations.Load() != 1 {
		t.Fatal("repeated command allocated another code")
	}
	recorder, err = NewRecorder(options)
	if err != nil {
		t.Fatal(err)
	}
	binding, err = recorder.RefreshBinding()
	if err != nil || binding.PairingCode != "0382" || allocations.Load() != 1 {
		t.Fatalf("restoration: %+v %v", binding, err)
	}
	expired.Store(true)
	binding, err = recorder.RefreshBinding()
	if err != nil || binding.PairingCode != "9021" || binding.State != "PENDING" {
		t.Fatalf("renewal: %+v %v", binding, err)
	}
	eventsData, _ := os.ReadFile(events)
	if !strings.Contains(string(eventsData), "binding-code 0382") || !strings.Contains(string(eventsData), "binding-code-renewed 9021") {
		t.Fatalf("missing voice guidance: %s", eventsData)
	}
	bound.Store(true)
	binding, err = recorder.RefreshBinding()
	if err != nil || binding.State != "BOUND" || binding.PairingCode != "" || recorder.options.DeviceToken != first.DeviceToken {
		t.Fatalf("bound: %+v %v", binding, err)
	}
	encrypted, _ := os.ReadFile(filepath.Join(root, "scan-credentials.enc"))
	if strings.Contains(string(encrypted), first.DeviceToken) {
		t.Fatal("plaintext device credential")
	}
	if recorder.Status().State != "idle" {
		t.Fatal("binding alone started recording without an active open command")
	}
}
