package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kiss-kedaya/chanshui/internal/history"
	"github.com/kiss-kedaya/chanshui/internal/iq"
)

// One thread has to keep the same first user block as its routing anchor while
// the history grows, otherwise a classifier gateway re-routes mid-session and
// the cache claim means nothing.
func TestConversationKeepsAnchorAndCountsTurns(t *testing.T) {
	type shape struct {
		messages int
		anchor   bool
	}
	var mu sync.Mutex
	var shapes []shape
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			System []struct {
				Text         string          `json:"text"`
				CacheControl json.RawMessage `json:"cache_control"`
			} `json:"system"`
			Messages []struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		if e := json.NewDecoder(r.Body).Decode(&body); e != nil {
			t.Errorf("decode request: %v", e)
		}
		// Self-grading uses its own two-message thread; only the chat thread
		// under test carries the conversation system block.
		conv := false
		for _, block := range body.System {
			if block.Text == convSystem {
				conv = true
			}
		}
		marked := false
		for _, msg := range body.Messages {
			if msg.Role == "user" {
				marked = strings.Contains(string(msg.Content), "cache_control")
				break
			}
		}
		if conv {
			mu.Lock()
			shapes = append(shapes, shape{messages: len(body.Messages), anchor: marked && len(body.System) > 0 && len(body.System[0].CacheControl) > 0})
			mu.Unlock()
		}
		json.NewEncoder(w).Encode(map[string]any{
			"content":     []any{map[string]string{"type": "text", "text": "最终答案: 开车去"}},
			"usage":       map[string]int{"input_tokens": 200, "output_tokens": 5, "cache_read_input_tokens": 100},
			"stop_reason": "end_turn",
		})
	}))
	defer up.Close()

	db, e := history.Open(":memory:")
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	s, e := New(db, Options{AllowPrivate: true, HTTP: up.Client()})
	if e != nil {
		t.Fatal(e)
	}
	g, _ := s.client(Request{Model: "claude-opus-5", BaseURL: up.URL, Timeout: 10})

	task := &Task{ID: "conv", ReportID: "conv", Status: "running", Model: "claude-opus-5", CreatedAt: time.Now().UTC().Format(time.RFC3339), answers: map[string]QuestionResult{}}
	for i := 1; i <= 6; i++ {
		id := "Q" + string(rune('0'+i))
		task.questions = append(task.questions, iq.Question{ID: id, Title: id, Prompt: "题目 " + id, MaxScore: 5, Answer: "开车去"})
		task.Probes = append(task.Probes, Probe{ID: id, Name: id, Status: "queued"})
	}
	s.converse(context.Background(), task, Request{Model: "claude-opus-5", BaseURL: up.URL, Timeout: 10}, g)

	report, ok := task.Verdict.Conversation.(map[string]any)
	if !ok {
		t.Fatalf("conversation report missing: %#v", task.Verdict.Conversation)
	}
	turns, _ := report["turns"].([]any)
	// opener + six questions + two small talks after Q3 and after Q6
	if len(turns) != 11 {
		t.Fatalf("turns = %d, want 11", len(turns))
	}
	if report["status"] != "pass" || report["drops"].(float64) != 0 {
		t.Fatalf("status = %v, drops = %v", report["status"], report["drops"])
	}
	mu.Lock()
	defer mu.Unlock()
	if len(shapes) != 11 {
		t.Fatalf("requests = %d, want 11", len(shapes))
	}
	for i, got := range shapes {
		if got.messages != i*2+1 {
			t.Fatalf("turn %d carries %d messages, want %d", i+1, got.messages, i*2+1)
		}
		if !got.anchor {
			t.Fatalf("turn %d lost the permanent anchor", i+1)
		}
	}
	if len(task.answers) != 6 {
		t.Fatalf("answers = %d, want 6", len(task.answers))
	}
}

// A read that falls to zero after the thread had cache is the signal the
// gateway changed upstream accounts mid-session.
func TestConversationFlagsMidThreadCacheDrop(t *testing.T) {
	var mu sync.Mutex
	convCalls := 0
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			System []struct {
				Text string `json:"text"`
			} `json:"system"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		usage := map[string]int{"input_tokens": 200, "output_tokens": 5}
		if len(body.System) > 0 && body.System[0].Text == convSystem {
			mu.Lock()
			convCalls++
			n := convCalls
			mu.Unlock()
			switch n {
			case 1, 2, 4:
				usage["cache_read_input_tokens"] = 1500
			case 3:
				usage["cache_read_input_tokens"] = 0
				usage["cache_creation_input_tokens"] = 1500
			}
		}
		json.NewEncoder(w).Encode(map[string]any{
			"content":     []any{map[string]string{"type": "text", "text": "最终答案: 开车去"}},
			"usage":       usage,
			"stop_reason": "end_turn",
		})
	}))
	defer up.Close()

	db, e := history.Open(":memory:")
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	s, e := New(db, Options{AllowPrivate: true, HTTP: up.Client()})
	if e != nil {
		t.Fatal(e)
	}
	g, _ := s.client(Request{Model: "claude-opus-5", BaseURL: up.URL, Timeout: 10})

	task := &Task{ID: "drop", ReportID: "drop", Status: "running", Model: "claude-opus-5", CreatedAt: time.Now().UTC().Format(time.RFC3339), answers: map[string]QuestionResult{}}
	for i := 1; i <= 3; i++ {
		id := "Q" + string(rune('0'+i))
		task.questions = append(task.questions, iq.Question{ID: id, Title: id, Prompt: "题目 " + id, MaxScore: 5, Answer: "开车去"})
	}
	s.converse(context.Background(), task, Request{Model: "claude-opus-5", BaseURL: up.URL, Timeout: 10}, g)

	report, ok := task.Verdict.Conversation.(map[string]any)
	if !ok {
		t.Fatalf("conversation report missing: %#v", task.Verdict.Conversation)
	}
	if report["status"] != "mismatch" || report["drops"].(float64) != 1 {
		t.Fatalf("status = %v, drops = %v", report["status"], report["drops"])
	}
}

// A flat read while every turn writes the growing conversation is a broken
// chain: only the shared system prefix hits, so the cache card and the
// conversation strip must both say it did not connect.
func TestConversationFlagsStalledReadGrowth(t *testing.T) {
	var mu sync.Mutex
	convCalls := 0
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			System []struct {
				Text string `json:"text"`
			} `json:"system"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		usage := map[string]int{"input_tokens": 2, "output_tokens": 4}
		if len(body.System) > 0 && body.System[0].Text == convSystem {
			mu.Lock()
			convCalls++
			n := convCalls
			mu.Unlock()
			usage["cache_read_input_tokens"] = 78
			if n == 1 {
				usage["cache_read_input_tokens"] = 0
			}
			if n >= 2 {
				usage["cache_creation_input_tokens"] = 700 + n*100
			}
		}
		json.NewEncoder(w).Encode(map[string]any{
			"content":     []any{map[string]string{"type": "text", "text": "最终答案: 开车去"}},
			"usage":       usage,
			"stop_reason": "end_turn",
		})
	}))
	defer up.Close()

	db, e := history.Open(":memory:")
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	s, e := New(db, Options{AllowPrivate: true, HTTP: up.Client()})
	if e != nil {
		t.Fatal(e)
	}
	g, _ := s.client(Request{Model: "claude-opus-5", BaseURL: up.URL, Timeout: 10})

	task := &Task{ID: "stall", ReportID: "stall", Status: "running", Model: "claude-opus-5", CreatedAt: time.Now().UTC().Format(time.RFC3339), answers: map[string]QuestionResult{}}
	for i := 1; i <= 6; i++ {
		id := "Q" + string(rune('0'+i))
		task.questions = append(task.questions, iq.Question{ID: id, Title: id, Prompt: "题目 " + id, MaxScore: 5, Answer: "开车去"})
	}
	s.converse(context.Background(), task, Request{Model: "claude-opus-5", BaseURL: up.URL, Timeout: 10}, g)

	report, ok := task.Verdict.Conversation.(map[string]any)
	if !ok {
		t.Fatalf("conversation report missing: %#v", task.Verdict.Conversation)
	}
	if report["status"] != "mismatch" || report["drops"].(float64) < 8 || report["hits"].(float64) != 0 {
		t.Fatalf("status = %v, drops = %v, hits = %v", report["status"], report["drops"], report["hits"])
	}
	cache, ok := task.Verdict.Cache.(map[string]any)
	if !ok || cache["pass"] != false {
		t.Fatalf("cache verdict not reused: %#v", task.Verdict.Cache)
	}
	if samples, _ := cache["samples"].([]any); len(samples) != 11 {
		t.Fatalf("cache samples = %d, want 11", len(samples))
	}
	if ratio, _ := cache["third_read_ratio"].(float64); ratio <= 0 || ratio >= 10 {
		t.Fatalf("last read ratio = %v, want a stalled chain under 10%%", cache["third_read_ratio"])
	}
	if transitions, _ := cache["transitions"].([]any); len(transitions) != 10 {
		t.Fatalf("transitions = %d, want 10", len(transitions))
	}
}
