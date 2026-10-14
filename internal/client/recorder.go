package client

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Options struct{ Server, Mirror, DataDir, CAFile, DeviceToken, CaptureHelper, NotifyHelper, MediaMonitor string }
type Status struct {
	State          string `json:"state"`
	MeetingID      string `json:"meetingId"`
	ViewURL        string `json:"viewUrl"`
	AudioPath      string `json:"audioPath"`
	Bytes          uint64 `json:"bytes"`
	CommittedBytes uint64 `json:"committedBytes"`
	DroppedFrames  uint64 `json:"droppedFrames"`
	ExcludedBytes  uint64 `json:"excludedBytes"`
	Error          string `json:"error"`
}
type Recorder struct {
	mu             sync.Mutex
	options        Options
	status         Status
	cancel         context.CancelFunc
	done           chan struct{}
	http           *http.Client
	dialer         websocket.Dialer
	credential     *credential
	unlockPassword string
	binding        Binding
	scanMode       bool
	scanning       bool
}

func NewRecorder(options Options) (*Recorder, error) {
	u, err := url.Parse(options.Server)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("server must be an http(s) origin without credentials, path or query")
	}
	transport, err := TLSConfig(options.CAFile)
	if err != nil {
		return nil, err
	}
	recorder := &Recorder{options: options, status: Status{State: "idle"}, http: &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{TLSClientConfig: transport}}, dialer: websocket.Dialer{HandshakeTimeout: 15 * time.Second, TLSClientConfig: transport}}
	if err := recorder.loadAutomaticCredential(); err != nil {
		return nil, err
	}
	return recorder, nil
}
func (r *Recorder) Status() Status { r.mu.Lock(); defer r.mu.Unlock(); return r.status }

// Transcript reads only this daemon's current meeting with its bound device credential.
func (r *Recorder) Transcript(ctx context.Context) (json.RawMessage, error) {
	r.mu.Lock()
	id, token, origin := r.status.MeetingID, r.options.DeviceToken, r.options.Server
	r.mu.Unlock()
	if id == "" || token == "" {
		return nil, errors.New("没有可查看的会议，或设备凭证尚未解锁")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, origin+"/api/v1/meetings/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	response, err := r.http.Do(req)
	if err != nil {
		return nil, errors.New("无法读取会议转录，请检查网络")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("会议转录读取被拒绝（HTTP %d）", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 256*1024+1))
	if err != nil || len(body) > 256*1024 || !json.Valid(body) {
		return nil, errors.New("会议转录返回内容无效或过大，请在网页查看")
	}
	return json.RawMessage(body), nil
}

func (r *Recorder) Start(title, terms string) error {
	if title == "" || len([]rune(title)) > 200 || len([]rune(terms)) > 400 {
		return errors.New("title required (max 200 characters), context max 400")
	}
	if _, err := r.RefreshBinding(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.options.DeviceToken == "" {
		switch r.binding.State {
		case "SCANNING":
			return errors.New("眼镜正在扫码，请继续看向网页二维码")
		case "PENDING":
			return errors.New("请先在网页确认绑定，再打开会议记录")
		default:
			return errors.New("请先扫码绑定会议账号：在网页生成二维码，看向二维码并说绑定会议账号，网页确认后再打开会议记录")
		}
	}
	if r.cancel != nil {
		return errors.New("a recording is already running")
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.done = make(chan struct{})
	r.status = Status{State: "connecting"}
	go func() {
		err := r.record(ctx, title, terms)
		r.mu.Lock()
		if err != nil {
			r.status.State = "interrupted"
			r.status.Error = err.Error()
			slog.Error("recording_interrupted", "meeting_id", r.status.MeetingID, "reason", err.Error())
		} else {
			r.status.State = "completed"
			slog.Info("recording_completed", "meeting_id", r.status.MeetingID, "bytes", r.status.Bytes)
		}
		r.cancel = nil
		close(r.done)
		r.mu.Unlock()
	}()
	return nil
}
func (r *Recorder) Stop() error {
	r.mu.Lock()
	cancel, done := r.cancel, r.done
	if cancel != nil {
		r.status.State = "finalizing"
	}
	r.mu.Unlock()
	if cancel == nil {
		return errors.New("no active recording")
	}
	cancel()
	select {
	case <-done:
		status := r.Status()
		if status.State != "completed" {
			return fmt.Errorf("recording did not finish cleanly: %s", status.Error)
		}
		return nil
	case <-time.After(40 * time.Second):
		return errors.New("stop confirmation timed out")
	}
}
func (r *Recorder) update(fn func(*Status)) { r.mu.Lock(); fn(&r.status); r.mu.Unlock() }
func (r *Recorder) record(ctx context.Context, title, terms string) error {
	mirror, err := Subscribe(r.options.Mirror)
	if err != nil {
		return fmt.Errorf("cannot subscribe to ai-core mirror: %w", err)
	}
	defer mirror.Close()
	// Prove the mirror produces valid frames before creating a business record.
	first, err := ReadPacket(mirror)
	if err != nil {
		return err
	}
	var guard *mediaGuard
	if r.options.MediaMonitor != "" {
		guard, err = startMediaGuard(r.options.MediaMonitor)
		if err != nil {
			return err
		}
		defer guard.close()
	}
	tokenBytes := make([]byte, 32)
	if _, err = rand.Read(tokenBytes); err != nil {
		return err
	}
	recorderToken := hex.EncodeToString(tokenBytes)
	requestID, err := randomUUID()
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]string{"title": title, "context": terms, "source": "GLASSES", "clientRequestId": requestID, "recorderToken": recorderToken})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.options.Server+"/api/v1/meetings", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.options.DeviceToken)
	response, err := r.http.Do(req)
	if err != nil {
		return errors.New("meeting service unreachable")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return fmt.Errorf("meeting create rejected: HTTP %d", response.StatusCode)
	}
	var created struct {
		Meeting struct {
			ID string `json:"id"`
		} `json:"meeting"`
		Token           string `json:"recorderToken"`
		ProtocolVersion int    `json:"protocolVersion"`
	}
	if err = json.NewDecoder(io.LimitReader(response.Body, 16384)).Decode(&created); err != nil || created.Meeting.ID == "" || created.Token != recorderToken || created.ProtocolVersion != 1 {
		return errors.New("invalid create response")
	}
	id := created.Meeting.ID
	if len(id) != 36 || filepath.Base(id) != id {
		return errors.New("invalid meeting ID")
	}
	view := r.options.Server + "/?meeting=" + id
	r.update(func(s *Status) { s.MeetingID = id; s.ViewURL = view })
	slog.Info("meeting_created", "meeting_id", id, "view_url", view)
	if err = os.MkdirAll(r.options.DataDir, 0700); err != nil {
		return err
	}
	path := filepath.Join(r.options.DataDir, id+".wav")
	wav, err := NewWAV(path)
	if err != nil {
		return err
	}
	defer wav.Close()
	r.update(func(s *Status) { s.AudioPath = path })
	u, _ := url.Parse(r.options.Server)
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Path = "/ws/v1/meetings/" + id + "/audio"
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+r.options.DeviceToken)
	socket, _, err := r.dialer.DialContext(ctx, u.String(), headers)
	if err != nil {
		return errors.New("recorder websocket connection failed")
	}
	defer socket.Close()
	socket.SetReadLimit(16384)
	if err = socket.WriteJSON(map[string]any{"type": "start", "protocolVersion": 1, "token": created.Token, "audio": map[string]any{"format": "pcm_s16le", "sampleRate": 16000, "channels": 1, "bitsPerSample": 16}}); err != nil {
		return err
	}
	socket.SetReadDeadline(time.Now().Add(45 * time.Second))
	var ready struct {
		Type, Message   string
		ProtocolVersion int
		OffsetBytes     uint64
	}
	if err = socket.ReadJSON(&ready); err != nil {
		return errors.New("cloud recognition did not become ready")
	}
	if ready.Type != "ready" {
		return fmt.Errorf("recording rejected: %s", ready.Message)
	}
	if ready.ProtocolVersion != 1 || ready.OffsetBytes != 0 {
		return errors.New("unsupported protocol or nonzero offset for a new meeting")
	}
	socket.SetReadDeadline(time.Time{})
	r.update(func(s *Status) { s.State = "recording" })
	slog.Info("recording_ready", "meeting_id", id, "sample_rate", first.Rate)
	// Drain old startup frames so the first transmitted frame reflects current speech.
	mirror.SetReadDeadline(time.Now())
	// A new subscription drops stale frames accumulated while the cloud connected.
	mirror.Close()
	mirror, err = Subscribe(r.options.Mirror)
	if err != nil {
		return err
	}
	defer mirror.Close()
	received := make(chan error, 1)
	go func() {
		for {
			var event struct {
				Type, Message  string
				CommittedBytes uint64
			}
			if err := socket.ReadJSON(&event); err != nil {
				received <- errors.New("recording websocket disconnected")
				return
			}
			if event.Type == "ack" || event.Type == "finished" {
				r.update(func(s *Status) { s.CommittedBytes = event.CommittedBytes })
			}
			if event.Type == "finished" {
				received <- nil
				return
			}
			if event.Type == "error" {
				received <- fmt.Errorf("backend recording interrupted: %s", event.Message)
				return
			}
		}
	}()
	packets := make(chan Packet, 8)
	audioErrors := make(chan error, 1)
	audioCtx, audioCancel := context.WithCancel(context.Background())
	defer audioCancel()
	go func() {
		defer close(packets)
		for {
			packet, err := ReadPacketWithTimeout(mirror, 45*time.Second)
			if err != nil {
				if audioCtx.Err() != nil {
					return
				}
				select {
				case audioErrors <- err:
				case <-audioCtx.Done():
				}
				return
			}
			select {
			case packets <- packet:
			case <-audioCtx.Done():
				return
			default:
				select {
				case audioErrors <- errors.New("audio sender cannot keep up; mirror queue full"):
				default:
				}
				return
			}
		}
	}()
	var resampler Resampler
	var sequence uint32
	var offset uint64
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	var previous uint64
	var lastCapture uint64
	sendPacket := func(p Packet, sourceTime int64) error {
		if previous != 0 && p.Sequence <= previous {
			return errors.New("mirror sequence moved backwards")
		}
		if lastCapture != 0 && p.CaptureUS <= lastCapture {
			return errors.New("mirror capture timestamp moved backwards")
		}
		if previous != 0 && p.Sequence > previous+1 {
			lost := p.Sequence - previous - 1
			r.update(func(s *Status) { s.DroppedFrames += lost })
			slog.Warn("audio_frame_gap", "meeting_id", id, "previous_seq", previous, "seq", p.Sequence, "missing", lost)
			return errors.New("mirror frames lost; recording stopped to preserve timing accuracy")
		}
		previous = p.Sequence
		lastCapture = p.CaptureUS
		pcm, err := resampler.Convert(p)
		if err != nil {
			return err
		}
		if guard != nil {
			excluded, gateErr := guard.excludes(p, sourceTime)
			if gateErr != nil {
				return gateErr
			}
			if excluded {
				clear(pcm)
				r.update(func(s *Status) { s.ExcludedBytes += uint64(len(pcm)) })
			}
		}
		if err = wav.Write(pcm); err != nil {
			return errors.New("local audio archive write failed")
		}
		socket.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err = socket.WriteMessage(websocket.BinaryMessage, audioFrame(sequence, offset, pcm)); err != nil {
			return errors.New("audio upload failed; local WAV preserved")
		}
		sequence++
		offset += uint64(len(pcm))
		r.update(func(s *Status) { s.Bytes = offset })
		return nil
	}
	type heldPacket struct {
		packet     Packet
		ready      time.Time
		sourceTime int64
	}
	var held []heldPacket
	flushHeld := func(all bool) error {
		for len(held) > 0 && (all || !time.Now().Before(held[0].ready)) {
			if err := sendPacket(held[0].packet, held[0].sourceTime); err != nil {
				return err
			}
			held = held[1:]
		}
		return nil
	}
	queuePacket := func(p Packet) error {
		if guard == nil {
			return sendPacket(p, 0)
		}
		held = append(held, heldPacket{p, time.Now().Add(350 * time.Millisecond), guard.sourceTime(p)})
		if len(held) > 32 {
			return errors.New("交互音频过滤缓冲区已满，录音已停止")
		}
		return flushHeld(false)
	}
	filterTick := time.NewTicker(40 * time.Millisecond)
	defer filterTick.Stop()
	for {
		select {
		case <-filterTick.C:
			if err = flushHeld(false); err != nil {
				return err
			}
		case <-heartbeat.C:
			socket.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err = socket.WriteJSON(map[string]string{"type": "ping"}); err != nil {
				return errors.New("recording heartbeat failed")
			}
		case <-ctx.Done():
			audioCancel()
			mirror.Close()
			// Flush packets already captured before stop, including the short tail.
			for p := range packets {
				if err = queuePacket(p); err != nil {
					return err
				}
			}
			if guard != nil {
				time.Sleep(350 * time.Millisecond)
			}
			if err = flushHeld(true); err != nil {
				return err
			}
			select {
			case err = <-audioErrors:
				return err
			default:
			}
			socket.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err = socket.WriteJSON(map[string]string{"type": "stop"}); err != nil {
				return errors.New("cannot send recording stop")
			}
			select {
			case err = <-received:
				if err == nil && r.Status().CommittedBytes != offset {
					return errors.New("backend did not confirm all uploaded audio")
				}
				if err == nil {
					r.requestSummary(id, created.Token)
				}
				return err
			case <-time.After(30 * time.Second):
				return errors.New("final transcript confirmation timed out")
			}
		case err = <-received:
			if err == nil {
				return errors.New("backend ended recording unexpectedly")
			}
			return err
		case err = <-audioErrors:
			return err
		case p, ok := <-packets:
			if !ok {
				return errors.New("audio mirror stopped")
			}
			if err = queuePacket(p); err != nil {
				return err
			}
		}
	}
}

// The device holds the recorder capability only in memory. Generate notes before discarding it.
func (r *Recorder) requestSummary(id, token string) {
	summaryCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(summaryCtx, http.MethodPost, r.options.Server+"/api/v1/meetings/"+id+"/summary", nil)
	if err != nil {
		return
	}
	request.Header.Set("X-Recorder-Token", token)
	request.Header.Set("Authorization", "Bearer "+r.options.DeviceToken)
	response, err := r.http.Do(request)
	if err != nil {
		slog.Warn("summary_request_failed", "meeting_id", id)
		return
	}
	defer response.Body.Close()
	if response.StatusCode == 200 {
		slog.Info("summary_requested", "meeting_id", id)
	} else {
		slog.Info("summary_not_requested", "meeting_id", id, "http_status", response.StatusCode)
	}
}

func audioFrame(sequence uint32, offset uint64, pcm []byte) []byte {
	frame := make([]byte, 16+len(pcm))
	copy(frame, "MM01")
	binary.LittleEndian.PutUint32(frame[4:], sequence)
	binary.LittleEndian.PutUint64(frame[8:], offset)
	copy(frame[16:], pcm)
	return frame
}
func randomUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}
