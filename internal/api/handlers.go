package api

import (
	"chanzhushui/internal/fingerprint"
	"chanzhushui/internal/gateway"
	"chanzhushui/internal/history"
	"chanzhushui/internal/iq"
	"context"

	"encoding/json"
	"fmt"
	"io"
	mathrand "math/rand"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Options controls local runtime and test transport injection.
type Options struct {
	AutoSync     bool
	AllowPrivate bool
	SkipRecover  bool
	HTTP         *http.Client
	Workers      int
	QueueSize    int
	MaxRequests  int
}
type Server struct {
	bankMu         sync.RWMutex
	feedbackMu     sync.Mutex
	feedbackLimits map[string]feedbackWindow
	ctx            context.Context
	stop           context.CancelFunc
	wg             sync.WaitGroup
	queue          chan queuedJob
	transport      *http.Transport
	httpClient     *http.Client
	globalSem      chan struct{}
	mu             sync.Mutex
	tasks          map[string]*Task
	store          *history.Store
	bank           *fingerprint.Bank
	opts           Options
	draining       atomic.Bool
}
type Request struct {
	Protocol    string `json:"protocol,omitempty"`
	BaseURL     string `json:"base_url"`
	Key         string `json:"key"`
	Model       string `json:"model"`
	Concurrency int    `json:"concurrency"`
	Timeout     int    `json:"timeout"`
	Cache       bool   `json:"cache"`
}
type Probe struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Ms     int64  `json:"ms"`
	Error  string `json:"error,omitempty"`
}
type Verdict struct {
	Timing         *Timing `json:"timing,omitempty"`
	Protocol       string  `json:"protocol,omitempty"`
	Tools          any     `json:"tools"`
	Web            any     `json:"web"`
	ScoringVersion string  `json:"scoring_version"`
	Injection      any     `json:"injection"`
	Fingerprint    any     `json:"fingerprint"`
	Thinking       any     `json:"thinking"`
	Cache          any     `json:"cache"`
	Knowledge      any     `json:"knowledge"`
	Integrity      any     `json:"integrity"`
	IQ             any     `json:"iq"`
}
type Task struct {
	ID          string  `json:"id"`
	Status      string  `json:"status"`
	BaseURL     string  `json:"base_url"`
	Model       string  `json:"model"`
	CreatedAt   string  `json:"created_at"`
	Probes      []Probe `json:"probes"`
	Verdict     Verdict `json:"verdict"`
	ReportID    string  `json:"report_id,omitempty"`
	HistoryID   int64   `json:"history_id,omitempty"`
	Warning     string  `json:"warning,omitempty"`
	ExpiresAt   int64   `json:"expires_at"`
	persistMu   sync.Mutex
	controlHash string
	finished    time.Time
	mu          sync.Mutex
	cancel      context.CancelFunc
	questions   []iq.Question
	answers     map[string]QuestionResult
}
type QuestionResult struct {
	ID           string          `json:"id"`
	Title        string          `json:"title"`
	Max          int             `json:"max"`
	Earned       int             `json:"earned"`
	Half         bool            `json:"half"`
	Tags         []string        `json:"tags"`
	Prompt       string          `json:"prompt"`
	Expected     string          `json:"expected"`
	Answer       string          `json:"answer"`
	ThinkingText string          `json:"thinking_text"`
	Signature    string          `json:"signature"`
	Ms           int64           `json:"ms"`
	Error        string          `json:"error,omitempty"`
	Metrics      gateway.Metrics `json:"metrics"`
}

func New(store *history.Store, opts Options) (*Server, error) {
	b, e := fingerprint.LoadBank()
	if e != nil {
		return nil, e
	}
	if opts.Workers <= 0 {
		opts.Workers = 16
	}
	if opts.QueueSize <= 0 {
		opts.QueueSize = 128
	}
	if opts.MaxRequests <= 0 {
		opts.MaxRequests = 64
	}
	if opts.Workers > 256 || opts.QueueSize > 4096 || opts.MaxRequests > 1024 {
		return nil, fmt.Errorf("worker/queue/request 配置超过上限")
	}
	ctx, stop := context.WithCancel(context.Background())
	s := &Server{tasks: map[string]*Task{}, store: store, bank: b, opts: opts, ctx: ctx, stop: stop, queue: make(chan queuedJob, opts.QueueSize), globalSem: make(chan struct{}, opts.MaxRequests)}
	s.feedbackLimits = map[string]feedbackWindow{}
	s.loadCachedBank()
	s.initTransport()
	if !opts.SkipRecover {
		if e := store.RecoverInterrupted(); e != nil {
			stop()
			return nil, e
		}
	}
	s.cleanup()
	s.startWorkers()
	if opts.AutoSync {
		s.startBankSync()
	}
	return s, nil
}
func (s *Server) Handler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		queued, running := s.runtimeCounts()
		reply(w, 200, map[string]any{"status": "ok", "mode": "local", "draining": s.draining.Load(), "queued_jobs": queued, "active_jobs": running})
	})
	m.HandleFunc("GET /api/ready", func(w http.ResponseWriter, r *http.Request) {
		if s.draining.Load() {
			reply(w, 503, map[string]any{"status": "draining"})
			return
		}
		reply(w, 200, map[string]any{"status": "ready"})
	})
	m.HandleFunc("GET /api/bank/stats", s.stats)
	m.HandleFunc("POST /api/detect", s.start)
	m.HandleFunc("GET /api/detect/{id}", s.get)
	m.HandleFunc("POST /api/detect/{id}/cancel", s.cancel)
	m.HandleFunc("POST /api/detect/{id}/retry", s.retry)
	m.HandleFunc("GET /api/history", s.list)
	m.HandleFunc("GET /api/history/{id}", s.detail)
	m.HandleFunc("GET /api/reports/{id}", s.report)
	m.HandleFunc("POST /api/models", s.models)
	m.HandleFunc("POST /api/ask", s.ask)
	m.HandleFunc("POST /api/feedback", s.feedback)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if o := r.Header.Get("Origin"); o != "" {
			u, e := url.Parse(o)
			if e != nil || u.Host != r.Host {
				reply(w, 403, map[string]string{"error": "仅允许同源本地请求"})
				return
			}
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			m.ServeHTTP(w, r)
		} else {
			http.NotFound(w, r)
		}
	})
}
func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		reply(w, 415, map[string]string{"error": "请求必须是 JSON"})
		return false
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		reply(w, 400, map[string]string{"error": "请求 JSON 或字段无效"})
		return false
	}
	if d.Decode(new(any)) != io.EOF {
		reply(w, 400, map[string]string{"error": "只接受一个 JSON 对象"})
		return false
	}
	return true
}
func (s *Server) validate(q *Request, model bool) error {
	q.BaseURL = strings.TrimRight(strings.TrimSpace(q.BaseURL), "/")
	q.Key = strings.TrimSpace(q.Key)
	q.Model = strings.TrimSpace(q.Model)
	if q.Protocol == "" {
		q.Protocol = "auto"
	}
	if q.Protocol != "auto" && q.Protocol != "chat" && q.Protocol != "anthropic" && q.Protocol != "openai" {
		return fmt.Errorf("接口模式无效")
	}
	u, e := url.Parse(q.BaseURL)
	if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("接口地址需为 HTTP(S) 地址，不能带凭据、查询参数或片段")
	}
	if q.Key == "" || len(q.Key) > 4096 {
		return fmt.Errorf("请填写有效的 API Key")
	}
	if model && !s.supported(q.Model) {
		return fmt.Errorf("请从可测模型清单选择模型")
	}
	if q.Timeout == 0 {
		q.Timeout = 360
	}
	if q.Timeout < 10 || q.Timeout > 600 {
		return fmt.Errorf("单请求超时应在 10～600 秒")
	}
	if q.Concurrency < 0 || q.Concurrency > 32 {
		return fmt.Errorf("并发应在 0～32，0 表示当前有限探针全部并行")
	}

	return nil
}
func cleanModel(id string) string {
	if i := strings.Index(id, "/"); i >= 0 {
		id = id[i+1:]
	}
	if strings.HasPrefix(id, "claude-") {
		id = strings.ReplaceAll(id, ".", "-")
		if id == "claude-haiku-4-5" {
			id = "claude-haiku-4-5-20251001"
		}
	}
	return id
}
func (s *Server) supported(id string) bool {
	bank := s.currentBank()
	for _, m := range bank.Numeric.Models {
		if m.ID == id {
			return true
		}
	}
	for _, m := range bank.Behavioral.Models {
		if cleanModel(m.ModelID) == id {
			return true
		}
	}
	return false
}
func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	bank := s.currentBank()
	seen := map[string]bool{}
	for _, m := range bank.Numeric.Models {
		seen[m.ID] = true
	}
	for _, m := range bank.Behavioral.Models {
		seen[cleanModel(m.ModelID)] = true
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	reply(w, 200, map[string]any{"supported_models": ids, "fingerprint_count": len(ids), "numeric_count": len(bank.Numeric.Models), "behavioral_count": len(bank.Behavioral.Models), "families": []string{"GPT", "Claude", "Gemini", "DeepSeek", "Grok", "GLM"}, "numeric_families": []string{"GPT", "Claude"}, "source": bank.Source})
}
func (s *Server) start(w http.ResponseWriter, r *http.Request) {
	if s.draining.Load() {
		w.Header().Set("Retry-After", "5")
		reply(w, 503, map[string]string{"error": "服务正在平滑切换，当前不接收新检测"})
		return
	}
	var q Request
	if !decode(w, r, &q) {
		return
	}
	if e := s.validate(&q, true); e != nil {
		reply(w, 400, map[string]string{"error": e.Error()})
		return
	}
	id, e := history.NewReportID()
	if e != nil {
		reply(w, 500, map[string]string{"error": "任务创建失败"})
		return
	}
	token, e := history.NewReportID()
	if e != nil {
		reply(w, 500, map[string]string{"error": "任务创建失败"})
		return
	}
	t := &Task{ID: id, ReportID: id, Status: "queued", BaseURL: q.BaseURL, Model: q.Model, CreatedAt: time.Now().UTC().Format(time.RFC3339), ExpiresAt: time.Now().Add(history.JobTTL).Unix(), controlHash: controlHash(token), answers: map[string]QuestionResult{}}
	rng := mathrand.New(mathrand.NewSource(time.Now().UnixNano()))
	t.questions = []iq.Question{iq.GenPill(), iq.GenCarWash(), iq.GenBatBall(), iq.GenColorBlind(rng), iq.GenAnchor(), iq.GenCandyClassic(rng), iq.GenCandyVariant(rng), iq.GenSocks(rng)}
	t.Verdict.Protocol = q.Protocol
	t.Verdict.ScoringVersion = "2026-10-01-v5-jobs"
	t.Probes = []Probe{{ID: "fingerprint", Name: "模型指纹归因", Status: "queued"}, {ID: "injection", Name: "注入大小", Status: "queued"}, {ID: "tools", Name: "工具调用检测", Status: "queued"}, {ID: "web", Name: "联网检测", Status: "queued"}, {ID: "knowledge", Name: "知识截止日期", Status: "queued"}}
	if q.Cache {
		t.Probes = append(t.Probes[:2], append([]Probe{{ID: "cache", Name: "缓存读写验证", Status: "queued"}}, t.Probes[2:]...)...)
	}
	if gateway.IsClaude(q.Model) {
		t.Probes = append(t.Probes, Probe{ID: "thinking", Name: "思维链回传", Status: "queued"})
	}
	for _, item := range t.questions {
		t.Probes = append(t.Probes, Probe{ID: item.ID, Name: item.Title, Status: "queued"})
	}
	ctx, cancel := s.executionContext(q, t.ExpiresAt)
	t.cancel = cancel
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.draining.Load() {
		cancel()
		w.Header().Set("Retry-After", "5")
		reply(w, 503, map[string]string{"error": "服务正在平滑切换，当前不接收新检测"})
		return
	}
	if len(s.queue) >= cap(s.queue) {
		cancel()
		w.Header().Set("Retry-After", "5")
		reply(w, 429, map[string]string{"error": "检测队列已满，请稍后再提交"})
		return
	}
	progress, _ := json.Marshal(t.Probes)
	raw, _ := json.Marshal(t.Verdict)
	questions, _ := json.Marshal(t.questions)
	historyID, e := s.store.CreateJob(r.Context(), history.Record{ReportID: id, CreatedAt: t.CreatedAt, BaseURL: t.BaseURL, Model: t.Model, Verdict: raw, Questions: questions}, progress, t.controlHash)
	if e != nil {
		cancel()
		reply(w, 503, map[string]string{"error": "任务存储繁忙，请稍后重试"})
		return
	}
	t.HistoryID = historyID
	s.tasks[id] = t
	s.queue <- queuedJob{t, q, nil, ctx}
	reply(w, 202, map[string]any{"task_id": id, "job_id": id, "report_id": id, "control_token": token, "expires_at": t.ExpiresAt})
}
func (s *Server) find(id string) *Task { s.mu.Lock(); defer s.mu.Unlock(); return s.tasks[id] }
func (s *Server) get(w http.ResponseWriter, r *http.Request) {
	id := strings.ToLower(r.PathValue("id"))
	t := s.find(id)
	if t == nil {
		if rec, e := s.store.GetByReportID(id); e == nil {
			t = s.fromRecord(rec)
		}
	}
	if t == nil {
		reply(w, 404, map[string]string{"error": "任务不存在或已过期（有效期24小时）"})
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.ExpiresAt <= time.Now().Unix() {
		reply(w, 410, map[string]string{"error": "检测报告已超过24小时有效期"})
		return
	}
	reveal := s.canReveal(r, t.ID, t.controlHash)
	base := maskedBaseURL(t.BaseURL)
	host := maskedHost(t.BaseURL)
	if reveal {
		base = publicBaseURL(t.BaseURL)
		host = displayHost(t.BaseURL)
	}
	payload := map[string]any{
		"id": t.ID, "task_id": t.ID, "job_id": t.ID, "report_id": t.ID,
		"status": t.Status, "base_url": base, "host": host, "model": t.Model,
		"created_at": t.CreatedAt, "expires_at": t.ExpiresAt,
		"probes": t.Probes, "verdict": t.Verdict,
	}
	if t.Warning != "" {
		payload["warning"] = t.Warning
	}
	replyConditional(w, r, payload)
}
func (s *Server) cancel(w http.ResponseWriter, r *http.Request) {
	reply(w, 403, map[string]string{"error": "操作 key 仅可重试对应任务的失败项，不支持停止任务"})
}
func (s *Server) retry(w http.ResponseWriter, r *http.Request) {
	if s.draining.Load() {
		w.Header().Set("Retry-After", "5")
		reply(w, 503, map[string]string{"error": "服务正在平滑切换，暂不接收重试任务"})
		return
	}
	t := s.find(r.PathValue("id"))
	if t == nil {
		if rec, e := s.store.GetByReportID(r.PathValue("id")); e == nil {
			hash, _ := s.store.ControlHash(rec.ReportID)
			if !controlHashMatches(r.Header.Get("X-Job-Token"), hash) {
				reply(w, 403, map[string]string{"error": "只有任务创建者可重跑检测"})
				return
			}
			t = s.fromRecord(rec)
			t.controlHash = hash
			s.mu.Lock()
			if existing := s.tasks[t.ID]; existing != nil {
				t = existing
			} else {
				s.tasks[t.ID] = t
			}
			s.mu.Unlock()
		}
	}
	if t == nil {
		reply(w, 404, map[string]string{"error": "任务已过期，请新建检测"})
		return
	}
	var q Request
	if !decode(w, r, &q) {
		return
	}
	t.mu.Lock()
	if t.ExpiresAt <= time.Now().Unix() {
		t.mu.Unlock()
		reply(w, 410, map[string]string{"error": "任务已超过24小时有效期"})
		return
	}
	if !s.authorize(r, t) {
		t.mu.Unlock()
		reply(w, 403, map[string]string{"error": "只有任务创建者可重跑检测"})
		return
	}
	q.BaseURL, q.Model = t.BaseURL, t.Model
	q.Protocol = t.Verdict.Protocol
	if e := s.validate(&q, true); e != nil {
		t.mu.Unlock()
		reply(w, 400, map[string]string{"error": e.Error()})
		return
	}
	if t.Status == "running" || t.Status == "queued" {
		t.mu.Unlock()
		reply(w, 409, map[string]string{"error": "任务仍在运行"})
		return
	}
	beforeProbes := append([]Probe(nil), t.Probes...)
	pick := map[string]bool{}
	for i, p := range t.Probes {
		if p.Status == "error" || p.Status == "cancelled" {
			pick[p.ID] = true
			t.Probes[i].Status = "queued"
			t.Probes[i].Error = ""
		}
	}
	if len(pick) == 0 {
		t.mu.Unlock()
		reply(w, 400, map[string]string{"error": "没有请求失败项；低分或未通过不是网络错误"})
		return
	}
	t.Status = "queued"
	t.Warning = ""
	ctx, cancel := s.executionContext(q, t.ExpiresAt)
	t.cancel = cancel
	t.mu.Unlock()
	s.mu.Lock()
	if s.draining.Load() {
		s.mu.Unlock()
		cancel()
		t.mu.Lock()
		t.Status = "partial"
		t.Probes = beforeProbes
		t.mu.Unlock()
		w.Header().Set("Retry-After", "5")
		reply(w, 503, map[string]string{"error": "服务正在平滑切换，暂不接收重试任务"})
		return
	}
	if len(s.queue) >= cap(s.queue) {
		s.mu.Unlock()
		cancel()
		t.mu.Lock()
		t.Status = "partial"
		t.Probes = beforeProbes
		t.mu.Unlock()
		reply(w, 429, map[string]string{"error": "检测队列已满，请稍后重试"})
		return
	}
	s.queue <- queuedJob{t, q, pick, ctx}
	s.mu.Unlock()
	reply(w, 202, map[string]string{"task_id": t.ID})
}
func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if n < 1 || n > 50 {
		n = 50
	}
	before := int64(0)
	if value := r.URL.Query().Get("before"); value != "" {
		var err error
		before, err = strconv.ParseInt(value, 10, 64)
		if err != nil || before < 1 {
			reply(w, 400, map[string]string{"error": "历史分页游标无效"})
			return
		}
	}
	items, e := s.store.ListBefore(n, before)
	if e != nil {
		reply(w, 500, map[string]string{"error": "历史读取失败"})
		return
	}
	if items == nil {
		items = []history.Summary{}
	}
	// History is a global, public index.  Keep the site signal, but never
	// return the upstream URL/path from this endpoint.  A report detail can
	// reveal the host only when its matching job capability is presented.
	for i := range items {
		items[i].BaseURL = maskedBaseURL(items[i].BaseURL)
	}
	replyConditional(w, r, items)
}
func (s *Server) detail(w http.ResponseWriter, r *http.Request) {
	id, e := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if e != nil || id <= 0 {
		reply(w, 400, map[string]string{"error": "历史编号无效"})
		return
	}
	rec, e := s.store.Get(id)
	if e != nil {
		reply(w, 404, map[string]string{"error": "历史记录不存在"})
		return
	}
	reveal := s.canReveal(r, rec.ReportID, "")
	replyConditional(w, r, reportPayload(rec.ID, rec.ReportID, rec.BaseURL, rec.Model, rec.CreatedAt, rec.Status, rec.ExpiresAt, rec.Progress, rec.Verdict, reveal))
}

func displayHost(base string) string {
	u, e := url.Parse(base)
	if e != nil || u.Host == "" {
		return "未知站点"
	}
	return u.Host
}

// maskedHost intentionally keeps only a tiny, deterministic site hint.  It
// does not expose the original hostname, path, query, credentials, or port.
func maskedHost(base string) string {
	u, e := url.Parse(base)
	if e != nil || u.Hostname() == "" {
		return "已脱敏站点"
	}
	name := u.Hostname()
	labels := strings.Split(name, ".")
	for i, label := range labels {
		if len(label) <= 1 {
			labels[i] = "*"
			continue
		}
		labels[i] = label[:1] + "***"
	}
	return strings.Join(labels, ".")
}

// maskedBaseURL is the public representation used by history and reports.
// It remains a URL so existing clients can derive a display host, but has no
// path (including /v1), query, credentials, port, or full hostname.
func maskedBaseURL(base string) string {
	u, e := url.Parse(base)
	if e != nil || u.Scheme == "" || u.Hostname() == "" {
		return "https://已脱敏站点"
	}
	return u.Scheme + "://" + maskedHost(base)
}

func publicBaseURL(base string) string {
	u, e := url.Parse(base)
	if e != nil || u.Scheme == "" || u.Host == "" {
		return "https://未知站点"
	}
	// Host excludes userinfo; deliberately discard path/query/fragment.
	return u.Scheme + "://" + u.Host
}

func (s *Server) canReveal(r *http.Request, reportID, inMemoryHash string) bool {
	expected := inMemoryHash
	if expected == "" && s.store != nil {
		expected, _ = s.store.ControlHash(reportID)
	}
	return controlHashMatches(r.Header.Get("X-Job-Token"), expected)
}

func reportPayload(id int64, reportID, baseURL, model, createdAt, status string, expiresAt int64, probes, verdict json.RawMessage, reveal bool) map[string]any {
	base := maskedBaseURL(baseURL)
	host := maskedHost(baseURL)
	if reveal {
		base = publicBaseURL(baseURL)
		host = displayHost(baseURL)
	}
	return map[string]any{
		"id": id, "report_id": reportID, "job_id": reportID,
		"host": host, "base_url": base, "model": model,
		"created_at": createdAt, "status": status, "expires_at": expiresAt,
		"probes": probes, "verdict_json": verdict,
	}
}

func (s *Server) report(w http.ResponseWriter, r *http.Request) {
	id := strings.ToLower(r.PathValue("id"))
	if !history.ValidReportID(id) {
		reply(w, 400, map[string]string{"error": "检测 ID 无效"})
		return
	}
	if t := s.find(id); t != nil {
		t.mu.Lock()
		defer t.mu.Unlock()
		if t.ExpiresAt <= time.Now().Unix() {
			reply(w, 410, map[string]string{"error": "报告已过期"})
			return
		}
		reveal := s.canReveal(r, t.ID, t.controlHash)
		verdict, _ := json.Marshal(t.Verdict)
		probes, _ := json.Marshal(t.Probes)
		replyConditional(w, r, reportPayload(t.HistoryID, t.ID, t.BaseURL, t.Model, t.CreatedAt, t.Status, t.ExpiresAt, probes, verdict, reveal))
		return
	}
	rec, e := s.store.GetByReportID(id)
	if e != nil {
		reply(w, 404, map[string]string{"error": "报告不存在或已过期（有效期24小时）"})
		return
	}
	reveal := s.canReveal(r, rec.ReportID, "")
	replyConditional(w, r, reportPayload(rec.ID, rec.ReportID, rec.BaseURL, rec.Model, rec.CreatedAt, rec.Status, rec.ExpiresAt, rec.Progress, rec.Verdict, reveal))
}
