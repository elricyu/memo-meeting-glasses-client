package client

import (
	"testing"
	"time"
)

func TestInteractionMaskCoversPressAndPlaybackBoundariesOnly(t *testing.T) {
	states := []mediaState{{AtUS: 1000000, Healthy: true}, {AtUS: 2000000, Pressed: true, Healthy: true}, {AtUS: 3000000, Healthy: true}, {AtUS: 5000000, Playing: true, Healthy: true}, {AtUS: 6000000, Healthy: true}}
	for _, v := range []struct {
		start, end int64
		want       bool
	}{{1000000, 1032000, false}, {1740000, 1772000, true}, {2200000, 2232000, true}, {3300000, 3332000, true}, {3500000, 3532000, false}, {4900000, 4932000, true}, {6400001, 6432001, false}} {
		if got := mediaOverlap(states, v.start, v.end); got != v.want {
			t.Fatalf("interval %d..%d mask=%v", v.start, v.end, got)
		}
	}
}
func TestStaleMonitorCannotUploadUnfilteredAudio(t *testing.T) {
	g := &mediaGuard{latest: mediaState{Healthy: true}, received: time.Now().Add(-3 * time.Second), samples: []mediaState{{AtUS: 1, Healthy: true}}}
	if _, err := g.excludes(Packet{CaptureUS: 1000000, Rate: 16000, PCM: make([]int16, 512)}, 1000000); err == nil {
		t.Fatal("stale monitor allowed audio")
	}
}
