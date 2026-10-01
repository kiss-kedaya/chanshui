package cachecheck

import (
	"chanzhushui/internal/gateway"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCacheEquation(t *testing.T) {
	for _, tc := range []struct {
		name                              string
		firstRead, firstWrite, secondRead int
		missing                           bool
		want                              string
	}{{"pass", 40, 960, 1000, false, "pass"}, {"mismatch", 40, 960, 900, false, "mismatch"}, {"zero", 0, 0, 0, false, "unknown"}, {"missing", 0, 0, 1000, true, "unknown"}} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			var previous map[string]any
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var q map[string]any
				json.NewDecoder(r.Body).Decode(&q)
				if calls == 1 {
					previous = q
				} else {
					a, _ := json.Marshal(previous)
					b, _ := json.Marshal(q)
					if string(a) != string(b) {
						t.Error("cache prefix changed")
					}
				}
				usage := map[string]any{"input_tokens": 10, "output_tokens": 1, "cache_read_input_tokens": tc.firstRead, "cache_creation_input_tokens": tc.firstWrite}
				if calls >= 2 {
					usage["cache_read_input_tokens"] = tc.secondRead
					usage["cache_creation_input_tokens"] = 0
				}
				if tc.missing {
					delete(usage, "cache_creation_input_tokens")
				}
				start, _ := json.Marshal(map[string]any{"type": "message_start", "message": map[string]any{"content": []any{}, "usage": usage}})
				w.Header().Set("Content-Type", "text/event-stream")
				w.Write([]byte("data: " + string(start) + "\n\ndata: {\"type\":\"message_stop\"}\n\n"))
			}))
			defer s.Close()
			r, e := Measure(context.Background(), gateway.Client{BaseURL: s.URL}, "claude-opus-5", 10)
			if e != nil || r.Status != tc.want || calls != 3 {
				t.Fatal(r, e, calls)
			}
			if r.Pass != nil && *r.Pass && r.Expected == 0 {
				t.Fatal("false pass")
			}
			if !strings.Contains(r.Reason, "缓存") {
				t.Fatal(r.Reason)
			}
		})
	}
}
func TestThreeRequestRateAndTransitions(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		read, write := 900, 0
		if calls == 1 {
			read, write = 0, 900
		}
		raw, _ := json.Marshal(map[string]any{"type": "message_start", "message": map[string]any{"content": []any{}, "usage": map[string]int{"input_tokens": 100, "cache_read_input_tokens": read, "cache_creation_input_tokens": write}}})
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: " + string(raw) + "\n\ndata: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer s.Close()
	r, e := Measure(context.Background(), gateway.Client{BaseURL: s.URL}, "claude-opus-5", 10)
	if e != nil || len(r.Samples) != 3 || len(r.Transitions) != 2 || r.ThirdReadRatio == nil || *r.ThirdReadRatio != 100 || r.TotalRead != 1800 || r.TotalOutput != 0 || r.Pass == nil || !*r.Pass {
		t.Fatal(r, e)
	}
	if r.PrefixHash == "" {
		t.Fatal("missing prefix identity")
	}
}
