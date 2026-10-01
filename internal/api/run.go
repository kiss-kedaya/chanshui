package api

import (
	"chanzhushui/internal/cachecheck"
	"chanzhushui/internal/capability"
	"chanzhushui/internal/fingerprint"
	"chanzhushui/internal/gateway"

	"chanzhushui/internal/inject"
	"chanzhushui/internal/integrity"
	"chanzhushui/internal/iq"
	"chanzhushui/internal/knowledge"
	"chanzhushui/internal/thinking"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"
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

func (a thinkAdapter) CallMessages(ctx context.Context, q thinking.CallRequest) (thinking.Metrics, error) {
	m, e := a.g.CallMessages(ctx, gateway.CallRequest{Model: q.Model, MaxTokens: q.MaxTokens, Prompt: q.Messages[0].Content, Thinking: &gateway.ThinkingConfig{Type: q.Thinking.Type, Display: q.Thinking.Display}, Stream: true, TimeoutSeconds: a.timeout})
	return thinking.Metrics{Status: m.Status, Ms: m.Ms, Err: m.Err, Answer: m.Answer, ThinkLen: m.ThinkLen, SigLen: m.SigLen, ThinkingTokens: m.ThinkingTokens, ThinkingText: m.ThinkingText, Signature: m.Signature}, e
}

type behaviorAdapter struct {
	g       gateway.Client
	timeout int
}

func (a behaviorAdapter) Call(ctx context.Context, model, prompt string, max int) (string, error) {
	m, e := a.g.CallMessages(ctx, gateway.CallRequest{Model: model, Prompt: prompt, MaxTokens: max, Stream: true, TimeoutSeconds: a.timeout})
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
	if !gateway.IsClaude(q.Model) && !strings.HasPrefix(strings.ToLower(q.Model), "gpt-") {
		b, e := fingerprint.CompareBehavior(ctx, behaviorAdapter{g, q.Timeout}, q.Model, bank, fingerprint.Attribution{LowConfidence: true})
		if e != nil {
			return nil, e
		}
		return map[string]any{"family": "unknown", "family_name": "未建立数字基线", "family_prob": 0, "top_model": "", "top_prob": 0, "results": []any{}, "behavioral_top3": b.Top3, "used_outputs": 0, "low_confidence": true, "note": "当前数字库仅覆盖GPT/Claude，本模型只给出探索性行为参考，不强制归因为库内模型。", "evidence": []any{}}, nil
	}
	chs := fingerprint.GenerateChallenges(3, rand.New(rand.NewSource(time.Now().UnixNano())))
	outs := make([]fingerprint.Output, 3)
	evidence := make([]any, 3)
	var wg sync.WaitGroup
	for i, ch := range chs {
		wg.Add(1)
		go func(i int, ch fingerprint.Challenge) {
			defer wg.Done()
			m, e := g.CallMessages(ctx, gateway.CallRequest{Model: q.Model, Prompt: ch.Prompt, MaxTokens: 4096, Stream: true, TimeoutSeconds: q.Timeout})
			errText := m.Err
			if e != nil {
				errText = e.Error()
			}
			outs[i] = fingerprint.Output{Text: m.Answer, ExpectedCount: ch.ExpectedCount}
			evidence[i] = map[string]any{"id": ch.ID, "expected_count": ch.ExpectedCount, "parsed_count": len(fingerprint.ParseNumbers(m.Answer)), "answer": m.Answer, "error": errText, "ms": m.Ms}
			if errText != "" {
				outs[i].Text = ""
			}
		}(i, ch)
	}
	wg.Wait()
	a := fingerprint.Attribute(outs, bank)
	if a.UsedOutputs == 0 {
		return map[string]any{"error": "没有可用的数字序列，查看探针证据", "evidence": evidence}, fmt.Errorf("没有可用数字序列")
	}
	res := []map[string]any{}
	for _, v := range a.Results {
		res = append(res, map[string]any{"model": v.ModelID, "probability": v.Prob, "similarity": v.JSSimilarity})
	}
	out := map[string]any{"family": a.Family, "family_name": a.FamilyName, "family_prob": a.FamilyProb, "top_model": a.TopModel, "top_prob": a.TopProb, "results": res, "used_outputs": a.UsedOutputs, "low_confidence": a.LowConfidence, "evidence": evidence, "note": "概率仅相对于数字库内候选；不是身份证明，库外模型也可能高置信命中。"}
	if a.LowConfidence {
		b, e := fingerprint.CompareBehavior(ctx, behaviorAdapter{g, q.Timeout}, q.Model, bank, a)
		if e != nil {
			out["behavior_note"] = "行为探针失败：" + e.Error()
		} else {
			out["behavioral_top3"] = b.Top3
			out["behavior_note"] = "轻量行为探针与原基线并非等价实验，仅作探索参考，不参与概率或评分。"
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
	job := func(id string, f func() (any, error)) {
		if selected != nil && !selected[id] {
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
	job("injection", func() (any, error) {
		v, e := inject.Measure(ctx, g, q.Model, q.Timeout)
		if e != nil {
			return map[string]any{"error": e.Error(), "points": v.Points}, e
		}
		return v, nil
	})
	job("cache", func() (any, error) {
		v, e := cachecheck.Measure(ctx, g, q.Model, q.Timeout)
		if e != nil {
			return v, e
		}
		return v, nil
	})
	job("fingerprint", func() (any, error) { return s.numeric(ctx, g, q) })
	job("tools", func() (any, error) { return capability.Tools(ctx, g, q.Model, q.Timeout) })
	job("web", func() (any, error) { return capability.Web(ctx, g, q.Model, q.Timeout) })
	job("knowledge", func() (any, error) {
		v, e := knowledge.Check(ctx, g, q.Model, q.Timeout)
		if e != nil {
			return v, e
		}
		return v, nil
	})
	if gateway.IsClaude(q.Model) {
		job("thinking", func() (any, error) {
			v, e := thinking.Check(ctx, thinkAdapter{g, q.Timeout}, q.Model)
			if e != nil {
				return map[string]string{"error": e.Error(), "reason": "请求失败，不能据此判定思考被清空"}, e
			}
			return v, nil
		})
	}
	for _, question := range t.questions {
		item := question
		job(item.ID, func() (any, error) {
			m, e := g.CallMessages(ctx, gateway.CallRequest{Model: q.Model, Prompt: item.Prompt, System: []gateway.SystemBlock{{Text: "禁止使用任何工具。请认真推导并核验；回答最后单独一行：最终答案: <短结论>。"}}, Thinking: iqThinking(q.Model), MaxTokens: 8192, Stream: true, TimeoutSeconds: q.Timeout, Validate: func(m gateway.Metrics) error {
				if m.StopReason == "max_tokens" || m.StopReason == "length" {
					return fmt.Errorf("max_tokens: response truncated")
				}
				if strings.TrimSpace(m.Answer) == "" && (!gateway.IsClaude(q.Model) || strings.TrimSpace(m.ThinkingText) == "") {
					return fmt.Errorf("empty response")
				}
				return nil
			}})
			if e == nil && m.Err != "" {
				e = fmt.Errorf("%s", m.Err)
			}
			if e == nil && m.StopReason == "max_tokens" {
				e = fmt.Errorf("输出被 max_tokens 截断，未作为错误答案判分")
			}
			text := m.Answer
			if strings.TrimSpace(text) == "" {
				if gateway.IsClaude(q.Model) {
					text = m.ThinkingText
				}
			}
			if e == nil && strings.TrimSpace(text) == "" {
				e = fmt.Errorf("正文与思考均为空")
			}
			if !gateway.IsClaude(q.Model) {
				m.ThinkingText = ""
				m.Signature = ""
				m.SSEFrames = nil
			}
			m.SSEFrames = nil
			v := QuestionResult{ID: item.ID, Title: item.Title, Max: item.MaxScore, Prompt: item.Prompt, Expected: item.Answer, Answer: m.Answer, ThinkingText: m.ThinkingText, Signature: m.Signature, Ms: m.Ms, Tags: []string{}, Metrics: m}
			// Text evidence already lives on the question, not twice in metrics.
			v.Metrics.Answer, v.Metrics.ThinkingText, v.Metrics.Signature = "", "", ""
			if e != nil {
				v.Error = redact(e.Error(), q.Key)
			} else {
				v.Earned, v.Half, v.Tags = iq.Judge(item, text)
				if v.Tags == nil {
					v.Tags = []string{}
				}
			}
			raw, _ := json.Marshal(sanitize(v, q.Key))
			_ = json.Unmarshal(raw, &v)
			t.mu.Lock()
			t.answers[item.ID] = v
			t.mu.Unlock()
			return v, e
		})
	}
	wg.Wait()
	t.mu.Lock()
	items := []iq.QItem{}
	lines := []QuestionResult{}
	complete := true
	for _, item := range t.questions {
		v := t.answers[item.ID]
		if v.ID == "" {
			v = QuestionResult{ID: item.ID, Title: item.Title, Max: item.MaxScore, Error: "请求未完成", Tags: []string{}}
		}
		if v.Error != "" {
			complete = false
		}
		lines = append(lines, v)
		items = append(items, iq.QItem{ID: v.ID, Title: v.Title, Max: v.Max, Earned: v.Earned, Half: v.Half, Tags: v.Tags})
	}
	score := iq.Score(items)
	var validScore any
	if complete {
		validScore = score.Score
	}
	t.Verdict.IQ = map[string]any{"score": validScore, "max": 100, "paper_max": score.Max, "complete": complete, "per_question": lines, "note": fmt.Sprintf("当前卷面 %d；半分仅在卷面展示，不进入百分制通过分。失败请求标记未完成，不伪装为模型答错。", score.Max)}
	t.Verdict.Integrity = buildIntegrity(t.Verdict, lines, q.Model)
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

func buildIntegrity(v Verdict, results []QuestionResult, model string) any {
	input := integrity.Inputs{Protocol: gateway.Protocol(model), ThinkingApply: gateway.IsClaude(model)}
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

func iqThinking(model string) *gateway.ThinkingConfig {
	if gateway.IsClaude(model) {
		return &gateway.ThinkingConfig{Type: "adaptive", Display: "summarized"}
	}
	return nil
}
