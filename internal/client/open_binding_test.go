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

func TestOpenChecksServerThenScansWithFeedbackWithoutStartingRecording(t *testing.T) {
	var checks, claims atomic.Int32
	var confirmed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/devices/binding-claims/check":
			checks.Add(1)
			json.NewEncoder(w).Encode(map[string]string{"state": "UNBOUND"})
		case "/api/v1/devices/binding-claims/scan":
			if checks.Load() == 0 {
				t.Error("scan preceded server check")
			}
			claims.Add(1)
			json.NewEncoder(w).Encode(map[string]string{"pairingId": "open-test-pair", "deviceId": "device", "expiresAt": "2099-01-01T00:00:00Z"})
		case "/api/v1/devices/pairings/open-test-pair":
			state := "PENDING"
			if confirmed.Load() {
				state = "BOUND"
			}
			json.NewEncoder(w).Encode(map[string]string{"status": state})
		default:
			t.Errorf("unexpected request before confirmation: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	root := t.TempDir()
	capture := filepath.Join(root, "capture")
	notify := filepath.Join(root, "notify")
	events := filepath.Join(root, "events")
	os.WriteFile(capture, []byte("#!/bin/sh\nprintf '\\377\\330fixture\\377\\331' > \"$1\"\n"), 0700)
	os.WriteFile(notify, []byte("#!/bin/sh\nprintf '%s\\n' \"$1\" >> '"+events+"'\n"), 0700)
	recorder, err := NewRecorder(Options{Server: server.URL, DataDir: filepath.Join(root, "audio"), CaptureHelper: capture, NotifyHelper: notify})
	if err != nil {
		t.Fatal(err)
	}
	preparation, err := recorder.prepareOpen("client", "02:11:22:33:44:55", "")
	if err != nil || preparation.Action != "SCAN" {
		t.Fatalf("no scan preparation: %+v %v", preparation, err)
	}
	for i := 0; i < 200; i++ {
		recorder.mu.Lock()
		done := !recorder.scanning
		recorder.mu.Unlock()
		if done {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if claims.Load() != 1 || recorder.Status().State != "idle" || recorder.Status().MeetingID != "" {
		t.Fatal("recording created before confirmation")
	}
	data, _ := os.ReadFile(events)
	if !strings.Contains(string(data), "scan-start") || !strings.Contains(string(data), "scan-recognized") {
		t.Fatalf("missing feedback: %s", data)
	}
	if err = recorder.Start("test", ""); err == nil {
		t.Fatal("unconfirmed recording allowed")
	}
	confirmed.Store(true)
	binding, err := recorder.RefreshBinding()
	if err != nil || binding.State != "BOUND" {
		t.Fatal("confirmation missing")
	}
}
func TestUnavailableBindingCheckDoesNotStartScanning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "unavailable", 503) }))
	defer server.Close()
	recorder, err := NewRecorder(Options{Server: server.URL, DataDir: filepath.Join(t.TempDir(), "audio")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = recorder.prepareOpen("client", "02:11:22:33:44:55", ""); err == nil {
		t.Fatal("unavailable server treated as unbound")
	}
	if recorder.scanning || recorder.credential != nil || recorder.Status().State != "idle" {
		t.Fatal("failed check mutated binding or started recording")
	}
}
