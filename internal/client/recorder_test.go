package client

import (
	"encoding/binary"
	"encoding/json"
	"github.com/gorilla/websocket"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestRecorderCompletesAndRequestsSummary(t *testing.T) {
	for _, mask := range []bool{false, true} {
		t.Run(map[bool]string{false: "normal audio", true: "device playback masked"}[mask], func(t *testing.T) { testRecorderFlow(t, mask) })
	}
}
func testRecorderFlow(t *testing.T, mask bool) {
	mirrorPath := filepath.Join(t.TempDir(), "mirror.sock")
	listener, err := net.ListenUnix("unixpacket", &net.UnixAddr{Name: mirrorPath, Net: "unixpacket"})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			c, err := listener.AcceptUnix()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				var seq uint64
				for {
					select {
					case <-done:
						return
					default:
					}
					p := packet()
					seq++
					n := seq
					binary.LittleEndian.PutUint64(p[36:], n)
					binary.LittleEndian.PutUint64(p[28:], n*32000)
					if _, err = c.Write(p); err != nil {
						return
					}
					time.Sleep(32 * time.Millisecond)
				}
			}()
		}
	}()
	var uploaded atomic.Int64
	var nonzero atomic.Int64
	var summary atomic.Bool
	var capability string
	mux := http.NewServeMux()
	const id = "12345678-1234-1234-1234-123456789abc"
	mux.HandleFunc("POST /api/v1/meetings", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-device" {
			http.Error(w, "unauthorized", 401)
			return
		}
		var input map[string]string
		json.NewDecoder(r.Body).Decode(&input)
		capability = input["recorderToken"]
		if input["source"] != "GLASSES" || len(capability) != 64 || len(input["clientRequestId"]) != 36 {
			t.Error("invalid create request")
		}
		json.NewEncoder(w).Encode(map[string]any{"meeting": map[string]string{"id": id}, "recorderToken": capability, "protocolVersion": 1})
	})
	mux.HandleFunc("POST /api/v1/meetings/"+id+"/summary", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Recorder-Token") != capability {
			http.Error(w, "forbidden", 403)
			return
		}
		summary.Store(true)
		w.Write([]byte(`{"accepted":true}`))
	})
	mux.HandleFunc("/ws/v1/meetings/"+id+"/audio", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-device" {
			http.Error(w, "unauthorized", 401)
			return
		}
		upgrader := websocket.Upgrader{}
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		var first map[string]any
		if err = c.ReadJSON(&first); err != nil {
			return
		}
		if first["token"] != capability {
			t.Error("missing recorder token")
			return
		}
		if first["protocolVersion"] != float64(1) {
			t.Error("invalid handshake")
		}
		c.WriteJSON(map[string]any{"type": "ready", "protocolVersion": 1, "offsetBytes": 0})
		var seq uint32
		var offset uint64
		for {
			kind, payload, err := c.ReadMessage()
			if err != nil {
				return
			}
			if kind == websocket.BinaryMessage {
				if len(payload) < 18 || string(payload[:4]) != "MM01" || binary.LittleEndian.Uint32(payload[4:]) != seq || binary.LittleEndian.Uint64(payload[8:]) != offset {
					t.Error("invalid audio frame")
					return
				}
				seq++
				offset += uint64(len(payload) - 16)
				uploaded.Add(int64(len(payload) - 16))
				for _, b := range payload[16:] {
					if b != 0 {
						nonzero.Add(1)
					}
				}
			} else {
				var message map[string]string
				json.Unmarshal(payload, &message)
				if message["type"] == "stop" {
					c.WriteJSON(map[string]any{"type": "finished", "committedBytes": offset})
					return
				}
			}
		}
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	helper := ""
	if mask {
		helper = filepath.Join(t.TempDir(), "monitor.py")
		if err := os.WriteFile(helper, []byte("#!/usr/bin/env python3\nimport time,json\nwhile True:\n print(json.dumps(dict(atUs=time.monotonic_ns()//1000,pressed=False,playing=True,healthy=True)),flush=True)\n time.sleep(.1)\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	recorder, err := NewRecorder(Options{MediaMonitor: helper, Server: server.URL, Mirror: mirrorPath, DataDir: t.TempDir(), DeviceToken: "test-device"})
	if err != nil {
		t.Fatal(err)
	}
	if err = recorder.Start("测试会议", ""); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100 && recorder.Status().Bytes < 2048; i++ {
		time.Sleep(20 * time.Millisecond)
	}
	if recorder.Status().State != "recording" {
		t.Fatalf("not recording: %+v", recorder.Status())
	}
	if err = recorder.Stop(); err != nil {
		t.Fatal(err)
	}
	status := recorder.Status()
	if status.State != "completed" || status.Bytes == 0 || uploaded.Load() != int64(status.Bytes) || !summary.Load() {
		t.Fatalf("invalid result: %+v uploaded=%d summary=%v", status, uploaded.Load(), summary.Load())
	}
	wav, err := os.ReadFile(status.AudioPath)
	if err != nil {
		t.Fatal(err)
	}
	if mask {
		if nonzero.Load() != 0 || status.ExcludedBytes != status.Bytes {
			t.Fatalf("device playback leaked: %+v nonzero=%d", status, nonzero.Load())
		}
		for _, b := range wav[44:] {
			if b != 0 {
				t.Fatal("playback leaked into archive")
			}
		}
	} else if nonzero.Load() == 0 {
		t.Fatal("ordinary audio was masked")
	}
	if len(wav) != int(status.Bytes)+44 {
		t.Fatal("local archive incomplete")
	}
}
