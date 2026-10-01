package api

import (
	"chanzhushui/internal/history"
	"chanzhushui/internal/iq"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"

	"time"
)

type queuedJob struct {
	task     *Task
	request  Request
	selected map[string]bool
	ctx      context.Context
}

func controlHash(value string) string {
	h := sha256.Sum256([]byte(value))
	return hex.EncodeToString(h[:])
}
func controlHashMatches(value, expected string) bool {
	if value == "" || expected == "" {
		return false
	}
	got := controlHash(value)
	return len(got) == len(expected) && subtle.ConstantTimeCompare([]byte(got), []byte(expected)) == 1
}
func (s *Server) startWorkers() {
	for i := 0; i < s.opts.Workers; i++ {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			for {
				select {
				case <-s.ctx.Done():
					return
				case j := <-s.queue:
					j.task.mu.Lock()
					if j.task.Status == "queued" {
						j.task.Status = "running"
					}
					j.task.mu.Unlock()
					s.persist(j.task)
					s.run(j.ctx, j.task, j.request, j.selected)
					j.request.Key = ""
				}
			}
		}()
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-ticker.C:
				s.cleanup()
			}
		}
	}()
}
func (s *Server) cleanup() {
	now := time.Now()
	for batch := 0; batch < 20; batch++ {
		n, e := s.store.PurgeExpired(s.ctx, now)
		if e != nil {
			if s.ctx.Err() == nil {
				log.Print("expired-job cleanup failed")
			}
			break
		}
		if n < 500 {
			break
		}
	}
	s.mu.Lock()
	for id, t := range s.tasks {
		t.mu.Lock()
		expired := now.Unix() >= t.ExpiresAt
		finished := t.Status != "running" && t.Status != "queued"
		old := now.Sub(t.finished) > time.Minute
		if expired && t.cancel != nil {
			t.cancel()
		}
		t.mu.Unlock()
		if expired || finished && old {
			delete(s.tasks, id)
		}
	}
	s.mu.Unlock()
}
func (s *Server) Close() { s.stop(); s.wg.Wait(); s.transport.CloseIdleConnections() }
func (s *Server) persist(t *Task) {
	t.persistMu.Lock()
	defer t.persistMu.Unlock()
	t.mu.Lock()
	r := history.Record{ReportID: t.ID, Status: t.Status, Model: t.Model, BaseURL: t.BaseURL, CreatedAt: t.CreatedAt}
	raw, e := json.Marshal(t.Verdict)
	if e != nil {
		t.mu.Unlock()
		return
	}
	progress, e := json.Marshal(t.Probes)
	if e != nil {
		t.mu.Unlock()
		return
	}
	r.Verdict = raw
	if m, ok := t.Verdict.IQ.(map[string]any); ok {
		if n, ok := m["score"].(int); ok {
			r.IQScore = &n
		}
	}
	if m, ok := t.Verdict.Thinking.(map[string]any); ok {
		if b, ok := m["pass"].(bool); ok {
			r.ThinkingPass = &b
		}
	}
	if m, ok := t.Verdict.Fingerprint.(map[string]any); ok {
		r.Family, _ = m["family"].(string)
	}
	t.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if e = s.store.UpdateJob(ctx, r, progress); e != nil {
		t.mu.Lock()
		t.Warning = "结果保存暂时失败，请稍后刷新"
		t.mu.Unlock()
	}
}
func (s *Server) authorize(r *http.Request, t *Task) bool {
	value := r.Header.Get("X-Job-Token")
	return controlHashMatches(value, t.controlHash)
}
func (s *Server) fromRecord(r *history.Record) *Task {
	t := &Task{ID: r.ReportID, ReportID: r.ReportID, HistoryID: r.ID, CreatedAt: r.CreatedAt, BaseURL: r.BaseURL, Model: r.Model, Status: r.Status, ExpiresAt: r.ExpiresAt}
	json.Unmarshal(r.Verdict, &t.Verdict)
	json.Unmarshal(r.Progress, &t.Probes)
	json.Unmarshal(r.Questions, &t.questions)
	t.answers = map[string]QuestionResult{}
	var data struct {
		IQ struct {
			PerQuestion []QuestionResult `json:"per_question"`
		}
	}
	json.Unmarshal(r.Verdict, &data)
	for _, q := range data.IQ.PerQuestion {
		t.answers[q.ID] = q
	}
	if t.Status == "interrupted" {
		for i := range t.Probes {
			if t.Probes[i].Status == "running" || t.Probes[i].Status == "queued" {
				t.Probes[i].Status = "error"
				t.Probes[i].Error = "服务重启，请重新提供密钥后重跑"
			}
		}
	}
	if t.questions == nil {
		t.questions = []iq.Question{}
	}
	return t
}
