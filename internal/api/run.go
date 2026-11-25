package api

import (
	"github.com/kiss-kedaya/chanshui/internal/cachecheck"
	"github.com/kiss-kedaya/chanshui/internal/capability"
	"github.com/kiss-kedaya/chanshui/internal/claudecheck"
	"github.com/kiss-kedaya/chanshui/internal/fingerprint"
	"github.com/kiss-kedaya/chanshui/internal/gateway"

	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/kiss-kedaya/chanshui/internal/inject"
	"github.com/kiss-kedaya/chanshui/internal/integrity"
	"github.com/kiss-kedaya/chanshui/internal/iq"
	"github.com/kiss-kedaya/chanshui/internal/knowledge"
	"github.com/kiss-kedaya/chanshui/internal/thinking"
	"math"
	"math/rand"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

func redact(text, key string) string {
	if key != "" {
		text = strings.ReplaceAll(text, key, "[密钥已隐藏]")
	}
	return text
}
func sanitize(v any, key string) any {
	b, e := json.Marshal(v)
	if e != nil {
		return map[string]string{"error": "结果序列化失败"}
	}
	var out any
	if e = json.Unmarshal([]byte(redact(string(b), key)), &out); e != nil {
		return map[string]string{"error": "结果脱敏失败"}
	}
	return out
}

type thinkAdapter struct {
	g       gateway.Client
	timeout int
}

type deepAdapter struct {
	g       gateway.Client
	timeout int
}

func (a deepAdapter) PostMessagesRaw(ctx context.Context, body map[string]any, beta string, timeoutSeconds int) (claudecheck.Result, error) {
	raw, e := a.g.PostMessagesRaw(ctx, body, beta, timeoutSeconds)
	if e != nil {
		return claudecheck.Result{}, e
	}
	return claudecheck.Result{Status: raw.Status, Err: raw.Err, Answer: raw.Answer, StopReason: raw.StopReason, Transforms: raw.Transforms, InputTokens: raw.InputTokens, OutputTokens: raw.OutputTokens}, nil
}

func (a thinkAdapter) ProbeStrictness(ctx context.Context, model string) []thinking.StrictnessCheck {
	raw := a.g.ProbeStrictness(ctx, model, a.timeout)
	out := make([]thinking.StrictnessCheck, 0, len(raw))
	for _, check := range raw {
		out = append(out, thinking.StrictnessCheck{Name: check.Name, Status: check.Status, Pass: check.Pass, Detail: check.Detail})
	}
	return out
}

func (a thinkAdapter) CallMessages(ctx context.Context, q thinking.CallRequest) (thinking.Metrics, error) {
	req := gateway.CallRequest{
		Model: q.Model, MaxTokens: q.MaxTokens, Stream: q.Stream, Effort: q.Effort, Temperature: q.Temperature,
		StopOnInvalidSignature: q.StopOnInvalidSignature, TimeoutSeconds: a.timeout,
	}
	if q.Replay != nil {
		req.PriorUser = q.Replay.PriorUser
		req.PriorSignature = q.Replay.Signature
		req.Prompt = q.Replay.FollowUp
		// Without an active thinking block the upstream can skip signature
		// validation and accept a tampered signature. Keep adaptive on replay.
		req.Thinking = &gateway.ThinkingConfig{Type: "adaptive"}
	} else {
		if len(q.Messages) == 0 {
			return thinking.Metrics{}, fmt.Errorf("thinking request has no message")
		}
		req.Prompt = q.Messages[0].Content
		req.Stream = true
		if q.Thinking != nil {
			req.Thinking = &gateway.ThinkingConfig{Type: q.Thinking.Type, Display: q.Thinking.Display}
		}
	}
	m, e := a.g.CallMessages(ctx, req)
	return thinking.Metrics{Status: m.Status, Ms: m.Ms, Err: m.Err, Answer: m.Answer, ThinkLen: m.ThinkLen, SigLen: m.SigLen, ThinkingTokens: m.ThinkingTokens, ThinkingText: m.ThinkingText, Signature: m.Signature, StopReason: m.StopReason, RefusalCategory: m.RefusalCategory}, e
}

type behaviorAdapter struct {
	g       gateway.Client
	timeout int
}

func (a behaviorAdapter) Call(ctx context.Context, model, prompt string, max int) (string, error) {
	m, e := a.g.CallMessages(ctx, gateway.CallRequest{Model: model, Prompt: prompt, MaxTokens: max, Stream: true, TimeoutSeconds: a.timeout, SuppressDefaultEffort: true})
	if e != nil {
		return "", e
	}
	if m.Err != "" {
		return "", fmt.Errorf("%s", m.Err)
	}
	return m.Answer, nil
}
func (s *Server) numeric(ctx context.Context, g gateway.Client, q Request) (any, error) {
	bank := s.currentBank()
	if !familyHasNumericBaseline(bank, q.Model) {
		b, e := fingerprint.CompareBehavior(ctx, behaviorAdapter{g, q.Timeout}, q.Model, bank, fingerprint.Attribution{LowConfidence: true})
		if e != nil {
			return nil, e
		}
		return behaviorFallback(q.Model, bank, b), nil
	}
	const minSamples = 3
	const maxSamples = 10
	const sampleBatch = 3
	const confidenceTarget = 0.90
	const maxRequests = 10
	minValid := bank.Numeric.MinimumValidNumbers
	challenges := fingerprint.GenerateChallenges(maxRequests, rand.New(rand.NewSource(time.Now().UnixNano())))
	outs := make([]fingerprint.Output, 0, maxSamples)
	evidence := make([]any, 0, maxRequests)
	requests := 0
	firstError := ""
	for len(outs) < maxSamples && requests < len(challenges) {
		if ctx.Err() != nil {
			break
		}
		batch := maxSamples - len(outs)
		if batch > sampleBatch {
			batch = sampleBatch
		}
		if room := len(challenges) - requests; batch > room {
			batch = room
		}
		type slot struct {
			out       fingerprint.Output
			ev        any
			ok        bool
			err       string
			permanent bool
		}
		slots := make([]slot, batch)
		var batchWG sync.WaitGroup
		for i := 0; i < batch; i++ {
			ch := challenges[requests+i]
			batchWG.Add(1)
			go func(i int, ch fingerprint.Challenge) {
				defer batchWG.Done()
				// Messages prefers a buffered response for long numeric samples.
				// Responses/Chat stream by default; some gateways require it.
				maxTokens := 4096
				if ref := bank.NumericModel(q.Model); (ref != nil && ref.ReferenceSamples > 0) || integrity.ModelFamily(q.Model) == "deepseek" {
					maxTokens = 16384
				}
				m, e := g.CallMessages(ctx, gateway.CallRequest{Model: q.Model, Prompt: ch.Prompt, MaxTokens: maxTokens, Stream: g.ProtocolFor(q.Model) != "messages", TimeoutSeconds: q.Timeout, SuppressDefaultEffort: true})
				errText := m.Err
				if e != nil {
					errText = e.Error()
				}
				ok := errText == "" && fingerprint.Usable(m.Answer, ch.ExpectedCount, minValid)
				answer := m.Answer
				if !ok {
					answer = ""
				}
				slots[i] = slot{
					out:       fingerprint.Output{Text: answer, ExpectedCount: ch.ExpectedCount},
					ev:        map[string]any{"id": ch.ID, "expected_count": ch.ExpectedCount, "parsed_count": len(fingerprint.ParseNumbers(m.Answer)), "answer": m.Answer, "error": errText, "ms": m.Ms, "usable": ok, "http_status": m.Status, "attempts": m.Attempts, "protocol": m.Protocol},
					ok:        ok,
					err:       errText,
					permanent: errText != "" && gateway.PermanentRequestError(m.Status),
				}
			}(i, ch)
		}
		batchWG.Wait()
		requests += batch
		failedRequests, permanent := 0, false
		for _, sl := range slots {
			evidence = append(evidence, sl.ev)
			if sl.err != "" {
				failedRequests++
				if firstError == "" {
					firstError = sl.err
				}
			}
			permanent = permanent || sl.permanent
			if sl.ok && len(outs) < maxSamples {
				outs = append(outs, sl.out)
			}
		}
		// More challenge text cannot fix credentials or an exhausted upstream
		// outage. Keep the ten-challenge budget for successful but unusable output.
		if permanent || failedRequests == batch {
			break
		}
		if len(outs) >= minSamples {
			snapshot := fingerprint.Attribute(outs, bank)
			if snapshot.TopProb >= confidenceTarget {
				break
			}
		}
	}
	a := fingerprint.Attribute(outs, bank)
	res := []map[string]any{}
	for _, v := range a.Results {
		if v.Prob <= 0 {
			continue
		}
		res = append(res, map[string]any{"model": v.ModelID, "display": v.DisplayName, "probability": v.Prob, "similarity": v.JSSimilarity})
	}
	note := "概率只在数字库内的候选之间比较。从 3 个有效样本开始，顶部概率达到 90% 就停；否则最多取 10 个样本。归因和所选模型一致，这项才记满分。"
	if bank.NumericModel(q.Model) == nil {
		note += " 所选名称不是库内 ID，已按同家族候选直接比对。"
	}
	if ref := bank.NumericModel(a.TopModel); ref != nil && ref.ReferenceSamples > 0 {
		note += " 此候选为单一接口采样基线；候选权重沿用原库温度，未完成跨供应商概率校准，不能单独证明模型身份。"
	}
	family, familyName := a.Family, a.FamilyName
	if a.UsedOutputs == 0 {
		family, familyName = "unknown", "没有有效样本"
		note = fmt.Sprintf("%d 次里没有拿到有效数字样本，这项记 0 分。", requests)
	} else if a.UsedOutputs < minSamples {
		note = "有效样本不足 3 个，这项记 0 分。"
	}
	topDisplay := a.TopModel
	if ref := bank.NumericModel(a.TopModel); ref != nil && ref.DisplayName != "" {
		topDisplay = ref.DisplayName
	}
	out := map[string]any{"basis": "numeric", "family": family, "family_name": familyName, "family_prob": a.FamilyProb, "top_model": a.TopModel, "top_display": topDisplay, "top_prob": a.TopProb, "results": res, "used_outputs": a.UsedOutputs, "requests": requests, "low_confidence": a.LowConfidence || a.UsedOutputs < minSamples, "sample_min": minSamples, "sample_limit": maxSamples, "confidence_target": confidenceTarget, "confidence_reached": a.TopProb >= confidenceTarget, "evidence": evidence, "note": note}
	if ref := bank.NumericModel(a.TopModel); ref != nil && ref.ReferenceSamples > 0 {
		out["reference"] = map[string]any{"training_samples": ref.ReferenceSamples, "aliases": ref.Aliases, "calibration": "inherited_candidate_weights_not_recalibrated"}
	}
	if ctx.Err() != nil {
		return out, ctx.Err()
	}
	if a.UsedOutputs < minSamples {
		if firstError == "" {
			firstError = "上游返回的数字不足或格式无效"
		}
		return out, fmt.Errorf("指纹采样未完成（有效样本 %d/%d）：%s", a.UsedOutputs, minSamples, firstError)
	}
	if a.UsedOutputs > 0 && a.LowConfidence {
		b, e := fingerprint.CompareBehavior(ctx, behaviorAdapter{g, q.Timeout}, q.Model, bank, a)
		if e != nil {
			out["behavior_note"] = "行为探针失败：" + e.Error()
		} else {
			out["behavioral_top3"] = b.Top3
			out["behavior_note"] = "轻量行为探针只作参考，不参与这一项的分数。"
		}
	}
	return out, nil
}

func (s *Server) run(ctx context.Context, t *Task, q Request, selected map[string]bool) {
	defer t.cancel()
	started := time.Now()
	t.mu.Lock()
	previousMs, runs := int64(0), 0
	if t.Verdict.Timing != nil {
		previousMs, runs = t.Verdict.Timing.TotalMs, t.Verdict.Timing.Runs
	}
	t.Verdict.Timing = &Timing{StartedAt: started.UTC().Format(time.RFC3339Nano), TotalMs: previousMs, Runs: runs + 1}
	t.mu.Unlock()

	g, clientErr := s.client(q)
	var wg sync.WaitGroup
	jobWithSignal := func(id string, f func() (any, error), signal chan struct{}) {
		if selected != nil && !selected[id] {
			if signal != nil {
				close(signal)
			}
			return
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			start := time.Now()
			t.mu.Lock()
			for i := range t.Probes {
				if t.Probes[i].ID == id {
					t.Probes[i].Status = "running"
				}
			}
			t.mu.Unlock()
			if signal != nil {
				close(signal)
			}
			var result any
			var e error
			if clientErr != nil {
				e = clientErr
			} else {
				result, e = f()
			}
			if e != nil && result == nil {
				result = map[string]string{"error": redact(e.Error(), q.Key)}
			}
			result = sanitize(result, q.Key)
			if e != nil {
				if obj, ok := result.(map[string]any); ok {
					obj["error"] = redact(e.Error(), q.Key)
				}
			}
			t.mu.Lock()
			switch id {
			case "tools":
				t.Verdict.Tools = result
			case "web":
				t.Verdict.Web = result
			case "knowledge":
				t.Verdict.Knowledge = result
			case "cache":
				t.Verdict.Cache = result
			case "injection":
				t.Verdict.Injection = result
			case "fingerprint":
				t.Verdict.Fingerprint = result
			case "thinking":
				t.Verdict.Thinking = result
			case "vision":
				t.Verdict.Vision = result
			}
			for i := range t.Probes {
				if t.Probes[i].ID == id {
					p := &t.Probes[i]
					p.Ms = time.Since(start).Milliseconds()
					p.Status = "done"
					if e != nil {
						p.Status = "error"
						p.Error = redact(e.Error(), q.Key)
					}
					if ctx.Err() != nil {
						p.Status = "cancelled"
						p.Error = "检测已停止"
					}
				}
			}
			t.mu.Unlock()
			s.persist(t)
		}()
	}
	job := func(id string, f func() (any, error)) {
		jobWithSignal(id, f, nil)
	}
	fingerprintStarted := make(chan struct{})
	jobWithSignal("fingerprint", func() (any, error) { return s.numeric(ctx, g, q) }, fingerprintStarted)
	// Give the fingerprint probes the first upstream slots. The other
	// sections are still concurrent, but they start after fingerprint has
	// entered its first request batch.
	<-fingerprintStarted
	time.Sleep(25 * time.Millisecond)
	job("injection", func() (any, error) {
		v, e := inject.Measure(ctx, g, q.Model, q.Timeout)
		if e != nil {
			return map[string]any{"error": e.Error(), "points": v.Points}, e
		}
		return v, nil
	})
	if (q.Cache || (selected != nil && selected["cache"])) && !q.Conversation {
		job("cache", func() (any, error) {
			v, e := cachecheck.Measure(ctx, g, q.Model, q.Timeout)
			if e != nil {
				return v, e
			}
			return v, nil
		})
	}
	job("tools", func() (any, error) { return capability.Tools(ctx, g, q.Model, q.Timeout) })
	job("web", func() (any, error) {
		if integrity.ModelFamily(q.Model) == "deepseek" {
			return capability.DeepSeekSearch(ctx, g, q.Timeout)
		}
		return capability.Web(ctx, g, q.Model, q.Timeout)
	})
	job("knowledge", func() (any, error) {
		v, e := knowledge.Check(ctx, g, q.Model, q.Timeout)
		if e != nil {
			return v, e
		}
		return v, nil
	})
	if gateway.IsClaude(q.Model) {
		job("thinking", func() (any, error) {
			adapter := thinkAdapter{g, q.Timeout}
			v, e := thinking.Check(ctx, adapter, q.Model)
			if e != nil {
				return map[string]string{"error": e.Error(), "reason": "请求失败，不能据此判定思考被清空"}, e
			}
			return v, nil
		})
	}
	if q.Conversation && t.Source != "api" && clientErr == nil {
		// One thread, one upstream account: every turn waits for the previous
		// answer, which is why this mode is opt-in.
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.converse(ctx, t, q, g)
		}()
	} else {
		for _, question := range t.questions {
			if question.ID == "vision" {
				continue
			}
			item := iq.WithChartImage(question)
			job(item.ID, func() (any, error) {
				v, e := s.askQuestion(ctx, g, q, item)
				t.mu.Lock()
				t.answers[item.ID] = v
				t.mu.Unlock()
				return v, e
			})
		}
	}
	if item, ok := visionQuestion(t.questions); ok {
		job("vision", func() (any, error) {
			v, e := s.askQuestion(ctx, g, q, item)
			if v.Skipped {
				e = nil
			}
			return visionResult(item, v, e), e
		})
	}
	wg.Wait()
	if selected == nil || selected["thinking"] {
		t.mu.Lock()
		// Ask the model to think again only when the probe left us without a
		// usable summary or signature. A probe that returned a signature but no
		// summary is still upgraded by replaying the question that produced the
		// longest real reasoning text.
		reuse := !thinkingHasSignature(t.Verdict.Thinking) || !thinkingHasText(t.Verdict.Thinking)
		var seed thinking.Seed
		best := -1
		if reuse {
			for _, item := range t.questions {
				v := t.answers[item.ID]
				if strings.TrimSpace(v.Signature) == "" {
					continue
				}
				weight := len(v.Signature) + 2*len(v.ThinkingText)
				if weight <= best {
					continue
				}
				best = weight
				seed = thinking.Seed{
					ID: v.ID, Prompt: v.Prompt,
					Metrics: thinking.Metrics{
						Status: 200, Answer: v.Answer, ThinkingText: v.ThinkingText, Signature: v.Signature,
						ThinkLen: len(v.ThinkingText), SigLen: len(v.Signature), Ms: v.Ms,
						ThinkingTokens: v.Metrics.ThinkingTokens,
					},
				}
			}
		}
		t.mu.Unlock()
		if seed.ID != "" {
			if v, e := thinking.CheckWithSeed(ctx, thinkAdapter{g, q.Timeout}, q.Model, seed); e == nil {
				t.mu.Lock()
				t.Verdict.Thinking = sanitize(v, q.Key)
				t.mu.Unlock()
			}
		}
	}
	if q.ClaudeDeep && gateway.IsClaude(q.Model) {
		sig, prior := "", ""
		t.mu.Lock()
		if m, ok := t.Verdict.Thinking.(map[string]any); ok {
			sig, _ = m["signature"].(string)
			if src, _ := m["source"].(string); src != "" {
				if answer, ok := t.answers[src]; ok {
					prior = answer.Prompt
				}
			}
		}
		for i := range t.Probes {
			if t.Probes[i].ID == "claude_deep" {
				t.Probes[i].Status = "running"
			}
		}
		t.mu.Unlock()
		if sig != "" && prior == "" {
			prior = thinking.ProbePrompt()
		}
		deepStart := time.Now()
		v, e := claudecheck.Run(ctx, deepAdapter{g, q.Timeout}, q.Model, sig, prior, q.Timeout)
		t.mu.Lock()
		t.Verdict.ClaudeDeep = sanitize(v, q.Key)
		for i := range t.Probes {
			if t.Probes[i].ID != "claude_deep" {
				continue
			}
			t.Probes[i].Ms = time.Since(deepStart).Milliseconds()
			if e != nil {
				t.Probes[i].Status = "error"
				t.Probes[i].Error = redact(e.Error(), q.Key)
			} else {
				t.Probes[i].Status = "done"
			}
		}
		t.mu.Unlock()
	}
	t.mu.Lock()
	items := []iq.QItem{}
	lines := []QuestionResult{}
	complete := true
	answered := 0
	for _, item := range t.questions {
		if item.ID == "vision" {
			continue
		}
		v := t.answers[item.ID]
		if v.ID == "" {
			v = QuestionResult{ID: item.ID, Title: item.Title, Max: item.MaxScore, Error: "请求未完成", Tags: []string{}}
		}
		if v.Error != "" && !v.Skipped {
			complete = false
		}
		if v.Error == "" && !v.Skipped {
			answered++
		}
		lines = append(lines, v)
		if v.Skipped {
			continue
		}
		items = append(items, iq.QItem{ID: v.ID, Title: v.Title, Max: v.Max, Earned: v.Earned, Half: v.Half, Tags: v.Tags})
	}
	if len(lines) > 0 {
		score := iq.Score(items)
		var validScore any
		if answered > 0 {
			validScore = score.Score
		}
		t.Verdict.IQ = map[string]any{"score": validScore, "max": 100, "paper_max": score.Max, "complete": complete, "per_question": lines, "note": fmt.Sprintf("当前卷面 %d；半分仅在卷面展示，不进入百分制通过分。失败请求标记未完成，但已答题目照常计分，不伪装为模型答错。", score.Max)}
	}
	sections := selected
	if t.Source != "api" {
		sections = nil
	}
	t.Verdict.Integrity = buildIntegrity(t.Verdict, lines, q.Model, sections)
	t.Verdict.Total = buildTotalFor(t.Verdict, q.Model, sections)
	t.Status = "done"
	if ctx.Err() != nil {
		t.Status = "cancelled"
	}
	for _, p := range t.Probes {
		if p.Status == "error" && t.Status == "done" {
			t.Status = "partial"
		}
	}
	t.finished = time.Now()
	t.Verdict.Timing.FinishedAt = t.finished.UTC().Format(time.RFC3339Nano)
	t.Verdict.Timing.LastRunMs = time.Since(started).Milliseconds()
	t.Verdict.Timing.TotalMs = previousMs + t.Verdict.Timing.LastRunMs
	t.mu.Unlock()
	s.persist(t)
}

func thinkingHasSignature(v any) bool {
	m, ok := v.(map[string]any)
	if !ok {
		return false
	}
	s, _ := m["signature"].(string)
	return strings.TrimSpace(s) != ""
}

func thinkingHasText(v any) bool {
	m, ok := v.(map[string]any)
	if !ok {
		return false
	}
	s, _ := m["thinking_text"].(string)
	return strings.TrimSpace(s) != ""
}

// convTurn is one request inside the serial conversation. Read and Write come
// straight from the upstream usage block. Dropped marks a turn that had to
// rewrite the whole prefix, which is what a mid-session re-route looks like.
type convTurn struct {
	Index          int      `json:"index"`
	Kind           string   `json:"kind"`
	ID             string   `json:"id,omitempty"`
	Title          string   `json:"title,omitempty"`
	Read           int      `json:"read"`
	Write          int      `json:"write"`
	Input          int      `json:"input"`
	Output         int      `json:"output"`
	HitRate        *float64 `json:"hit_rate"`
	Expected       int      `json:"expected,omitempty"`
	ReadReported   bool     `json:"read_reported"`
	WriteReported  bool     `json:"write_reported"`
	InputReported  bool     `json:"input_reported"`
	OutputReported bool     `json:"output_reported"`
	Dropped        bool     `json:"dropped"`
	Ms             int64    `json:"ms"`
	Error          string   `json:"error,omitempty"`
}

type conversationReport struct {
	Rounds int        `json:"rounds"`
	Hits   int        `json:"hits"`
	Drops  int        `json:"drops"`
	Status string     `json:"status"`
	Pass   *bool      `json:"pass"`
	Reason string     `json:"reason"`
	Turns  []convTurn `json:"turns"`
}

// convSystem stays byte-identical for every turn; together with the first user
// line it is the cache prefix the whole thread reuses.
const convSystem = "禁止使用任何工具。回答问题直接给结论，不要展开推导过程；做推理题时在最后单独一行以「最终答案: 」开头给出短结论。"

// The 0.8/5m route first writes cache at 512 tokens; the 1.2/1h route shows
// the first write near 368 because about 144 hidden system tokens are deducted
// from usage. Pad the conversation above 512 so the first turn already writes
// instead of leaving the first two rows at zero. Haiku needs a larger block.
func convWarmup(model string) string {
	n := 40
	if strings.Contains(strings.ToLower(model), "haiku") {
		n = 160
	}
	var b strings.Builder
	b.WriteString("Fixed cache warm-up block; ignore it when answering.\n")
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "Cache warm-up sentence %03d stays byte-for-byte identical.\n", i)
	}
	return b.String()
}

// convSmallTalk is the unrelated chat dropped in after Q3 and after Q6. A
// gateway that classifies each request instead of continuing the session
// breaks the cached prefix exactly here.
var convSmallTalk = map[int][]string{
	2: {"先不聊题了，随便问问：你平时怎么安排一天的工作？", "换个话题，下午这个点你会建议喝点什么？"},
	5: {"先停一下，你更喜欢早上还是晚上做事？", "再插一句题外话，长时间工作怎么休息比较有效？"},
}

func convHit(read, total int) *float64 {
	if total <= 0 {
		return nil
	}
	v := math.Round(float64(read)/float64(total)*10000) / 100
	return &v
}

func convEcho(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return "（本轮未返回内容）"
	}
	if utf8.RuneCountInString(text) <= 1500 {
		return text
	}
	return string([]rune(text)[:1500])
}

// converse runs the reasoning paper as one serial chat thread: opener, nine
// questions, small talk in between. Serial on purpose: it is the only way to
// see whether the gateway keeps the same upstream account for the session.
func (s *Server) converse(ctx context.Context, t *Task, q Request, g gateway.Client) {
	system := []gateway.SystemBlock{{Text: convSystem + "\n\n" + convWarmup(q.Model)}}
	protocol := g.ProtocolFor(q.Model)
	var history []gateway.ConversationMessage
	report := conversationReport{Turns: []convTurn{}}
	prevTotal := 0
	prevWrote := false
	wroteBefore := false
	readFields := false

	setProbe := func(id, status string, ms int64, errText string) {
		t.mu.Lock()
		for i := range t.Probes {
			if t.Probes[i].ID != id {
				continue
			}
			t.Probes[i].Status = status
			t.Probes[i].Error = errText
			if status != "running" {
				t.Probes[i].Ms = ms
			}
		}
		t.mu.Unlock()
	}
	record := func(kind, id, title string, m gateway.Metrics, ms int64, errText string) {
		turn := convTurn{Index: len(report.Turns) + 1, Kind: kind, ID: id, Title: title, Read: m.CacheRead, Write: m.CacheCreation5m + m.CacheCreation1h, Input: m.InputTokens, Output: m.OutputTokens, Ms: ms, Error: errText,
			ReadReported: m.CacheReadReported, WriteReported: m.CacheWriteReported, InputReported: m.UsagePresent || m.InputTokens > 0, OutputReported: m.UsagePresent || m.OutputTokens > 0}
		turn.HitRate = convHit(turn.Read, turn.Input+turn.Read+turn.Write)
		// A cache chain is a prefix, not a boolean: this turn has to read the
		// prefix the last usable turn wrote. A flat 78-token read means only the
		// shared system block hit while the conversation was rewritten.
		usable := errText == "" && (m.CacheReadReported || m.CacheWriteReported || m.InputTokens > 0 || m.OutputTokens > 0)
		if usable && m.CacheReadReported {
			turn.Expected = prevTotal
			if prevTotal > 0 {
				// Under-read drops the tail; a jump over the previous prefix means
				// the gateway landed on an account with a different cache.
				if turn.Read*4 < prevTotal*3 || (prevWrote && turn.Read*4 > prevTotal*5) {
					turn.Dropped = true
				}
			} else if wroteBefore && turn.Read == 0 && turn.Write > 64 {
				turn.Dropped = true
			}
			prevTotal = turn.Read + turn.Write
			prevWrote = turn.Write > 64
			if turn.Write > 64 {
				wroteBefore = true
			}
			readFields = true
		} else if usable && m.CacheWriteReported && turn.Write > 64 {
			wroteBefore = true
		}
		report.Turns = append(report.Turns, turn)
	}
	call := func(prompt string, max int, thinking *gateway.ThinkingConfig) (gateway.Metrics, string, int64) {
		req := gateway.CallRequest{Model: q.Model, Prompt: prompt, MaxTokens: max, Stream: true, TimeoutSeconds: q.Timeout, Thinking: thinking,
			History: append([]gateway.ConversationMessage(nil), history...), System: system}
		if protocol == "messages" {
			cachecheck.MarkConversation(&req)
		}
		start := time.Now()
		m, e := g.CallMessages(ctx, req)
		if e != nil {
			m.Err = e.Error()
		}
		text := strings.TrimSpace(m.Answer)
		if text == "" && gateway.IsClaude(q.Model) && m.StopReason != "max_tokens" && m.StopReason != "length" {
			text = strings.TrimSpace(m.ThinkingText)
		}
		// Same turn retried without thinking keeps the thread intact; a fresh
		// request would drop the prefix this mode is measuring.
		if text == "" && m.Err == "" && m.Status == 200 {
			plain := req
			plain.Thinking = nil
			plain.SuppressDefaultEffort = true
			if m2, e2 := g.CallMessages(ctx, plain); e2 == nil && m2.Err == "" {
				m = m2
				text = strings.TrimSpace(m2.Answer)
				if text == "" && gateway.IsClaude(q.Model) {
					text = strings.TrimSpace(m2.ThinkingText)
				}
			}
		}
		return m, text, time.Since(start).Milliseconds()
	}
	turn := func(kind, id, title, prompt string, max int, thinking *gateway.ThinkingConfig) (gateway.Metrics, string) {
		m, text, ms := call(prompt, max, thinking)
		record(kind, id, title, m, ms, redact(m.Err, q.Key))
		history = append(history,
			gateway.ConversationMessage{Role: "user", Content: prompt},
			gateway.ConversationMessage{Role: "assistant", Content: convEcho(text)})
		if m.Protocol != "" {
			protocol = m.Protocol
			g.PinProtocol(m.Protocol)
		}
		return m, text
	}

	turn("opener", "", "开场白", "你好，你能帮我做什么？", 256, nil)
	for i, item := range t.questions {
		if item.ID == "vision" {
			continue
		}
		if ctx.Err() != nil {
			break
		}
		setProbe(item.ID, "running", 0, "")
		start := time.Now()
		m, text := turn("question", item.ID, item.Title, item.Prompt, 8192, iqThinking(q.Model))
		var e error
		if m.Err != "" {
			e = fmt.Errorf("%s", m.Err)
		}
		v, _ := s.finishQuestion(ctx, g, q, item, m, e, text)
		t.mu.Lock()
		t.answers[item.ID] = v
		t.mu.Unlock()
		status := "done"
		if v.Error != "" && !v.Skipped {
			status = "error"
		}
		setProbe(item.ID, status, time.Since(start).Milliseconds(), redact(v.Error, q.Key))
		s.persist(t)
		for _, extra := range convSmallTalk[i] {
			if ctx.Err() != nil {
				break
			}
			turn("smalltalk", "", "", extra, 256, nil)
			s.persist(t)
		}
	}
	t.mu.Lock()
	for i := range t.Probes {
		if t.Probes[i].Status == "queued" && (strings.HasPrefix(t.Probes[i].ID, "Q") || t.Probes[i].ID == "vision") {
			t.Probes[i].Status = "cancelled"
			t.Probes[i].Error = "检测已停止"
		}
	}
	t.mu.Unlock()

	report.Rounds = len(report.Turns)
	for _, item := range report.Turns {
		if item.Expected > 0 && !item.Dropped {
			report.Hits++
		}
		if item.Dropped {
			report.Drops++
		}
	}
	switch {
	case !readFields:
		report.Status = "unknown"
		report.Reason = "上游没有缓存读取，无法判断。"
	case report.Drops > 0:
		fail := false
		report.Pass = &fail
		report.Status = "mismatch"
		report.Reason = fmt.Sprintf("%d 轮未接上前缀，疑似中途换号。", report.Drops)
	case report.Hits >= 2:
		pass := true
		report.Pass = &pass
		report.Status = "pass"
		report.Reason = fmt.Sprintf("%d 轮接上前缀。", report.Hits)
	default:
		report.Status = "unknown"
		report.Reason = "前缀还没进入缓存。"
	}
	t.mu.Lock()
	t.Verdict.Conversation = sanitize(report, q.Key)
	// Unknown cache evidence must not make the cache section applicable and
	// score 0; leave it out so the total reweights like an unchecked probe.
	if report.Pass != nil {
		t.Verdict.Cache = sanitize(conversationCache(report, protocol), q.Key)
	}
	t.mu.Unlock()
	s.persist(t)
}

// conversationCache shapes the serial thread like the old six-turn cache probe
// so the cache card can reuse it without a second network run.
func conversationCache(report conversationReport, protocol string) cachecheck.Result {
	res := cachecheck.Result{Status: report.Status, Pass: report.Pass, Reason: report.Reason, Protocol: protocol, Samples: []cachecheck.Sample{}, Transitions: []cachecheck.Transition{}}
	for _, turn := range report.Turns {
		s := cachecheck.Sample{Index: turn.Index, Read: turn.Read, Write: turn.Write, Input: turn.Input, Output: turn.Output,
			Total: turn.Input + turn.Read + turn.Write, ReadReported: turn.ReadReported, WriteReported: turn.WriteReported,
			InputReported: turn.InputReported, OutputReported: turn.OutputReported, Attempts: 1, HitRate: turn.HitRate, Error: turn.Error}
		res.Samples = append(res.Samples, s)
		res.TotalRead += s.Read
		res.TotalOutput += s.Output
	}
	for i := 1; i < len(res.Samples); i++ {
		prev, cur := res.Samples[i-1], res.Samples[i]
		if !prev.ReadReported {
			continue
		}
		expected := prev.Read + prev.Write
		tr := cachecheck.Transition{From: prev.Index, To: cur.Index, Expected: expected, Actual: cur.Read, Delta: cur.Read - expected}
		if expected > 0 && cur.ReadReported {
			pass := !report.Turns[i].Dropped
			tr.Pass = &pass
		}
		res.Transitions = append(res.Transitions, tr)
	}
	if n := len(res.Samples); n >= 2 {
		prev, last := res.Samples[n-2], res.Samples[n-1]
		if prev.ReadReported {
			expected := prev.Read + prev.Write
			res.Expected, res.Actual, res.Delta = expected, last.Read, last.Read-expected
			res.ThirdReadBaseline = expected
			if expected > 0 {
				ratio := float64(last.Read) / float64(expected) * 100
				res.ThirdReadRatio = &ratio
			}
		}
		res.First, res.Second = &prev, &last
		res.Third = &last
	}
	return res
}

// askQuestion runs one IQ or vision item. It is shared so the standalone
// vision section never forks the judging and self-score path.
func (s *Server) askQuestion(ctx context.Context, g gateway.Client, q Request, item iq.Question) (QuestionResult, error) {
	prompt := item.Prompt
	if item.ID == "vision" {
		// 题面已经画在图上，再发一遍文字等于把题泄给看不到图的接口。
		prompt = ""
	}
	// 截断不等于失败：先按已经吐出的正文判分；整段只剩思考或空白时，
	// 去掉 thinking 与 effort 再要一次短答，别把 8192 全花在推理上。
	truncated := func(m gateway.Metrics) bool {
		return m.StopReason == "max_tokens" || m.StopReason == "length"
	}
	ask := func(plain bool) (gateway.Metrics, error) {
		req := gateway.CallRequest{Model: q.Model, Prompt: prompt, Images: questionImages(item), System: []gateway.SystemBlock{{Text: "禁止使用任何工具。直接给出结论，不要展开推导过程；回答最后单独一行：最终答案: <短结论>。"}}, Thinking: iqThinking(q.Model), MaxTokens: 8192, Stream: true, TimeoutSeconds: q.Timeout, Validate: func(m gateway.Metrics) error {
			if !truncated(m) && strings.TrimSpace(m.Answer) == "" && (!gateway.IsClaude(q.Model) || strings.TrimSpace(m.ThinkingText) == "") {
				return fmt.Errorf("empty response")
			}
			return nil
		}}
		if plain {
			req.Thinking = nil
			req.Effort = ""
			req.SuppressDefaultEffort = true
			if gateway.IsDeepSeek(q.Model) {
				req.Effort = "none"
			}
		} else if item.ID == "vision" {
			// 识图题显式 high；上游不认 effort 时由 CallMessages 回退到不指定。
			req.Effort = "high"
			req.SuppressDefaultEffort = false
		}
		return g.CallMessages(ctx, req)
	}
	m, e := ask(false)
	if e == nil && m.Err != "" {
		e = fmt.Errorf("%s", m.Err)
	}
	// 截断的思考不是答案，别拿它冒充识图结果。
	text := m.Answer
	if strings.TrimSpace(text) == "" && gateway.IsClaude(q.Model) && !truncated(m) {
		text = m.ThinkingText
	}
	if strings.TrimSpace(text) == "" && (e == nil || m.Status == 200) {
		if m2, e2 := ask(true); e2 == nil && m2.Status == 200 {
			m, e = m2, nil
			if m.Err != "" {
				e = fmt.Errorf("%s", m.Err)
			}
			text = m.Answer
			if strings.TrimSpace(text) == "" && !truncated(m) {
				text = m.ThinkingText
			}
		}
	}
	if e == nil && strings.TrimSpace(text) == "" {
		if truncated(m) {
			e = fmt.Errorf("输出被 max_tokens 截断，未作为错误答案判分")
		} else {
			e = fmt.Errorf("正文与思考均为空")
		}
	}
	return s.finishQuestion(ctx, g, q, item, m, e, text)
}

// finishQuestion grades one answer and shapes the report row. Shared so the
// conversation mode scores with the same rules as the parallel paper.
func (s *Server) finishQuestion(ctx context.Context, g gateway.Client, q Request, item iq.Question, m gateway.Metrics, e error, text string) (QuestionResult, error) {
	if !gateway.IsClaude(q.Model) {
		m.ThinkingText = ""
		m.Signature = ""
		m.SSEFrames = nil
	}
	m.SSEFrames = nil
	v := QuestionResult{ID: item.ID, Title: item.Title, Max: item.MaxScore, Prompt: item.Prompt, Expected: item.Answer, Answer: m.Answer, ThinkingText: m.ThinkingText, Signature: m.Signature, Ms: m.Ms, Tags: []string{}, Metrics: m}
	if len(item.ImagePNG) > 0 {
		v.Image = "data:image/png;base64," + base64.StdEncoding.EncodeToString(item.ImagePNG)
	}
	// Text evidence already lives on the question, not twice in metrics.
	v.Metrics.Answer, v.Metrics.ThinkingText, v.Metrics.Signature = "", "", ""
	if e != nil {
		v.Error = redact(e.Error(), q.Key)
		if item.ID == "vision" && unsupportedImage(v.Error) {
			v.Skipped = true
			v.Error = "该模型不接受图片输入，识图项记 0 分"
		}
	} else {
		earned, half, tags := iq.Judge(item, text)
		// The chart answer is an exact integer; a self-grade would only make
		// the correctness half of the vision score softer.
		if item.ID != "vision" && earned == 0 {
			grade, ge := g.CallMessages(ctx, gateway.CallRequest{
				Model: q.Model,
				History: []gateway.ConversationMessage{
					{Role: "user", Content: item.Prompt},
					{Role: "assistant", Content: text},
				},
				Prompt:         fmt.Sprintf("评分开始。标准答案是：%s。本题满分 %d。对照你上一轮的回答，完全符合只返回 %d，部分符合返回中间的整数，完全不对返回最低分。返回的是本题得分，不要把标准答案再写一遍。只返回一个整数。", item.Answer, item.MaxScore, item.MaxScore),
				System:         []gateway.SystemBlock{{Text: "沿用上一轮对话。现在只给自己的上一轮回答打分，只返回一个整数。"}},
				Thinking:       gradeThinking(q.Model),
				MaxTokens:      256,
				Stream:         true,
				TimeoutSeconds: q.Timeout,
			})
			if ge == nil && grade.Err == "" {
				scored, ok := iq.ParseSelfScore(grade.Answer, item.MaxScore)
				if !ok && gateway.IsClaude(q.Model) {
					scored, ok = iq.ParseSelfScore(grade.ThinkingText, item.MaxScore)
				}
				if ok {
					earned = scored
					half = false
					if scored > 0 {
						tags = []string{"自评"}
					} else {
						tags = nil
					}
				}
			}
		}
		v.Earned = earned
		v.Half = half
		if tags == nil {
			tags = []string{}
		}
		v.Tags = tags
	}
	raw, _ := json.Marshal(sanitize(v, q.Key))
	_ = json.Unmarshal(raw, &v)
	return v, e
}

func visionQuestion(list []iq.Question) (iq.Question, bool) {
	for _, q := range list {
		if q.ID == "vision" {
			return iq.WithChartImage(q), true
		}
	}
	return iq.Question{}, false
}

// visionResult scores the two dimensions separately: whether the image was
// read at all (50) and whether the closing digits were summed correctly (50).
func visionResult(item iq.Question, v QuestionResult, e error) map[string]any {
	v.Prompt = "" // 题面只在图上，报告里不再回显文字。
	recognized := e == nil && v.Error == "" && !v.Skipped && iq.NumericAnswer(v.Answer)
	seen := 0
	if recognized {
		seen = 50
	}
	answer := 0
	if item.MaxScore > 0 {
		answer = (v.Earned*50 + item.MaxScore/2) / item.MaxScore
	}
	out := map[string]any{
		"score": seen + answer, "max": 100,
		"recognized": recognized, "recognized_score": seen, "recognized_max": 50,
		"answer_score": answer, "answer_max": 50,
		"correct": v.Earned == item.MaxScore && !v.Skipped,
		"half":    v.Half, "earned": v.Earned, "question_max": item.MaxScore,
		"skipped": v.Skipped, "question": v,
		"note": "识图 50 分看图片是否被接受并读出一个整数；答对 50 分按 40 根 K 线收盘价的个位数字之和判分，误差 ±2 记一半。",
	}
	if v.Error != "" {
		out["error"] = v.Error
	}
	return out
}

func buildIntegrity(v Verdict, results []QuestionResult, model string, selected map[string]bool) any {
	input := integrity.Inputs{Protocol: gateway.Protocol(model), ThinkingApply: gateway.IsClaude(model), InjectionApply: true}
	if selected != nil {
		input.InjectionApply = selected["injection"]
		input.ThinkingApply = input.ThinkingApply && selected["thinking"]
	}
	if m, ok := v.Injection.(map[string]any); ok {
		if n, ok := asInt(m["score"]); ok {
			input.InjectionScore = &n
		}
	}
	if m, ok := v.Cache.(map[string]any); ok {
		input.CacheApplicable = true
		input.CachePass, _ = asBoolPtr(m["pass"])
	}
	if m, ok := v.Tools.(map[string]any); ok {
		input.ToolsApplicable = true
		input.ToolsPass, _ = asBoolPtr(m["pass"])
	}
	if m, ok := v.Web.(map[string]any); ok {
		input.WebApplicable = m["status"] != "unknown"
		input.WebPass, _ = asBoolPtr(m["pass"])
	}
	if m, ok := v.Thinking.(map[string]any); ok {
		if n, ok := asInt(m["score"]); ok {
			input.ThinkingScore = &n
		}
	}
	stream := false
	streamKnown := false
	for _, result := range results {
		if result.Error == "" {
			streamKnown = true
			stream = result.Metrics.StreamComplete
			break
		}
	}
	if streamKnown {
		input.StreamPass = &stream
	}
	return integrity.Calculate(input)
}

func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}
func asBoolPtr(v any) (*bool, bool) {
	if v == nil {
		return nil, false
	}
	b, ok := v.(bool)
	if !ok {
		return nil, false
	}
	return &b, true
}

// iqThinking 给“答题”这一步挑 thinking 配置：Claude 走 adaptive+摘要。
func iqThinking(model string) *gateway.ThinkingConfig {
	if gateway.IsClaude(model) {
		return &gateway.ThinkingConfig{Type: "adaptive", Display: "summarized"}
	}
	return nil
}

// gradeThinking 给“自评打分”这一步挑 thinking 配置。
//
// 以前这里固定发 thinking.type=disabled，但 claude-opus-5-5 明确拒绝该值
// （400: "thinking.type.disabled" is not supported for this model），导致该题
// 自评环节报错、分数拿不回来。实测 claude-sonnet-5 / sonnet-4-5 / opus-4-6 /
// haiku-4-5 都接受 disabled，唯独 opus-5-5 不行。
//
// 打分只要一个整数，不需要思考。统一不传 thinking 字段，让上游用默认行为：
// 实测五个模型“不传”全部 200，是最稳的一档。
func gradeThinking(model string) *gateway.ThinkingConfig {
	return nil
}

func buildTotal(v Verdict, model string) integrity.Total {
	return buildTotalFor(v, model, nil)
}

func buildTotalFor(v Verdict, model string, selected map[string]bool) integrity.Total {
	fpPoints, fpKnown := 0, false
	if m, ok := v.Fingerprint.(map[string]any); ok {
		errText, _ := m["error"].(string)
		if errText == "" {
			fpKnown = true
			family, _ := m["family"].(string)
			top, _ := m["top_model"].(string)
			used, _ := asInt(m["used_outputs"])
			// A near-tie between two candidates is not evidence of dilution.
			// Without this guard gpt-6.1-sol and gpt-6-astra, whose endpoints
			// produce near-identical number distributions, marked each other as
			// diluted on roughly one in six live draws.
			lowConfidence, _ := m["low_confidence"].(bool)
			got := integrity.FingerprintFamily(family)
			basis, _ := m["basis"].(string)
			prob, _ := m["top_prob"].(float64)
			// Only a numeric baseline that pins a different model outright may
			// cost points. Behavioral attribution, close candidates and thin
			// samples keep the section whole, so it never scores on guesswork.
			confirmed := !lowConfidence && integrity.Confirmed(basis, prob) &&
				integrity.FingerprintState(model, family, top) != integrity.FingerprintMatch
			if used >= 3 && got != "" && got != "unknown" {
				if confirmed {
					fpPoints = 0
				} else {
					fpPoints = 10
				}
			}
		}
	}
	iqPoints, iqKnown := 0, false
	if m, ok := v.IQ.(map[string]any); ok {
		iqKnown = true
		if n, ok := asInt(m["score"]); ok {
			iqPoints = n
		}
	}
	visionApply := false
	if selected == nil {
		visionApply = v.Vision != nil
	} else {
		visionApply = selected["vision"]
	}
	visionPoints, visionKnown := 0, false
	if m, ok := v.Vision.(map[string]any); ok {
		visionKnown = true
		if errText, _ := m["error"].(string); errText == "" {
			if n, ok := asInt(m["score"]); ok {
				visionPoints = n
			}
		}
	}
	if !visionApply {
		visionKnown, visionPoints = false, 0
	}
	injApply := true
	injPoints, injKnown := 0, false
	if m, ok := v.Injection.(map[string]any); ok {
		errText, _ := m["error"].(string)
		if errText == "" {
			if n, ok := asInt(m["score"]); ok {
				injKnown = true
				injPoints = n
			} else {
				// An unreliable two-sample fit has no score. Do not turn it
				// into a known 0; leave the section out and reweight.
				injApply = false
			}
		} else {
			injKnown = true
		}
	}
	toolPoints, toolKnown := boolPart(v.Tools)
	webApply, webPoints, webKnown := webPart(v.Web)
	thinkApply := gateway.IsClaude(model)
	thinkPoints, thinkKnown, thinkMax := 0, false, 5
	if thinkApply {
		if m, ok := v.Thinking.(map[string]any); ok {
			thinkKnown = true
			if n, ok := asInt(m["max_score"]); ok && n > 0 {
				thinkMax = n
			}
			errText, _ := m["error"].(string)
			if errText == "" {
				if n, ok := asInt(m["score"]); ok {
					thinkPoints = n
				}
			}
		}
	}
	cacheApply := v.Cache != nil
	cachePoints, cacheKnown := 0, false
	if cacheApply {
		cachePoints, cacheKnown = boolPart(v.Cache)
	}
	want := func(id string) bool { return selected == nil || selected[id] }
	if !want("fingerprint") {
		fpKnown, fpPoints = false, 0
	}
	if !want("iq") {
		iqKnown, iqPoints = false, 0
	}
	if !want("injection") {
		injApply = false
		injKnown, injPoints = false, 0
	}
	if !want("tools") {
		toolKnown, toolPoints = false, 0
	}
	if !want("web") {
		webApply = false
	}
	if !want("thinking") {
		thinkApply = false
	}
	if selected != nil {
		cacheApply = selected["cache"]
	}
	return integrity.Compose([]integrity.Part{
		{Key: "fingerprint", Label: "模型指纹归因", Applicable: want("fingerprint"), Known: fpKnown, Points: fpPoints, MaxPoints: 10},
		{Key: "iq", Label: "推理表现", Applicable: want("iq"), Known: iqKnown, Points: iqPoints, MaxPoints: 100},
		{Key: "vision", Label: "识图能力", Applicable: visionApply, Known: visionKnown, Points: visionPoints, MaxPoints: 100},
		{Key: "injection", Label: "注入大小", Applicable: injApply, Known: injKnown, Points: injPoints, MaxPoints: 10},
		{Key: "tools", Label: "工具调用", Applicable: want("tools"), Known: toolKnown, Points: toolPoints, MaxPoints: 1},
		{Key: "web", Label: "联网检测", Applicable: webApply, Known: webKnown, Points: webPoints, MaxPoints: 1},
		{Key: "thinking", Label: "思维链回传", Applicable: thinkApply, Known: thinkKnown, Points: thinkPoints, MaxPoints: thinkMax},
		{Key: "cache", Label: "缓存读写", Applicable: cacheApply, Known: cacheKnown, Points: cachePoints, MaxPoints: 1},
	})
}

func boolPart(v any) (int, bool) {
	m, ok := v.(map[string]any)
	if !ok {
		return 0, false
	}
	if errText, _ := m["error"].(string); errText != "" {
		return 0, true
	}
	pass, ok := asBoolPtr(m["pass"])
	if !ok || pass == nil {
		return 0, true
	}
	if *pass {
		return 1, true
	}
	return 0, true
}

func webPart(v any) (bool, int, bool) {
	m, ok := v.(map[string]any)
	if !ok {
		return true, 0, false
	}
	status, _ := m["status"].(string)
	protocol, _ := m["protocol"].(string)
	_, hasPass := asBoolPtr(m["pass"])
	if !hasPass && (status == "unsupported" || (protocol == "deepseek_search" && status == "unknown")) {
		return false, 0, false
	}
	if status == "unknown" && !hasPass && protocol == "chat" {
		return false, 0, false
	}
	points, known := boolPart(v)
	return true, points, known
}

// questionImages hands Q10's rendered chart to the gateway.
func questionImages(q iq.Question) []gateway.Image {
	if len(q.ImagePNG) == 0 {
		return nil
	}
	return []gateway.Image{{MIME: "image/png", Data: q.ImagePNG}}
}

// unsupportedImage recognises an upstream that rejects inline images, so the
// vision item is scored "not applicable" instead of nulling the whole IQ
// section on text-only models.
// shortcut: keyword match on the 400/422 detail; revisit if upstreams start
// returning structured error codes.
func unsupportedImage(msg string) bool {
	lower := strings.ToLower(msg)
	if !strings.Contains(lower, "http 400") && !strings.Contains(lower, "http 422") {
		return false
	}
	for _, kw := range []string{"image", "vision", "multimodal", "图片"} {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}
