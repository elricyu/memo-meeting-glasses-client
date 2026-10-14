package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

func WaitReady(ctx context.Context, origin string, timeout time.Duration) (Status, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	httpClient := &http.Client{Timeout: 3 * time.Second}
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, origin+"/status", nil)
		if err != nil {
			return Status{}, err
		}
		response, err := httpClient.Do(req)
		if err != nil {
			return Status{}, err
		}
		var status Status
		err = json.NewDecoder(response.Body).Decode(&status)
		response.Body.Close()
		if err != nil {
			return Status{}, err
		}
		switch status.State {
		case "recording":
			return status, nil
		case "interrupted":
			return status, fmt.Errorf("recording start failed: %s", status.Error)
		case "completed", "idle":
			return status, errors.New("recording ended before it became ready")
		}
		select {
		case <-ctx.Done():
			return status, errors.New("recording start timed out")
		case <-ticker.C:
		}
	}
}

func WaitBound(ctx context.Context, origin string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, origin+"/binding", nil)
		if err != nil {
			return err
		}
		response, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
		if err != nil {
			return err
		}
		var value Binding
		err = json.NewDecoder(response.Body).Decode(&value)
		response.Body.Close()
		if err != nil {
			return err
		}
		if value.State == "BOUND" {
			return nil
		}
		if value.State == "SCAN_FAILED" || value.State == "EXPIRED" || value.State == "REVOKED" {
			if value.Error != "" {
				return errors.New(value.Error)
			}
			return errors.New("扫码未完成，请重新生成二维码、调整距离并重试")
		}
		select {
		case <-ctx.Done():
			return errors.New("等待网页确认绑定超时，请在网页查看申请状态")
		case <-ticker.C:
		}
	}
}
