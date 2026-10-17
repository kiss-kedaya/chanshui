package cachecheck

import (
	"context"
	"encoding/json"
	"github.com/kiss-kedaya/chanshui/internal/gateway"
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
		{"growing", []gateway.Metrics{cacheMetric(0, 4600, 8, 1), cacheMetric(4600, 2100, 8, 1), cacheMetric(6700, 1100, 8, 1)}, "pass", "100.00"},
		{"partial", []gateway.Metrics{cacheMetric(0, 4600, 8, 1), cacheMetric(4500, 2200, 8, 1), cacheMetric(5360, 2400, 8, 1)}, "mismatch", "80.00"},
		{"overread-visible", []gateway.Metrics{cacheMetric(0, 1000, 8, 1), cacheMetric(1000, 0, 8, 1), cacheMetric(1010, 20, 8, 1)}, "pass", "101.00"},
		{"zero", []gateway.Metrics{cacheMetric(0, 0, 8, 1), cacheMetric(0, 0, 8, 1), cacheMetric(0, 0, 8, 1)}, "unknown", ""},
		{"slack-20", []gateway.Metrics{cacheMetric(0, 10000, 8, 1), cacheMetric(10000, 1380, 8, 1), cacheMetric(11400, 92, 8, 1)}, "pass", "100.18"},
		{"warm", []gateway.Metrics{cacheMetric(3635, 0, 12, 4), cacheMetric(3904, 0, 3, 4), cacheMetric(3905, 0, 3, 4)}, "pass", "100.03"},
		{"flat", []gateway.Metrics{cacheMetric(4739, 0, 7, 16), cacheMetric(4739, 0, 816, 16), cacheMetric(4739, 0, 826, 16)}, "mismatch", "100.00"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := FromMetrics(tc.metrics, "prefix", "messages", false)
			if r.Status != tc.status || ratioText(r.ThirdReadRatio) != tc.ratio {
				t.Fatalf("status %s ratio %s, want %s %s", r.Status, ratioText(r.ThirdReadRatio), tc.status, tc.ratio)
			}
			if tc.name != "zero" && r.Samples[1].HitRate == nil {
				t.Fatal("per-request hit rate missing")
			}
			if tc.name == "zero" && r.Samples[1].HitRate != nil {
				t.Fatal("zero baseline invented a hit rate")
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
	if r.Pass != nil || r.ThirdReadRatio != nil || r.Samples[2].HitRate != nil || ratioText(r.Samples[1].HitRate) != "100.00" {
		t.Fatalf("missing usage became zero: %+v", r)
	}
	for i := range m {
		m[i].Protocol = "chat"
	}
	r = FromMetrics(m, "prefix", "chat", false)
	if r.Pass != nil || r.ThirdReadRatio != nil || r.Samples[2].HitRate != nil || ratioText(r.Samples[1].HitRate) != "100.00" {
		t.Fatalf("missing write must not invent the next rate: %+v", r)
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
				read, write := 0, 2000
				if turn > 1 {
					read = 2000 + (turn-2)*40
					write = 40
				}
				if turn == cacheTurns {
					write = 20
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
			if err != nil || len(bodies) != cacheTurns || r.Pass == nil || !*r.Pass {
				t.Fatalf("%+v %v bodies=%d", r, err, len(bodies))
			}
			if ratioText(r.ThirdReadRatio) != "100.00" {
				t.Fatal(ratioText(r.ThirdReadRatio), r)
			}
			if r.Samples[cacheTurns-2].RequestHash == r.Samples[cacheTurns-1].RequestHash {
				t.Fatal("last two requests are identical")
			}
			if maxFlight != 1 {
				t.Fatalf("cache rounds overlapped: %d", maxFlight)
			}
			if !strings.Contains(r.Reason, "缓存") {
				t.Fatal(r.Reason)
			}
			var system0 string
			for i, body := range bodies {
				system, msgs := conversation(t, mode, body)
				if i == 0 {
					system0 = system
				} else if system != system0 {
					t.Fatal("system prefix changed between rounds")
				}
				if len(msgs) != 1+i*2 {
					t.Fatalf("turn %d message count %d", i+1, len(msgs))
				}
				if msgs[len(msgs)-1].role != "user" || msgs[len(msgs)-1].text != cacheTurnPrompt(i) {
					t.Fatalf("turn %d tail %#v", i+1, msgs[len(msgs)-1])
				}
				if i > 0 && (msgs[len(msgs)-2].role != "assistant" || msgs[len(msgs)-2].text != "OK") {
					t.Fatalf("turn %d replay %#v", i+1, msgs[len(msgs)-2])
				}
			}
			if len(system0) < 4500 {
				t.Fatalf("system prefix too short: %d", len(system0))
			}
			if mode == "anthropic" {
				assertAnthropicBreakpoints(t, bodies)
			}
		})
	}
}

type spoken struct {
	role string
	text string
}

func conversation(t *testing.T, mode string, body map[string]any) (string, []spoken) {
	t.Helper()
	raw := body["messages"]
	if mode == "openai" {
		raw = body["input"]
	}
	msgs, _ := raw.([]any)
	if mode == "anthropic" {
		system, _ := body["system"].([]any)
		if len(system) != 1 {
			t.Fatalf("anthropic system blocks %d", len(system))
		}
		block := system[0].(map[string]any)
		cc := block["cache_control"].(map[string]any)
		if cc["type"] != "ephemeral" {
			t.Fatalf("system cache_control %#v", cc)
		}
		if _, ok := cc["ttl"]; ok {
			t.Fatalf("system ttl %#v", cc)
		}
		out := make([]spoken, 0, len(msgs))
		for _, item := range msgs {
			msg := item.(map[string]any)
			out = append(out, spoken{role: msg["role"].(string), text: userText(t, msg["content"])})
		}
		return block["text"].(string), out
	}
	if len(msgs) < 2 {
		t.Fatalf("message count %d", len(msgs))
	}
	system := msgs[0].(map[string]any)
	if system["role"] != "system" {
		t.Fatalf("system role %#v", system["role"])
	}
	out := make([]spoken, 0, len(msgs)-1)
	for _, item := range msgs[1:] {
		msg := item.(map[string]any)
		out = append(out, spoken{role: msg["role"].(string), text: msg["content"].(string)})
	}
	return strings.TrimRight(system["content"].(string), "\n"), out
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

func assertAnthropicBreakpoints(t *testing.T, bodies []map[string]any) {
	t.Helper()
	for i, body := range bodies {
		msgs := body["messages"].([]any)
		markedUsers := 0
		for n, item := range msgs {
			msg := item.(map[string]any)
			content := msg["content"]
			isLast := n == len(msgs)-1
			prevUser := i > 0 && n == len(msgs)-3
			if msg["role"] == "assistant" {
				if _, ok := content.([]any); ok {
					t.Fatalf("turn %d assistant should be plain text", i+1)
				}
				continue
			}
			part, ok := content.([]any)
			if !ok {
				if isLast || prevUser {
					t.Fatalf("turn %d user %d missing breakpoint %#v", i+1, n, content)
				}
				continue
			}
			if len(part) != 1 {
				t.Fatalf("turn %d user %d content %#v", i+1, n, content)
			}
			block := part[0].(map[string]any)
			cc, exists := block["cache_control"].(map[string]any)
			if isLast || prevUser {
				if !exists || cc["type"] != "ephemeral" {
					t.Fatalf("turn %d user breakpoint %#v", i+1, block)
				}
				if _, hasTTL := cc["ttl"]; hasTTL {
					t.Fatalf("turn %d ttl %#v", i+1, cc)
				}
				markedUsers++
			} else if exists {
				t.Fatalf("turn %d extra breakpoint %#v", i+1, block)
			}
		}
		want := 1
		if i > 0 {
			want = 2
		}
		if markedUsers != want {
			t.Fatalf("turn %d marked users %d want %d", i+1, markedUsers, want)
		}
	}
}

func TestReportedJobChainIsOneHundredOnTheLastLink(t *testing.T) {
	rounds := [][2]int{{0, 3968}, {3968, 54}, {0, 4060}, {4060, 38}, {4098, 38}, {4136, 38}}
	m := make([]gateway.Metrics, len(rounds))
	for i, pair := range rounds {
		m[i] = cacheMetric(pair[0], pair[1], 4, 6)
	}
	r := FromMetrics(m, "prefix", "messages", false)
	if r.Status != "pass" || ratioText(r.ThirdReadRatio) != "100.00" || r.Delta != 0 {
		t.Fatalf("last link = status %s ratio %s delta %d", r.Status, ratioText(r.ThirdReadRatio), r.Delta)
	}
	if ratioText(r.Samples[2].HitRate) != "0.00" || ratioText(r.Samples[5].HitRate) != "100.00" || r.Samples[0].HitRate != nil {
		t.Fatalf("row rates = %s %s %v", ratioText(r.Samples[2].HitRate), ratioText(r.Samples[5].HitRate), r.Samples[0].HitRate)
	}
	if r.Transitions[1].Pass == nil || *r.Transitions[1].Pass {
		t.Fatal("round 3 must stay a miss")
	}
}

func TestDelayedCacheReadCanPassWhenLaterLinksAreStable(t *testing.T) {
	m := []gateway.Metrics{
		cacheMetric(0, 0, 6022, 8),
		cacheMetric(0, 0, 6054, 8),
		cacheMetric(5888, 0, 198, 8),
		cacheMetric(5888, 0, 230, 8),
		cacheMetric(5888, 0, 262, 8),
		cacheMetric(5888, 0, 294, 8),
	}
	r := FromMetrics(m, "prefix", "responses", false)
	if r.Status != "pass" || r.Pass == nil || !*r.Pass || ratioText(r.ThirdReadRatio) != "100.00" {
		t.Fatalf("delayed cache read = status %s ratio %s pass %v reason %s", r.Status, ratioText(r.ThirdReadRatio), r.Pass, r.Reason)
	}
	// Delayed warmup must not hide a broken final link or missing usage.
	m[5].CacheRead = 5000
	r = FromMetrics(m, "prefix", "responses", false)
	if r.Pass == nil || *r.Pass {
		t.Fatal("broken final link passed after delayed warmup")
	}
	m[4].CacheWriteReported = false
	r = FromMetrics(m, "prefix", "responses", false)
	if r.Pass != nil {
		t.Fatal("missing final-link usage became a verdict")
	}
}

func TestReadOnlyCacheUsesLastReadOverTotal(t *testing.T) {
	rows := [][2]int{{0, 6367}, {6144, 255}, {6272, 255}, {6272, 200}, {6272, 180}, {6272, 255}}
	m := make([]gateway.Metrics, len(rows))
	for i, row := range rows {
		m[i] = cacheMetric(row[0], 0, row[1], 1)
		m[i].CacheWriteReported = false
		m[i].CacheCreation1h = 0
		m[i].CacheCreation5m = 0
	}
	r := FromMetrics(m, "prefix", "messages", false)
	if r.Status != "pass" || r.Pass == nil || !*r.Pass || ratioText(r.ThirdReadRatio) != "96.09" {
		t.Fatalf("readonly pass = status %s ratio %s pass %v", r.Status, ratioText(r.ThirdReadRatio), r.Pass)
	}
	if r.ThirdReadBaseline != 6272 {
		t.Fatalf("baseline %d, want 6272", r.ThirdReadBaseline)
	}
	if ratioText(r.Samples[0].HitRate) != "0.00" || ratioText(r.Samples[1].HitRate) != "96.02" || ratioText(r.Samples[5].HitRate) != "96.09" {
		t.Fatalf("row rates %s %s %s", ratioText(r.Samples[0].HitRate), ratioText(r.Samples[1].HitRate), ratioText(r.Samples[5].HitRate))
	}
}

func TestReadOnlyCacheColdStartWithoutReadField(t *testing.T) {
	inputs := []int{1909, 143, 169, 195, 221, 247}
	m := make([]gateway.Metrics, len(inputs))
	for i, input := range inputs {
		m[i] = gateway.Metrics{
			Protocol:       "chat",
			UsagePresent:   true,
			InputTokens:    input,
			OutputTokens:   4,
			OutputReported: true,
			Attempts:       1,
		}
		if i == 0 {
			continue
		}
		m[i].CacheRead = 1792
		m[i].CacheReadReported = true
		m[i].CacheWriteReported = false
	}
	r := FromMetrics(m, "prefix", "chat", false)
	if r.Status != "pass" || r.Pass == nil || !*r.Pass {
		t.Fatalf("cold-start read-only = status %s pass %v reason %s", r.Status, r.Pass, r.Reason)
	}
	if ratioText(r.ThirdReadRatio) != "87.89" {
		t.Fatalf("ratio %s, want 87.89", ratioText(r.ThirdReadRatio))
	}
	if r.ThirdReadBaseline != 1792 || r.Expected != 1792 || r.Actual != 1792 || r.Delta != 0 {
		t.Fatalf("baseline=%d expected=%d actual=%d delta=%d", r.ThirdReadBaseline, r.Expected, r.Actual, r.Delta)
	}
	if r.Samples[0].HitRate != nil {
		t.Fatalf("unreported first read got hit rate %v", *r.Samples[0].HitRate)
	}
	if ratioText(r.Samples[1].HitRate) != "92.61" {
		t.Fatalf("second hit rate %s, want 92.61", ratioText(r.Samples[1].HitRate))
	}
	if len(r.Transitions) != 5 || r.Transitions[0].Pass == nil || !*r.Transitions[0].Pass || r.Transitions[4].Pass == nil || !*r.Transitions[4].Pass {
		t.Fatalf("transitions %+v", r.Transitions)
	}
}

func TestReadOnlyZeroReadIsZeroNotUnknown(t *testing.T) {
	m := []gateway.Metrics{cacheMetric(0, 0, 100, 1), cacheMetric(0, 0, 80, 1)}
	for i := range m {
		m[i].CacheWriteReported = false
		m[i].CacheCreation1h = 0
	}
	r := FromMetrics(m, "prefix", "chat", false)
	if r.Status == "unknown" || r.Pass == nil || *r.Pass || ratioText(r.ThirdReadRatio) != "0.00" {
		t.Fatalf("zero read = status %s ratio %s pass %v", r.Status, ratioText(r.ThirdReadRatio), r.Pass)
	}
	if r.ThirdReadBaseline != 0 || r.Samples[1].HitRate == nil || ratioText(r.Samples[1].HitRate) != "0.00" {
		t.Fatal("zero read invented a write baseline")
	}
}
