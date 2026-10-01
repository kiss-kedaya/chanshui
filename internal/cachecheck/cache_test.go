package cachecheck

import (
	"chanzhushui/internal/gateway"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func cacheMetric(read, write, input, output int) gateway.Metrics {
	return gateway.Metrics{Protocol: "messages", UsagePresent: true, CacheReadReported: true, CacheWriteReported: true, CacheRead: read, CacheCreation1h: write, InputTokens: input, OutputTokens: output}
}
func TestCacheEquation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		metrics []gateway.Metrics
		status  string
		ratio   *float64
	}{
		{"growing", []gateway.Metrics{cacheMetric(0, 4600, 8, 1), cacheMetric(4600, 2100, 8, 1), cacheMetric(6700, 1100, 8, 1)}, "pass", floatPtr(100)},
		{"partial", []gateway.Metrics{cacheMetric(0, 4600, 8, 1), cacheMetric(4500, 2200, 8, 1), cacheMetric(5360, 2400, 8, 1)}, "mismatch", floatPtr(80)},
		{"overread-visible", []gateway.Metrics{cacheMetric(0, 1000, 8, 1), cacheMetric(1000, 0, 8, 1), cacheMetric(1010, 20, 8, 1)}, "unknown", floatPtr(101)},
		{"zero", []gateway.Metrics{cacheMetric(0, 0, 8, 1), cacheMetric(0, 0, 8, 1), cacheMetric(0, 0, 8, 1)}, "unknown", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := FromMetrics(tc.metrics, "prefix", "messages", false)
			if r.Status != tc.status || !reflect.DeepEqual(r.ThirdReadRatio, tc.ratio) {
				t.Fatalf("got %+v", r)
			}
			if r.Samples[1].HitRate == nil {
				t.Fatal("per-request hit rate missing")
			}
			if r.Pass != nil && *r.Pass && r.Expected == 0 {
				t.Fatal("zero cannot pass")
			}
		})
	}
}
func floatPtr(n float64) *float64 { return &n }
func TestMissingCacheWriteDoesNotBecomeZero(t *testing.T) {
	m := []gateway.Metrics{cacheMetric(0, 1000, 8, 1), cacheMetric(1000, 0, 8, 1), cacheMetric(1000, 0, 8, 1)}
	m[1].CacheWriteReported = false
	r := FromMetrics(m, "prefix", "messages", false)
	if r.Pass != nil || r.ThirdReadRatio != nil || r.Samples[1].HitRate != nil {
		t.Fatalf("missing usage became zero: %+v", r)
	}
	for i := range m {
		m[i].Protocol = "chat"
	}
	r = FromMetrics(m, "prefix", "chat", false)
	if r.Samples[1].HitRate == nil || r.ThirdReadRatio != nil {
		t.Fatalf("compatible read rate should remain visible: %+v", r)
	}
}
func TestFailedThirdRequestCannotPass(t *testing.T) {
	m := []gateway.Metrics{cacheMetric(0, 1000, 8, 1), cacheMetric(1000, 100, 8, 1), cacheMetric(1100, 100, 8, 1)}
	m[2].Err = "failed"
	r := FromMetrics(m, "prefix", "messages", false)
	if r.Pass != nil || r.ThirdReadRatio != nil {
		t.Fatalf("failed request passed: %+v", r)
	}
}
func TestCollectKeepsGrowingConversationAndBreakpoints(t *testing.T) {
	for _, mode := range []string{"anthropic", "chat", "openai"} {
		t.Run(mode, func(t *testing.T) {
			var calls [][]map[string]any
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var q map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
					t.Error(err)
				}
				if q["system"] != nil {
					t.Error("fixed system prefix must not be injected")
				}
				raw := q["messages"]
				if mode == "openai" {
					raw = q["input"]
				}
				var messages []map[string]any
				if err := json.Unmarshal(raw, &messages); err != nil {
					t.Error(err)
				}
				calls = append(calls, messages)
				turn := len(calls)
				if len(messages) != turn*2-1 {
					t.Errorf("round %d message count %d", turn, len(messages))
				}
				for i, msg := range messages {
					wantRole := "user"
					if i%2 == 1 {
						wantRole = "assistant"
					}
					if msg["role"] != wantRole {
						t.Errorf("missing turn: %+v", messages)
					}
					if mode == "anthropic" && i%2 == 0 {
						blocks, ok := msg["content"].([]any)
						if !ok || len(blocks) != 1 {
							t.Fatalf("missing user content block: %+v", msg)
						}
						block := blocks[0].(map[string]any)
						cc := block["cache_control"].(map[string]any)
						if cc["ttl"] != "1h" || cc["type"] != "ephemeral" {
							t.Error(cc)
						}
					}
				}
				if turn > 1 && !reflect.DeepEqual(calls[turn-2], messages[:len(messages)-2]) {
					t.Error("previous user/assistant context was changed")
				}
				read, write := 0, 4608
				if turn == 2 {
					read, write = 4608, 2048
				}
				if turn == 3 {
					read, write = 6656, 1024
				}
				usage := map[string]int{"input_tokens": 8, "output_tokens": 1, "cache_read_input_tokens": read, "cache_creation_input_tokens": write}
				w.Header().Set("Content-Type", "application/json")
				if mode == "anthropic" {
					json.NewEncoder(w).Encode(map[string]any{"content": []any{map[string]string{"type": "text", "text": "OK"}}, "stop_reason": "end_turn", "usage": usage})
				} else if mode == "chat" {
					usage["prompt_tokens"] = read + write + 8
					json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": "OK"}, "finish_reason": "stop"}}, "usage": usage})
				} else {
					usage["input_tokens"] = read + write + 8
					json.NewEncoder(w).Encode(map[string]any{"status": "completed", "output": []any{map[string]any{"type": "message", "content": []any{map[string]string{"type": "output_text", "text": "OK"}}}}, "usage": usage})
				}
			}))
			defer s.Close()
			r, err := Measure(context.Background(), gateway.Client{BaseURL: s.URL, Mode: mode, NoRetry: true}, "claude-opus-5", 10)
			if err != nil || len(calls) != 3 || r.Pass == nil || !*r.Pass {
				t.Fatalf("%+v %v", r, err)
			}
			if r.ThirdReadRatio == nil || *r.ThirdReadRatio != 100 {
				t.Fatal(r)
			}
			if r.Samples[1].RequestHash == r.Samples[2].RequestHash || r.Samples[1].PromptChars == r.Samples[2].PromptChars {
				t.Fatal("second and third requests are identical")
			}
			if !strings.Contains(r.Reason, "缓存") {
				t.Fatal(r.Reason)
			}
		})
	}
}
