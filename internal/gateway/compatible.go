package gateway

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func (c *Client) callCompatibleOnce(ctx context.Context, q CallRequest) (Metrics, error) {
	protocol := c.ProtocolFor(q.Model)
	if q.protocolOverride != "" {
		protocol = q.protocolOverride
	}
	base := apiBase(c.BaseURL)
	messages := []map[string]string{}
	var system strings.Builder
	for _, b := range q.System {
		system.WriteString(b.Text)
		system.WriteByte('\n')
	}
	if system.Len() > 0 {
		messages = append(messages, map[string]string{"role": "system", "content": system.String()})
	}
	for _, message := range q.History {
		messages = append(messages, map[string]string{"role": message.Role, "content": message.Content})
	}
	messages = append(messages, map[string]string{"role": "user", "content": userPlain(q)})
	body := map[string]any{"model": q.Model, "stream": q.Stream}
	path := "/chat/completions"
	if protocol == "responses" {
		path = "/responses"
		body["input"] = messages
		body["max_output_tokens"] = q.MaxTokens
		if q.Effort != "" {
			body["reasoning"] = map[string]string{"effort": q.Effort}
		}
	} else {
		body["messages"] = messages
		body["max_tokens"] = q.MaxTokens
		if q.Stream {
			body["stream_options"] = map[string]bool{"include_usage": true}
		}
	}
	if defs, choice := toolWire(q, protocol); len(defs) > 0 {
		body["tools"] = defs
		if choice != nil {
			body["tool_choice"] = choice
		}
	}
	if q.ToolReply != nil {
		r := q.ToolReply
		if protocol == "responses" {
			items := []any{}
			for _, msg := range messages {
				items = append(items, msg)
			}
			call := map[string]any{"type": "function_call", "call_id": r.Call.ID, "name": r.Call.Name, "arguments": string(r.Call.Arguments)}
			if r.Call.ResponseID != "" {
				call["id"] = r.Call.ResponseID
			}
			if r.Call.ResponseState != "" {
				call["status"] = r.Call.ResponseState
			}
			items = append(items, call, map[string]any{"type": "function_call_output", "call_id": r.Call.ID, "output": r.Output})
			body["input"] = items
		} else {
			items := []any{}
			for _, msg := range messages {
				items = append(items, msg)
			}
			items = append(items, map[string]any{"role": "assistant", "content": "", "tool_calls": []any{map[string]any{"type": "function", "id": r.Call.ID, "function": map[string]string{"name": r.Call.Name, "arguments": string(r.Call.Arguments)}}}}, map[string]string{"role": "tool", "tool_call_id": r.Call.ID, "content": r.Output})
			body["messages"] = items
		}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return Metrics{}, err
	}
	if q.TimeoutSeconds > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(q.TimeoutSeconds)*time.Second)
		defer cancel()
	}
	req, err := http.NewRequestWithContext(ctx, "POST", base+path, bytes.NewReader(raw))
	if err != nil {
		return Metrics{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", c.ua())
	start := time.Now()
	resp, err := c.http().Do(req)
	if err != nil {
		return Metrics{}, err
	}
	defer resp.Body.Close()
	m := Metrics{Status: resp.StatusCode, Protocol: protocol, Via: "direct", FirstByteMs: time.Since(start).Milliseconds()}
	if resp.StatusCode != 200 {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		if resp.StatusCode == http.StatusForbidden && strings.Contains(strings.ToLower(resp.Header.Get("content-type")), "text/html") && strings.Contains(strings.ToLower(resp.Header.Get("server")), "cloudflare") {
			m.Err = "HTTP 403 Cloudflare WAF blocked the source address"
			return m, nil
		}
		var apiErr struct {
			Error struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(errBody, &apiErr) == nil && apiErr.Error.Message != "" {
			m.Err = fmt.Sprintf("HTTP %d %s %s", resp.StatusCode, apiErr.Error.Type, apiErr.Error.Message)
		} else {
			m.Err = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		return m, nil
	}
	if q.Stream && strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		err = parseCompatibleStream(resp.Body, protocol, &m)
	} else {
		raw, err = io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		if err == nil {
			err = parseCompatibleJSON(raw, protocol, &m)
		}
	}
	if err != nil {
		m.Err = err.Error()
	}
	m.Ms = time.Since(start).Milliseconds()
	return m, nil
}
func compatibleUsage(u map[string]json.RawMessage, m *Metrics) {
	if len(u) == 0 {
		return
	}
	present := func(v json.RawMessage) bool {
		return len(v) > 0 && string(v) != "null"
	}
	readInt := func(k string) int {
		var n int
		json.Unmarshal(u[k], &n)
		return n
	}
	nonNegRaw := func(v json.RawMessage) (int, bool) {
		if len(v) == 0 {
			return 0, false
		}
		if string(v) == "null" {
			return 0, true
		}
		var n int
		if json.Unmarshal(v, &n) != nil {
			return 0, false
		}
		if n < 0 {
			n = 0
		}
		return n, true
	}
	_, hasInput := u["input_tokens"]
	_, hasPrompt := u["prompt_tokens"]
	m.UsagePresent = (hasInput && present(u["input_tokens"])) || (hasPrompt && present(u["prompt_tokens"]))
	m.InputTokens = readInt("input_tokens")
	if present(u["prompt_tokens"]) {
		m.InputTokens = readInt("prompt_tokens")
	}
	m.OutputTokens = readInt("output_tokens")
	m.OutputReported = present(u["output_tokens"])
	if present(u["completion_tokens"]) {
		m.OutputTokens = readInt("completion_tokens")
		m.OutputReported = true
	}
	var inputDetails, promptDetails map[string]json.RawMessage
	json.Unmarshal(u["input_tokens_details"], &inputDetails)
	json.Unmarshal(u["prompt_tokens_details"], &promptDetails)
	// Nested keys win even when they are zero or null. Positive top-level
	// aliases are only a fallback. This is the sub2api responses/chat order.
	switch {
	case len(inputDetails["cached_tokens"]) > 0:
		m.CacheRead, m.CacheReadReported = nonNegRaw(inputDetails["cached_tokens"])
	case len(promptDetails["cached_tokens"]) > 0:
		m.CacheRead, m.CacheReadReported = nonNegRaw(promptDetails["cached_tokens"])
	case present(u["cache_read_input_tokens"]):
		m.CacheRead, m.CacheReadReported = nonNegRaw(u["cache_read_input_tokens"])
	case present(u["cache_read_tokens"]):
		m.CacheRead, m.CacheReadReported = nonNegRaw(u["cache_read_tokens"])
	case present(u["cached_tokens"]):
		m.CacheRead, m.CacheReadReported = nonNegRaw(u["cached_tokens"])
	case present(u["prompt_cache_hit_tokens"]):
		m.CacheRead, m.CacheReadReported = nonNegRaw(u["prompt_cache_hit_tokens"])
	}
	nestedWrite := false
	switch {
	case len(inputDetails["cache_write_tokens"]) > 0:
		m.CacheCreation5m, m.CacheWriteReported = nonNegRaw(inputDetails["cache_write_tokens"])
		nestedWrite = m.CacheWriteReported
	case len(promptDetails["cache_write_tokens"]) > 0:
		m.CacheCreation5m, m.CacheWriteReported = nonNegRaw(promptDetails["cache_write_tokens"])
		nestedWrite = m.CacheWriteReported
	case len(inputDetails["cache_creation_tokens"]) > 0:
		m.CacheCreation5m, m.CacheWriteReported = nonNegRaw(inputDetails["cache_creation_tokens"])
		nestedWrite = m.CacheWriteReported
	case len(promptDetails["cache_creation_tokens"]) > 0:
		m.CacheCreation5m, m.CacheWriteReported = nonNegRaw(promptDetails["cache_creation_tokens"])
		nestedWrite = m.CacheWriteReported
	}
	if nestedWrite {
		m.CacheCreation1h = 0
	} else {
		switch {
		case present(u["cache_write_tokens"]) && readInt("cache_write_tokens") > 0:
			m.CacheCreation5m = readInt("cache_write_tokens")
			m.CacheWriteReported = true
		case present(u["cache_creation_input_tokens"]) && readInt("cache_creation_input_tokens") > 0:
			m.CacheCreation5m = readInt("cache_creation_input_tokens")
			m.CacheWriteReported = true
		case present(u["cache_write_input_tokens"]) && readInt("cache_write_input_tokens") > 0:
			m.CacheCreation5m = readInt("cache_write_input_tokens")
			m.CacheWriteReported = true
		case present(u["cache_creation_tokens"]) && readInt("cache_creation_tokens") > 0:
			m.CacheCreation5m = readInt("cache_creation_tokens")
			m.CacheWriteReported = true
		}
		var creation struct {
			Five *int `json:"ephemeral_5m_input_tokens"`
			Hour *int `json:"ephemeral_1h_input_tokens"`
		}
		if json.Unmarshal(u["cache_creation"], &creation) == nil && (creation.Five != nil || creation.Hour != nil) {
			aggregate := m.CacheCreation5m
			m.CacheCreation5m, m.CacheCreation1h = 0, 0
			if creation.Five != nil {
				m.CacheCreation5m = *creation.Five
			}
			if creation.Hour != nil {
				m.CacheCreation1h = *creation.Hour
			}
			if aggregate > m.CacheCreation5m+m.CacheCreation1h {
				m.CacheCreation5m = aggregate - m.CacheCreation1h
			}
			m.CacheWriteReported = true
		}
		if !m.CacheWriteReported && present(u["prompt_cache_miss_tokens"]) {
			m.CacheCreation5m, m.CacheWriteReported = nonNegRaw(u["prompt_cache_miss_tokens"])
		}
	}
	// Compatible APIs count cached tokens inside input/prompt_tokens.
	m.InputTokens -= m.CacheRead + m.CacheCreation5m + m.CacheCreation1h
	if m.InputTokens < 0 {
		m.InputTokens = 0
	}
	var outputDetails struct {
		Thinking  int `json:"thinking_tokens"`
		Reasoning int `json:"reasoning_tokens"`
	}
	outputRaw := u["output_tokens_details"]
	if outputRaw == nil {
		outputRaw = u["completion_tokens_details"]
	}
	if json.Unmarshal(outputRaw, &outputDetails) == nil {
		m.ThinkingTokens = outputDetails.Thinking
		if outputDetails.Reasoning > m.ThinkingTokens {
			m.ThinkingTokens = outputDetails.Reasoning
		}
	}
}
func parseCompatibleJSON(raw []byte, protocol string, m *Metrics) error {
	extractToolEvidence(raw, protocol, m)
	var d struct {
		Error  json.RawMessage
		Status string
		Usage  map[string]json.RawMessage
		Output []struct {
			Type    string
			Content []struct{ Type, Text string }
		}
		Choices []struct {
			Message      struct{ Content string }
			FinishReason string `json:"finish_reason"`
		}
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		return fmt.Errorf("invalid upstream JSON")
	}
	if len(d.Error) > 0 && string(d.Error) != "null" {
		return fmt.Errorf("upstream error")
	}
	compatibleUsage(d.Usage, m)
	if protocol == "responses" {
		if d.Output == nil {
			return fmt.Errorf("missing response output")
		}
		for _, o := range d.Output {
			for _, b := range o.Content {
				if b.Type == "output_text" {
					m.Answer += b.Text
				}
			}
		}
		m.StopReason = "end_turn"
		if d.Status == "incomplete" {
			m.StopReason = "max_tokens"
		}
	} else {
		if len(d.Choices) == 0 {
			return fmt.Errorf("missing choices")
		}
		m.Answer = d.Choices[0].Message.Content
		m.StopReason = d.Choices[0].FinishReason
		if m.StopReason == "length" {
			m.StopReason = "max_tokens"
		}
	}
	return nil
}
func parseCompatibleStream(body io.Reader, protocol string, m *Metrics) error {
	scanner := bufio.NewScanner(io.LimitReader(body, 8<<20))
	scanner.Buffer(make([]byte, 65536), 4<<20)
	var lines []string
	flush := func() error {
		if len(lines) == 0 {
			return nil
		}
		raw := strings.Join(lines, "\n")
		lines = nil
		if raw == "[DONE]" {
			m.StreamComplete = true
			return nil
		}
		var d struct {
			Type     string
			Delta    string
			Response json.RawMessage
			Usage    map[string]json.RawMessage
			Error    json.RawMessage
			Choices  []struct {
				Delta        struct{ Content string }
				FinishReason *string `json:"finish_reason"`
			}
		}
		if err := json.Unmarshal([]byte(raw), &d); err != nil {
			return fmt.Errorf("invalid SSE JSON")
		}
		if d.Type == "error" || d.Type == "response.failed" || len(d.Error) > 0 {
			return fmt.Errorf("upstream stream error")
		}
		if protocol == "responses" {
			switch d.Type {
			case "response.output_text.delta":
				m.Answer += d.Delta
			case "response.completed", "response.incomplete":
				m.Answer = ""
				if err := parseCompatibleJSON(d.Response, protocol, m); err != nil {
					return err
				}
				m.StreamComplete = true
			}
		} else {
			for _, c := range d.Choices {
				m.Answer += c.Delta.Content
				if c.FinishReason != nil {
					m.StopReason = *c.FinishReason
					if m.StopReason == "length" {
						m.StopReason = "max_tokens"
					}
				}
			}
			if len(d.Usage) > 0 {
				compatibleUsage(d.Usage, m)
			}
		}
		return nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := flush(); err != nil {
				return err
			}
			if m.StreamComplete {
				return nil
			}
		} else if strings.HasPrefix(line, "data:") {
			lines = append(lines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if err := flush(); err != nil {
		return err
	}
	if !m.StreamComplete {
		return fmt.Errorf("upstream stream interrupted")
	}
	return nil
}
