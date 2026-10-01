package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestAutoTool400FallsBackToChat(t *testing.T) {
	var messages, chat int
	var chatChoice bool
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch r.URL.Path {
		case "/v1/messages":
			messages++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"type":"invalid_request_error","message":"tools are not supported on messages"}}`))
		case "/v1/chat/completions":
			chat++
			_, chatChoice = body["tool_choice"]
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"tool_calls": []any{map[string]any{"id": "call_1", "type": "function", "function": map[string]any{"name": "audit_echo", "arguments": map[string]any{"nonce": "n", "sum": 42}}}}}, "finish_reason": "tool_calls"}}})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer s.Close()
	c := Client{BaseURL: s.URL, Mode: "auto", RetryDelay: time.Millisecond}
	m, err := c.CallMessages(context.Background(), CallRequest{Model: "claude-opus-5", Tools: []ToolDefinition{{Name: "audit_echo", Parameters: map[string]any{"type": "object"}}}, ForceTool: "audit_echo"})
	if err != nil || m.Err != "" || m.Protocol != "chat" || len(m.ToolCalls) != 1 || messages != 1 || chat != 1 || !chatChoice {
		t.Fatalf("m=%+v err=%v messages=%d chat=%d chatChoice=%v", m, err, messages, chat, chatChoice)
	}
}

func TestChatTool400DropsForcedChoiceInsteadOfRepeatingIt(t *testing.T) {
	var choices []any
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if choice, ok := body["tool_choice"]; ok {
			choices = append(choices, choice)
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"type":"invalid_request_error","message":"tool_choice unsupported"}}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": "no tool"}, "finish_reason": "stop"}}})
	}))
	defer s.Close()
	c := Client{BaseURL: s.URL, Mode: "chat", RetryDelay: time.Millisecond}
	m, err := c.CallMessages(context.Background(), CallRequest{Model: "fixture", Tools: []ToolDefinition{{Name: "audit_echo", Parameters: map[string]any{"type": "object"}}}, ForceTool: "audit_echo"})
	if err != nil || m.Err != "" || m.Attempts != 2 || len(choices) != 1 {
		t.Fatalf("m=%+v err=%v choices=%d", m, err, len(choices))
	}
}

func TestForcedToolChoiceParameterErrorsDoNotRepeatAcrossProtocols(t *testing.T) {
	for _, tc := range []struct {
		name, mode, path string
		status           int
	}{
		{name: "chat-422", mode: "chat", path: "/v1/chat/completions", status: http.StatusUnprocessableEntity},
		{name: "anthropic-400", mode: "anthropic", path: "/v1/messages", status: http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var forced int
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tc.path {
					t.Fatalf("path=%s want %s", r.URL.Path, tc.path)
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatalf("decode request: %v", err)
				}
				if _, ok := body["tool_choice"]; ok {
					forced++
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(`{"error":{"type":"invalid_request_error","message":"tool_choice unsupported"}}`))
					return
				}
				if tc.mode == "anthropic" {
					_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":1,"output_tokens":1}}`))
					return
				}
				_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
			}))
			defer s.Close()
			c := Client{BaseURL: s.URL, Mode: tc.mode, RetryDelay: time.Millisecond}
			m, err := c.CallMessages(context.Background(), CallRequest{Model: "fixture", Tools: []ToolDefinition{{Name: "audit_echo", Parameters: map[string]any{"type": "object"}}}, ForceTool: "audit_echo"})
			if err != nil || m.Err != "" || m.Attempts != 2 || forced != 1 || m.Answer != "ok" {
				t.Fatalf("m=%+v err=%v forced=%d", m, err, forced)
			}
		})
	}
}

func TestResponses404FallbackUsesSameRetryBudget(t *testing.T) {
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path == "/v1/responses" {
			w.WriteHeader(404)
			return
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Error(r.URL.Path)
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"fallback"},"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":1}}`))
	}))
	defer s.Close()
	c := Client{BaseURL: s.URL, RetryDelay: time.Millisecond}
	m, e := c.CallMessages(context.Background(), CallRequest{Model: "gpt-6-sol", Prompt: "x"})
	if e != nil || m.Err != "" || m.Answer != "fallback" || m.Attempts != 2 || calls.Load() != 2 {
		t.Fatal(m, e, calls.Load())
	}
}
func TestResponsesAndChatStreaming(t *testing.T) {
	for _, c := range []struct{ protocol, body, answer string }{{"responses", "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hi\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"hi\"}]}],\"usage\":{\"input_tokens\":10,\"output_tokens\":1}}}\n\n", "hi"}, {"chat", "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":1}}\n\ndata: [DONE]\n\n", "hi"}} {
		t.Run(c.protocol, func(t *testing.T) {
			var m Metrics
			if e := parseCompatibleStream(bytes.NewBufferString(c.body), c.protocol, &m); e != nil || m.Answer != c.answer || m.InputTokens != 10 || !m.StreamComplete {
				t.Fatal(m, e)
			}
			var bad Metrics
			if e := parseCompatibleStream(strings.NewReader("data: {}\n\n"), c.protocol, &bad); e == nil {
				t.Fatal("truncated stream accepted")
			}
		})
	}
}
