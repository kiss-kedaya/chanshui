package inject

import (
	"context"
	"encoding/json"
	"github.com/kiss-kedaya/chanshui/internal/gateway"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestMeasureUsesExactlyTwoIndependentRequests(t *testing.T) {
	var count atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		var q struct {
			Messages []struct{ Content string } `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&q)
		n := len(q.Messages[len(q.Messages)-1].Content)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"content": []any{map[string]string{"type": "text", "text": "OK"}}, "stop_reason": "end_turn", "usage": map[string]int{"input_tokens": n + 6, "output_tokens": 1}})
	}))
	defer s.Close()
	r, err := Measure(context.Background(), gateway.Client{BaseURL: s.URL, NoRetry: true}, "fixture")
	if err != nil || count.Load() != 2 || len(r.Points) != 2 || r.Inj != 6 {
		t.Fatalf("calls=%d result=%+v err=%v", count.Load(), r, err)
	}
}
