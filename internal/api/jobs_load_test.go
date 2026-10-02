package api

import (
	"chanzhushui/internal/history"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBoundedQueueAndGlobalRequests(t *testing.T) {
	var inFlight, peak atomic.Int32
	entered := make(chan struct{}, 20)
	release := make(chan struct{})
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := inFlight.Add(1)
		defer inFlight.Add(-1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		select {
		case entered <- struct{}{}:
		default:
		}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"content": []any{}, "usage": map[string]int{"input_tokens": 10}})
	}))
	defer up.Close()
	db, e := history.Open(filepath.Join(t.TempDir(), "load.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	s, e := New(db, Options{AllowPrivate: true, Workers: 1, QueueSize: 2, MaxRequests: 2})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	q := Request{BaseURL: up.URL, Key: "fixture", Model: "claude-opus-5", Timeout: 10}
	first := post(t, s, "/api/detect", q)
	if first.Code != 202 {
		t.Fatal(first.Body.String())
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("worker not started")
	}
	accepted, rejected := atomic.Int32{}, atomic.Int32{}
	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := post(t, s, "/api/detect", q)
			switch w.Code {
			case 202:
				accepted.Add(1)
				var d map[string]any
				json.Unmarshal(w.Body.Bytes(), &d)
				if d["task_id"] != d["report_id"] {
					t.Error("job/share ID mismatch")
				}
				if r := getJSON(t, s.Handler(), "/api/reports/"+d["report_id"].(string)); r.Code != 200 {
					t.Error("queued report inaccessible")
				}
			case 429:
				rejected.Add(1)
			default:
				t.Errorf("status %d", w.Code)
			}
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)
	if accepted.Load() != 2 || rejected.Load() != 38 || peak.Load() > 2 {
		t.Errorf("admission accepted=%d rejected=%d peak=%d", accepted.Load(), rejected.Load(), peak.Load())
	}
	// Stop all requests rather than waiting for fake streaming retries.
	s.stop()
	close(release)
	t.Logf("40 simultaneous submissions: accepted=%d queue-rejected=%d elapsed=%s peak upstream=%d", accepted.Load(), rejected.Load(), elapsed, peak.Load())
}
func TestNonClaudeJobHasNoThinkingProbe(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q map[string]any
		json.NewDecoder(r.Body).Decode(&q)
		if q["thinking"] != nil {
			t.Error("thinking leaked")
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"42"},"finish_reason":"stop"}],"usage":{"prompt_tokens":120,"completion_tokens":2}}`)
	}))
	defer up.Close()
	db, _ := history.Open(":memory:")
	defer db.Close()
	s, e := New(db, Options{AllowPrivate: true, HTTP: up.Client()})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	w := post(t, s, "/api/detect", Request{BaseURL: up.URL, Key: "fixture", Model: "gemini-3.1-pro-preview", Timeout: 10})
	var d map[string]any
	json.Unmarshal(w.Body.Bytes(), &d)
	task := waitTask(t, s, d["task_id"].(string))
	task.mu.Lock()
	defer task.mu.Unlock()
	if task.Verdict.Thinking != nil {
		t.Fatal("non-Claude thinking not null")
	}
	for _, p := range task.Probes {
		if p.ID == "thinking" {
			t.Fatal("thinking was scheduled")
		}
	}
	_ = context.Background()
}
func TestRetryAfterMemoryEvictionKeepsJobID(t *testing.T) {
	s, up, fail, calls := testSetup(t)
	fail.Store(true)
	q := Request{BaseURL: up.URL, Key: "fixture", Model: "claude-opus-5", Timeout: 10}
	w := post(t, s, "/api/detect", q)
	var created map[string]any
	json.Unmarshal(w.Body.Bytes(), &created)
	id := created["job_id"].(string)
	task := waitTask(t, s, id)
	task.persistMu.Lock()
	task.persistMu.Unlock()
	s.mu.Lock()
	delete(s.tasks, id)
	s.mu.Unlock()
	fail.Store(false)
	before := calls.Load()
	w = postToken(t, s, "/api/detect/"+id+"/retry", q, created["control_token"].(string))
	if w.Code != 202 {
		t.Fatal(w.Body.String())
	}
	result := waitTask(t, s, id)
	if result.ID != id || calls.Load()-before != 1 {
		t.Fatal("rehydrated job changed ID or repeated successful probes")
	}
}
func BenchmarkSharedReportRead(b *testing.B) {
	db, e := history.Open(filepath.Join(b.TempDir(), "read.db"))
	if e != nil {
		b.Fatal(e)
	}
	defer db.Close()
	s, e := New(db, Options{})
	if e != nil {
		b.Fatal(e)
	}
	defer s.Close()
	reportID, _ := history.NewReportID()
	if _, e = db.Save(history.Record{ReportID: reportID, CreatedAt: time.Now().UTC().Format(time.RFC3339), BaseURL: "https://fixture.invalid", Model: "gpt-6-sol", Verdict: json.RawMessage("{}")}); e != nil {
		b.Fatal(e)
	}
	h := s.Handler()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", "/api/reports/"+reportID, nil))
			if w.Code != 200 {
				b.Errorf("report read HTTP%d", w.Code)
			}
		}
	})
}
