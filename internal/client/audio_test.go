package client

import (
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func packet() []byte {
	p := make([]byte, 44+512)
	binary.LittleEndian.PutUint32(p, 0x41415346)
	binary.LittleEndian.PutUint16(p[4:], 1)
	binary.LittleEndian.PutUint16(p[6:], 44)
	binary.LittleEndian.PutUint32(p[8:], 1)
	binary.LittleEndian.PutUint32(p[12:], 16000)
	binary.LittleEndian.PutUint16(p[16:], 1)
	binary.LittleEndian.PutUint16(p[18:], 8)
	binary.LittleEndian.PutUint32(p[20:], 512)
	binary.LittleEndian.PutUint32(p[24:], 512)
	binary.LittleEndian.PutUint64(p[28:], 123456)
	binary.LittleEndian.PutUint64(p[36:], 7)
	for i := 44; i < len(p); i++ {
		p[i] = 0xd5
	}
	return p
}
func TestMirrorPacketValidation(t *testing.T) {
	p, err := ParsePacket(packet())
	if err != nil {
		t.Fatal(err)
	}
	if len(p.PCM) != 512 || p.Sequence != 7 || p.CaptureUS != 123456 || p.PCM[0] != 8 {
		t.Fatal(p)
	}
	for _, data := range [][]byte{packet()[:43], packet()[:500], append(packet(), 0)} {
		if _, err := ParsePacket(data); err == nil {
			t.Fatal("invalid packet accepted")
		}
	}
	invalid := packet()
	invalid[16] = 2
	if _, err := ParsePacket(invalid); err == nil {
		t.Fatal("stereo accepted")
	}
}
func TestALawKnownValues(t *testing.T) {
	if DecodeALaw(0xd5) != 8 || DecodeALaw(0x55) != -8 || DecodeALaw(0xaa) != 32256 || DecodeALaw(0x2a) != -32256 {
		t.Fatal("G711A decode mismatch")
	}
}
func TestResamplingContinuousBlocks(t *testing.T) {
	for _, rate := range []uint32{8000, 16000, 32000, 48000} {
		var r Resampler
		total := 0
		for remaining := int(rate); remaining > 0; {
			n := min(remaining, 512)
			samples := make([]int16, n)
			for i := range samples {
				samples[i] = 1234
			}
			out, err := r.Convert(Packet{Rate: rate, PCM: samples})
			if err != nil {
				t.Fatal(err)
			}
			total += len(out)
			for i := 0; i < len(out); i += 2 {
				if int16(binary.LittleEndian.Uint16(out[i:])) != 1234 {
					t.Fatal("sample mismatch")
				}
			}
			remaining -= n
		}
		if total != 32000 {
			t.Fatalf("rate %d -> %d bytes", rate, total)
		}
	}
}
func TestWAVArchive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audio.wav")
	w, err := NewWAV(path)
	if err != nil {
		t.Fatal(err)
	}
	w.Write(make([]byte, 3200))
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if len(data) != 3244 || binary.LittleEndian.Uint32(data[40:]) != 3200 {
		t.Fatal("invalid WAV")
	}
	if _, err = NewWAV(path); err == nil {
		t.Fatal("overwrote existing audio")
	}
}
func TestControlRejectsBrowserOrigin(t *testing.T) {
	r, _ := NewRecorder(Options{Server: "http://localhost:8090"})
	req := httptest.NewRequest("POST", "http://localhost/start", strings.NewReader(`{"title":"test"}`))
	req.Header.Set("Origin", "https://evil.example")
	response := httptest.NewRecorder()
	Handler(r).ServeHTTP(response, req)
	if response.Code != http.StatusForbidden {
		t.Fatal("browser control accepted")
	}
}
func TestConfigurationValidation(t *testing.T) {
	for _, origin := range []string{"file:///tmp", "http://user:secret@host", "http://host/path"} {
		if _, err := NewRecorder(Options{Server: origin}); err == nil {
			t.Fatal("invalid origin accepted")
		}
	}
	r, _ := NewRecorder(Options{Server: "http://localhost:8090"})
	if _, _, err := Serve(r, "0.0.0.0:18765"); err == nil {
		t.Fatal("public daemon bind allowed")
	}
}
