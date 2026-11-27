package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kiss-kedaya/chanshui/internal/history"
)

func seqNumbers(start, count int) string {
	parts := make([]string, count)
	for i := 0; i < count; i++ {
		parts[i] = fmt.Sprintf("%d", 1+((start+i*17)%355))
	}
	return strings.Join(parts, ", ")
}

func seqNumbersDistinct(seed uint64, count int) string {
	parts := make([]string, count)
	x := seed
	for i := 0; i < count; i++ {
		x = x*6364136223846793005 + 1442695040888963407
		parts[i] = fmt.Sprintf("%d", 1+int((x>>33)%355))
	}
	return strings.Join(parts, ", ")
}

func TestAntiCheatCatchesRouterNerverPattern(t *testing.T) {
	v := Verdict{
		Fingerprint: map[string]any{
			"basis": "numeric", "family": "claude", "top_model": "claude-opus-5-5", "top_prob": 0.9999, "used_outputs": 3,
			"evidence": []map[string]any{
				{"id": "numeric-1", "expected_count": 304, "parsed_count": 304, "answer": seqNumbers(1, 304), "ms": 46},
				{"id": "numeric-2", "expected_count": 318, "parsed_count": 318, "answer": seqNumbers(1, 318), "ms": 46},
				{"id": "numeric-3", "expected_count": 342, "parsed_count": 342, "answer": seqNumbers(1, 342), "ms": 46},
			},
		},
		Thinking: map[string]any{
			"score": 0, "max_score": 12, "sig_forged": true,
			"sig_class": "伪造信封（仿 protobuf）", "sig_forge_reason": "缺少版本头",
		},
		IQ:        map[string]any{"score": 56},
		Injection: map[string]any{"score": 10},
		Tools:     map[string]any{"pass": true},
	}
	ac := evaluateAntiCheat(v, "claude-opus-5-5")
	if ac == nil || !ac.Detected {
		t.Fatalf("expected anticheat detection, got %+v", ac)
	}
	v.AntiCheat = ac
	total := buildTotal(v, "claude-opus-5-5")
	for _, p := range total.Parts {
		if (p.Key == "fingerprint" || p.Key == "thinking") && p.Contribution != 0 {
			t.Fatalf("part %s should be penalized to 0, got %+v", p.Key, p)
		}
	}
}

func TestAntiCheatCatchesA6APIFixed300Replay(t *testing.T) {
	fixed := seqNumbers(19, 300)
	v := Verdict{
		Fingerprint: map[string]any{
			"basis": "numeric", "family": "claude", "top_model": "claude-opus-5-5", "top_prob": 0.9999, "used_outputs": 3,
			"evidence": []map[string]any{
				{"id": "numeric-1", "expected_count": 304, "parsed_count": 300, "answer": fixed, "ms": 401},
				{"id": "numeric-2", "expected_count": 318, "parsed_count": 300, "answer": fixed, "ms": 423},
				{"id": "numeric-3", "expected_count": 327, "parsed_count": 300, "answer": fixed, "ms": 426},
			},
		},
		IQ: map[string]any{"score": 45},
	}
	ac := evaluateAntiCheat(v, "claude-opus-5-5")
	if ac == nil || !ac.Detected {
		t.Fatalf("expected anticheat detection, got %+v", ac)
	}
	codes := map[string]bool{}
	for _, s := range ac.Signals {
		codes[s.Code] = true
	}
	for _, want := range []string{"fp_impossible_speed", "fp_static_replay", "fp_fixed_count"} {
		if !codes[want] {
			t.Fatalf("missing signal %s in %+v", want, ac.Signals)
		}
	}
}

func TestAntiCheatCatchesJaaakAndNgrokFullReplay(t *testing.T) {
	v := Verdict{
		Fingerprint: map[string]any{
			"basis": "numeric", "family": "claude", "top_model": "claude-opus-5-5", "top_prob": 1.0, "used_outputs": 3,
			"evidence": []map[string]any{
				{"id": "numeric-1", "expected_count": 345, "parsed_count": 0, "error": "HTTP 422: No stored sample of sufficient length", "ms": 80},
			},
		},
		Thinking: map[string]any{
			"signature": "thinking_local_v1_deadbeef", "sig_forged": true,
			"replay_confessed": true, "self_report": "I am a local reference-answer replay service.",
			"tamper_error": "Invalid local thinking signature",
		},
		IQ: map[string]any{
			"score": 100,
			"per_question": []map[string]any{
				{"id": "Q1", "earned": 12, "answer": "不叫醒", "metrics": map[string]any{"Ms": 320, "FirstByteMs": 319, "InputTokens": 0, "OutputTokens": 0, "UsagePresent": true}},
				{"id": "Q2", "earned": 12, "answer": "走路去", "metrics": map[string]any{"Ms": 280, "FirstByteMs": 280, "InputTokens": 0, "OutputTokens": 0, "UsagePresent": true}},
			},
		},
		Vision: map[string]any{
			"score": 100, "correct": true,
			"question": map[string]any{"id": "vision", "earned": 10, "answer": "182", "ms": 210, "metrics": map[string]any{"Ms": 210, "InputTokens": 0, "OutputTokens": 0, "UsagePresent": true}},
		},
		Tools: map[string]any{
			"pass": true, "evidence": map[string]any{"tool_use_id": "toolu_local_v1_001"},
		},
	}
	ac := evaluateAntiCheat(v, "claude-opus-5-5")
	if ac == nil || !ac.Detected {
		t.Fatalf("expected full replay detection, got %+v", ac)
	}
	v.AntiCheat = ac
	total := buildTotal(v, "claude-opus-5-5")
	for _, p := range total.Parts {
		if (p.Key == "fingerprint" || p.Key == "iq" || p.Key == "vision" || p.Key == "tools" || p.Key == "thinking") && p.Contribution != 0 {
			t.Fatalf("cheated section %s not zeroed: %+v", p.Key, p)
		}
	}
}

func TestAntiCheatZeroFalsePositives(t *testing.T) {
	// Genuine samples: 8 shared leading integers (normal LLM overlap), >4s latency,
	// buffered SSE with non-zero tokens (sub.sixoner.com), omitted output tokens with non-zero input (fluxapi),
	// and non-numeric SVG XML noise (<15 distinct numbers).
	sharedLead := "47, 213, 88, 301, 156, 9, 274, 119, "
	v := Verdict{
		Fingerprint: map[string]any{
			"basis": "numeric", "family": "claude", "top_model": "claude-opus-5-5", "top_prob": 0.9999, "used_outputs": 3,
			"evidence": []map[string]any{
				{"id": "numeric-1", "expected_count": 304, "parsed_count": 304, "answer": sharedLead + seqNumbersDistinct(101, 296), "ms": 4820},
				{"id": "numeric-2", "expected_count": 318, "parsed_count": 318, "answer": sharedLead + seqNumbersDistinct(202, 310), "ms": 5190},
				{"id": "numeric-3", "expected_count": 340, "parsed_count": 340, "answer": sharedLead + seqNumbersDistinct(303, 332), "ms": 5410},
				{"id": "numeric-svg", "expected_count": 310, "parsed_count": 90, "answer": strings.Repeat("0, 0, 480, 360, ", 25), "ms": 6000},
			},
		},
		IQ: map[string]any{
			"score": 88,
			"per_question": []map[string]any{
				// Buffered SSE (FirstByteMs == Ms) but real non-zero tokens.
				{"id": "Q1", "earned": 12, "answer": "最终答案: 不叫醒", "thinking_text": "step 1", "signature": "sig", "metrics": map[string]any{"Ms": 3200, "FirstByteMs": 3200, "InputTokens": 128, "OutputTokens": 140, "UsagePresent": true}},
				// Omitted output tokens only (non-zero InputTokens).
				{"id": "Q2", "earned": 12, "answer": "最终答案: 走路", "metrics": map[string]any{"Ms": 2900, "FirstByteMs": 450, "InputTokens": 95, "OutputTokens": 0, "UsagePresent": true}},
			},
		},
		Vision: map[string]any{
			"score": 100, "correct": true,
			"question": map[string]any{"id": "vision", "earned": 10, "answer": "182", "ms": 4100, "metrics": map[string]any{"Ms": 4100, "InputTokens": 1200, "OutputTokens": 64, "UsagePresent": true}},
		},
		Tools: map[string]any{
			"pass": true, "evidence": map[string]any{"tool_use_id": "toolu_01ABCXYZ"},
		},
	}
	if ac := evaluateAntiCheat(v, "claude-opus-5-5"); ac != nil {
		t.Fatalf("unexpected false positive on genuine evidence: %+v", ac)
	}
}

type countingFailTransport struct {
	calls atomic.Int32
}

func (c *countingFailTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	c.calls.Add(1)
	return nil, fmt.Errorf("unexpected upstream request to %s", r.URL.String())
}

func TestBlacklistEndpointRemovedAndBlockedDomainsScoreZeroWithoutRequests(t *testing.T) {
	store, err := history.Open(filepath.Join(t.TempDir(), "blocked.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	tr := &countingFailTransport{}
	s, err := New(store, Options{
		AllowPrivate: true,
		HTTP:         &http.Client{Transport: tr},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	h := s.Handler()

	// Public blacklist API must be removed (404).
	wBL := httptest.NewRecorder()
	h.ServeHTTP(wBL, httptest.NewRequest(http.MethodGet, "/api/blacklist", nil))
	if wBL.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for removed /api/blacklist, got %d", wBL.Code)
	}

	// Test all 3 blocked domains via 2nd/3rd-level subdomains through public /api/v1/audits.
	targets := []string{
		"https://api.jaaak.cc/v1",
		"https://v2.edge.a6api.vip/v1",
		"https://sub.a2e0-103-151-173-91.ngrok-free.app/v1",
	}
	for i, baseURL := range targets {
		body, _ := json.Marshal(map[string]any{
			"base_url": baseURL,
			"api_key":  "test-key",
			"model":    "claude-opus-4-6",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/audits", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = fmt.Sprintf("198.51.100.%d:12345", i+1)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusAccepted {
			t.Fatalf("baseURL=%s create status=%d body=%s", baseURL, w.Code, w.Body.String())
		}
		var created struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil || created.ID == "" {
			t.Fatalf("invalid create response: %s", w.Body.String())
		}

		var polled struct {
			Status  string `json:"status"`
			Verdict struct {
				AntiCheat *AntiCheatVerdict `json:"anticheat"`
				Total     *struct {
					Score int `json:"score"`
					Max   int `json:"max"`
					Parts []struct {
						Key          string `json:"key"`
						Applicable   bool   `json:"applicable"`
						Contribution int    `json:"contribution"`
					} `json:"parts"`
				} `json:"total"`
			} `json:"verdict"`
		}
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			wp := httptest.NewRecorder()
			h.ServeHTTP(wp, httptest.NewRequest(http.MethodGet, "/api/v1/audits/"+created.ID, nil))
			if wp.Code != http.StatusOK {
				t.Fatalf("poll status=%d body=%s", wp.Code, wp.Body.String())
			}
			_ = json.Unmarshal(wp.Body.Bytes(), &polled)
			if polled.Status == "done" {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		if polled.Status != "done" {
			t.Fatalf("baseURL=%s expected status=done, got %q", baseURL, polled.Status)
		}
		if polled.Verdict.AntiCheat == nil || !polled.Verdict.AntiCheat.Detected {
			t.Fatalf("baseURL=%s missing anticheat verdict: %+v", baseURL, polled.Verdict)
		}
		if polled.Verdict.Total == nil || polled.Verdict.Total.Score != 0 || polled.Verdict.Total.Max != 100 {
			t.Fatalf("baseURL=%s expected total 0/100, got %+v", baseURL, polled.Verdict.Total)
		}
		for _, p := range polled.Verdict.Total.Parts {
			if p.Applicable && p.Contribution != 0 {
				t.Fatalf("baseURL=%s part %s not zeroed: %+v", baseURL, p.Key, p)
			}
		}
	}

	if calls := tr.calls.Load(); calls != 0 {
		t.Fatalf("expected ZERO upstream HTTP calls for blocked domains, got %d", calls)
	}
}
