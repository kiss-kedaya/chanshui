package inject

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/kiss-kedaya/chanshui/internal/gateway"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func injectionServer(t *testing.T, injected, framing int, scale float64) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Messages []struct{ Content string } `json:"messages"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatal(err)
		}
		n := len(req.Messages[len(req.Messages)-1].Content)
		total := int(float64(n)*scale) + injected + framing
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content":     []any{map[string]string{"type": "text", "text": "OK"}},
			"stop_reason": "end_turn",
			"usage":       map[string]int{"input_tokens": total, "output_tokens": 1},
		})
	}))
}

func TestMeasureUsesTwoIndependentRequests(t *testing.T) {
	s := injectionServer(t, 44, 10, 0.75)
	defer s.Close()
	r, err := Measure(context.Background(), gateway.Client{BaseURL: s.URL, NoRetry: true}, "fixture")
	if err != nil || len(r.Points) != 2 || r.Inj < 53 || r.Inj > 55 || !r.Reliable || r.Score == nil {
		t.Fatalf("result=%+v err=%v", r, err)
	}
	if r.Slope < 0.74 || r.Slope > 0.76 || r.Note == "" {
		t.Fatalf("fit=%+v", r)
	}
}

func TestMeasureDoesNotReturnCacheReport(t *testing.T) {
	s := injectionServer(t, 6, 12, 1)
	defer s.Close()
	r, err := Measure(context.Background(), gateway.Client{BaseURL: s.URL, NoRetry: true}, "fixture")
	if err != nil || r.Inj != 18 || len(r.Points) != 2 {
		t.Fatalf("result=%+v err=%v", r, err)
	}
	if raw, _ := json.Marshal(r); string(raw) == "" || string(raw) == "null" {
		t.Fatal("missing independent injection result")
	}
}

func TestMeasureRetriesEachFailedRequestWithoutRepeatingWholeProbe(t *testing.T) {
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) <= 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Messages []struct{ Content string } `json:"messages"`
		}
		_ = json.Unmarshal(body, &req)
		n := len(req.Messages[len(req.Messages)-1].Content)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"content":[{"type":"text","text":"OK"}],"stop_reason":"end_turn","usage":{"input_tokens":%d,"output_tokens":1}}`, n+44)
	}))
	defer s.Close()
	old := BackoffDelay
	BackoffDelay = time.Millisecond
	defer func() { BackoffDelay = old }()
	r, err := Measure(context.Background(), gateway.Client{BaseURL: s.URL, RetryDelay: time.Millisecond}, "fixture")
	if err != nil || r.Score == nil || calls.Load() != 4 {
		t.Fatalf("result=%+v err=%v calls=%d", r, err, calls.Load())
	}
}

func TestMeasureCannotScoreFailedSample(t *testing.T) {
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer s.Close()
	r, err := Measure(context.Background(), gateway.Client{BaseURL: s.URL, NoRetry: true}, "fixture")
	if err == nil || r.Score != nil || calls.Load() != 1 {
		t.Fatalf("result=%+v err=%v calls=%d", r, err, calls.Load())
	}
}

func TestMeasureKeepsUsageWhenReplyThinks(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Messages []struct{ Content string } `json:"messages"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatal(err)
		}
		n := len(req.Messages[len(req.Messages)-1].Content)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"content":[{"type":"thinking","thinking":"need more room","signature":"sig"},{"type":"text","text":""}],"stop_reason":"max_tokens","usage":{"input_tokens":%d,"output_tokens":16}}`, n+40)
	}))
	defer s.Close()
	r, err := Measure(context.Background(), gateway.Client{BaseURL: s.URL, NoRetry: true}, "fixture")
	if err != nil || r.Score == nil || *r.Score != 10 || r.Inj < 39 || r.Inj > 41 {
		t.Fatalf("result=%+v err=%v", r, err)
	}
}

func TestNegativeInterceptIsNotScored(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Messages []struct{ Content string } `json:"messages"`
		}
		_ = json.Unmarshal(body, &req)
		n := len(req.Messages[len(req.Messages)-1].Content)
		total := 20
		if n > 10 {
			total = 10
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"content":[{"type":"text","text":"OK"}],"stop_reason":"end_turn","usage":{"input_tokens":%d,"output_tokens":1}}`, total)
	}))
	defer s.Close()
	r, err := Measure(context.Background(), gateway.Client{BaseURL: s.URL, NoRetry: true}, "fixture")
	if err != nil || r.Reliable || r.Score != nil || r.Inj != 0 {
		t.Fatalf("negative intercept scored: %+v %v", r, err)
	}
}

// Claude routes hide a system prefix and deduct it from usage, so the old
// two-sample intercept sees zero. The cache write threshold reveals it.
func TestMeasureClaudeUsesCacheThreshold(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			System []struct {
				Text string `json:"text"`
			} `json:"system"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		n := 0
		if len(body.System) > 0 {
			n = len(body.System[0].Text)
		}
		usage := map[string]int{"input_tokens": n/4 + 11, "output_tokens": 1}
		if n >= 1600 {
			usage = map[string]int{"input_tokens": 11, "output_tokens": 1, "cache_creation_input_tokens": n / 4}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content":     []any{map[string]string{"type": "text", "text": "OK"}},
			"usage":       usage,
			"stop_reason": "end_turn",
		})
	}))
	defer s.Close()
	r, err := Measure(context.Background(), gateway.Client{BaseURL: s.URL, NoRetry: true}, "claude-opus-5-5")
	if err != nil || r.Mode != "cache_threshold" || r.Threshold < 380 || r.Threshold > 420 || r.Inj < 90 || r.Inj > 132 || r.Score == nil || *r.Score != 8 {
		t.Fatalf("result=%+v err=%v", r, err)
	}
}
