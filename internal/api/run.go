package api

import (
	"chanzhushui/internal/cachecheck"
	"chanzhushui/internal/capability"
	"chanzhushui/internal/fingerprint"
	"chanzhushui/internal/gateway"

	"chanzhushui/internal/inject"
	"chanzhushui/internal/injection"
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
	req := gateway.CallRequest{
		Model: q.Model, MaxTokens: q.MaxTokens, Stream: q.Stream, Effort: q.Effort,
		StopOnInvalidSignature: q.StopOnInvalidSignature, TimeoutSeconds: a.timeout,
	}
	if q.Replay != nil {
		req.PriorUser = q.Replay.PriorUser
		req.PriorSignature = q.Replay.Signature
		req.Prompt = q.Replay.FollowUp
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
		return map[string]any{"family": "unknown", "family_name": "未建立数字基线", "family_prob": 0, "top_model": "", "top_prob": 0, "results": []any{}, "behavioral_top3": b.Top3, "used_outputs": 0, "requests": 0, "low_confidence": true, "note": "当前数字库只覆盖 GPT 和 Claude。没有有效数字样本，这项记 0 分。", "evidence": []any{}}, nil
	}
	const wantSamples = 3
	const maxRequests = 10
	minValid := bank.Numeric.MinimumValidNumbers
	challenges := fingerprint.GenerateChallenges(maxRequests, rand.New(rand.NewSource(time.Now().UnixNano())))
	outs := make([]fingerprint.Output, 0, wantSamples)
	evidence := make([]any, 0, maxRequests)
	requests := 0
	for len(outs) < wantSamples && requests < len(challenges) {
		if ctx.Err() != nil {
			break
		}
		batch := wantSamples - len(outs)
		if room := len(challenges) - requests; batch > room {
			batch = room
		}
		type slot struct {
			out fingerprint.Output
			ev  any
			ok  bool
		}
		slots := make([]slot, batch)
		var batchWG sync.WaitGroup
		for i := 0; i < batch; i++ {
			ch := challenges[requests+i]
			batchWG.Add(1)
			go func(i int, ch fingerprint.Challenge) {
				defer batchWG.Done()
				// 指纹题要一次输出几百个数字，属于长响应。实测同一题面各跑 6 次：
				// 非流式 6/6 成功，流式有 1/6 断在半路（HTTP 仍 200，无 message_stop）。
				// 数字样本只要最终文本，不需要逐字回传，所以走非流式更稳。
				m, e := g.CallMessages(ctx, gateway.CallRequest{Model: q.Model, Prompt: ch.Prompt, MaxTokens: 4096, TimeoutSeconds: q.Timeout})
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
					out: fingerprint.Output{Text: answer, ExpectedCount: ch.ExpectedCount},
					ev:  map[string]any{"id": ch.ID, "expected_count": ch.ExpectedCount, "parsed_count": len(fingerprint.ParseNumbers(m.Answer)), "answer": m.Answer, "error": errText, "ms": m.Ms, "usable": ok},
					ok:  ok,
				}
			}(i, ch)
		}
		batchWG.Wait()
		requests += batch
		for _, sl := range slots {
			evidence = append(evidence, sl.ev)
			if sl.ok && len(outs) < wantSamples {
				outs = append(outs, sl.out)
			}
		}
	}
	a := fingerprint.Attribute(outs, bank)
	res := []map[string]any{}
	for _, v := range a.Results {
		if v.Prob <= 0 {
			continue
		}
		res = append(res, map[string]any{"model": v.ModelID, "probability": v.Prob, "similarity": v.JSSimilarity})
	}
	note := "概率只在数字库内的候选之间比较。凑满 3 个有效样本且归因和所选模型一致，这项才记满分。"
	family, familyName := a.Family, a.FamilyName
	if a.UsedOutputs == 0 {
		family, familyName = "unknown", "没有有效样本"
		note = "10 次里没有拿到有效数字样本，这项记 0 分。"
	} else if a.UsedOutputs < wantSamples {
		note = "有效样本不足 3 个，这项记 0 分。"
	}
	out := map[string]any{"family": family, "family_name": familyName, "family_prob": a.FamilyProb, "top_model": a.TopModel, "top_prob": a.TopProb, "results": res, "used_outputs": a.UsedOutputs, "requests": requests, "low_confidence": a.LowConfidence || a.UsedOutputs < wantSamples, "evidence": evidence, "note": note}
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
			case "hidden":
				t.Verdict.Hidden = result
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
	job("fingerprint", func() (any, error) { return s.numeric(ctx, g, q) })
	job("injection", func() (any, error) {
		v, e := inject.Measure(ctx, g, q.Model, q.Timeout)
		if e != nil {
			return map[string]any{"error": e.Error(), "points": v.Points}, e
		}
		return v, nil
	})
	job("hidden", func() (any, error) {
		v, e := injection.Detect(ctx, &g, q.Model, q.Timeout)
		if e != nil {
			return map[string]any{"error": e.Error(), "score": v.Score}, e
		}
		return v, nil
	})
	if q.Cache || (selected != nil && selected["cache"]) {
		job("cache", func() (any, error) {
			v, e := cachecheck.Measure(ctx, g, q.Model, q.Timeout)
			if e != nil {
				return v, e
			}
			return v, nil
		})
	}
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
			adapter := thinkAdapter{g, q.Timeout}
			v, e := thinking.Check(ctx, adapter, q.Model)
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
				earned, half, tags := iq.Judge(item, text)
				if earned == 0 {
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
	if len(t.questions) > 0 {
		score := iq.Score(items)
		var validScore any
		if complete {
			validScore = score.Score
		}
		t.Verdict.IQ = map[string]any{"score": validScore, "max": 100, "paper_max": score.Max, "complete": complete, "per_question": lines, "note": fmt.Sprintf("当前卷面 %d；半分仅在卷面展示，不进入百分制通过分。失败请求标记未完成，不伪装为模型答错。", score.Max)}
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
			got := integrity.FingerprintFamily(family)
			if used >= 3 && got != "" && got != "unknown" && !integrity.Diluted(model, family, top) {
				fpPoints = 10
			}
		}
	}
	iqPoints, iqKnown := 0, false
	if m, ok := v.IQ.(map[string]any); ok {
		iqKnown = true
		if complete, _ := m["complete"].(bool); complete {
			if n, ok := asInt(m["score"]); ok {
				iqPoints = n
			}
		}
	}
	injPoints, injKnown := 0, false
	if m, ok := v.Injection.(map[string]any); ok {
		injKnown = true
		errText, _ := m["error"].(string)
		if errText == "" {
			if n, ok := asInt(m["score"]); ok {
				injPoints = n
			}
		}
	}
	toolPoints, toolKnown := boolPart(v.Tools)
	hiddenPoints, hiddenKnown := 0, false
	if m, ok := v.Hidden.(map[string]any); ok {
		errText, _ := m["error"].(string)
		if errText == "" {
			// 只有拿到明确判定（injected/suspect/clean）才计入分数；
			// unknown/error 视为未完成，按 0 分计入，不伪装通过。
			switch status, _ := m["status"].(string); status {
			case "clean", "suspect", "injected":
				hiddenKnown = true
				if n, ok := asInt(m["score"]); ok {
					hiddenPoints = n
				}
			}
		}
	}
	webApply, webPoints, webKnown := webPart(v.Web)
	thinkApply := gateway.IsClaude(model)
	thinkPoints, thinkKnown := 0, false
	if thinkApply {
		if m, ok := v.Thinking.(map[string]any); ok {
			thinkKnown = true
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
		{Key: "injection", Label: "注入大小", Applicable: want("injection"), Known: injKnown, Points: injPoints, MaxPoints: 10},
		{Key: "hidden", Label: "隐藏指令检测", Applicable: want("hidden"), Known: hiddenKnown, Points: hiddenPoints, MaxPoints: 10},
		{Key: "tools", Label: "工具调用", Applicable: want("tools"), Known: toolKnown, Points: toolPoints, MaxPoints: 1},
		{Key: "web", Label: "联网检测", Applicable: webApply, Known: webKnown, Points: webPoints, MaxPoints: 1},
		{Key: "thinking", Label: "思维链回传", Applicable: thinkApply, Known: thinkKnown, Points: thinkPoints, MaxPoints: 5},
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
	if status == "unknown" && !hasPass && protocol == "chat" {
		return false, 0, false
	}
	points, known := boolPart(v)
	return true, points, known
}
