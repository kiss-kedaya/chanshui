package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// RawResult is the subset of a Messages response the deep Claude checks need.
type RawResult struct {
	Status       int
	Err          string
	Answer       string
	StopReason   string
	Transforms   []string
	InputTokens  int
	OutputTokens int
}

// PostMessagesRaw POSTs an arbitrary Messages body. The deep checks need
// request shapes the typed client does not model: block_binding, context
// management edits, assistant tool_use turns and extra beta headers.
func (c *Client) PostMessagesRaw(ctx context.Context, body map[string]any, beta string, timeoutSeconds int) (RawResult, error) {
	var out RawResult
	raw, err := json.Marshal(body)
	if err != nil {
		return out, err
	}
	callCtx := ctx
	if timeoutSeconds > 0 {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
		defer cancel()
	}
	req, err := http.NewRequestWithContext(callCtx, http.MethodPost, apiBase(c.BaseURL)+"/messages", bytes.NewReader(raw))
	if err != nil {
		return out, err
	}
	if c.Key != "" {
		req.Header.Set("x-api-key", c.Key)
		req.Header.Set("Authorization", "Bearer "+c.Key)
	}
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")
	req.Header.Set("User-Agent", c.ua())
	if beta != "" {
		req.Header.Set("anthropic-beta", beta)
	}
	resp, err := c.http().Do(req)
	if err != nil {
		return out, fmt.Errorf("%s", Redact(err.Error()))
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	out.Status = resp.StatusCode
	var parsed struct {
		StopReason string `json:"stop_reason"`
		Usage      struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		InputTransformations []struct {
			Type string `json:"type"`
		} `json:"input_transformations"`
	}
	if err := json.Unmarshal(payload, &parsed); err == nil {
		out.StopReason = parsed.StopReason
		out.InputTokens = parsed.Usage.InputTokens
		out.OutputTokens = parsed.Usage.OutputTokens
		out.Err = strings.TrimSpace(parsed.Error.Message)
		for _, block := range parsed.Content {
			if block.Type == "text" {
				out.Answer += block.Text
			}
		}
		for _, t := range parsed.InputTransformations {
			out.Transforms = append(out.Transforms, t.Type)
		}
	}
	if out.Err == "" && out.Status != 200 {
		out.Err = strings.TrimSpace(string(payload))
		if len(out.Err) > 300 {
			out.Err = out.Err[:300]
		}
	}
	return out, nil
}
