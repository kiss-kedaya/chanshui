package api

import (
	"github.com/kiss-kedaya/chanzhushui/internal/history"
	"net"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

type feedbackWindow struct {
	At    time.Time
	Count int
}

func (s *Server) feedback(w http.ResponseWriter, r *http.Request) {
	var f struct {
		Contact string `json:"contact"`
		Message string `json:"message"`
		Kind    string `json:"kind"`
	}
	if !s.readJSON(w, r, &f, false, true) {
		return
	}
	f.Contact = strings.TrimSpace(f.Contact)
	f.Message = strings.TrimSpace(f.Message)
	if utf8.RuneCountInString(f.Contact) < 2 || utf8.RuneCountInString(f.Contact) > 200 || utf8.RuneCountInString(f.Message) < 5 || utf8.RuneCountInString(f.Message) > 4000 {
		reply(w, 400, map[string]string{"error": "请填写联系方式（2～200字）与问题（5～4000字）"})
		return
	}
	if f.Kind != "bug" && f.Kind != "method" && f.Kind != "suggestion" {
		reply(w, 400, map[string]string{"error": "请选择反馈类型"})
		return
	}
	if strings.Contains(f.Message, "sk-") || strings.Contains(f.Contact, "sk-") {
		reply(w, 400, map[string]string{"error": "请移除反馈中的 API 密钥后提交"})
		return
	}
	host, _, e := net.SplitHostPort(r.RemoteAddr)
	if e != nil {
		host = r.RemoteAddr
	}
	now := time.Now()
	s.feedbackMu.Lock()
	for key, v := range s.feedbackLimits {
		if now.Sub(v.At) > time.Minute {
			delete(s.feedbackLimits, key)
		}
	}
	window := s.feedbackLimits[host]
	if window.Count >= 5 || len(s.feedbackLimits) >= 4096 {
		s.feedbackMu.Unlock()
		w.Header().Set("Retry-After", "60")
		reply(w, 429, map[string]string{"error": "反馈提交太频繁，请一分钟后再试"})
		return
	}
	if window.Count == 0 {
		window.At = now
	}
	window.Count++
	s.feedbackLimits[host] = window
	s.feedbackMu.Unlock()
	id, e := history.NewReportID()
	if e != nil {
		reply(w, 500, map[string]string{"error": "反馈编号生成失败"})
		return
	}
	if e = s.store.SaveFeedback(r.Context(), history.Feedback{ID: id, Kind: f.Kind, Contact: f.Contact, Message: f.Message}); e != nil {
		reply(w, 503, map[string]string{"error": "反馈保存失败，请稍后重试"})
		return
	}
	reply(w, 201, map[string]string{"id": id, "message": "反馈已提交"})
}
