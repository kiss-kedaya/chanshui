package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestUnifiedRetriesExactlyThree(t *testing.T) {
	for _, model := range []string{"claude-opus-5", "gpt-6-sol", "gemini-3.1-pro-preview", "deepseek-v4-pro", "grok-4.5", "glm-5.2"} {
		t.Run(model, func(t *testing.T) {
			var calls atomic.Int32
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(503) }))
			defer s.Close()
			c := Client{BaseURL: s.URL, RetryDelay: time.Millisecond}
			m, e := c.CallMessages(context.Background(), CallRequest{Model: model, Prompt: "x"})
			if e != nil || calls.Load() != 4 || m.Attempts != 4 || !strings.Contains(m.Err, "已尝试 4 次") {
				t.Fatal(calls.Load(), m, e)
			}
		})
	}
}
func TestCompatibleRoutingAndUsage(t *testing.T) {
	for _, model := range []string{"gpt-6-sol", "gemini-3.1-pro-preview", "deepseek-v4-pro", "grok-4.5", "glm-5.2"} {
		t.Run(model, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				json.NewDecoder(r.Body).Decode(&body)
				if body["thinking"] != nil {
					t.Error("non-Claude thinking sent")
				}
				if r.Header.Get("Authorization") != "Bearer fixture" {
					t.Error("auth missing")
				}
				w.Header().Set("Content-Type", "application/json")
				if strings.HasPrefix(model, "gpt-") {
					if r.URL.Path != "/v1/responses" {
						t.Error(r.URL.Path)
					}
					w.Write([]byte(`{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":110,"input_tokens_details":{"cached_tokens":100},"output_tokens":2}}`))
				} else {
					if r.URL.Path != "/v1/chat/completions" {
						t.Error(r.URL.Path)
					}
					w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":110,"prompt_tokens_details":{"cached_tokens":100},"completion_tokens":2}}`))
				}
			}))
			defer s.Close()
			c := Client{BaseURL: s.URL + "/v1", Key: "fixture"}
			m, e := c.CallMessages(context.Background(), CallRequest{Model: model, Prompt: "x", Thinking: &ThinkingConfig{Type: "adaptive"}})
			if e != nil || m.Err != "" || m.Answer != "ok" || m.InputTokens != 10 || m.CacheRead != 100 || !m.CacheReadReported || m.CacheWriteReported {
				t.Fatal(m, e)
			}
		})
	}
}
func TestCancellationStopsRetries(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); cancel(); w.WriteHeader(503) }))
	defer s.Close()
	c := Client{BaseURL: s.URL}
	_, e := c.CallMessages(ctx, CallRequest{Model: "claude-opus-5"})
	if e == nil || calls.Load() != 1 {
		t.Fatal(e, calls.Load())
	}
}
