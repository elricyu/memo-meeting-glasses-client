package client

import (
	"context"
	"log/slog"
	"os/exec"
	"time"
)

func (r *Recorder) notify(kind string) {
	helper := r.options.NotifyHelper
	if helper == "" {
		helper = "/userdata/apps/memo-meeting/memo-notify"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, helper, kind).Run(); err != nil {
		slog.Warn("voice_guidance_unavailable", "stage", kind)
	} else {
		slog.Info("voice_guidance_queued", "stage", kind)
	}
}
func Notify(kind string) { (&Recorder{}).notify(kind) }
