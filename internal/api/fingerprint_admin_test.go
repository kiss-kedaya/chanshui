package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kiss-kedaya/chanshui/internal/fingerprint"
)

func adminRequest(t *testing.T, s *Server, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == nil {
		r = httptest.NewRequest(method, path, nil)
	} else {
		encoded, err := s.keys.seal(body)
		if err != nil {
			t.Fatal(err)
		}
		r = httptest.NewRequest(method, path, strings.NewReader(string(encoded)))
		r.Header.Set("Content-Type", "application/json")
	}
	r.Header.Set("Authorization", "Bearer "+s.adminToken)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}

func TestAdminFingerprintsRequiresAuthAndStartsEmpty(t *testing.T) {
	s, _, _, _ := testSetup(t)
	s.adminToken = "fixture-admin-token-0123456789"

	anonymous := getJSON(t, s.Handler(), "/api/admin/fingerprints")
	if anonymous.Code != 401 {
		t.Fatalf("anonymous admin fingerprints HTTP %d", anonymous.Code)
	}
	reply := adminRequest(t, s, http.MethodGet, "/api/admin/fingerprints", nil)
	if reply.Code != 200 || !strings.Contains(reply.Body.String(), `"items":[]`) {
		t.Fatalf("admin fingerprints HTTP %d: %s", reply.Code, reply.Body.String())
	}
}

func TestAdminCaptureRejectsInvalidAndOfficialConflicts(t *testing.T) {
	s, up, _, _ := testSetup(t)
	s.adminToken = "fixture-admin-token-0123456789"

	bad := adminRequest(t, s, http.MethodPost, "/api/admin/fingerprints/capture", map[string]any{
		"base_url": up.URL, "key": "fixture", "model": "custom-model",
		"id": "custom-model", "train": 5, "holdout": 3,
	})
	if bad.Code != 400 {
		t.Fatalf("invalid sample count HTTP %d: %s", bad.Code, bad.Body.String())
	}
	conflict := adminRequest(t, s, http.MethodPost, "/api/admin/fingerprints/capture", map[string]any{
		"base_url": up.URL, "key": "fixture", "model": "claude-opus-5",
		"id": "claude-opus-5", "train": 30, "holdout": 9,
	})
	if conflict.Code != 409 {
		t.Fatalf("official id accepted HTTP %d: %s", conflict.Code, conflict.Body.String())
	}
	aliasConflict := adminRequest(t, s, http.MethodPost, "/api/admin/fingerprints/capture", map[string]any{
		"base_url": up.URL, "key": "fixture", "model": "custom-model",
		"id": "custom-model", "aliases": "gpt-5.6-luna", "train": 30, "holdout": 9,
	})
	if aliasConflict.Code != 409 {
		t.Fatalf("official alias accepted HTTP %d: %s", aliasConflict.Code, aliasConflict.Body.String())
	}
}

func TestAdminFingerprintDeleteRefusesMissingAndOfficial(t *testing.T) {
	s, _, _, _ := testSetup(t)
	s.adminToken = "fixture-admin-token-0123456789"

	missing := adminRequest(t, s, http.MethodDelete, "/api/admin/fingerprints/custom-missing", nil)
	if missing.Code != 404 {
		t.Fatalf("missing fingerprint HTTP %d: %s", missing.Code, missing.Body.String())
	}
	official := adminRequest(t, s, http.MethodDelete, "/api/admin/fingerprints/gpt-5.6-luna", nil)
	if official.Code != 404 && official.Code != 409 {
		t.Fatalf("official fingerprint delete HTTP %d: %s", official.Code, official.Body.String())
	}
}

func waitAdminCapture(t *testing.T, s *Server) *adminCapture {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		capture := s.currentCapture()
		if capture == nil {
			t.Fatal("capture disappeared")
		}
		capture.mu.Lock()
		status := capture.Status
		capture.mu.Unlock()
		if status != "running" && status != "validating" {
			return capture
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("capture did not finish")
	return nil
}

func TestAdminCaptureAddsAndRemovesCustomReference(t *testing.T) {
	s, up, _, _ := testSetup(t)
	s.adminToken = "fixture-admin-token-0123456789"
	const id = "fixture-fingerprint-1017"
	start := adminRequest(t, s, http.MethodPost, "/api/admin/fingerprints/capture", map[string]any{
		"base_url": up.URL, "key": "fixture-key",
		"model": "fixture-model", "id": id, "display_name": "Fixture 1017",
		"aliases": "fixture-alias,fixture-alias", "protocol": "auto",
		"train": 30, "holdout": 9, "concurrency": 3, "timeout": 10,
	})
	if start.Code != 202 {
		t.Fatalf("start HTTP %d: %s", start.Code, start.Body.String())
	}
	capture := waitAdminCapture(t, s)
	if capture.snapshot()["status"] != "done" {
		t.Fatalf("capture failed: %+v", capture.snapshot())
	}
	model := s.currentBank().NumericModel(id)
	if model == nil || !model.Custom || model.ReferenceSamples != 30 || model.DisplayName != "Fixture 1017" {
		t.Fatalf("custom reference not published: %+v", model)
	}
	if len(model.Aliases) != 1 || model.Aliases[0] != "fixture-alias" {
		t.Fatalf("aliases not normalized: %+v", model.Aliases)
	}
	var refs struct {
		Items []adminReferenceMeta `json:"items"`
	}
	list := adminRequest(t, s, http.MethodGet, "/api/admin/fingerprints", nil)
	if list.Code != 200 {
		t.Fatal(list.Code, list.Body.String())
	}
	if err := json.Unmarshal(list.Body.Bytes(), &refs); err != nil || len(refs.Items) != 1 || refs.Items[0].ID != id || refs.Items[0].Samples != 30 {
		t.Fatalf("custom list: %s", list.Body.String())
	}
	if strings.Contains(list.Body.String(), "fixture-key") {
		t.Fatal("API key leaked")
	}
	if err := fingerprint.RemoveLocalReference(s.currentBank(), "claude-opus-5"); err == nil {
		t.Fatal("removed a non-custom reference")
	}
	del := adminRequest(t, s, http.MethodDelete, "/api/admin/fingerprints/"+id, nil)
	if del.Code != 200 {
		t.Fatal(del.Code, del.Body.String())
	}
	if s.currentBank().NumericModel(id) != nil {
		t.Fatal("custom reference still active after delete")
	}
	list = adminRequest(t, s, http.MethodGet, "/api/admin/fingerprints", nil)
	if !strings.Contains(list.Body.String(), `"items":[]`) {
		t.Fatalf("custom list after delete: %s", list.Body.String())
	}
}
