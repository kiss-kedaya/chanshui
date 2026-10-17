package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/kiss-kedaya/chanzhushui/internal/fingerprint"
	"github.com/kiss-kedaya/chanzhushui/internal/gateway"
	"github.com/kiss-kedaya/chanzhushui/internal/history"
)

const customRefsStateKey = "custom-fingerprint-references-v1"
const maxCustomReferences = 20

type adminReferenceMeta struct {
	ID          string   `json:"id"`
	DisplayName string   `json:"display_name"`
	Family      string   `json:"family"`
	FamilyName  string   `json:"family_name"`
	Source      string   `json:"source"`
	CapturedAt  string   `json:"captured_at"`
	Samples     int      `json:"samples"`
	Aliases     []string `json:"aliases,omitempty"`
}

type adminCaptureGroup struct {
	Group       int     `json:"group"`
	TopModel    string  `json:"top_model"`
	TopDisplay  string  `json:"top_display"`
	Probability float64 `json:"probability"`
	Pass        bool    `json:"pass"`
}

type adminCapture struct {
	mu               sync.Mutex
	ID               string
	Status           string
	StartedAt        string
	UpdatedAt        string
	TrainRequested   int
	HoldoutRequested int
	Total            int
	Completed        int
	TrainUsable      int
	HoldoutUsable    int
	Error            string
	Groups           []adminCaptureGroup
	Reference        *adminReferenceMeta
	cancel           context.CancelFunc
}

func (c *adminCapture) update(fn func()) {
	c.mu.Lock()
	fn()
	c.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	c.mu.Unlock()
}

func (c *adminCapture) finish(status, errText string) {
	c.mu.Lock()
	c.Status = status
	c.Error = errText
	c.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	cancel := c.cancel
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (c *adminCapture) snapshot() map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	groups := make([]adminCaptureGroup, len(c.Groups))
	copy(groups, c.Groups)
	out := map[string]any{
		"id": c.ID, "status": c.Status, "started_at": c.StartedAt, "updated_at": c.UpdatedAt,
		"train_requested": c.TrainRequested, "holdout_requested": c.HoldoutRequested,
		"total": c.Total, "completed": c.Completed,
		"train_usable": c.TrainUsable, "holdout_usable": c.HoldoutUsable,
		"error": c.Error, "groups": groups,
	}
	if c.Reference != nil {
		ref := *c.Reference
		out["reference"] = ref
	}
	return out
}

func (s *Server) currentCapture() *adminCapture {
	s.captureMu.Lock()
	defer s.captureMu.Unlock()
	return s.capture
}

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if s.adminToken == "" {
		reply(w, 404, map[string]string{"error": "管理员入口未开启"})
		return false
	}
	if !s.adminAuthorized(r) {
		reply(w, 401, map[string]string{"error": "需要管理员口令"})
		return false
	}
	return true
}

func (s *Server) loadCustomReferences() ([]fingerprint.LocalReference, error) {
	raw, err := s.store.LoadRuntimeState(s.ctx, customRefsStateKey)
	if err != nil {
		return nil, nil
	}
	var refs []fingerprint.LocalReference
	if err := json.Unmarshal(raw, &refs); err != nil {
		return nil, fmt.Errorf("custom reference state is invalid")
	}
	return refs, nil
}

func (s *Server) saveCustomReferences(ctx context.Context, refs []fingerprint.LocalReference) error {
	if refs == nil {
		refs = []fingerprint.LocalReference{}
	}
	raw, err := json.Marshal(refs)
	if err != nil {
		return err
	}
	return s.store.SaveRuntimeState(ctx, customRefsStateKey, raw)
}

// mergeCustomReferences is deliberately conservative: a custom reference may
// update another custom reference, but it can never shadow an official model
// or one of its aliases.
func (s *Server) mergeCustomReferences(b *fingerprint.Bank) error {
	refs, err := s.loadCustomReferences()
	if err != nil {
		return err
	}
	for _, ref := range refs {
		if !ref.Custom {
			continue
		}
		conflict := false
		for _, name := range append([]string{ref.ID}, ref.Aliases...) {
			if m := b.NumericModel(name); m != nil {
				if m.ID != ref.ID || !m.Custom {
					conflict = true
					break
				}
			}
		}
		if conflict {
			log.Printf("custom fingerprint %q conflicts with an existing model; skipped", ref.ID)
			continue
		}
		if err := fingerprint.AddLocalReference(b, ref); err != nil {
			log.Printf("custom fingerprint %q is invalid: %v", ref.ID, err)
		}
	}
	return nil
}

func referenceMeta(ref fingerprint.LocalReference) adminReferenceMeta {
	return adminReferenceMeta{
		ID: ref.ID, DisplayName: ref.DisplayName, Family: ref.Family, FamilyName: ref.FamilyName,
		Source: ref.Source, CapturedAt: ref.CapturedAt, Samples: len(ref.Samples), Aliases: ref.Aliases,
	}
}

func splitAliases(raw string) []string {
	var out []string
	seen := map[string]bool{}
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}

func (s *Server) adminFingerprints(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	refs, err := s.loadCustomReferences()
	if err != nil {
		reply(w, 503, map[string]string{"error": "自定义指纹读取失败"})
		return
	}
	items := make([]adminReferenceMeta, 0, len(refs))
	for _, ref := range refs {
		items = append(items, referenceMeta(ref))
	}
	capture := s.currentCapture()
	var snapshot any
	if capture != nil {
		snapshot = capture.snapshot()
	}
	reply(w, 200, map[string]any{"items": items, "capture": snapshot})
}

func (s *Server) adminCaptureStart(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	var body struct {
		BaseURL     string `json:"base_url"`
		Key         string `json:"key"`
		Model       string `json:"model"`
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
		Aliases     string `json:"aliases"`
		Protocol    string `json:"protocol"`
		Train       int    `json:"train"`
		Holdout     int    `json:"holdout"`
		Concurrency int    `json:"concurrency"`
		Timeout     int    `json:"timeout"`
	}
	if !s.readJSON(w, r, &body, false, false) {
		return
	}
	if body.Train == 0 {
		body.Train = 36
	}
	if body.Holdout == 0 {
		body.Holdout = 12
	}
	if body.Concurrency == 0 {
		body.Concurrency = 3
	}
	if body.Train < 30 || body.Train > 40 || body.Holdout < 9 || body.Holdout > 12 || body.Holdout%3 != 0 {
		reply(w, 400, map[string]string{"error": "训练样本需 30～40 个，保留样本需 9～12 个且为 3 的倍数"})
		return
	}
	if body.Concurrency < 1 || body.Concurrency > 4 {
		reply(w, 400, map[string]string{"error": "并发需在 1～4"})
		return
	}
	q := Request{Protocol: body.Protocol, BaseURL: body.BaseURL, Key: body.Key, Model: body.Model, Concurrency: body.Concurrency, Timeout: body.Timeout}
	if err := s.validate(&q, true); err != nil {
		reply(w, 400, map[string]string{"error": err.Error()})
		return
	}
	id := strings.TrimSpace(body.ID)
	if id == "" {
		id = cleanModel(q.Model)
	}
	if id == "" || len(id) > 120 || strings.ContainsAny(id, " \t\r\n") {
		reply(w, 400, map[string]string{"error": "请填写 1～120 字节、无空白的归因 ID"})
		return
	}
	display := strings.TrimSpace(body.DisplayName)
	if display == "" {
		display = id
	}
	aliases := splitAliases(body.Aliases)
	if len(aliases) > 20 {
		reply(w, 400, map[string]string{"error": "别名最多 20 个"})
		return
	}
	bank := s.currentBank()
	for _, name := range append([]string{id}, aliases...) {
		if m := bank.NumericModel(name); m != nil {
			if m.ID != id || !m.Custom {
				reply(w, 409, map[string]string{"error": "ID 或别名与已有模型冲突：" + name})
				return
			}
		}
	}
	refs, err := s.loadCustomReferences()
	if err != nil {
		reply(w, 503, map[string]string{"error": "自定义指纹读取失败"})
		return
	}
	exists := false
	for _, ref := range refs {
		if ref.ID == id {
			exists = true
			break
		}
	}
	if !exists && len(refs) >= maxCustomReferences {
		reply(w, 409, map[string]string{"error": "自定义指纹已达上限，请先删除旧条目"})
		return
	}
	s.captureMu.Lock()
	if s.capture != nil && (s.capture.Status == "running" || s.capture.Status == "validating") {
		current := s.capture.snapshot()
		s.captureMu.Unlock()
		reply(w, 409, map[string]any{"error": "已有采集任务在运行", "capture": current})
		return
	}
	captureID, err := history.NewReportID()
	if err != nil {
		s.captureMu.Unlock()
		reply(w, 500, map[string]string{"error": "采集任务创建失败"})
		return
	}
	ctx, cancel := context.WithTimeout(s.ctx, 60*time.Minute)
	job := &adminCapture{
		ID: captureID, Status: "running",
		StartedAt: time.Now().UTC().Format(time.RFC3339), UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		TrainRequested: body.Train, HoldoutRequested: body.Holdout, Total: body.Train + body.Holdout,
		cancel: cancel,
	}
	s.capture = job
	s.captureMu.Unlock()
	go s.runAdminCapture(ctx, job, q, id, display, aliases)
	reply(w, 202, job.snapshot())
}

func (s *Server) runAdminCapture(ctx context.Context, job *adminCapture, q Request, id, display string, aliases []string) {
	defer func() {
		if p := recover(); p != nil {
			job.finish("failed", "采集内部错误")
		}
	}()
	g, err := s.client(q)
	if err != nil {
		job.finish("failed", err.Error())
		return
	}
	bank := s.currentBank()
	minValid := bank.Numeric.MinimumValidNumbers
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	var challenges []fingerprint.Challenge
	for len(challenges) < job.Total {
		challenges = append(challenges, fingerprint.GenerateChallenges(min(41, job.Total-len(challenges)), rng)...)
	}
	type captureResult struct {
		index    int
		split    string
		nums     []int
		answer   string
		expected int
		usable   bool
		err      string
	}
	results := make([]captureResult, job.Total)
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < min(q.Concurrency, job.Total); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				if ctx.Err() != nil {
					return
				}
				ch := challenges[i]
				m, callErr := g.CallMessages(ctx, gateway.CallRequest{
					Model: q.Model, Prompt: ch.Prompt, MaxTokens: 16384,
					Stream: g.ProtocolFor(q.Model) != "messages", TimeoutSeconds: q.Timeout,
				})
				errText := m.Err
				if callErr != nil {
					errText = callErr.Error()
				}
				errText = redact(errText, q.Key)
				nums := fingerprint.ParseNumbers(m.Answer)
				usable := errText == "" && m.StopReason != "max_tokens" && m.StopReason != "length" && fingerprint.Usable(m.Answer, ch.ExpectedCount, minValid)
				split := "train"
				if i >= job.TrainRequested {
					split = "holdout"
				}
				results[i] = captureResult{index: i, split: split, nums: nums, answer: strings.ReplaceAll(m.Answer, q.Key, "[redacted]"), expected: ch.ExpectedCount, usable: usable, err: errText}
				job.update(func() {
					job.Completed++
					if usable {
						if split == "train" {
							job.TrainUsable++
						} else {
							job.HoldoutUsable++
						}
					}
				})
			}
		}()
	}
	for i := 0; i < job.Total; i++ {
		select {
		case jobs <- i:
		case <-ctx.Done():
			break
		}
		if ctx.Err() != nil {
			break
		}
	}
	close(jobs)
	wg.Wait()
	if ctx.Err() == context.Canceled {
		job.finish("cancelled", "采集已取消")
		return
	}
	if ctx.Err() == context.DeadlineExceeded {
		job.finish("failed", "采集超时")
		return
	}
	var trainSamples [][]int
	var holdout []fingerprint.Output
	firstErr := ""
	for _, r := range results {
		if !r.usable {
			if firstErr == "" && r.err != "" {
				firstErr = r.err
			}
			continue
		}
		if r.split == "train" {
			trainSamples = append(trainSamples, r.nums)
		} else {
			holdout = append(holdout, fingerprint.Output{Text: r.answer, ExpectedCount: r.expected})
		}
	}
	if len(trainSamples) < 30 {
		msg := fmt.Sprintf("有效训练样本不足（%d/30）", len(trainSamples))
		if firstErr != "" {
			msg += "：" + firstErr
		}
		job.finish("failed", msg)
		return
	}
	if len(holdout) != job.HoldoutRequested {
		job.finish("failed", fmt.Sprintf("有效保留样本不足（%d/%d）", len(holdout), job.HoldoutRequested))
		return
	}
	family, familyName := fingerprint.FamilyForModel(id)
	ref := fingerprint.LocalReference{
		ID: id, DisplayName: display, Aliases: aliases, Family: family, FamilyName: familyName,
		Source: q.BaseURL, CapturedAt: time.Now().UTC().Format(time.RFC3339), Custom: true, Samples: trainSamples,
	}
	candidate, err := fingerprint.CloneBank(bank)
	if err != nil {
		job.finish("failed", "候选库复制失败")
		return
	}
	if err := fingerprint.AddLocalReference(candidate, ref); err != nil {
		job.finish("failed", err.Error())
		return
	}
	groups := make([]adminCaptureGroup, 0, len(holdout)/3)
	for i := 0; i+3 <= len(holdout); i += 3 {
		a := fingerprint.Attribute(holdout[i:i+3], candidate)
		topDisplay := a.TopModel
		if m := candidate.NumericModel(a.TopModel); m != nil && m.DisplayName != "" {
			topDisplay = m.DisplayName
		}
		group := adminCaptureGroup{Group: i/3 + 1, TopModel: a.TopModel, TopDisplay: topDisplay, Probability: a.TopProb, Pass: a.TopModel == id}
		groups = append(groups, group)
	}
	job.update(func() { job.Status = "validating"; job.Groups = groups })
	passed := len(groups) == job.HoldoutRequested/3
	for _, group := range groups {
		passed = passed && group.Pass
	}
	if !passed {
		job.finish("failed", "保留样本未全部归因到新指纹，未写入归因库")
		return
	}
	s.adminRefsMu.Lock()
	refs, loadErr := s.loadCustomReferences()
	if loadErr != nil {
		s.adminRefsMu.Unlock()
		job.finish("failed", "自定义指纹读取失败")
		return
	}
	replaced := false
	for i := range refs {
		if refs[i].ID == id {
			refs[i] = ref
			replaced = true
			break
		}
	}
	if !replaced {
		refs = append(refs, ref)
	}
	if err := s.saveCustomReferences(ctx, refs); err != nil {
		s.adminRefsMu.Unlock()
		job.finish("failed", "自定义指纹保存失败")
		return
	}
	s.adminRefsMu.Unlock()
	if err := s.publishBank(ctx, candidate); err != nil {
		job.finish("failed", "归因库切换失败")
		return
	}
	meta := referenceMeta(ref)
	job.update(func() { job.Reference = &meta })
	job.finish("done", "")
}

func (s *Server) adminCaptureStatus(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	capture := s.currentCapture()
	if capture == nil || capture.ID != r.PathValue("id") {
		reply(w, 404, map[string]string{"error": "采集任务不存在"})
		return
	}
	reply(w, 200, capture.snapshot())
}

func (s *Server) adminCaptureCancel(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	capture := s.currentCapture()
	if capture == nil || capture.ID != r.PathValue("id") {
		reply(w, 404, map[string]string{"error": "采集任务不存在"})
		return
	}
	capture.mu.Lock()
	cancel := capture.cancel
	status := capture.Status
	capture.mu.Unlock()
	if status == "running" || status == "validating" {
		if cancel != nil {
			cancel()
		}
	}
	reply(w, 200, capture.snapshot())
}

func (s *Server) adminFingerprintDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		reply(w, 400, map[string]string{"error": "指纹 ID 无效"})
		return
	}
	s.adminRefsMu.Lock()
	defer s.adminRefsMu.Unlock()
	refs, err := s.loadCustomReferences()
	if err != nil {
		reply(w, 503, map[string]string{"error": "自定义指纹读取失败"})
		return
	}
	index := -1
	for i := range refs {
		if refs[i].ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		reply(w, 404, map[string]string{"error": "指纹不存在"})
		return
	}
	bank, err := fingerprint.CloneBank(s.currentBank())
	if err != nil {
		reply(w, 500, map[string]string{"error": "归因库复制失败"})
		return
	}
	if err := fingerprint.RemoveLocalReference(bank, id); err != nil && !strings.Contains(err.Error(), "not found") {
		reply(w, 409, map[string]string{"error": err.Error()})
		return
	}
	old := refs[index]
	refs = append(refs[:index], refs[index+1:]...)
	if err := s.saveCustomReferences(r.Context(), refs); err != nil {
		reply(w, 503, map[string]string{"error": "自定义指纹保存失败"})
		return
	}
	if err := s.publishBank(r.Context(), bank); err != nil {
		refs = append(refs, old)
		_ = s.saveCustomReferences(context.Background(), refs)
		reply(w, 500, map[string]string{"error": "归因库切换失败"})
		return
	}
	reply(w, 200, map[string]string{"ok": "1"})
}
