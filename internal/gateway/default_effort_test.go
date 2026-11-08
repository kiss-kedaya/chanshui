package gateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestDefaultEffortIsSentAndReported(t *testing.T) {
	var got map[string]any
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		got = map[string]any{}
		_ = json.Unmarshal(raw, &got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, successRespJSON)
	}))
	defer up.Close()

	c := Client{BaseURL: up.URL, Key: "sk-test", DefaultEffort: "high"}
	m, err := c.CallMessages(context.Background(), CallRequest{Model: "claude-fixture", Prompt: "hi"})
	if err != nil || m.Err != "" {
		t.Fatalf("call: %+v %v", m, err)
	}
	oc, _ := got["output_config"].(map[string]any)
	if oc["effort"] != "high" {
		t.Fatalf("output_config = %#v", got["output_config"])
	}
	if m.Effort != "high" || m.EffortFallback {
		t.Fatalf("metrics effort=%q fallback=%v", m.Effort, m.EffortFallback)
	}
}

func TestDefaultEffortFallsBackWhenUpstreamRejectsIt(t *testing.T) {
	var mu sync.Mutex
	var bodies []map[string]any
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body := map[string]any{}
		_ = json.Unmarshal(raw, &body)
		mu.Lock()
		bodies = append(bodies, body)
		n := len(bodies)
		mu.Unlock()
		if n == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"type":"error","error":{"type":"invalid_request_error","message":"output_config: extra fields are not permitted"}}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, successRespJSON)
	}))
	defer up.Close()

	c := Client{BaseURL: up.URL, Key: "sk-test", DefaultEffort: "high", RetryDelay: time.Millisecond}
	m, err := c.CallMessages(context.Background(), CallRequest{Model: "claude-fixture", Prompt: "hi"})
	if err != nil || m.Err != "" {
		t.Fatalf("call: %+v %v", m, err)
	}
	if len(bodies) != 2 {
		t.Fatalf("requests = %d, want 2", len(bodies))
	}
	if _, ok := bodies[0]["output_config"]; !ok {
		t.Fatal("first attempt must carry effort")
	}
	if _, ok := bodies[1]["output_config"]; ok {
		t.Fatal("fallback attempt must not carry effort")
	}
	if !m.EffortFallback || m.Effort != "" || m.Attempts != 2 {
		t.Fatalf("metrics = %+v", m)
	}
}

func TestExplicitEffortIsNotDropped(t *testing.T) {
	calls := 0
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"type":"error","error":{"type":"invalid_request_error","message":"bad request"}}`)
	}))
	defer up.Close()

	c := Client{BaseURL: up.URL, Key: "sk-test", DefaultEffort: "high", RetryDelay: time.Millisecond}
	m, err := c.CallMessages(context.Background(), CallRequest{Model: "claude-fixture", Prompt: "hi", Effort: "low"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if calls != 1 || m.Attempts != 1 {
		t.Fatalf("calls=%d attempts=%d, explicit effort must not be silently dropped", calls, m.Attempts)
	}
	if m.Effort != "low" || m.EffortFallback {
		t.Fatalf("metrics = %+v", m)
	}
}

func TestChatProtocolCarriesReasoningEffort(t *testing.T) {
	var got map[string]any
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		got = map[string]any{}
		_ = json.Unmarshal(raw, &got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":1}}`)
	}))
	defer up.Close()

	c := Client{BaseURL: up.URL, Key: "sk-test", DefaultEffort: "high"}
	m, err := c.CallMessages(context.Background(), CallRequest{Model: "deepseek-flash", Prompt: "hi"})
	if err != nil || m.Err != "" || m.Answer != "ok" {
		t.Fatalf("call: %+v %v", m, err)
	}
	if got["reasoning_effort"] != "high" {
		t.Fatalf("reasoning_effort = %#v", got["reasoning_effort"])
	}
	if m.Effort != "high" {
		t.Fatalf("metrics effort = %q", m.Effort)
	}
}
