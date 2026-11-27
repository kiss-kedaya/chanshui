package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/kiss-kedaya/chanshui/internal/gateway"
	"github.com/kiss-kedaya/chanshui/internal/history"
	"github.com/kiss-kedaya/chanshui/internal/iq"
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
			System   []struct{ Text string }
			Messages []struct{ Content string }
			Thinking any
			Stream   bool
		}
		json.NewDecoder(r.Body).Decode(&q)
		prompt := ""
		if len(q.Messages) > 0 {
			prompt = q.Messages[0].Content
		}
		decoded, _ := iq.DecodePrompt(prompt)
		if fail.Load() && strings.Contains(decoded, "suspects") {
			w.WriteHeader(503)
			fmt.Fprint(w, `{"error":{"type":"busy","message":"busy"}}`)
			return
		}
		answer := "ok"
		input := 44 + int(.738*float64(len(prompt))+.5)
		if q.Thinking != nil {
			answer = "7417"
			switch {
			case strings.Contains(decoded, "suspects"):
				answer = fmt.Sprintf("最终答案: %d", solveMockTruthLiar(decoded))
			case strings.Contains(decoded, "X-linked"):
				answer = "最终答案: 1/6"
			case strings.Contains(decoded, "Circle:"):
				var ca, ct, cm, sa, st, sm int
				if i := strings.Index(decoded, "Circle:"); i >= 0 {
					fmt.Sscanf(decoded[i:], "Circle: %d apple, %d peach, %d melon.\nStar: %d apple, %d peach, %d melon.", &ca, &ct, &cm, &sa, &st, &sm)
					answer = fmt.Sprintf("最终答案: %d", iq.SolveCandy([3]int{ca, ct, cm}, [3]int{sa, st, sm}))
				}
			case strings.Contains(decoded, "same-color pairs"):
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
		usage := map[string]int{"input_tokens": input, "output_tokens": 50}
		if len(q.System) > 0 && strings.Contains(q.System[0].Text, "cache threshold probe") {
			if n := len(q.System[0].Text); n >= 1600 {
				usage = map[string]int{"input_tokens": 11, "output_tokens": 50, "cache_creation_input_tokens": n / 4}
			} else {
				usage = map[string]int{"input_tokens": n/4 + 11, "output_tokens": 50}
			}
		}
		resp := map[string]any{"content": content, "usage": usage, "stop_reason": "end_turn"}
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
	for _, q := range []Request{{BaseURL: up.URL, Key: "secret", Model: ""}, {BaseURL: "file:///tmp", Key: "secret", Model: "claude-opus-5"}, {BaseURL: up.URL + "?key=oops", Key: "secret", Model: "claude-opus-5"}} {
		if w := post(t, s, "/api/detect", q); w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid request called upstream")
	}
	w := post(t, s, "/api/models", Request{BaseURL: up.URL, Key: "secret"})
	if w.Code != 200 || !strings.Contains(w.Body.String(), "unlisted-model") {
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

// solveMockTruthLiar solves a decoded Q1 prompt by evaluating all 5 candidate
// thieves so retry-isolation tests get exact full marks in a single call.
func solveMockTruthLiar(decoded string) int {
	flat := strings.Join(strings.Fields(decoded), " ")
	var wantK int
	if i := strings.Index(flat, "exactly "); i >= 0 {
		fmt.Sscanf(flat[i:], "exactly %d tell", &wantK)
	}
	stmts := map[int]string{}
	// Statements are formatted as "1: ... 2: ... 3: ... 4: ... 5: ... Who is the thief?"
	for sp := 1; sp <= 5; sp++ {
		marker := fmt.Sprintf("%d: ", sp)
		idx := strings.Index(flat, marker)
		if idx < 0 {
			continue
		}
		rest := flat[idx+len(marker):]
		end := strings.IndexByte(rest, '.')
		if end >= 0 {
			stmts[sp] = strings.TrimSpace(rest[:end])
		}
	}
	for thief := 1; thief <= 5; thief++ {
		known := [6]bool{}
		val := [6]bool{}
		for pass := 0; pass < 6; pass++ {
			for sp := 1; sp <= 5; sp++ {
				if known[sp] {
					continue
				}
				s := stmts[sp]
				var a, b int
				switch {
				case strings.Contains(s, " or "):
					fmt.Sscanf(s, "%d or %d is the thief", &a, &b)
					val[sp], known[sp] = (thief == a || thief == b), true
				case strings.Contains(s, "is not the thief"):
					fmt.Sscanf(s, "%d is not the thief", &a)
					val[sp], known[sp] = (thief != a), true
				case strings.Contains(s, "is the thief"):
					fmt.Sscanf(s, "%d is the thief", &a)
					val[sp], known[sp] = (thief == a), true
				case strings.HasPrefix(s, "Both "):
					fmt.Sscanf(s, "Both %d and %d lie", &a, &b)
					if known[a] && known[b] {
						val[sp], known[sp] = (!val[a] && !val[b]), true
					}
				case strings.HasSuffix(s, " lies"):
					fmt.Sscanf(s, "%d lies", &a)
					if known[a] {
						val[sp], known[sp] = !val[a], true
					}
				case strings.HasSuffix(s, " tells truth"):
					fmt.Sscanf(s, "%d tells truth", &a)
					if known[a] {
						val[sp], known[sp] = val[a], true
					}
				}
			}
		}
		tc := 0
		for sp := 1; sp <= 5; sp++ {
			if val[sp] {
				tc++
			}
		}
		if tc == wantK {
			return thief
		}
	}
	return 1
}
