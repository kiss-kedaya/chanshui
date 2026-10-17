package api

import (
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestPublicReportValidatorsRespectChangeAndExpiry(t *testing.T) {
	s, _, _, _ := testSetup(t)
	id := strings.Repeat("ab", 16)
	task := &Task{ID: id, ReportID: id, Status: "running", ExpiresAt: time.Now().Add(time.Hour).Unix()}
	s.mu.Lock()
	s.tasks[id] = task
	s.mu.Unlock()
	for _, path := range []string{"/api/detect/" + id, "/api/reports/" + id} {
		first := getJSON(t, s.Handler(), path)
		etag := first.Header().Get("ETag")
		if first.Code != 200 || etag == "" {
			t.Fatalf("missing validator: %s", first.Body.String())
		}
		if got, want := first.Header().Get("Content-Length"), strconv.Itoa(first.Body.Len()); got != want {
			t.Fatalf("content length = %q, want %q", got, want)
		}
		request := httptest.NewRequest("GET", path, nil)
		// SafeLine weakens the validator on the public path. Accept both the
		// strong and weak forms so a conditional read can still become a 304.
		request.Header.Set("If-None-Match", "W/"+etag)
		unchanged := httptest.NewRecorder()
		s.Handler().ServeHTTP(unchanged, request)
		if unchanged.Code != 304 || unchanged.Body.Len() != 0 || unchanged.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("unchanged report resent")
		}
		task.mu.Lock()
		task.Warning += "updated"
		task.Probes = append(task.Probes, Probe{ID: path, Status: "done"})
		task.mu.Unlock()
		changed := httptest.NewRecorder()
		s.Handler().ServeHTTP(changed, request)
		if changed.Code != 200 || changed.Header().Get("ETag") == etag {
			t.Fatal("changed report suppressed")
		}
	}
	task.mu.Lock()
	task.ExpiresAt = time.Now().Unix() - 1
	task.mu.Unlock()
	if w := getJSON(t, s.Handler(), "/api/detect/"+id); w.Code != 410 {
		t.Fatal("expired report served")
	}
}

func TestReportCacheHeadersFollowCompletionAndRetryability(t *testing.T) {
	s, _, _, _ := testSetup(t)
	id := strings.Repeat("cd", 16)
	task := &Task{ID: id, ReportID: id, Status: "done", ExpiresAt: time.Now().Add(time.Hour).Unix()}
	s.mu.Lock()
	s.tasks[id] = task
	s.mu.Unlock()

	first := getJSON(t, s.Handler(), "/api/reports/"+id)
	if first.Code != 200 {
		t.Fatalf("completed report HTTP %d: %s", first.Code, first.Body.String())
	}
	if got := first.Header().Get("Cache-Control"); got != "private, max-age=300, stale-while-revalidate=60" {
		t.Fatalf("completed report cache = %q", got)
	}
	if got := first.Header().Get("Vary"); got != "X-Job-Token, X-Visitor-ID" {
		t.Fatalf("completed report vary = %q", got)
	}

	task.Probes = []Probe{{ID: "Q1", Status: "error"}}
	retryable := getJSON(t, s.Handler(), "/api/reports/"+id)
	if got := retryable.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("retryable report cache = %q", got)
	}
}
