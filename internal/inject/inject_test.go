package inject

import (
	"chanzhushui/internal/gateway"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func decodeConversation(t *testing.T, r *http.Request) (chars, round int) {
	t.Helper()
	var req struct {
		Messages []struct {
			Role    string
			Content json.RawMessage
		}
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		t.Error(err)
		return
	}
	for _, msg := range req.Messages {
		if msg.Role != "user" {
			continue
		}
		round++
		var text string
		if json.Unmarshal(msg.Content, &text) != nil {
			var blocks []struct{ Text string }
			if err := json.Unmarshal(msg.Content, &blocks); err != nil {
				t.Error(err)
				continue
			}
			for _, b := range blocks {
				text += b.Text
			}
		}
		chars += len(text)
	}
	return
}
func writeUsage(w http.ResponseWriter, total, read, write, output int) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("x-cache", "MISS")
	json.NewEncoder(w).Encode(map[string]any{"content": []any{map[string]string{"type": "text", "text": "OK"}}, "stop_reason": "end_turn", "usage": map[string]int{
		"input_tokens": total - read - write, "output_tokens": output, "cache_read_input_tokens": read, "cache_creation_input_tokens": write,
	}})
}
func TestMeasureRemovesAuditPrefixAndConversationFraming(t *testing.T) {
	for _, tc := range []struct {
		name              string
		injected, framing int
		tokenPerUnit      float64
	}{
		{"none", 6, 12, 1}, {"injection", 516, 12, 1}, {"large-injection", 6001, 7, 1}, {"different-tokenizer", 44, 22, 1.5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			previousCache := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				units, round := decodeConversation(t, r)
				total := int(float64(units)*tc.tokenPerUnit/2) + tc.injected + tc.framing*(round-1) + 2*(round-1)
				read, write := previousCache, total-previousCache-4
				previousCache = read + write
				writeUsage(w, total, read, write, 2)
			}))
			defer s.Close()
			r, err := Measure(context.Background(), gateway.Client{BaseURL: s.URL, NoRetry: true}, "claude-opus-5")
			if err != nil || r.Inj != tc.injected || !r.Reliable || r.Score == nil || *r.Score != InjectionScore(tc.injected) || calls.Load() != 3 {
				t.Fatalf("result=%+v err=%v calls=%d", r, err, calls.Load())
			}
			if r.Slope != tc.tokenPerUnit/2 || r.RoundOverhead < float64(tc.framing)-1 || r.RoundOverhead > float64(tc.framing)+1 {
				t.Fatalf("fit=%+v", r)
			}
			if r.Cache == nil || r.Cache.ThirdReadRatio == nil || *r.Cache.ThirdReadRatio != 100 {
				t.Fatal("cache report was not reused", r)
			}
			if r.CacheHeader != "MISS" || r.Via != "direct" {
				t.Fatal(r)
			}
			if r.R2 != nil || r.MaxResidual != nil || r.Probe != nil {
				t.Fatal("fit must not claim independent linearity validation", r)
			}
		})
	}
}
func TestMeasureRetriesOnlyFailedContinuation(t *testing.T) {
	var calls atomic.Int32
	var rounds []int
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		chars, round := decodeConversation(t, r)
		rounds = append(rounds, round)
		if n == 2 || n == 3 {
			w.WriteHeader(429)
			return
		}
		writeUsage(w, chars/2+44+8*(round-1)+2*(round-1), 0, 0, 2)
	}))
	defer s.Close()
	r, err := Measure(context.Background(), gateway.Client{BaseURL: s.URL, RetryDelay: time.Millisecond}, "claude-opus-5")
	if err != nil || r.Inj != 44 || calls.Load() != 5 || fmt.Sprint(rounds) != "[1 2 2 2 3]" {
		t.Fatalf("result=%+v err=%v rounds=%v", r, err, rounds)
	}
	if r.Cache == nil || !r.Cache.RetriesObserved {
		t.Fatal("retries not reported")
	}
}
func TestMeasureGivesUpAfterThreeRetries(t *testing.T) {
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(429) }))
	defer s.Close()
	r, err := Measure(context.Background(), gateway.Client{BaseURL: s.URL, RetryDelay: time.Millisecond}, "claude-opus-5")
	if err == nil || r.Score != nil || calls.Load() != gateway.RetryCount+1 {
		t.Fatal(r, err, calls.Load())
	}
}
func TestConversationFitRejectsDegenerateData(t *testing.T) {
	for _, points := range [][]Point{
		{}, {{N: 1, AdjustedTotal: 10}}, {{N: 1, AdjustedTotal: 10}, {N: 2, AdjustedTotal: 20}, {N: 3, AdjustedTotal: 30}},
	} {
		s, b, i := fitConversation(points)
		if s != 0 || b != 0 || i != 0 {
			t.Fatal(s, b, i)
		}
	}
}
