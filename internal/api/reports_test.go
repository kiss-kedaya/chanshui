package api

import (
	"chanzhushui/internal/history"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func getJSON(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
	return w
}
func TestSavedReportShareIndependentFromTask(t *testing.T) {
	s, up, fail, calls := testSetup(t)
	fail.Store(true)
	q := Request{BaseURL: up.URL + "/v1", Key: "do-not-share-this-key", Model: "claude-opus-5", Timeout: 10}
	w := post(t, s.Handler(), "/api/detect", q)
	var started map[string]string
	json.Unmarshal(w.Body.Bytes(), &started)
	task := waitTask(t, s, started["task_id"])
	oldID := task.ReportID
	if !history.ValidReportID(oldID) || oldID != task.ID {
		t.Fatal("public ID missing or shares mutation task ID")
	}
	response := getJSON(t, s.Handler(), "/api/reports/"+oldID)
	if response.Code != 200 || strings.Contains(response.Body.String(), q.Key) {
		t.Fatal("cannot anonymously read sanitized report", response.Code)
	}
	var detail struct {
		ReportID string `json:"report_id"`
		Host     string `json:"host"`
	}
	json.Unmarshal(response.Body.Bytes(), &detail)
	if detail.Host != strings.TrimPrefix(up.URL, "http://") || detail.ReportID != oldID {
		t.Fatal(detail)
	}
	fail.Store(false)
	before := calls.Load()
	postToken(t, s.Handler(), "/api/detect/"+task.ID+"/retry", q, started["control_token"])
	waitTask(t, s, task.ID)
	if task.ReportID != oldID || calls.Load()-before != 1 {
		t.Fatal("retry replaced shared snapshot or repeated successful probes")
	}
	if getJSON(t, s.Handler(), "/api/reports/"+oldID).Code != 200 {
		t.Fatal("job URL changed on retry")
	}
	restarted, e := New(s.store, Options{})
	if e != nil {
		t.Fatal(e)
	}
	defer restarted.Close()
	if restarted.find(task.ID) != nil {
		t.Fatal("test server unexpectedly has memory task")
	}
	if r := getJSON(t, restarted.Handler(), "/api/reports/"+task.ReportID); r.Code != 200 {
		t.Fatal("persistent report cannot survive task eviction", r.Body.String())
	}
	list := getJSON(t, restarted.Handler(), "/api/history?limit=1")
	var entries []history.Summary
	json.Unmarshal(list.Body.Bytes(), &entries)
	if len(entries) != 1 || entries[0].ReportID != task.ReportID {
		t.Fatal("global history missing newest report")
	}
	// Public IDs do not allow someone to cancel or retry the private task.
	if r := post(t, restarted.Handler(), "/api/detect/"+oldID+"/cancel", map[string]any{}); r.Code != 404 {
		t.Fatal("public ID permitted mutation")
	}
}
func TestReportIDsAndHostValidation(t *testing.T) {
	s, _, _, _ := testSetup(t)
	for _, c := range []struct {
		path   string
		status int
	}{{"/api/reports/nope", 400}, {"/api/reports/00000000000000000000000000000000", 404}, {"/api/history?before=-1", 400}, {"/api/history?before=bad", 400}} {
		w := getJSON(t, s.Handler(), c.path)
		if w.Code != c.status {
			t.Errorf("%s: %d", c.path, w.Code)
		}
	}
	for _, c := range []struct{ base, want string }{{"https://gateway.example/v1", "gateway.example"}, {"https://gateway.example:8443/prefix/v1", "gateway.example:8443"}, {"http://[::1]:8770/v1", "[::1]:8770"}} {
		if got := displayHost(c.base); got != c.want {
			t.Errorf("host %s: %s", c.base, got)
		}
	}
}
