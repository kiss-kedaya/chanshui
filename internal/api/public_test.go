package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func postPublic(s *Server, body, addr string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/audits", strings.NewReader(body))
	r.RemoteAddr = addr
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}

func TestPublicAuditSectionsRateAndHistory(t *testing.T) {
	s, up, _, _ := testSetup(t)
	body := `{"base_url":"` + up.URL + `","api_key":"secret-never-persist","model":"claude-opus-5","timeout":10,"sections":["injection"]}`
	first := postPublic(s, body, "203.0.113.10:1234")
	if first.Code != 202 {
		t.Fatalf("create %d %s", first.Code, first.Body.String())
	}
	var created struct {
		ID       string   `json:"id"`
		Sections []string `json:"sections"`
		PollURL  string   `json:"poll_url"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(first.Body.String(), "secret-never-persist") || created.PollURL != "/api/v1/audits/"+created.ID {
		t.Fatalf("response leaked or poll url wrong: %s", first.Body.String())
	}
	if len(created.Sections) != 1 || created.Sections[0] != "injection" {
		t.Fatalf("sections %+v", created.Sections)
	}
	task := s.find(created.ID)
	task.mu.Lock()
	ids := make([]string, 0, len(task.Probes))
	for _, probe := range task.Probes {
		ids = append(ids, probe.ID)
	}
	source := task.Source
	task.mu.Unlock()
	if source != "api" || strings.Join(ids, ",") != "injection" {
		t.Fatalf("task source %s probes %v", source, ids)
	}
	items, err := s.store.ListBefore(20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("api audit entered history: %+v", items)
	}
	for _, path := range []string{"/api/reports/" + created.ID, "/api/detect/" + created.ID} {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != 404 {
			t.Fatalf("%s -> %d %s", path, w.Code, w.Body.String())
		}
	}
	poll := httptest.NewRequest(http.MethodGet, created.PollURL, nil)
	pw := httptest.NewRecorder()
	s.Handler().ServeHTTP(pw, poll)
	if pw.Code != 200 || !strings.Contains(pw.Body.String(), `"model":"claude-opus-5"`) {
		t.Fatalf("poll %d %s", pw.Code, pw.Body.String())
	}
	countReq := httptest.NewRequest(http.MethodGet, "/api/v1", nil)
	cw := httptest.NewRecorder()
	s.Handler().ServeHTTP(cw, countReq)
	if cw.Code != 200 || !strings.Contains(cw.Body.String(), `"public_audits":1`) {
		t.Fatalf("count %d %s", cw.Code, cw.Body.String())
	}
	spec := httptest.NewRequest(http.MethodGet, "/api/v1/openapi.json", nil)
	sw := httptest.NewRecorder()
	s.Handler().ServeHTTP(sw, spec)
	if sw.Code != 200 || !strings.Contains(sw.Body.String(), `"openapi":"3.1.0"`) || !strings.Contains(sw.Body.String(), "/api/v1/audits/{id}") {
		t.Fatalf("openapi %d %s", sw.Code, sw.Body.String())
	}
	second := postPublic(s, body, "203.0.113.10:4321")
	if second.Code != 429 || second.Header().Get("Retry-After") == "" {
		t.Fatalf("rate %d %s retry %q", second.Code, second.Body.String(), second.Header().Get("Retry-After"))
	}
	pollAgain := httptest.NewRequest(http.MethodGet, created.PollURL, nil)
	pollAgain.RemoteAddr = "203.0.113.10:4321"
	pw = httptest.NewRecorder()
	s.Handler().ServeHTTP(pw, pollAgain)
	if pw.Code != 200 {
		t.Fatalf("poll counted against rate limit: %d", pw.Code)
	}
	denied := postPublic(s, `{"base_url":"`+up.URL+`","api_key":"secret-never-persist","model":"gpt-5.4","timeout":10,"sections":["thinking"]}`, "203.0.113.20:1")
	if denied.Code != 400 {
		t.Fatalf("thinking-only non-claude %d %s", denied.Code, denied.Body.String())
	}
	skipped := postPublic(s, `{"base_url":"`+up.URL+`","api_key":"secret-never-persist","model":"gpt-5.4","timeout":10,"sections":"all"}`, "203.0.113.20:1")
	var all struct {
		Sections []string `json:"sections"`
		Skipped  []struct {
			ID string `json:"id"`
		} `json:"skipped"`
	}
	if err := json.Unmarshal(skipped.Body.Bytes(), &all); err != nil {
		t.Fatal(err, skipped.Body.String())
	}
	if skipped.Code != 202 {
		t.Fatalf("all %d %s", skipped.Code, skipped.Body.String())
	}
	for _, id := range all.Sections {
		if id == "thinking" {
			t.Fatalf("non-claude all included thinking: %+v", all)
		}
	}
	foundSkip := false
	for _, item := range all.Skipped {
		if item.ID == "thinking" {
			foundSkip = true
		}
	}
	if !foundSkip || !containsSection(all.Sections, "cache") || !containsSection(all.Sections, "iq") {
		t.Fatalf("all sections %+v skipped %+v", all.Sections, all.Skipped)
	}
	count, err := s.store.PublicAuditCount(context.Background())
	if err != nil || count != 2 {
		t.Fatalf("count %d %v", count, err)
	}
}

func TestBuildTotalSelectedSectionOnly(t *testing.T) {
	total := buildTotalFor(Verdict{Injection: map[string]any{"score": 10}}, "claude-opus-4-6", map[string]bool{"injection": true})
	if total.Score != 100 {
		t.Fatalf("selected injection score %d %+v", total.Score, total.Parts)
	}
	for _, part := range total.Parts {
		if part.Key != "injection" && part.Applicable {
			t.Fatalf("unrequested section counted %+v", part)
		}
	}
}
