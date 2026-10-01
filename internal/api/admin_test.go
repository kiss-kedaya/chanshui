package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdminFeedbackRequiresConfiguredToken(t *testing.T) {
	s, _, _, _ := testSetup(t)
	payload := map[string]string{"kind": "method", "contact": "fixture-contact@example.invalid", "message": "建议补充模型测试对照样本。"}
	if w := post(t, s.Handler(), "/api/feedback", payload); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	if r := getJSON(t, s.Handler(), "/api/admin/feedback"); r.Code != 404 {
		t.Fatal("unconfigured admin exposed", r.Code)
	}
	if r := post(t, s.Handler(), "/api/admin/login", map[string]string{"token": "x"}); r.Code != 404 {
		t.Fatal("unconfigured login exposed", r.Code)
	}
	s.adminToken = "fixture-admin-token-0123456789"
	if r := getJSON(t, s.Handler(), "/api/admin/feedback?token="+s.adminToken); r.Code != 401 || strings.Contains(r.Body.String(), "fixture-contact") {
		t.Fatal("query token accepted or contact leaked", r.Code, r.Body.String())
	}
	req := httptest.NewRequest(http.MethodGet, "/api/admin/feedback", nil)
	req.Header.Set("Authorization", "Bearer wrong-admin-token-0123456789")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != 401 || strings.Contains(w.Body.String(), "fixture-contact") {
		t.Fatal(w.Code, w.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "/api/admin/feedback", nil)
	req.Header.Set("Authorization", "Bearer "+s.adminToken)
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "fixture-contact@example.invalid") {
		t.Fatal(w.Code, w.Body.String())
	}
	login := post(t, s.Handler(), "/api/admin/login", map[string]string{"token": s.adminToken})
	cookie := login.Header().Get("Set-Cookie")
	if login.Code != 200 || !strings.Contains(cookie, "chanshui_admin=") || !strings.Contains(cookie, "HttpOnly") || strings.Contains(login.Body.String(), s.adminToken) {
		t.Fatal(login.Code, cookie, login.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "/api/admin/feedback", nil)
	req.AddCookie(&http.Cookie{Name: "chanshui_admin", Value: s.adminToken})
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "建议补充") {
		t.Fatal("cookie auth failed", w.Code, w.Body.String())
	}
}
