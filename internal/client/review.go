package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

func (r *Recorder) ExcludeSegments(ctx context.Context, meeting string, ids []string) (json.RawMessage, error) {
	r.mu.Lock()
	token, server := r.options.DeviceToken, r.options.Server
	r.mu.Unlock()
	if token == "" {
		return nil, errors.New("需要有效绑定凭证")
	}
	body, _ := json.Marshal(map[string]any{"segmentIds": ids, "reason": "用户确认的眼镜播报和物理按键命令"})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server+"/api/v1/meetings/"+meeting+"/transcript/exclusions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	response, err := r.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, errors.New("原文排除操作未完成")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 1048576))
	if err != nil || !json.Valid(data) {
		return nil, errors.New("原文排除返回无效")
	}
	return data, nil
}
func (r *Recorder) GenerateSummary(ctx context.Context, meeting string) error {
	r.mu.Lock()
	token, server := r.options.DeviceToken, r.options.Server
	r.mu.Unlock()
	if token == "" {
		return errors.New("需要有效绑定凭证")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server+"/api/v1/meetings/"+meeting+"/summary", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	response, err := r.http.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return errors.New("纪要申请未完成")
	}
	return nil
}
func (r *Recorder) AiStatus(ctx context.Context) (json.RawMessage, error) {
	r.mu.Lock()
	token, server := r.options.DeviceToken, r.options.Server
	r.mu.Unlock()
	if token == "" {
		return nil, errors.New("需要有效绑定凭证")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server+"/api/v1/config", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	response, err := r.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, errors.New("AI配置状态查询失败")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 16384))
	if err != nil || !json.Valid(body) {
		return nil, errors.New("AI配置状态无效")
	}
	return body, nil
}

func (r *Recorder) ReviewDetail(ctx context.Context, meeting string) (json.RawMessage, error) {
	r.mu.Lock()
	token, server := r.options.DeviceToken, r.options.Server
	r.mu.Unlock()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server+"/api/v1/meetings/"+meeting, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	response, err := r.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, errors.New("会议详情读取失败")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 1048576))
	if err != nil || !json.Valid(body) {
		return nil, errors.New("会议详情无效")
	}
	return body, nil
}
