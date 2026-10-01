package api

import (
	"chanzhushui/internal/history"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type retryKeyGuardTransport struct {
	base  http.RoundTripper
	token string
	t     *testing.T
}

func (g retryKeyGuardTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Header.Get("X-Job-Token") != "" || strings.Contains(r.URL.String(), g.token) {
		g.t.Error("job capability was forwarded to the upstream")
	}
	for _, values := range r.Header {
		for _, value := range values {
			if strings.Contains(value, g.token) {
				g.t.Error("job capability leaked into an upstream header")
			}
		}
	}
	return g.base.RoundTrip(r)
}

func TestRetryCapabilityIsJobScopedAndExpiredKeysFail(t *testing.T) {
	s, up, _, calls := testSetup(t)
	id, _ := history.NewReportID()
	key, _ := history.NewReportID()
	otherKey, _ := history.NewReportID()
	task := &Task{ID: id, BaseURL: up.URL, Model: "claude-opus-5", Status: "partial", ExpiresAt: time.Now().Add(time.Hour).Unix(), controlHash: controlHash(key), Probes: []Probe{{ID: "Q1", Status: "error"}}}
	otherID, _ := history.NewReportID()
	other := &Task{ID: otherID, BaseURL: up.URL, Model: "claude-opus-5", Status: "partial", ExpiresAt: time.Now().Add(time.Hour).Unix(), controlHash: controlHash(otherKey), Probes: []Probe{{ID: "Q1", Status: "error"}}}
	s.mu.Lock()
	s.tasks[id] = task
	s.tasks[otherID] = other
	s.mu.Unlock()
	q := Request{BaseURL: up.URL, Model: task.Model, Key: "upstream-fixture"}
	for _, token := range []string{"", otherKey, key + "x"} {
		if w := postToken(t, s.Handler(), "/api/detect/"+id+"/retry", q, token); w.Code != 403 {
			t.Fatalf("unrelated or invalid key accepted: HTTP %d", w.Code)
		}
	}
	if w := postToken(t, s.Handler(), "/api/detect/"+otherID+"/retry", q, key); w.Code != 403 {
		t.Fatalf("key for another job accepted: HTTP %d", w.Code)
	}
	if w := post(t, s.Handler(), "/api/detect/"+id+"/retry?control_token="+key, q); w.Code != 403 {
		t.Fatal("key in URL accepted")
	}
	if w := postToken(t, s.Handler(), "/api/detect/"+id+"/cancel", map[string]any{}, key); w.Code != 403 {
		t.Fatal("retry capability permitted cancellation")
	}
	for _, path := range []string{"/api/detect/" + id, "/api/reports/" + id} {
		w := getJSON(t, s.Handler(), path)
		if w.Code != 200 || strings.Contains(w.Body.String(), key) || strings.Contains(w.Body.String(), task.controlHash) {
			t.Fatal("public read exposed retry capability")
		}
	}
	task.mu.Lock()
	task.ExpiresAt = time.Now().Add(-time.Second).Unix()
	task.mu.Unlock()
	if w := postToken(t, s.Handler(), "/api/detect/"+id+"/retry", q, key); w.Code != 410 {
		t.Fatalf("expired key accepted: HTTP %d", w.Code)
	}
	if calls.Load() != 0 {
		t.Fatal("denied retry issued an upstream request")
	}
}

func TestAllCompletedJobRevokesRetryCapability(t *testing.T) {
	s, up, _, calls := testSetup(t)
	id, _ := history.NewReportID()
	key, _ := history.NewReportID()
	task := &Task{ID: id, BaseURL: up.URL, Model: "claude-opus-5", Status: "done", ExpiresAt: time.Now().Add(time.Hour).Unix(), controlHash: controlHash(key), Probes: []Probe{{ID: "Q1", Status: "done"}}}
	_, err := s.store.CreateJob(context.Background(), history.Record{ReportID: id, Verdict: json.RawMessage(`{}`)}, json.RawMessage(`[]`), task.controlHash)
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.tasks[id] = task
	s.mu.Unlock()
	s.persist(task)
	if task.controlHash != "" {
		t.Fatal("completed memory task retained capability")
	}
	hash, err := s.store.ControlHash(id)
	if err != nil || hash != "" {
		t.Fatal("completed database job retained capability", err)
	}
	q := Request{BaseURL: up.URL, Key: "upstream-fixture", Model: task.Model}
	if w := postToken(t, s.Handler(), "/api/detect/"+id+"/retry", q, key); w.Code != 403 {
		t.Fatal("completed job accepted a revoked key")
	}
	if calls.Load() != 0 {
		t.Fatal("completed job retried successful probes")
	}
}

func TestRetryCannotChangeTargetOrForwardJobKey(t *testing.T) {
	s, up, fail, calls := testSetup(t)
	var foreignCalls atomic.Int32
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		foreignCalls.Add(1)
		w.WriteHeader(400)
	}))
	defer foreign.Close()
	fail.Store(true)
	q := Request{BaseURL: up.URL, Key: "upstream-fixture", Model: "claude-opus-5", Protocol: "anthropic", Timeout: 10}
	w := post(t, s.Handler(), "/api/detect", q)
	var created struct {
		ID    string `json:"task_id"`
		Token string `json:"control_token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil || created.ID == "" {
		t.Fatal("job not created", w.Body.String())
	}
	task := waitTask(t, s, created.ID)
	s.httpClient.Transport = retryKeyGuardTransport{s.httpClient.Transport, created.Token, t}
	fail.Store(false)
	q.BaseURL, q.Model, q.Protocol = foreign.URL, "gemini-3.1-pro-preview", "chat"
	before := calls.Load()
	w = postToken(t, s.Handler(), "/api/detect/"+task.ID+"/retry", q, created.Token)
	if w.Code != 202 {
		t.Fatal(w.Body.String())
	}
	finished := waitTask(t, s, task.ID)
	if finished.BaseURL != up.URL || finished.Model != "claude-opus-5" || finished.Verdict.Protocol != "anthropic" {
		t.Fatal("retry replaced the original target")
	}
	if foreignCalls.Load() != 0 || calls.Load()-before != 2 {
		t.Fatal("retry reached a substituted target or repeated successful probes")
	}
}
