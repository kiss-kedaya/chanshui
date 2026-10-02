package api

import (
	"chanzhushui/internal/gateway"
	"chanzhushui/internal/history"
	"chanzhushui/internal/iq"
	"encoding/json"
	"fmt"
	"math"
	mathrand "math/rand"
	"net"
	"net/http"
	"strings"
	"time"
)

const publicRate = time.Minute

var publicSectionOrder = []string{"fingerprint", "injection", "cache", "tools", "web", "knowledge", "thinking", "iq"}

func clientHost(r *http.Request) string {
	if ip := net.ParseIP(strings.TrimSpace(r.Header.Get("CF-Connecting-IP"))); ip != nil {
		return ip.String()
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Server) takePublicSlot(host string) (bool, int) {
	now := time.Now()
	s.publicMu.Lock()
	defer s.publicMu.Unlock()
	if s.publicHits == nil {
		s.publicHits = map[string]time.Time{}
	}
	for key, at := range s.publicHits {
		if now.Sub(at) >= publicRate {
			delete(s.publicHits, key)
		}
	}
	if at, ok := s.publicHits[host]; ok {
		wait := publicRate - now.Sub(at)
		secs := int(math.Ceil(wait.Seconds()))
		if secs < 1 {
			secs = 1
		}
		return false, secs
	}
	if len(s.publicHits) >= 8192 {
		return false, 60
	}
	s.publicHits[host] = now
	return true, 0
}

func (s *Server) releasePublicSlot(host string) {
	s.publicMu.Lock()
	delete(s.publicHits, host)
	s.publicMu.Unlock()
}

func parsePublicSections(raw json.RawMessage, model string) (map[string]bool, []map[string]string, error) {
	allowed := map[string]bool{}
	for _, id := range publicSectionOrder {
		allowed[id] = true
	}
	all := len(strings.TrimSpace(string(raw))) == 0 || string(raw) == "null"
	var list []string
	if !all {
		var text string
		if json.Unmarshal(raw, &text) == nil {
			if text == "" || text == "all" {
				all = true
			} else {
				return nil, nil, fmt.Errorf("sections 只能是 all 或板块数组")
			}
		} else if json.Unmarshal(raw, &list) != nil {
			return nil, nil, fmt.Errorf("sections 只能是 all 或板块数组")
		} else if len(list) == 1 && list[0] == "all" {
			all = true
		}
	}
	var skipped []map[string]string
	if all {
		list = append([]string{}, publicSectionOrder...)
	}
	if len(list) == 0 {
		return nil, nil, fmt.Errorf("至少选择一个检测板块")
	}
	selected := map[string]bool{}
	sawOther := false
	for _, id := range list {
		if !allowed[id] {
			return nil, nil, fmt.Errorf("未知检测板块: %s", id)
		}
		if id != "thinking" {
			sawOther = true
		}
		if id == "thinking" && !gateway.IsClaude(model) {
			continue
		}
		selected[id] = true
	}
	if !gateway.IsClaude(model) && containsSection(list, "thinking") {
		if !sawOther {
			return nil, nil, fmt.Errorf("思维链回传只适用于 claude- 前缀模型")
		}
		skipped = append(skipped, map[string]string{"id": "thinking", "reason": "思维链回传只适用于 claude- 前缀模型，已跳过"})
	}
	if len(selected) == 0 {
		return nil, nil, fmt.Errorf("至少选择一个检测板块")
	}
	return selected, skipped, nil
}

func containsSection(list []string, id string) bool {
	for _, item := range list {
		if item == id {
			return true
		}
	}
	return false
}

func orderedSections(selected map[string]bool) []string {
	out := make([]string, 0, len(selected))
	for _, id := range publicSectionOrder {
		if selected[id] {
			out = append(out, id)
		}
	}
	return out
}

type publicAuditRequest struct {
	BaseURL  string          `json:"base_url"`
	APIKey   string          `json:"api_key"`
	Model    string          `json:"model"`
	Protocol string          `json:"protocol,omitempty"`
	Timeout  int             `json:"timeout,omitempty"`
	Sections json.RawMessage `json:"sections,omitempty"`
}

func (s *Server) publicIndex(w http.ResponseWriter, r *http.Request) {
	count, err := s.store.PublicAuditCount(r.Context())
	if err != nil {
		reply(w, 503, map[string]string{"error": "公益次数暂时读不到"})
		return
	}
	reply(w, 200, map[string]any{
		"name":          "chanshui",
		"version":       "v1",
		"public_audits": count,
		"rate_limit":    map[string]any{"limit": 1, "window_seconds": 60, "scope": "client_ip", "counts": "POST /api/v1/audits"},
		"sections":      publicSectionOrder,
		"docs":          "/docs",
		"openapi":       "/api/v1/openapi.json",
		"history":       false,
		"retention":     "24h memory, dropped on restart",
	})
}

func (s *Server) publicOpenAPI(w http.ResponseWriter, r *http.Request) {
	reply(w, 200, publicOpenAPI())
}

func (s *Server) publicCreate(w http.ResponseWriter, r *http.Request) {
	if s.draining.Load() {
		w.Header().Set("Retry-After", "5")
		reply(w, 503, map[string]string{"error": "服务正在平滑切换，当前不接收新检测"})
		return
	}
	var body publicAuditRequest
	if !s.readJSON(w, r, &body, false, true) {
		return
	}
	q := Request{Protocol: body.Protocol, BaseURL: body.BaseURL, Key: body.APIKey, Model: body.Model, Timeout: body.Timeout}
	if err := s.validate(&q, true); err != nil {
		reply(w, 400, map[string]string{"error": err.Error()})
		return
	}
	selected, skipped, err := parsePublicSections(body.Sections, q.Model)
	if err != nil {
		reply(w, 400, map[string]string{"error": err.Error()})
		return
	}
	host := clientHost(r)
	ok, wait := s.takePublicSlot(host)
	if !ok {
		w.Header().Set("Retry-After", fmt.Sprintf("%d", wait))
		reply(w, 429, map[string]any{"error": "公共检测每分钟 1 次", "retry_after": wait})
		return
	}
	fail := func(status int, msg, retry string) {
		s.releasePublicSlot(host)
		if retry != "" {
			w.Header().Set("Retry-After", retry)
		}
		reply(w, status, map[string]string{"error": msg})
	}
	id, err := history.NewReportID()
	if err != nil {
		fail(500, "任务创建失败", "")
		return
	}
	now := time.Now()
	t := &Task{
		ID: id, ReportID: id, Status: "queued", Source: "api",
		BaseURL: q.BaseURL, Model: q.Model,
		CreatedAt: now.UTC().Format(time.RFC3339),
		ExpiresAt: now.Add(history.JobTTL).Unix(),
		answers:   map[string]QuestionResult{},
	}
	t.Verdict.Protocol = q.Protocol
	t.Verdict.ScoringVersion = "2026-10-02-v6-total"
	names := map[string]string{
		"fingerprint": "模型指纹归因", "injection": "注入大小", "cache": "缓存读写验证",
		"tools": "工具调用检测", "web": "联网检测", "knowledge": "知识截止日期", "thinking": "思维链回传",
	}
	for _, section := range orderedSections(selected) {
		if section == "iq" {
			continue
		}
		t.Probes = append(t.Probes, Probe{ID: section, Name: names[section], Status: "queued"})
	}
	if selected["iq"] {
		rng := mathrand.New(mathrand.NewSource(now.UnixNano()))
		t.questions = []iq.Question{iq.GenPill(), iq.GenCarWash(), iq.GenBatBall(), iq.GenColorBlind(rng), iq.GenAnchor(), iq.GenCandyClassic(rng), iq.GenCandyVariant(rng), iq.GenSocks(rng)}
		for _, item := range t.questions {
			t.Probes = append(t.Probes, Probe{ID: item.ID, Name: item.Title, Status: "queued"})
		}
	}
	ctx, cancel := s.executionContext(q, t.ExpiresAt)
	t.cancel = cancel
	s.mu.Lock()
	if s.draining.Load() {
		s.mu.Unlock()
		cancel()
		fail(503, "服务正在平滑切换，当前不接收新检测", "5")
		return
	}
	if len(s.queue) >= cap(s.queue) {
		s.mu.Unlock()
		cancel()
		fail(429, "检测队列已满，请稍后再提交", "5")
		return
	}
	if err = s.store.AddPublicAudit(r.Context()); err != nil {
		s.mu.Unlock()
		cancel()
		fail(503, "公益次数暂时记不上，请稍后重试", "5")
		return
	}
	s.tasks[id] = t
	t.mu.Lock()
	status, expires := t.Status, t.ExpiresAt
	t.mu.Unlock()
	s.queue <- queuedJob{t, q, selected, ctx}
	s.mu.Unlock()
	reply(w, 202, map[string]any{
		"id": id, "status": status, "sections": orderedSections(selected), "skipped": skipped,
		"poll_url": "/api/v1/audits/" + id, "expires_at": expires,
	})
}

func (s *Server) publicGet(w http.ResponseWriter, r *http.Request) {
	id := strings.ToLower(r.PathValue("id"))
	t := s.find(id)
	if t == nil || t.Source != "api" {
		reply(w, 404, map[string]string{"error": "检测不存在或已过期（有效期24小时，重启后不保留）"})
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.ExpiresAt <= time.Now().Unix() {
		reply(w, 410, map[string]string{"error": "检测已超过24小时有效期"})
		return
	}
	payload := map[string]any{
		"id": t.ID, "status": t.Status, "model": t.Model,
		"base_url": t.BaseURL, "host": displayHost(t.BaseURL),
		"created_at": t.CreatedAt, "expires_at": t.ExpiresAt,
		"probes": t.Probes, "verdict": t.Verdict,
	}
	reply(w, 200, payload)
}

func publicOpenAPI() map[string]any {
	sectionEnum := []any{"fingerprint", "injection", "cache", "tools", "web", "knowledge", "thinking", "iq"}
	return map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":       "掺水？公共检测 API",
			"version":     "v1",
			"description": "给 Agent 和接入方调用。POST /api/v1/audits 每个客户端 IP 每 60 秒 1 次。轮询不计数。结果只留在内存 24 小时，不写入全站检测历史。省略 sections 或传 all 时跑全部板块，包含缓存。非 claude- 模型会跳过 thinking。只传 thinking 且模型不是 claude- 前缀时返回 400。总分只按本次选中的板块重算到 100。知识截止日期不进总分。",
		},
		"servers": []any{map[string]any{"url": "/"}},
		"paths": map[string]any{
			"/api/v1": map[string]any{
				"get": map[string]any{
					"operationId": "getPublicMeta",
					"summary":     "Rate limit, sections, and donated audit count",
					"responses":   map[string]any{"200": map[string]any{"description": "Service metadata"}},
				},
			},
			"/api/v1/openapi.json": map[string]any{
				"get": map[string]any{
					"operationId": "getOpenAPI",
					"summary":     "This document",
					"responses":   map[string]any{"200": map[string]any{"description": "OpenAPI 3.1"}},
				},
			},
			"/api/v1/audits": map[string]any{
				"post": map[string]any{
					"operationId": "createAudit",
					"summary":     "Start one audit",
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type":     "object",
									"required": []string{"base_url", "api_key", "model"},
									"properties": map[string]any{
										"base_url": map[string]any{"type": "string"},
										"api_key":  map[string]any{"type": "string"},
										"model":    map[string]any{"type": "string"},
										"protocol": map[string]any{"type": "string", "enum": []string{"auto", "chat", "anthropic", "openai"}, "default": "auto"},
										"timeout":  map[string]any{"type": "integer", "minimum": 10, "maximum": 600, "default": 360},
										"sections": map[string]any{
											"description": "Omit, all, or a list of section ids. all includes cache.",
											"oneOf": []any{
												map[string]any{"type": "string", "enum": []string{"all"}},
												map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": sectionEnum}},
											},
										},
									},
								},
							},
						},
					},
					"responses": map[string]any{
						"202": map[string]any{"description": "Accepted. Poll poll_url. No control token and no site history row."},
						"400": map[string]any{"description": "Invalid body, model, or sections"},
						"429": map[string]any{"description": "Rate limit or full queue. Retry-After is seconds."},
						"503": map[string]any{"description": "Draining or counter unavailable"},
					},
				},
			},
			"/api/v1/audits/{id}": map[string]any{
				"get": map[string]any{
					"operationId": "getAudit",
					"summary":     "Poll one API audit",
					"parameters":  []any{map[string]any{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "string"}}},
					"responses": map[string]any{
						"200": map[string]any{"description": "Current probes and verdict"},
						"404": map[string]any{"description": "Unknown id, website job, or already dropped"},
						"410": map[string]any{"description": "Older than 24 hours"},
					},
				},
			},
		},
	}
}
