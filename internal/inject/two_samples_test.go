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

func TestMergedProbeUsesExactlyThreeRequests(t *testing.T) {
	var count atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		chars, round := decodeConversation(t, r)
		writeUsage(w, chars/2+6+12*(round-1), 0, 0, 2)
	}))
	defer s.Close()
	got, err := Measure(context.Background(), gateway.Client{BaseURL: s.URL, NoRetry: true}, "fixture")
	if err != nil || count.Load() != 3 || len(got.Points) != 3 || got.Inj != 6 {
		t.Fatalf("calls=%d result=%+v err=%v", count.Load(), got, err)
	}
	raw, _ := json.Marshal(got)
	var fields map[string]any
	json.Unmarshal(raw, &fields)
	if fields["r2"] != nil || fields["max_residual"] != nil {
		t.Fatal("three-parameter fit must not claim independent validation")
	}
}
func TestOneFailedSampleCannotScore(t *testing.T) {
	var count atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { count.Add(1); w.WriteHeader(502) }))
	defer s.Close()
	got, err := Measure(context.Background(), gateway.Client{BaseURL: s.URL, NoRetry: true}, "fixture")
	if err == nil || got.Score != nil || count.Load() != 1 {
		t.Fatal(got, err, count.Load())
	}
}
