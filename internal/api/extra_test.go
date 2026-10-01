package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestJobRetryKeyCannotCancel(t *testing.T) {
	s, up, _, _ := testSetup(t)
	q := Request{BaseURL: up.URL, Key: "cancel-fixture", Model: "claude-opus-5", Concurrency: 1, Timeout: 10}
	w := post(t, s.Handler(), "/api/detect", q)
	if w.Code != 202 {
		t.Fatal(w.Body.String())
	}
	var started map[string]string
	json.Unmarshal(w.Body.Bytes(), &started)
	s.mu.Lock()
	var task *Task
	for _, v := range s.tasks {
		task = v
	}
	s.mu.Unlock()
	w = postToken(t, s.Handler(), "/api/detect/"+task.ID+"/cancel", map[string]any{}, started["control_token"])
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
	waitTask(t, s, task.ID)
	task.mu.Lock()
	status := task.Status
	task.mu.Unlock()
	if status != "done" {
		t.Fatal(status)
	}
}
func TestJSONSSEAndThinkingModes(t *testing.T) {
	s, up, _, _ := testSetup(t)
	for _, mode := range []any{nil, map[string]any{"type": "disabled"}, map[string]any{"type": "adaptive"}, map[string]any{"type": "adaptive", "display": "summarized"}, map[string]any{"type": "enabled", "budget_tokens": 2048}} {
		w := post(t, s.Handler(), "/api/ask", map[string]any{"base_url": up.URL, "key": "ephemeral-fixture", "model": "claude-opus-5", "prompt": "hi", "thinking": mode, "effort": "medium"})
		if w.Code != 200 || !strings.Contains(w.Body.String(), "Answer") {
			t.Fatal(w.Body.String())
		}
	}
	for _, payload := range []map[string]any{{"effort": "nonsense"}, {"thinking": map[string]any{"type": "invalid"}}, {"thinking": map[string]any{"type": "enabled", "budget_tokens": 1}}} {
		payload["base_url"] = up.URL
		payload["key"] = "fixture"
		payload["model"] = "claude-opus-5"
		payload["prompt"] = "hi"
		w := post(t, s.Handler(), "/api/ask", payload)
		if w.Code != 400 {
			t.Fatal(w.Body.String())
		}
	}
}
func TestLocalDefaultBlocksLoopback(t *testing.T) {
	s, up, _, _ := testSetup(t)
	s.opts = Options{}
	s.initTransport()
	g, e := s.client(Request{BaseURL: up.URL, Key: "fixture", Timeout: 1})
	if e != nil {
		t.Fatal(e)
	}
	r := httptest.NewRequest(http.MethodGet, up.URL, nil)
	r.RequestURI = ""
	start := time.Now()
	_, e = g.HTTP.Do(r)
	if e == nil {
		t.Fatal("private upstream permitted by default")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("private rejection not immediate")
	}
}
