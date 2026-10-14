package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTranscriptUsesOnlyBoundCredentialAndCurrentMeeting(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/api/v1/meetings/current-meeting" || r.Header.Get("Authorization") != "Bearer device-fixture" {
			t.Error("wrong meeting scope or credential")
		}
		if calls == 2 {
			http.Error(w, "secret diagnostic", 403)
			return
		}
		w.Write([]byte(`{"draft":"正在讨论","segments":[]}`))
	}))
	defer server.Close()
	recorder, err := NewRecorder(Options{Server: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = recorder.Transcript(context.Background()); err == nil || calls != 0 {
		t.Fatal("locked device accessed backend")
	}
	recorder.status.MeetingID = "current-meeting"
	recorder.options.DeviceToken = "device-fixture"
	body, err := recorder.Transcript(context.Background())
	if err != nil || !json.Valid(body) || !strings.Contains(string(body), "正在讨论") {
		t.Fatalf("invalid transcript: %s %v", body, err)
	}
	body, err = recorder.Transcript(context.Background())
	if err == nil || len(body) != 0 || strings.Contains(err.Error(), "secret diagnostic") {
		t.Fatal("rejected request disclosed provider body")
	}
	local := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/transcript", nil)
	request.Header.Set("Origin", "https://example.com")
	Handler(recorder).ServeHTTP(local, request)
	if local.Code != 403 || calls != 2 {
		t.Fatal("cross-origin browser bypassed daemon restriction")
	}
}
