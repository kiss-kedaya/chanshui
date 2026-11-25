package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestPreflightAbortsOnAuthError(t *testing.T) {
	s, _, _, _ := testSetup(t)
	var calls atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(403)
		fmt.Fprint(w, "IP not allowed")
	}))
	defer up.Close()
	ctx, cancel := context.WithCancel(context.Background())
	task := &Task{Source: "api", Status: "running", Verdict: Verdict{}, Probes: []Probe{{ID: "fingerprint", Status: "queued"}, {ID: "Q1", Status: "queued"}}, cancel: cancel}
	s.run(ctx, task, Request{Model: "claude-opus-5", BaseURL: up.URL, Timeout: 10}, nil)
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1", calls.Load())
	}
	if task.Status != "partial" {
		t.Fatalf("status = %s", task.Status)
	}
	for _, probe := range task.Probes {
		if probe.Status != "error" || !strings.Contains(probe.Error, "整单失败") {
			t.Fatalf("probe = %+v", probe)
		}
	}
}

func TestPreflightAbortsOnUnknownModel(t *testing.T) {
	s, _, _, _ := testSetup(t)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		fmt.Fprint(w, "{\"type\":\"error\",\"error\":{\"type\":\"not_found_error\",\"message\":\"model not found\"}}")
	}))
	defer up.Close()
	ctx, cancel := context.WithCancel(context.Background())
	task := &Task{Source: "api", Status: "running", Verdict: Verdict{}, Probes: []Probe{{ID: "fingerprint", Status: "queued"}}, cancel: cancel}
	s.run(ctx, task, Request{Model: "claude-opus-5.5", BaseURL: up.URL, Timeout: 10}, nil)
	if task.Status != "partial" || !strings.Contains(task.Probes[0].Error, "整单失败") {
		t.Fatalf("task = %+v", task.Probes)
	}
}
