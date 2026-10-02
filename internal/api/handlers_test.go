package api

import (
	"bytes"
	"chanzhushui/internal/gateway"
	"chanzhushui/internal/history"
	"chanzhushui/internal/iq"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testSetup(t *testing.T) (*Server, *httptest.Server, *atomic.Bool, *atomic.Int32) {
	t.Helper()
	fail := &atomic.Bool{}
	calls := &atomic.Int32{}
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path == "/v1/models" {
			fmt.Fprint(w, `{"data":[{"id":"claude-opus-5"},{"id":"unlisted-model"}]}`)
			return
		}
		var q struct {
			Messages []struct{ Content string }
			Thinking any
			Stream   bool
		}
		json.NewDecoder(r.Body).Decode(&q)
		prompt := q.Messages[0].Content
		if fail.Load() && strings.Contains(prompt, "医嘱") {
			w.WriteHeader(503)
			fmt.Fprint(w, `{"error":{"type":"busy","message":"busy"}}`)
			return
		}
		answer := "ok"
		input := 44 + int(.738*float64(len(prompt))+.5)
		if q.Thinking != nil {
			answer = "7417"
			switch {
			case strings.Contains(prompt, "医嘱"):
				answer = "最终答案: 不叫醒，暂缓给药"
			case strings.Contains(prompt, "亲生"):
				answer = "最终答案: 非亲生"
			case strings.Contains(prompt, "糖果"):
				var ca, ct, cm, sa, st, sm int
				fmt.Sscanf(prompt, "一盒糖果中，圆形的有苹果味 %d 颗、桃子味 %d 颗、西瓜味 %d 颗；星形的有苹果味 %d 颗、桃子味 %d 颗、西瓜味 %d 颗。", &ca, &ct, &cm, &sa, &st, &sm)
				answer = fmt.Sprintf("最终答案: %d", iq.SolveCandy([3]int{ca, ct, cm}, [3]int{sa, st, sm}))
			case strings.Contains(prompt, "袜子"):
				answer = "最终答案: 6"
			}
		}
		if strings.Contains(prompt, "1 到 355") {
			rng := rand.New(rand.NewSource(7))
			var b strings.Builder
			for i := 0; i < 320; i++ {
				fmt.Fprintf(&b, "%d,", 1+rng.Intn(355))
			}
			answer = b.String()
		}
		content := []any{map[string]string{"type": "text", "text": answer}}
		if q.Thinking != nil {
			content = append([]any{map[string]string{"type": "thinking", "thinking": "逐项推导并核验", "signature": "fixture-signature"}}, content...)
		}
		resp := map[string]any{"content": content, "usage": map[string]int{"input_tokens": input, "output_tokens": 50}, "stop_reason": "end_turn"}
		if q.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			start := map[string]any{"type": "message_start", "message": resp}
			b, _ := json.Marshal(start)
			fmt.Fprintf(w, "data: %s\n\ndata: {\"type\":\"message_stop\"}\n\n", b)
		} else {
			json.NewEncoder(w).Encode(resp)
		}
	}))
	t.Cleanup(up.Close)
	db, e := history.Open(":memory:")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Close() })
	s, e := New(db, Options{AllowPrivate: true, HTTP: up.Client()})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(s.Close)
	return s, up, fail, calls
}
func post(t *testing.T, s *Server, path string, v any) *httptest.ResponseRecorder {
	t.Helper()
	b, e := s.keys.seal(v)
	if e != nil {
		t.Fatal(e)
	}
	r := httptest.NewRequest("POST", path, bytes.NewReader(b))
	h := s.Handler()
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func waitTask(t *testing.T, s *Server, id string) *Task {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		q := s.find(id)
		q.mu.Lock()
		done := q.Status != "running" && q.Status != "queued"
		q.mu.Unlock()
		if done {
			return q
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("task hung")
	return nil
}
func TestDetectHistoryIsolationRetry(t *testing.T) {
	s, up, fail, calls := testSetup(t)
	fail.Store(true)
	q := Request{BaseURL: up.URL + "/v1", Key: "secret-never-persist", Model: "claude-opus-5", Timeout: 10}
	w := post(t, s, "/api/detect", q)
	if w.Code != 202 {
		t.Fatal(w.Body.String())
	}
	var d map[string]string
	json.Unmarshal(w.Body.Bytes(), &d)
	task := waitTask(t, s, d["task_id"])
	if task.Status != "partial" || task.HistoryID == 0 {
		t.Fatalf("bad status %s", task.Status)
	}
	for _, p := range task.Probes {
		if p.ID == "Q1" && p.Status != "error" {
			t.Fatal("failure not isolated")
		}
	}
	rec, e := s.store.Get(task.HistoryID)
	if e != nil || strings.Contains(string(rec.Verdict), q.Key) {
		t.Fatalf("persisted secret %v", e)
	}
	before := calls.Load()
	fail.Store(false)
	w = postToken(t, s, "/api/detect/"+task.ID+"/retry", q, d["control_token"])
	if w.Code != 202 {
		t.Fatal(w.Body.String())
	}
	waitTask(t, s, task.ID)
	if calls.Load()-before != 1 {
		t.Fatalf("retry repeated successful probes: %d", calls.Load()-before)
	}
	if task.Status != "done" {
		t.Fatal(task.Status)
	}
}
func TestValidationAndModels(t *testing.T) {
	s, up, _, calls := testSetup(t)
	for _, q := range []Request{{BaseURL: up.URL, Key: "secret", Model: "unknown"}, {BaseURL: "file:///tmp", Key: "secret", Model: "claude-opus-5"}, {BaseURL: up.URL + "?key=oops", Key: "secret", Model: "claude-opus-5"}} {
		if w := post(t, s, "/api/detect", q); w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid request called upstream")
	}
	w := post(t, s, "/api/models", Request{BaseURL: up.URL, Key: "secret"})
	if w.Code != 200 || strings.Contains(w.Body.String(), "unlisted-model") {
		t.Fatal(w.Body.String())
	}
	plain := httptest.NewRequest("POST", "/api/models", strings.NewReader(`{"base_url":"`+up.URL+`","key":"secret","model":"claude-opus-5","protocol":"auto","concurrency":0,"timeout":360,"cache":false}`))
	plain.Header.Set("Content-Type", "application/json")
	pw := httptest.NewRecorder()
	s.Handler().ServeHTTP(pw, plain)
	if pw.Code != 400 || !strings.Contains(pw.Body.String(), "加密") {
		t.Fatal(pw.Body.String())
	}
	r := httptest.NewRequest("POST", "/api/detect", strings.NewReader("{}"))
	r.Header.Set("Origin", "https://untrusted.invalid")
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("cross origin accepted")
	}
}
func TestRedactionAndCancelledTransport(t *testing.T) {
	b, _ := json.Marshal(sanitize(map[string]string{"answer": "hello secret"}, "secret"))
	if strings.Contains(string(b), "secret") {
		t.Fatal("secret exposed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tr := boundedTransport{base: http.DefaultTransport, sem: make(chan struct{}, 1)}
	tr.sem <- struct{}{}
	r, _ := http.NewRequestWithContext(ctx, "GET", "http://127.0.0.1", nil)
	if _, e := tr.RoundTrip(r); e == nil {
		t.Fatal("cancel ignored")
	}
	_ = gateway.DefaultUA
}

func postToken(t *testing.T, s *Server, path string, v any, token string) *httptest.ResponseRecorder {
	t.Helper()
	b, e := s.keys.seal(v)
	if e != nil {
		t.Fatal(e)
	}
	r := httptest.NewRequest("POST", path, bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Job-Token", token)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}
