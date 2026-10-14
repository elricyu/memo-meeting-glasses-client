package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"memomeeting/glasses-client/internal/client"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

var buildVersion = "development"

func main() {
	if err := run(); err != nil {
		slog.Error("command_failed", "reason", err.Error())
		if len(os.Args) > 1 && (os.Args[1] == "start" || os.Args[1] == "open" || os.Args[1] == "scan-binding") {
			guidance := ""
			if strings.Contains(err.Error(), "请先扫码绑定会议账号") {
				guidance = "bind-required"
			} else if strings.Contains(err.Error(), "请先在网页确认绑定") {
				guidance = "confirm-binding"
			} else if strings.Contains(err.Error(), "无法连接会议服务器") {
				guidance = "network-failed"
			} else if strings.Contains(err.Error(), "等待网页确认绑定超时") {
				guidance = "confirm-timeout"
			}
			if guidance != "" {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				notifyErr := exec.CommandContext(ctx, "/userdata/apps/memo-meeting/memo-notify", guidance).Run()
				cancel()
				if notifyErr != nil {
					slog.Warn("binding_guidance_unavailable")
				}
			}
		}
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) == 2 && os.Args[1] == "version" {
		fmt.Println(buildVersion)
		return nil
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))
	if len(os.Args) < 2 {
		fmt.Println("memo-glasses serve|open|start|stop|status|transcript|scan-binding|pair|unlock|binding|check|record|probe [options]")
		return nil
	}
	command := os.Args[1]
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	server := flags.String("server", "https://memo-meeting.easymr.cn", "Memo Meeting origin")
	address := flags.String("listen", "127.0.0.1:18765", "local control address")
	mirror := flags.String("mirror", client.DefaultMirror, "ai-core mirror socket")
	data := flags.String("data", "/userdata/apps/memo-meeting/audio", "local WAV directory")
	ca := flags.String("ca", "", "additional PEM CA certificate for HTTPS")
	mediaMonitor := flags.String("media-monitor", "/userdata/apps/memo-meeting/memo-media-state", "read-only interaction monitor")
	title := flags.String("title", "眼镜会议记录", "meeting title")
	terms := flags.String("context", "", "reference terminology")
	waitReady := flags.Bool("wait", true, "wait until recording is ready before returning success")
	seconds := flags.Int("seconds", 0, "recording duration; 0 waits for SIGINT/SIGTERM")
	clientID := flags.String("client-id", "", "device client ID (not a secret)")
	mac := flags.String("mac", "", "device MAC address")
	displayName := flags.String("name", "Memo Meeting 眼镜", "device display name")
	reviewMeeting := flags.String("meeting", "", "owned meeting ID for review")
	reviewSegments := flags.String("segments", "", "explicit segment IDs to exclude")
	if err := flags.Parse(os.Args[2:]); err != nil {
		return err
	}
	if command == "probe" {
		duration := *seconds
		if duration <= 0 {
			duration = 3
		}
		stream, err := client.Subscribe(*mirror)
		if err != nil {
			return err
		}
		defer stream.Close()
		until := time.Now().Add(time.Duration(duration) * time.Second)
		var packets, samples uint64
		started := time.Now()
		var firstCapture, lastCapture, firstSeq, lastSeq uint64
		var rate uint32
		var peak int16
		for time.Now().Before(until) {
			packet, err := client.ReadPacket(stream)
			if err != nil {
				return err
			}
			if packets == 0 {
				firstCapture = packet.CaptureUS
				firstSeq = packet.Sequence
			}
			lastCapture = packet.CaptureUS
			lastSeq = packet.Sequence
			packets++
			samples += uint64(len(packet.PCM))
			rate = packet.Rate
			for _, v := range packet.PCM {
				a := int32(v)
				if a < 0 {
					a = -a
				}
				if a > int32(peak) {
					peak = int16(min(a, 32767))
				}
			}
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"packets": packets, "samples": samples, "sampleRate": rate, "peak": peak, "durationMs": samples * 1000 / uint64(rate), "wallMs": time.Since(started).Milliseconds(), "captureSpanMs": (lastCapture - firstCapture) / 1000, "firstSeq": firstSeq, "lastSeq": lastSeq, "firstCaptureUs": firstCapture})
	}
	if command == "open" {
		req, err := http.NewRequest(http.MethodPost, "http://"+*address+"/prepare-open", nil)
		if err != nil {
			return err
		}
		response, err := (&http.Client{Timeout: 45 * time.Second}).Do(req)
		if err != nil {
			return err
		}
		body, err := io.ReadAll(io.LimitReader(response.Body, 16384))
		response.Body.Close()
		if err != nil {
			return err
		}
		if response.StatusCode != 200 {
			// Repeating the voice command while already recording is a success,
			// not a new meeting or a misleading failure toast.
			if strings.Contains(string(body), "会议记录正在运行") {
				status, readyErr := client.WaitReady(context.Background(), "http://"+*address, 50*time.Second)
				if readyErr != nil {
					return readyErr
				}
				return json.NewEncoder(os.Stdout).Encode(status)
			}
			return fmt.Errorf("打开准备失败：%s", body)
		}
		var preparation client.OpenPreparation
		if json.Unmarshal(body, &preparation) != nil {
			return fmt.Errorf("打开准备返回无效")
		}
		switch preparation.Action {
		case "SCAN", "CONFIRM":
			if preparation.Guidance == "rebind-required" || preparation.Guidance == "confirm-binding" {
				client.Notify(preparation.Guidance)
			}
			if err := client.WaitBound(context.Background(), "http://"+*address, 240*time.Second); err != nil {
				return err
			}
			client.Notify("binding-success")
		case "RECORD":
		default:
			return fmt.Errorf("服务器返回未知打开动作")
		}
		command = "start"
	}
	if command == "scan-binding" || command == "start" || command == "stop" || command == "status" || command == "transcript" || command == "pair" || command == "unlock" || command == "binding" {
		method, path := http.MethodGet, "/status"
		var body []byte
		if command == "scan-binding" {
			method, path = http.MethodPost, "/scan-binding"
		}
		if command == "transcript" {
			path = "/transcript"
		}
		if command == "binding" {
			path = "/binding"
		}
		if command == "pair" || command == "unlock" {
			if command == "pair" && (*clientID == "" || *mac == "") {
				return fmt.Errorf("pair requires --client-id and --mac")
			}
			// Read through stdin rather than command arguments, history or persistent env files.
			password, err := io.ReadAll(io.LimitReader(os.Stdin, 1025))
			if err != nil {
				return err
			}
			method, path = http.MethodPost, "/"+command
			body, _ = json.Marshal(map[string]string{"clientId": *clientID, "mac": *mac, "displayName": *displayName, "password": strings.TrimRight(string(password), "\r\n")})
		}
		if command == "start" {
			method, path = http.MethodPost, "/start"
			body, _ = json.Marshal(map[string]string{"title": *title, "context": *terms})
		}
		if command == "stop" {
			method, path = http.MethodPost, "/stop"
		}
		req, err := http.NewRequest(method, "http://"+*address+path, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		response, err := (&http.Client{Timeout: 45 * time.Second}).Do(req)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		output, _ := io.ReadAll(io.LimitReader(response.Body, 256*1024))
		if response.StatusCode >= 400 {
			return fmt.Errorf("control rejected (%d): %s", response.StatusCode, output)
		}
		if command == "scan-binding" {
			if err := client.WaitBound(context.Background(), "http://"+*address, 240*time.Second); err != nil {
				return err
			}
			client.Notify("binding-success")
			return nil
		}
		if command == "start" && *waitReady {
			status, err := client.WaitReady(context.Background(), "http://"+*address, 50*time.Second)
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(status)
		}
		fmt.Print(string(output))
		return nil
	}
	recorder, err := client.NewRecorder(client.Options{Server: *server, Mirror: *mirror, DataDir: *data, CAFile: *ca, DeviceToken: os.Getenv("MEMO_DEVICE_TOKEN"), MediaMonitor: *mediaMonitor})
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	switch command {
	case "review-detail":
		if len(*reviewMeeting) != 36 {
			return fmt.Errorf("meeting required")
		}
		if _, err := recorder.RefreshBinding(); err != nil {
			return err
		}
		body, err := recorder.ReviewDetail(ctx, *reviewMeeting)
		if err != nil {
			return err
		}
		fmt.Print(string(body))
		return nil
	case "ai-status":
		if _, err := recorder.RefreshBinding(); err != nil {
			return err
		}
		body, err := recorder.AiStatus(ctx)
		if err != nil {
			return err
		}
		fmt.Print(string(body))
		return nil
	case "exclude-device-segments":
		if len(*reviewMeeting) != 36 || *reviewSegments == "" {
			return fmt.Errorf("meeting and explicit segment IDs required")
		}
		if _, err := recorder.RefreshBinding(); err != nil {
			return err
		}
		body, err := recorder.ExcludeSegments(ctx, *reviewMeeting, strings.Split(*reviewSegments, ","))
		if err != nil {
			return err
		}
		fmt.Print(string(body))
		return nil
	case "generate-summary":
		if len(*reviewMeeting) != 36 {
			return fmt.Errorf("meeting required")
		}
		if _, err := recorder.RefreshBinding(); err != nil {
			return err
		}
		return recorder.GenerateSummary(ctx, *reviewMeeting)
	case "check":
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, *server+"/api/v1/health", nil)
		if err != nil {
			return err
		}
		transport, err := client.TLSConfig(*ca)
		if err != nil {
			return err
		}
		response, err := (&http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{TLSClientConfig: transport}}).Do(req)
		if err != nil {
			return fmt.Errorf("HTTPS check failed: %w", err)
		}
		defer response.Body.Close()
		if response.StatusCode != 200 {
			return fmt.Errorf("health check rejected: HTTP %d", response.StatusCode)
		}
		var health struct{ Status string }
		if err = json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&health); err != nil || health.Status != "ok" {
			return fmt.Errorf("address is not a Memo Meeting backend")
		}
		fmt.Println("Memo Meeting HTTPS health check passed")
		return nil
	case "record":
		if err = recorder.Start(*title, *terms); err != nil {
			return err
		}
		var timer <-chan time.Time
		if *seconds > 0 {
			timer = time.After(time.Duration(*seconds) * time.Second)
		}
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return recorder.Stop()
			case <-timer:
				return recorder.Stop()
			case <-ticker.C:
				status := recorder.Status()
				if status.State == "interrupted" {
					return fmt.Errorf("recording failed: %s", status.Error)
				}
				if status.State == "completed" {
					return nil
				}
			}
		}
	case "serve":
		srv, listener, err := client.Serve(recorder, *address)
		if err != nil {
			return err
		}
		failure := make(chan error, 1)
		go func() { failure <- srv.Serve(listener) }()
		slog.Info("daemon_ready", "control", *address, "server", *server)
		select {
		case <-ctx.Done():
			if status := recorder.Status(); status.State == "recording" || status.State == "connecting" {
				_ = recorder.Stop()
			}
			shutdown, done := context.WithTimeout(context.Background(), 5*time.Second)
			defer done()
			return srv.Shutdown(shutdown)
		case err = <-failure:
			if err == http.ErrServerClosed {
				return nil
			}
			return err
		}
	default:
		return fmt.Errorf("unknown command %q", command)
	}
}
