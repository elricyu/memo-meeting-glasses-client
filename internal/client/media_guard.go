package client

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

type mediaState struct {
	AtUS    uint64 `json:"atUs"`
	Pressed bool   `json:"pressed"`
	Playing bool   `json:"playing"`
	Healthy bool   `json:"healthy"`
}
type mediaGuard struct {
	mu       sync.Mutex
	samples  []mediaState
	latest   mediaState
	received time.Time
	ended    bool
	cancel   context.CancelFunc
}

func startMediaGuard(helper string) (*mediaGuard, error) {
	ctx, cancel := context.WithCancel(context.Background())
	command := exec.CommandContext(ctx, helper)
	command.Cancel = func() error { return command.Process.Signal(syscall.SIGTERM) }
	command.WaitDelay = 2 * time.Second
	pipe, err := command.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	if err = command.Start(); err != nil {
		cancel()
		return nil, errors.New("无法启动眼镜交互音频过滤，录音尚未开始")
	}
	guard := &mediaGuard{cancel: cancel}
	go func() {
		scanner := bufio.NewScanner(pipe)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "{") {
				continue
			}
			var value mediaState
			if json.Unmarshal([]byte(line), &value) != nil || value.AtUS == 0 {
				continue
			}
			guard.mu.Lock()
			if len(guard.samples) > 0 && value.AtUS <= guard.latest.AtUS {
				guard.ended = true
				guard.mu.Unlock()
				break
			}
			if len(guard.samples) == 0 || (value.Pressed || value.Playing) != (guard.latest.Pressed || guard.latest.Playing) || value.Healthy != guard.latest.Healthy {
				guard.samples = append(guard.samples, value)
			}
			guard.latest = value
			guard.received = time.Now()
			// Recording sends with a short holdback, so retain recent state history.
			if len(guard.samples) > 128 {
				guard.samples = append([]mediaState(nil), guard.samples[len(guard.samples)-64:]...)
			}
			guard.mu.Unlock()
		}
		command.Wait()
		guard.mu.Lock()
		guard.ended = true
		guard.mu.Unlock()
	}()
	for i := 0; i < 100; i++ {
		guard.mu.Lock()
		ready := len(guard.samples) > 0 && !guard.ended && guard.samples[len(guard.samples)-1].Healthy
		guard.mu.Unlock()
		if ready {
			return guard, nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	return nil, errors.New("无法确认眼镜按键和播放状态，录音尚未开始")
}
func (g *mediaGuard) close() { g.cancel() }

// The firmware audio timestamp can have an independent/drifting origin.
// Map each freshly received packet to the observer's monotonic clock instead.
func (g *mediaGuard) sourceTime(p Packet) int64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	return int64(g.latest.AtUS) + time.Since(g.received).Microseconds() - int64(len(p.PCM))*1000000/int64(p.Rate)
}
func (g *mediaGuard) excludes(p Packet, sourceTime int64) (bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.ended || len(g.samples) == 0 || time.Since(g.received) > 1500*time.Millisecond || !g.latest.Healthy {
		return false, errors.New("眼镜交互状态监控中断，已停止录音以避免记录指令或播报")
	}
	start := sourceTime
	end := start + int64(len(p.PCM))*1000000/int64(p.Rate)
	return mediaOverlap(g.samples, start, end), nil
}
func mediaOverlap(states []mediaState, start, end int64) bool {
	for i, state := range states {
		if !state.Pressed && !state.Playing {
			continue
		}
		from := int64(state.AtUS) - 250000
		until := int64(^uint64(0) >> 1)
		for j := i + 1; j < len(states); j++ {
			if !states[j].Pressed && !states[j].Playing {
				until = int64(states[j].AtUS) + 400000
				break
			}
		}
		if start < until && end > from {
			return true
		}
	}
	return false
}
