package api

import (
	"chanzhushui/internal/gateway"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNumericSelectsStreamByProtocol(t *testing.T) {
	for _, tc := range []struct {
		mode, model, path string
		stream            bool
	}{
		{"auto", "gpt-6-astra", "/v1/responses", true},
		{"auto", "claude-opus-5", "/v1/messages", false},
		{"chat", "claude-opus-5", "/v1/chat/completions", true},
		{"anthropic", "gpt-6-astra", "/v1/messages", false},
		{"auto", "deepseek-flash", "/v1/chat/completions", true},
		{"auto", "deepseek-v4.1-flash", "/v1/chat/completions", true},
	} {
		t.Run(tc.mode+"-"+tc.model, func(t *testing.T) {
			s, _, _, _ := testSetup(t)
			var calls atomic.Int32
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var body struct{ Stream bool }
				json.NewDecoder(r.Body).Decode(&body)
				if body.Stream != tc.stream {
					t.Errorf("stream=%v want %v", body.Stream, tc.stream)
				}
				if r.URL.Path != tc.path {
					t.Errorf("path=%s want %s", r.URL.Path, tc.path)
				}
				answer := strings.Repeat("247,83,319,", 105)
				var response any
				switch tc.path {
				case "/v1/responses":
					response = map[string]any{"status": "completed", "output": []any{map[string]any{"content": []any{map[string]string{"type": "output_text", "text": answer}}}}}
				case "/v1/messages":
					response = map[string]any{"content": []any{map[string]string{"type": "text", "text": answer}}}
				default:
					response = map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": answer}, "finish_reason": "stop"}}}
				}
				json.NewEncoder(w).Encode(response)
			}))
			defer up.Close()
			result, err := s.numeric(context.Background(), gateway.Client{BaseURL: up.URL, Mode: tc.mode}, Request{Model: tc.model, Timeout: 10})
			if err != nil {
				t.Fatal(err)
			}
			out := result.(map[string]any)
			if out["used_outputs"] != 3 || out["requests"] != 3 || calls.Load() != 3 {
				t.Fatalf("outputs=%v requests=%v calls=%d", out["used_outputs"], out["requests"], calls.Load())
			}
		})
	}
}

func TestNumericStopsAfterExhaustedRequestErrors(t *testing.T) {
	for _, status := range []int{400, 401, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			s, _, _, _ := testSetup(t)
			var calls atomic.Int32
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(status)
				fmt.Fprint(w, `{"error":{"message":"rejected"}}`)
			}))
			defer up.Close()
			result, err := s.numeric(context.Background(), gateway.Client{BaseURL: up.URL, RetryDelay: time.Millisecond}, Request{Model: "claude-opus-5", Timeout: 10})
			out := result.(map[string]any)
			wantCalls := int32(3)
			if status == 500 {
				wantCalls *= gateway.RetryCount + 1
			}
			if err == nil || out["requests"] != 3 || out["used_outputs"] != 0 || calls.Load() != wantCalls {
				t.Fatalf("err=%v requests=%v outputs=%v calls=%d want %d", err, out["requests"], out["used_outputs"], calls.Load(), wantCalls)
			}
			if len(out["evidence"].([]any)) != 3 {
				t.Fatal("lost failed evidence")
			}
		})
	}
}

func TestFailedNumericJobRemainsRetryable(t *testing.T) {
	s, _, _, _ := testSetup(t)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) }))
	defer up.Close()
	ctx, cancel := context.WithCancel(context.Background())
	task := &Task{Source: "api", Status: "running", Verdict: Verdict{}, Probes: []Probe{{ID: "fingerprint", Status: "queued"}}, cancel: cancel}
	s.run(ctx, task, Request{Model: "claude-opus-5", BaseURL: up.URL, Timeout: 10}, map[string]bool{"fingerprint": true})
	if task.Status != "partial" || task.Probes[0].Status != "error" || task.Probes[0].Error == "" {
		t.Fatalf("status=%s probe=%+v", task.Status, task.Probes[0])
	}
}

func TestNumericResamplesInvalidAnswersWithinChallengeBudget(t *testing.T) {
	for _, recover := range []bool{false, true} {
		t.Run(fmt.Sprint(recover), func(t *testing.T) {
			s, _, _, _ := testSetup(t)
			var calls atomic.Int32
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				answer := "not a numeric sample"
				if calls.Add(1) > 3 && recover {
					answer = strings.Repeat("247,83,319,", 105)
				}
				json.NewEncoder(w).Encode(map[string]any{"content": []any{map[string]string{"type": "text", "text": answer}}})
			}))
			defer up.Close()
			result, err := s.numeric(context.Background(), gateway.Client{BaseURL: up.URL}, Request{Model: "claude-opus-5", Timeout: 10})
			out := result.(map[string]any)
			if recover {
				if err != nil || out["used_outputs"] != 3 || out["requests"] != 6 {
					t.Fatalf("err=%v outputs=%v requests=%v", err, out["used_outputs"], out["requests"])
				}
			} else if err == nil || out["used_outputs"] != 0 || calls.Load() != 10 {
				t.Fatalf("err=%v outputs=%v calls=%d", err, out["used_outputs"], calls.Load())
			}
		})
	}
}
