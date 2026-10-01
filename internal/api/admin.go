package api

import (
	"crypto/subtle"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

const adminCookie = "chanshui_admin"

func (s *Server) loadAdminToken() {
	s.adminToken = strings.TrimSpace(os.Getenv("CHANSHUI_ADMIN_TOKEN"))
	if len(s.adminToken) < 24 {
		s.adminToken = ""
	}
}

func (s *Server) adminAuthorized(r *http.Request) bool {
	expected := s.adminToken
	if expected == "" {
		return false
	}
	got := adminPresented(r)
	if len(got) != len(expected) || got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(expected)) == 1
}

func adminClient(r *http.Request) string {
	host, _, e := net.SplitHostPort(r.RemoteAddr)
	if e != nil {
		host = r.RemoteAddr
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		if real := strings.TrimSpace(r.Header.Get("X-Real-IP")); net.ParseIP(real) != nil {
			return real
		}
	}
	return host
}

func adminPresented(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	}
	c, e := r.Cookie(adminCookie)
	if e != nil {
		return ""
	}
	return c.Value
}

func (s *Server) adminLogin(w http.ResponseWriter, r *http.Request) {
	if s.adminToken == "" {
		reply(w, 404, map[string]string{"error": "管理员入口未开启"})
		return
	}
	var body struct {
		Token string `json:"token"`
	}
	if !decode(w, r, &body) {
		return
	}
	host := adminClient(r)
	now := time.Now()
	s.adminMu.Lock()
	window := s.adminHits["admin:"+host]
	if now.Sub(window.At) > 10*time.Minute {
		window = feedbackWindow{At: now}
	}
	if window.Count >= 8 {
		s.adminMu.Unlock()
		w.Header().Set("Retry-After", "600")
		reply(w, 429, map[string]string{"error": "口令尝试过多，请十分钟后再试"})
		return
	}
	window.Count++
	if window.At.IsZero() {
		window.At = now
	}
	s.adminHits["admin:"+host] = window
	s.adminMu.Unlock()
	token := strings.TrimSpace(body.Token)
	if len(token) != len(s.adminToken) || subtle.ConstantTimeCompare([]byte(token), []byte(s.adminToken)) != 1 {
		reply(w, 401, map[string]string{"error": "管理员口令不正确"})
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: adminCookie, Value: s.adminToken, Path: "/api/admin",
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode, MaxAge: 12 * 3600,
	})
	reply(w, 200, map[string]string{"ok": "1"})
}

func (s *Server) adminLogout(w http.ResponseWriter, r *http.Request) {
	if s.adminToken == "" {
		reply(w, 404, map[string]string{"error": "管理员入口未开启"})
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: adminCookie, Value: "", Path: "/api/admin",
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode, MaxAge: -1,
	})
	reply(w, 200, map[string]string{"ok": "1"})
}

func (s *Server) adminFeedback(w http.ResponseWriter, r *http.Request) {
	if s.adminToken == "" {
		reply(w, 404, map[string]string{"error": "管理员入口未开启"})
		return
	}
	if !s.adminAuthorized(r) {
		reply(w, 401, map[string]string{"error": "需要管理员口令"})
		return
	}
	rows, e := s.store.ListFeedback(r.Context())
	if e != nil {
		reply(w, 503, map[string]string{"error": "反馈读取失败"})
		return
	}
	reply(w, 200, map[string]any{"items": rows})
}
