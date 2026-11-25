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
	Status     int
	Err        string
	Answer     string
	StopReason string
	Transforms []string
	// HasInputTransformations reports whether the upstream sent the field at
	// all; official Anthropic does not, so absence must not read as "dropped".
	HasInputTransformations bool
	Fields                  []string
	InputTokens             int
	OutputTokens            int
	Signature               string
	CacheCreation5m         int
	CacheCreation1h         int
	CacheRead               int
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
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	out.Status = resp.StatusCode
	if out.Status == 200 && wantsStream(body) {
		parseSSEUsage(payload, &out)
		return out, nil
	}
	var parsed struct {
		StopReason string `json:"stop_reason"`
		Usage      struct {
			InputTokens    int `json:"input_tokens"`
			OutputTokens   int `json:"output_tokens"`
			CacheReadInput int `json:"cache_read_input_tokens"`
			CacheCreation  struct {
				Ephemeral5m int `json:"ephemeral_5m_input_tokens"`
				Ephemeral1h int `json:"ephemeral_1h_input_tokens"`
			} `json:"cache_creation"`
		} `json:"usage"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Content []struct {
			Type      string `json:"type"`
			Text      string `json:"text"`
			Signature string `json:"signature"`
		} `json:"content"`
		InputTransformations json.RawMessage `json:"input_transformations"`
	}
	if err := json.Unmarshal(payload, &parsed); err == nil {
		out.StopReason = parsed.StopReason
		out.InputTokens = parsed.Usage.InputTokens
		out.OutputTokens = parsed.Usage.OutputTokens
		out.CacheRead = parsed.Usage.CacheReadInput
		out.CacheCreation5m = parsed.Usage.CacheCreation.Ephemeral5m
		out.CacheCreation1h = parsed.Usage.CacheCreation.Ephemeral1h
		out.Err = strings.TrimSpace(parsed.Error.Message)
		for _, block := range parsed.Content {
			switch block.Type {
			case "text":
				out.Answer += block.Text
			case "thinking", "redacted_thinking":
				if block.Signature != "" {
					out.Signature = block.Signature
				}
			}
		}
		if len(parsed.InputTransformations) > 0 && string(parsed.InputTransformations) != "null" {
			out.HasInputTransformations = true
			var transforms []struct {
				Type string `json:"type"`
			}
			if json.Unmarshal(parsed.InputTransformations, &transforms) == nil {
				for _, t := range transforms {
					out.Transforms = append(out.Transforms, t.Type)
				}
			}
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(payload, &fields) == nil {
			for key := range fields {
				out.Fields = append(out.Fields, key)
			}
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

type rawUsage struct {
	InputTokens    int `json:"input_tokens"`
	OutputTokens   int `json:"output_tokens"`
	CacheReadInput int `json:"cache_read_input_tokens"`
	CacheCreation  struct {
		Ephemeral5m int `json:"ephemeral_5m_input_tokens"`
		Ephemeral1h int `json:"ephemeral_1h_input_tokens"`
	} `json:"cache_creation"`
}

func wantsStream(body map[string]any) bool {
	stream, _ := body["stream"].(bool)
	return stream
}

// parseSSEUsage walks Stream events. Usage and cache counters arrive in
// message_start / message_delta; text and signatures arrive as deltas.
func parseSSEUsage(payload []byte, out *RawResult) {
	apply := func(usage rawUsage) {
		if usage.InputTokens > 0 {
			out.InputTokens = usage.InputTokens
		}
		if usage.OutputTokens > 0 {
			out.OutputTokens = usage.OutputTokens
		}
		if usage.CacheReadInput > 0 {
			out.CacheRead = usage.CacheReadInput
		}
		if usage.CacheCreation.Ephemeral5m > 0 {
			out.CacheCreation5m = usage.CacheCreation.Ephemeral5m
		}
		if usage.CacheCreation.Ephemeral1h > 0 {
			out.CacheCreation1h = usage.CacheCreation.Ephemeral1h
		}
	}
	for _, line := range strings.Split(string(payload), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var event struct {
			Type    string `json:"type"`
			Message struct {
				Usage rawUsage `json:"usage"`
			} `json:"message"`
			Delta struct {
				Type       string `json:"type"`
				Text       string `json:"text"`
				Thinking   string `json:"thinking"`
				Signature  string `json:"signature"`
				StopReason string `json:"stop_reason"`
			} `json:"delta"`
			Usage rawUsage `json:"usage"`
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal([]byte(data), &event) != nil {
			continue
		}
		switch event.Type {
		case "message_start":
			apply(event.Message.Usage)
		case "content_block_delta":
			switch event.Delta.Type {
			case "text_delta":
				out.Answer += event.Delta.Text
			case "thinking_delta":
				out.Answer += event.Delta.Thinking
			case "signature_delta":
				if event.Delta.Signature != "" {
					out.Signature = event.Delta.Signature
				}
			}
		case "message_delta":
			apply(event.Usage)
			if event.Delta.StopReason != "" {
				out.StopReason = event.Delta.StopReason
			}
		case "error":
			out.Err = strings.TrimSpace(event.Error.Message)
		}
	}
}
