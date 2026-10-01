package api

import (
	"net/http/httptest"
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
		request := httptest.NewRequest("GET", path, nil)
		request.Header.Set("If-None-Match", etag)
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
