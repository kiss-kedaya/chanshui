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
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"tool_calls": []any{map[string]any{"id": "call_1", "type": "function", "function": map[string]any{"name": "audit_echo", "arguments": map[string]any{"nonce": "n", "sum": 42}}}}}, "finish_reason": "tool_calls"}}})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer s.Close()
	c := Client{BaseURL: s.URL, Mode: "auto", RetryDelay: time.Millisecond}
	m, err := c.CallMessages(context.Background(), CallRequest{Model: "claude-opus-5", Tools: []ToolDefinition{{Name: "audit_echo", Parameters: map[string]any{"type": "object"}}}, ForceTool: "audit_echo"})
	if err != nil || m.Err != "" || m.Protocol != "chat" || len(m.ToolCalls) != 1 || messages != 1 || chat != 1 {
		t.Fatalf("m=%+v err=%v messages=%d chat=%d", m, err, messages, chat)
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
