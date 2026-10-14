package client

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCredentialEncryptionRejectsWrongPasswordAndTampering(t *testing.T) {
	value := credential{DeviceToken: strings.Repeat("a", 64), Server: "https://meeting.example"}
	sealed, err := sealCredential(value, "correct-password-for-test")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte(value.DeviceToken)) {
		t.Fatal("plaintext credential persisted")
	}
	if _, err = openCredential(sealed, "wrong-password"); err == nil {
		t.Fatal("wrong password accepted")
	}
	decoded, err := openCredential(sealed, "correct-password-for-test")
	if err != nil || decoded.DeviceToken != value.DeviceToken {
		t.Fatal("credential roundtrip failed")
	}
	sealed[len(sealed)-1] ^= 1
	if _, err = openCredential(sealed, "correct-password-for-test"); err == nil {
		t.Fatal("tampered vault accepted")
	}
}

func TestPairUnlockAfterRestartAndRevocation(t *testing.T) {
	var token string
	state := "PENDING"
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/devices/pairings", func(w http.ResponseWriter, r *http.Request) {
		var input map[string]string
		json.NewDecoder(r.Body).Decode(&input)
		if len(input["deviceToken"]) != 64 || input["platform"] != "RV1106B" || input["mac"] != "AA:BB:CC:DD:EE:FF" {
			t.Error("invalid pairing identity")
		}
		token = input["deviceToken"]
		json.NewEncoder(w).Encode(map[string]string{"pairingId": "pair-id", "pairingCode": "ABCD1234", "deviceId": "device-id", "expiresAt": "2026-10-13T12:00:00Z"})
	})
	mux.HandleFunc("GET /api/v1/devices/pairings/pair-id", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "not found", 404)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": state, "deviceId": "device-id"})
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	options := Options{Server: server.URL, DataDir: filepath.Join(t.TempDir(), "audio")}
	recorder, _ := NewRecorder(options)
	if err := recorder.Start("test", ""); err == nil {
		t.Fatal("unbound device accepted recording")
	}
	binding, err := recorder.Pair("rv1106-test", "aa:bb:cc:dd:ee:ff", "test glasses", "test-unlock-password")
	if err != nil || binding.State != "PENDING" {
		t.Fatalf("pair failed: %v", err)
	}
	if _, err = recorder.Pair("rv1106-test", "aa:bb:cc:dd:ee:ff", "test glasses", "test-unlock-password"); err == nil {
		t.Fatal("existing credential silently replaced")
	}
	data, err := os.ReadFile(recorder.vaultPath())
	if err != nil || bytes.Contains(data, []byte(token)) {
		t.Fatal("plaintext token on disk")
	}
	info, _ := os.Stat(recorder.vaultPath())
	if info.Mode().Perm() != 0600 {
		t.Fatal("unsafe vault permissions")
	}
	state = "BOUND"
	restarted, _ := NewRecorder(options)
	if _, err = restarted.Unlock("incorrect"); err == nil {
		t.Fatal("wrong unlock password accepted")
	}
	binding, err = restarted.Unlock("test-unlock-password")
	if err != nil || binding.State != "BOUND" || restarted.options.DeviceToken != token {
		t.Fatalf("restart unlock failed: %v", err)
	}
	state = "REVOKED"
	binding, err = restarted.RefreshBinding()
	if err != nil || binding.State != "REVOKED" || restarted.options.DeviceToken != "" {
		t.Fatal("revoked token kept enabled")
	}
	if err = restarted.Start("test", ""); err == nil {
		t.Fatal("revoked device accepted recording")
	}
}
