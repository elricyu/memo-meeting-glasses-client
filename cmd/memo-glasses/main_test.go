package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRepeatedOpenUsesExistingRecordingWithoutAnotherStart(t *testing.T) {
	var starts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/prepare-open":
			http.Error(w, "会议记录正在运行", 409)
		case "/status":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"state":"recording","meetingId":"existing-meeting"}`))
		case "/start":
			starts.Add(1)
			http.Error(w, "duplicate", 409)
		default:
			t.Errorf("unexpected control path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	original := os.Args
	t.Cleanup(func() { os.Args = original })
	os.Args = []string{"memo-glasses", "open", "--listen", strings.TrimPrefix(server.URL, "http://")}
	if err := run(); err != nil {
		t.Fatal(err)
	}
	if starts.Load() != 0 {
		t.Fatal("repeated open started another recording")
	}
}
