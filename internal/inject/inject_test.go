package inject

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"chanzhushui/internal/gateway"
)

// fakeMessagesServer answers every /v1/messages call with
// input_tokens = round(0.738*n + 44) where n is the content length. If
// cacheCreation != nil, n==800 answers like a cache-hit upstream: input 44,
// cache_creation 5m 590 — total still lands on the 0.738*n+44 line.
func fakeMessagesServer(t *testing.T, cacheCreation map[string]any) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			w.WriteHeader(500)
			return
		}
		var req struct {
			Model     string `json:"model"`
			MaxTokens int    `json:"max_tokens"`
			Stream    bool   `json:"stream"`
			Messages  []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("unmarshal request: %v", err)
			w.WriteHeader(500)
			return
		}
		if len(req.Messages) < 1 {
			t.Errorf("want at least 1 message, got %d", len(req.Messages))
			w.WriteHeader(500)
			return
		}
		n := len(req.Messages[len(req.Messages)-1].Content)
		if req.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: {\"type\":\"message_start\",\"message\":{\"content\":[],\"usage\":{\"input_tokens\":1}}}\n\n"+
				"data: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"ok\"}}\n\n"+
				"data: [DONE]\n\n")
			return
		}
		resp := map[string]any{
			"content":     []any{map[string]any{"type": "text", "text": "ok"}},
			"stop_reason": "end_turn",
			"usage": map[string]any{
				"input_tokens":  int64(0.738*float64(n) + 44 + 0.5),
				"output_tokens": 5,
			},
		}
		if cacheCreation != nil && n == 800 {
			// cache-hit style upstream: billing lands in cache_creation, so
			// input_tokens drops and cache_creation carries the rest, keeping
			// input + CC on the same 0.738*n+44 line (634 = 44 + 590).
			resp["usage"] = map[string]any{
				"input_tokens":   44,
				"output_tokens":  5,
				"cache_creation": map[string]any{"ephemeral_5m_input_tokens": 590, "ephemeral_1h_input_tokens": 0},
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-cache", "MISS")
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

func setupMeasure(t *testing.T, srv *httptest.Server) Result {
	t.Helper()
	gw := gateway.Client{BaseURL: srv.URL, Key: "sk-k"}
	res, err := Measure(context.Background(), gw, "target-model")
	if err != nil {
		t.Fatalf("Measure: %v", err)
	}
	return res
}

func TestMeasureLinearInjection(t *testing.T) {
	srv := fakeMessagesServer(t, nil)
	defer srv.Close()
	res := setupMeasure(t, srv)

	if len(res.Points) != 2 {
		t.Fatalf("points = %d, want 2", len(res.Points))
	}
	for i, want := range []int{1, 800} {
		p := res.Points[i]
		if p.N != want {
			t.Errorf("points[%d].N = %d, want %d", i, p.N, want)
		}
		wantInput := int(0.738*float64(want) + 44 + 0.5)
		if p.Input != wantInput {
			t.Errorf("points[%d].Input = %d, want %d", i, p.Input, wantInput)
		}
		wantTotal := wantInput
		if p.Total != wantTotal {
			t.Errorf("points[%d].Total = %d, want %d", i, p.Total, wantTotal)
		}
	}
	if res.Inj < 43 || res.Inj > 45 {
		t.Errorf("Inj = %d, want in [43,45]", res.Inj)
	}
	if res.Slope < 0.735 || res.Slope > 0.741 {
		t.Errorf("Slope = %f, want in [0.735,0.741]", res.Slope)
	}
	if res.CacheHeader != "MISS" {
		t.Errorf("CacheHeader = %q, want MISS", res.CacheHeader)
	}
	if res.Via != "direct" {
		t.Errorf("Via = %q, want direct", res.Via)
	}
	if res.Probe != nil {
		t.Fatal("unexpected third stream probe")
	}
}

func TestMeasureCacheCreationStillLinear(t *testing.T) {
	srv := fakeMessagesServer(t, map[string]any{"sentinel": true})
	defer srv.Close()
	res := setupMeasure(t, srv)

	for _, p := range res.Points {
		if p.N == 800 {
			if p.CC != 590 {
				t.Errorf("n=800 CC = %d, want 590", p.CC)
			}
			if p.Input != 44 {
				t.Errorf("n=800 Input = %d, want 44", p.Input)
			}
			// input 44 + CC 590 = 634 = 0.738*800+44, still on the line.
			if p.Total != 634 {
				t.Errorf("n=800 Total = %d, want 634", p.Total)
			}
		}
	}
	if res.Inj < 43 || res.Inj > 45 {
		t.Errorf("Inj = %d, want in [43,45]", res.Inj)
	}
	if res.Slope < 0.735 || res.Slope > 0.741 {
		t.Errorf("Slope = %f, want in [0.735,0.741]", res.Slope)
	}
}

func TestMeasureRetriesOn429(t *testing.T) {
	var calls int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := atomic.AddInt64(&calls, 1)
		// first two calls of the very first point get 429, then 200 forever
		if c <= 2 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.Unmarshal(body, &req)
		n := len(req.Messages[len(req.Messages)-1].Content)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content":     []any{map[string]any{"type": "text", "text": "ok"}},
			"stop_reason": "end_turn",
			"usage": map[string]any{
				"input_tokens":  int64(0.738*float64(n) + 44 + 0.5),
				"output_tokens": 5,
			},
		})
	}))
	defer srv.Close()

	old := BackoffDelay
	BackoffDelay = retryDelay
	defer func() { BackoffDelay = old }()

	res := setupMeasure(t, srv)
	if res.Inj < 43 || res.Inj > 45 {
		t.Errorf("Inj = %d, want in [43,45]", res.Inj)
	}
	if got := atomic.LoadInt64(&calls); got != 5 {
		// Two retried responses + three sequential merged samples.
		t.Errorf("total gateway calls = %d, want 5", got)
	}
}

func TestMeasureGivesUpAfterMaxAttempts(t *testing.T) {
	var calls int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"type":"error","error":{"type":"rate_limit_error","message":"429 forever"}}`)
	}))
	defer srv.Close()

	old := BackoffDelay
	BackoffDelay = retryDelay
	defer func() { BackoffDelay = old }()

	gw := gateway.Client{BaseURL: srv.URL, Key: "k"}
	_, err := Measure(context.Background(), gw, "m")
	if err == nil {
		t.Fatal("Measure returned nil error, want failure after max attempts")
	}
	// The first merged sample burns the retry budget before the sequence can continue.
	if got := atomic.LoadInt64(&calls); got != int64(gateway.RetryCount+1) {
		t.Errorf("gateway calls = %d, want %d", got, int64(gateway.RetryCount+1))
	}
}

// compile-time contract checks
var (
	_ = fmt.Sprintf
	_ = retryDelay
)
