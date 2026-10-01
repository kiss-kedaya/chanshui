package cachecheck

import (
	"chanzhushui/internal/gateway"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func cacheMetric(read, write, input, output int) gateway.Metrics {
	return gateway.Metrics{Protocol: "messages", UsagePresent: true, CacheReadReported: true, CacheWriteReported: true, CacheRead: read, CacheCreation1h: write, InputTokens: input, OutputTokens: output}
}

func ratioText(v *float64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatFloat(*v, 'f', 2, 64)
}

func TestCacheEquation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		metrics []gateway.Metrics
		status  string
		ratio   string
	}{
		{"growing", []gateway.Metrics{cacheMetric(0, 4600, 8, 1), cacheMetric(4600, 2100, 8, 1), cacheMetric(6700, 1100, 8, 1)}, "pass", "85.81"},
		{"partial", []gateway.Metrics{cacheMetric(0, 4600, 8, 1), cacheMetric(4500, 2200, 8, 1), cacheMetric(5360, 2400, 8, 1)}, "mismatch", "69.00"},
		{"overread-visible", []gateway.Metrics{cacheMetric(0, 1000, 8, 1), cacheMetric(1000, 0, 8, 1), cacheMetric(1010, 20, 8, 1)}, "pass", "97.30"},
		{"zero", []gateway.Metrics{cacheMetric(0, 0, 8, 1), cacheMetric(0, 0, 8, 1), cacheMetric(0, 0, 8, 1)}, "unknown", "0.00"},
		{"slack-20", []gateway.Metrics{cacheMetric(0, 10000, 8, 1), cacheMetric(10000, 1380, 8, 1), cacheMetric(11400, 92, 8, 1)}, "pass", "99.13"},
		{"warm", []gateway.Metrics{cacheMetric(3635, 0, 12, 4), cacheMetric(3904, 0, 3, 4), cacheMetric(3905, 0, 3, 4)}, "pass", "99.92"},
		{"flat", []gateway.Metrics{cacheMetric(4739, 0, 7, 16), cacheMetric(4739, 0, 816, 16), cacheMetric(4739, 0, 826, 16)}, "mismatch", "85.16"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := FromMetrics(tc.metrics, "prefix", "messages", false)
			if r.Status != tc.status || ratioText(r.ThirdReadRatio) != tc.ratio {
				t.Fatalf("status %s ratio %s, want %s %s", r.Status, ratioText(r.ThirdReadRatio), tc.status, tc.ratio)
			}
			if r.Samples[1].HitRate == nil {
				t.Fatal("per-request hit rate missing")
			}
			if r.Pass != nil && *r.Pass && r.Expected == 0 {
				t.Fatal("zero cannot pass")
			}
			if tc.name == "slack-20" && r.Delta != 20 {
				t.Fatalf("delta = %d, want 20", r.Delta)
			}
			if tc.name == "partial" && r.Pass != nil && *r.Pass {
				t.Fatal("100 token gap must stay a mismatch")
			}
		})
	}
}

func TestMissingCacheWriteDoesNotBecomeZero(t *testing.T) {
	m := []gateway.Metrics{cacheMetric(0, 1000, 8, 1), cacheMetric(1000, 0, 8, 1), cacheMetric(1000, 0, 8, 1)}
	m[1].CacheWriteReported = false
	r := FromMetrics(m, "prefix", "messages", false)
	if r.Pass != nil || r.Samples[1].HitRate != nil {
		t.Fatalf("missing usage became zero: %+v", r)
	}
	if ratioText(r.ThirdReadRatio) != "99.21" {
		t.Fatalf("third rate = %s, want 99.21", ratioText(r.ThirdReadRatio))
	}
	for i := range m {
		m[i].Protocol = "chat"
	}
	r = FromMetrics(m, "prefix", "chat", false)
	if r.Samples[1].HitRate == nil || ratioText(r.ThirdReadRatio) != "99.21" || r.Pass != nil {
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

func TestCollectKeepsStablePrefixBreakpoints(t *testing.T) {
	for _, mode := range []string{"anthropic", "chat", "openai"} {
		t.Run(mode, func(t *testing.T) {
			var bodies []map[string]any
			inFlight, maxFlight := 0, 0
			var flight sync.Mutex
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				flight.Lock()
				inFlight++
				if inFlight > maxFlight {
					maxFlight = inFlight
				}
				flight.Unlock()
				defer func() {
					flight.Lock()
					inFlight--
					flight.Unlock()
				}()
				var q map[string]any
				if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
					t.Error(err)
				}
				bodies = append(bodies, q)
				turn := len(bodies)
				read, write := 0, 4096
				if turn == 2 {
					read, write = 4096, 512
				}
				if turn == 3 {
					read, write = 4608, 0
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
			if err != nil || len(bodies) != 3 || r.Pass == nil || !*r.Pass {
				t.Fatalf("%+v %v", r, err)
			}
			if ratioText(r.ThirdReadRatio) != "99.83" {
				t.Fatal(ratioText(r.ThirdReadRatio), r)
			}
			if r.Samples[1].RequestHash == r.Samples[2].RequestHash {
				t.Fatal("second and third requests are identical")
			}
			if maxFlight != 1 {
				t.Fatalf("cache rounds overlapped: %d", maxFlight)
			}
			if !strings.Contains(r.Reason, "缓存") {
				t.Fatal(r.Reason)
			}
			systems := make([]string, 3)
			users := make([]string, 3)
			for i, body := range bodies {
				systems[i], users[i] = prefixAndUser(t, mode, body)
				if i > 0 && systems[i] != systems[0] {
					t.Fatal("system prefix changed between rounds")
				}
			}
			if len(systems[0]) <= 8000 {
				t.Fatalf("system prefix too short: %d", len(systems[0]))
			}
			extra := strings.TrimSuffix(users[1], cacheTailSame)
			if extra == "" || users[0] != cacheTailSame || users[1] != extra+cacheTailSame || users[2] != extra+cacheTailLast {
				t.Fatalf("prefix shape %#v", users)
			}
			if mode == "anthropic" {
				assertAnthropicBlocks(t, bodies, extra)
			}
		})
	}
}

func prefixAndUser(t *testing.T, mode string, body map[string]any) (string, string) {
	t.Helper()
	raw := body["messages"]
	if mode == "openai" {
		raw = body["input"]
	}
	msgs, _ := raw.([]any)
	if mode == "anthropic" {
		system, _ := body["system"].([]any)
		if len(system) != 1 || len(msgs) != 1 {
			t.Fatalf("anthropic shape system=%d messages=%d", len(system), len(msgs))
		}
		block := system[0].(map[string]any)
		cc := block["cache_control"].(map[string]any)
		if cc["type"] != "ephemeral" || cc["ttl"] != "1h" {
			t.Fatalf("system cache_control %#v", cc)
		}
		msg := msgs[0].(map[string]any)
		if msg["role"] != "user" {
			t.Fatalf("role %#v", msg["role"])
		}
		return block["text"].(string), userText(t, msg["content"])
	}
	if len(msgs) != 2 {
		t.Fatalf("message count %d", len(msgs))
	}
	system := msgs[0].(map[string]any)
	user := msgs[1].(map[string]any)
	if system["role"] != "system" || user["role"] != "user" {
		t.Fatalf("roles %#v %#v", system["role"], user["role"])
	}
	return strings.TrimRight(system["content"].(string), "\n"), user["content"].(string)
}

func userText(t *testing.T, content any) string {
	t.Helper()
	switch v := content.(type) {
	case string:
		return v
	case []any:
		var b strings.Builder
		for _, item := range v {
			b.WriteString(item.(map[string]any)["text"].(string))
		}
		return b.String()
	default:
		t.Fatalf("content %#v", content)
		return ""
	}
}

func assertAnthropicBlocks(t *testing.T, bodies []map[string]any, extra string) {
	t.Helper()
	for i, body := range bodies {
		msg := body["messages"].([]any)[0].(map[string]any)
		content := msg["content"]
		if i == 0 {
			if _, ok := content.(string); !ok {
				t.Fatalf("round 1 content %#v", content)
			}
			continue
		}
		blocks, ok := content.([]any)
		if !ok || len(blocks) != 2 {
			t.Fatalf("round %d blocks %#v", i+1, content)
		}
		first := blocks[0].(map[string]any)
		second := blocks[1].(map[string]any)
		cc := first["cache_control"].(map[string]any)
		if first["text"] != extra || cc["ttl"] != "1h" || cc["type"] != "ephemeral" {
			t.Fatalf("B block %#v", first)
		}
		if _, exists := second["cache_control"]; exists {
			t.Fatalf("tail must not be cached: %#v", second)
		}
	}
}
