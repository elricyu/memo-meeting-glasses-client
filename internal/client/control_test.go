package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWaitReadyRequiresActualRecording(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		state := "connecting"
		if attempts > 2 {
			state = "recording"
		}
		json.NewEncoder(w).Encode(Status{State: state})
	}))
	defer server.Close()
	status, err := WaitReady(context.Background(), server.URL, time.Second)
	if err != nil || status.State != "recording" || attempts < 3 {
		t.Fatalf("premature readiness: %+v %v", status, err)
	}
}
func TestWaitReadyReportsFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(Status{State: "interrupted", Error: "no API key"})
	}))
	defer server.Close()
	if _, err := WaitReady(context.Background(), server.URL, time.Second); err == nil {
		t.Fatal("failed recording reported as success")
	}
}
func TestWaitReadyTimesOut(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(Status{State: "connecting"}) }))
	defer server.Close()
	if _, err := WaitReady(context.Background(), server.URL, 200*time.Millisecond); err == nil {
		t.Fatal("pending recording reported as success")
	}
}
func TestStopReportsInterruptedState(t *testing.T) {
	r := &Recorder{status: Status{State: "interrupted", Error: "cloud disconnected"}, done: make(chan struct{}), cancel: func() {}}
	close(r.done)
	if err := r.Stop(); err == nil {
		t.Fatal("interrupted recording reported as successful stop")
	}
}
