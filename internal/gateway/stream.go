package gateway

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

func parseStream(body io.Reader, m *Metrics) error {
	sc := bufio.NewScanner(io.LimitReader(body, 8<<20))
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	var parts []string
	type toolBuffer struct {
		id       string
		name     string
		args     strings.Builder
		fallback json.RawMessage
	}
	tools := map[int]*toolBuffer{}
	flush := func() error {
		if len(parts) == 0 {
			return nil
		}
		payload := strings.Join(parts, "\n")
		parts = nil
		if m.StreamComplete {
			return nil
		}
		if payload == "[DONE]" {
			m.StreamComplete = true
			return nil
		}
		m.SSEFrames = append(m.SSEFrames, payload)
		var e struct {
			Index        int
			Type         string
			Message      json.RawMessage
			ContentBlock contentBlock `json:"content_block"`
			Delta        struct {
				Type, Text, Thinking, Signature string
				PartialJSON                     string `json:"partial_json"`
				StopReason                      string `json:"stop_reason"`
			}
			Usage map[string]json.RawMessage
			Error *apiErrorInner
		}
		if err := json.Unmarshal([]byte(payload), &e); err != nil {
			return fmt.Errorf("invalid SSE JSON")
		}
		switch e.Type {
		case "message_start":
			if err := parseMessagesBody(e.Message, m); err != nil {
				return err
			}
		case "content_block_start":
			m.Blocks = append(m.Blocks, e.ContentBlock.Type)
			m.Answer += e.ContentBlock.Text
			m.ThinkingText += e.ContentBlock.Thinking
			m.Signature += e.ContentBlock.Signature
			if e.ContentBlock.Type == "tool_use" {
				tools[e.Index] = &toolBuffer{id: e.ContentBlock.ID, name: e.ContentBlock.Name, fallback: e.ContentBlock.Input}
			}
		case "content_block_delta":
			m.Answer += e.Delta.Text
			m.ThinkingText += e.Delta.Thinking
			m.Signature += e.Delta.Signature
			if e.Delta.Type == "input_json_delta" {
				if tool := tools[e.Index]; tool != nil {
					tool.args.WriteString(e.Delta.PartialJSON)
				}
			}
		case "content_block_stop":
			if tool := tools[e.Index]; tool != nil {
				args := json.RawMessage(tool.args.String())
				if len(args) == 0 {
					args = tool.fallback
				}
				if tool.id != "" && tool.name != "" && json.Valid(args) {
					m.ToolCalls = append(m.ToolCalls, ToolCall{ID: tool.id, Name: tool.name, Arguments: args})
				}
				delete(tools, e.Index)
			}
		case "message_delta":
			m.StopReason = e.Delta.StopReason
		case "message_stop":
			m.StreamComplete = true
		case "error":
			return fmt.Errorf("upstream SSE error")
		}
		if e.Error != nil {
			return fmt.Errorf("upstream SSE error: %s", e.Error.Type)
		}
		for k, v := range e.Usage {
			if string(v) == "null" {
				continue
			}
			switch k {
			case "input_tokens":
				if err := json.Unmarshal(v, &m.InputTokens); err != nil {
					return err
				}
				m.UsagePresent = true
			case "output_tokens":
				if err := json.Unmarshal(v, &m.OutputTokens); err != nil {
					return err
				}
				m.OutputReported = true
			case "cache_read_input_tokens":
				m.CacheReadReported = true
				if err := json.Unmarshal(v, &m.CacheRead); err != nil {
					return err
				}
			case "cache_creation":
				var cache struct {
					Five int `json:"ephemeral_5m_input_tokens"`
					Hour int `json:"ephemeral_1h_input_tokens"`
				}
				if err := json.Unmarshal(v, &cache); err != nil {
					return err
				}
				m.CacheCreation5m = cache.Five
				m.CacheCreation1h = cache.Hour
				m.CacheWriteReported = true
			case "cache_creation_input_tokens":
				m.CacheWriteReported = true
				var n int
				if err := json.Unmarshal(v, &n); err != nil {
					return err
				}
				if _, ok := e.Usage["cache_creation"]; !ok {
					m.CacheCreation5m = n - m.CacheCreation1h
					if m.CacheCreation5m < 0 {
						m.CacheCreation5m = 0
					}
				}
			case "output_tokens_details":
				var d struct {
					Thinking int `json:"thinking_tokens"`
				}
				if err := json.Unmarshal(v, &d); err != nil {
					return err
				}
				m.ThinkingTokens = d.Thinking
			}
		}
		m.ThinkLen = utf8.RuneCountInString(m.ThinkingText)
		m.SigLen = len(m.Signature)
		return nil
	}
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			if err := flush(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			parts = append(parts, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if err := flush(); err != nil {
		return err
	}
	if !m.StreamComplete {
		return fmt.Errorf("SSE stream ended without message_stop or [DONE]")
	}
	return nil
}
