package inject

import (
	"chanzhushui/internal/gateway"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestMergedProbeUsesThreeRequests(t *testing.T) {
	var count atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		var q struct {
			Stream   bool
			Messages []struct{ Content string }
		}
		json.NewDecoder(r.Body).Decode(&q)
		if q.Stream {
			t.Error("unexpected extra stream request")
		}
		json.NewEncoder(w).Encode(map[string]any{"content": []any{}, "usage": map[string]int{"input_tokens": len(q.Messages[len(q.Messages)-1].Content) + 6}})
	}))
	defer s.Close()
	got, e := Measure(context.Background(), gateway.Client{BaseURL: s.URL, NoRetry: true}, "fixture")
	if e != nil || count.Load() != 3 || len(got.Points) != 2 || got.Inj != 6 {
		t.Fatalf("merged probe: calls=%d result=%+v err=%v", count.Load(), got, e)
	}
	raw, _ := json.Marshal(got)
	var fields map[string]any
	json.Unmarshal(raw, &fields)
	if fields["r2"] != nil || fields["max_residual"] != nil {
		t.Fatal("two samples must not claim linearity validation")
	}
}
func TestOneFailedSampleCannotScore(t *testing.T) {
	var count atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := count.Add(1)
		if n == 1 {
			w.WriteHeader(502)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"content": []any{}, "usage": map[string]int{"input_tokens": 20}})
	}))
	defer s.Close()
	got, e := Measure(context.Background(), gateway.Client{BaseURL: s.URL, NoRetry: true}, "fixture")
	if e == nil || got.Score != nil || count.Load() != 1 {
		t.Fatal(got, e, count.Load())
	}
}
