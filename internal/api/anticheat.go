package api

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/kiss-kedaya/chanshui/internal/fingerprint"
	"github.com/kiss-kedaya/chanshui/internal/gateway"
	"github.com/kiss-kedaya/chanshui/internal/thinking"
)

// AntiCheatSignal is one confirmed targeted-cheating indicator extracted from
// the report's raw probe evidence.
type AntiCheatSignal struct {
	Code     string `json:"code"`
	Section  string `json:"section"`
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Detail   string `json:"detail"`
}

// AntiCheatVerdict aggregates confirmed anti-detection / replay / forgery
// findings for a single report. Only high-confidence, zero-false-positive
// indicators are emitted here; upstream official downgrades are never flagged.
type AntiCheatVerdict struct {
	Detected       bool              `json:"detected"`
	ConfirmedCount int               `json:"confirmed_count"`
	PenalizedParts []string          `json:"penalized_parts,omitempty"`
	Signals        []AntiCheatSignal `json:"signals"`
	Summary        string            `json:"summary,omitempty"`
}

type acFingerprintEvidence struct {
	ID            string `json:"id"`
	ExpectedCount int    `json:"expected_count"`
	ParsedCount   int    `json:"parsed_count"`
	Answer        string `json:"answer"`
	Error         string `json:"error"`
	Ms            int64  `json:"ms"`
	Usable        bool   `json:"usable"`
}

type acFingerprint struct {
	Basis    string                  `json:"basis"`
	Evidence []acFingerprintEvidence `json:"evidence"`
}

type acThinking struct {
	Signature       string `json:"signature"`
	SigClass        string `json:"sig_class"`
	SigForged       bool   `json:"sig_forged"`
	SigForgeReason  string `json:"sig_forge_reason"`
	ReplayConfessed bool   `json:"replay_confessed"`
	SelfReport      string `json:"self_report"`
	TamperError     string `json:"tamper_error"`
}

type acMetrics struct {
	Ms             int64 `json:"Ms"`
	FirstByteMs    int64 `json:"FirstByteMs"`
	InputTokens    int   `json:"InputTokens"`
	OutputTokens   int   `json:"OutputTokens"`
	OutputReported bool  `json:"OutputReported"`
	UsagePresent   bool  `json:"UsagePresent"`
}

type acQuestion struct {
	ID           string    `json:"id"`
	Earned       int       `json:"earned"`
	Answer       string    `json:"answer"`
	ThinkingText string    `json:"thinking_text"`
	Signature    string    `json:"signature"`
	Ms           int64     `json:"ms"`
	Error        string    `json:"error"`
	Skipped      bool      `json:"skipped"`
	Metrics      acMetrics `json:"metrics"`
}

type acIQ struct {
	PerQuestion []acQuestion `json:"per_question"`
}

type acVision struct {
	Score    *int        `json:"score"`
	Correct  bool        `json:"correct"`
	Skipped  bool        `json:"skipped"`
	Error    string      `json:"error"`
	Question *acQuestion `json:"question"`
}

type acTools struct {
	Evidence json.RawMessage `json:"evidence"`
}

// evaluateAntiCheat inspects completed probe evidence for targeted cheating
// (local replay banks, static fingerprint arrays, forged protobuf signatures,
// zero-token instant answers, and synthetic tool IDs).
func evaluateAntiCheat(v Verdict, model string) *AntiCheatVerdict {
	var signals []AntiCheatSignal
	penalized := map[string]bool{}
	add := func(code, section, title, detail string) {
		for _, existing := range signals {
			if existing.Code == code {
				return
			}
		}
		signals = append(signals, AntiCheatSignal{
			Code:     code,
			Section:  section,
			Severity: "confirmed",
			Title:    title,
			Detail:   detail,
		})
		penalized[section] = true
	}

	// 1. Fingerprint anti-replay checks.
	if fp, ok := decodeInto[acFingerprint](v.Fingerprint); ok && len(fp.Evidence) > 0 {
		checkFingerprintAntiCheat(fp, add)
	}

	// 2. Thinking signature forgery & local replay confession checks.
	if th, ok := decodeInto[acThinking](v.Thinking); ok {
		if th.SigForged || strings.HasPrefix(th.SigClass, "伪造") || thinking.IsForgedPrefix(th.Signature) {
			detail := th.SigForgeReason
			if detail == "" {
				detail = th.SigClass
			}
			if detail == "" {
				detail = "检测到非官方伪造思维链签名"
			}
			add("thinking_forged_sig", "thinking", "伪造思维链签名", detail)
		}
		if thinking.IsLocalReplayTamperError(th.TamperError) {
			add("thinking_local_tamper", "thinking", "本地模拟篡改拒签报错", "篡改签名报错含本地模拟特征："+clipAntiCheat(th.TamperError, 120))
		}
		if th.ReplayConfessed || thinking.ConfessesLocalReplay(th.SelfReport) {
			add("thinking_replay_confession", "thinking", "自述暴露本地回放服务", "续轮自述："+clipAntiCheat(th.SelfReport, 120))
		}
	}

	// 3. IQ zero-token instant / no-thinking local replay check.
	if iqRes, ok := decodeInto[acIQ](v.IQ); ok && len(iqRes.PerQuestion) > 0 {
		isClaude := gateway.IsClaude(model)
		zeroTokenSuspects := []string{}
		for _, q := range iqRes.PerQuestion {
			if q.Error != "" || q.Skipped || strings.TrimSpace(q.Answer) == "" {
				continue
			}
			if !isZeroTokenReported(q.Metrics) {
				continue
			}
			singlePacket := q.Metrics.Ms > 0 && q.Metrics.FirstByteMs > 0 && abs64(q.Metrics.Ms-q.Metrics.FirstByteMs) <= 10
			instant := q.Metrics.Ms > 0 && q.Metrics.Ms < 500
			claudeNoThink := isClaude && strings.TrimSpace(q.ThinkingText) == "" && strings.TrimSpace(q.Signature) == "" && q.Earned > 0
			if singlePacket || instant || claudeNoThink {
				zeroTokenSuspects = append(zeroTokenSuspects, q.ID)
			}
		}
		if len(zeroTokenSuspects) >= 2 {
			add(
				"iq_zero_token_replay",
				"iq",
				"推理题零 token 本地回放",
				fmt.Sprintf("%d 道题（%s）上报 input=0/output=0 且呈现单包瞬发或缺失思维链，疑似针对题库本地回放。", len(zeroTokenSuspects), strings.Join(zeroTokenSuspects, "、")),
			)
		}
	}

	// 4. Vision zero-token replay check.
	if vis, ok := decodeInto[acVision](v.Vision); ok && vis.Error == "" && !vis.Skipped && vis.Question != nil {
		q := vis.Question
		fullScore := vis.Correct || (vis.Score != nil && *vis.Score >= 100)
		if fullScore && q.Error == "" && !q.Skipped && strings.TrimSpace(q.Answer) != "" && isZeroTokenReported(q.Metrics) {
			add(
				"vision_zero_token_replay",
				"vision",
				"识图题零 token 本地直出",
				fmt.Sprintf("识图满分但上游上报 input=0/output=0（耗时 %dms），疑似脚本读图或本地回放。", q.Ms),
			)
		}
	}

	// 5. Tools synthetic tool_use_id check.
	if tl, ok := decodeInto[acTools](v.Tools); ok && len(tl.Evidence) > 0 {
		raw := strings.ToLower(string(tl.Evidence))
		for _, marker := range []string{"toolu_local_v", "call_local_v", "local_replay_tool"} {
			if strings.Contains(raw, marker) {
				add(
					"tools_synthetic_id",
					"tools",
					"工具调用伪造本地 ID",
					fmt.Sprintf("工具调用返回含本地伪造标识 %q，非真实上游生成。", marker),
				)
				break
			}
		}
	}

	if len(signals) == 0 {
		return nil
	}

	parts := make([]string, 0, len(penalized))
	for k := range penalized {
		parts = append(parts, k)
	}
	sort.Strings(parts)

	titles := make([]string, 0, len(signals))
	for _, s := range signals {
		titles = append(titles, s.Title)
	}
	return &AntiCheatVerdict{
		Detected:       true,
		ConfirmedCount: len(signals),
		PenalizedParts: parts,
		Signals:        signals,
		Summary:        fmt.Sprintf("检测到 %d 项针对性作假证据（%s），对应板块已按 0 分计入。", len(signals), strings.Join(titles, "、")),
	}
}

func checkFingerprintAntiCheat(fp acFingerprint, add func(code, section, title, detail string)) {
	type parsedSample struct {
		ev   acFingerprintEvidence
		nums []int
	}
	valid := make([]parsedSample, 0, len(fp.Evidence))
	fastSamples := []string{}

	for _, ev := range fp.Evidence {
		lowerErr := strings.ToLower(ev.Error)
		for _, kw := range []string{
			"no stored sample of sufficient length",
			"本地题库未收录",
			"local reference-answer",
			"replay service",
		} {
			if strings.Contains(lowerErr, kw) {
				add("fp_replay_error", "fingerprint", "数字指纹暴露本地题库报错", fmt.Sprintf("%s 返回本地回放报错：%s", ev.ID, clipAntiCheat(ev.Error, 120)))
				break
			}
		}
		if ev.Error != "" {
			continue
		}
		nums := fingerprint.ParseNumbers(ev.Answer)
		parsedCount := ev.ParsedCount
		if parsedCount == 0 {
			parsedCount = len(nums)
		}
		if parsedCount >= 200 && ev.Ms > 0 && ev.Ms < 1500 {
			fastSamples = append(fastSamples, fmt.Sprintf("%s(%d个数/%dms)", ev.ID, parsedCount, ev.Ms))
		}
		if len(nums) >= 80 {
			valid = append(valid, parsedSample{ev: ev, nums: nums})
		}
	}

	if len(fastSamples) > 0 {
		add(
			"fp_impossible_speed",
			"fingerprint",
			"数字指纹返回耗时异常（查表/回放）",
			fmt.Sprintf("超 200 个整数在 1.5s 内返回：%s，明显低于大语言模型正常生成耗时。", strings.Join(fastSamples, "、")),
		)
	}

	// Cross-sample consecutive identical integer sequence check (>= 25 contiguous
	// integers with >= 15 distinct values excludes natural 6-10 leading overlaps
	// and non-numeric markup noise).
	bestRun := 0
	bestDistinct := 0
	bestPair := ""
	for i := 0; i < len(valid); i++ {
		for j := i + 1; j < len(valid); j++ {
			run, distinct := longestCommonNumericRun(valid[i].nums, valid[j].nums)
			if run > bestRun || (run == bestRun && distinct > bestDistinct) {
				bestRun = run
				bestDistinct = distinct
				bestPair = valid[i].ev.ID + " 与 " + valid[j].ev.ID
			}
		}
	}
	if bestRun >= 25 && bestDistinct >= 15 {
		add(
			"fp_static_replay",
			"fingerprint",
			"数字指纹跨样本静态数组回放",
			fmt.Sprintf("%s 出现连续 %d 个完全相同的数字片段（含 %d 个不同值），属于预置数组或切片回放。", bestPair, bestRun, bestDistinct),
		)
	}

	// Fixed-count replay check: >= 3 valid samples return the exact same
	// parsed_count across distinct expected_count values differing by >= 5,
	// corroborated by fast latency (< 2500ms) or shared >= 12-number fragment.
	byCount := map[int][]parsedSample{}
	for _, s := range valid {
		c := s.ev.ParsedCount
		if c == 0 {
			c = len(s.nums)
		}
		if c >= 150 {
			byCount[c] = append(byCount[c], s)
		}
	}
	for count, group := range byCount {
		if len(group) < 3 {
			continue
		}
		minExp, maxExp := group[0].ev.ExpectedCount, group[0].ev.ExpectedCount
		fastCount := 0
		for _, s := range group {
			if s.ev.ExpectedCount < minExp {
				minExp = s.ev.ExpectedCount
			}
			if s.ev.ExpectedCount > maxExp {
				maxExp = s.ev.ExpectedCount
			}
			if s.ev.Ms > 0 && s.ev.Ms < 2500 {
				fastCount++
			}
		}
		if maxExp-minExp >= 5 && (fastCount >= 2 || (bestRun >= 12 && bestDistinct >= 8)) {
			add(
				"fp_fixed_count",
				"fingerprint",
				"数字指纹忽略题面长度固定回放",
				fmt.Sprintf("%d 次采样在不同要求长度（%d～%d）下均固定返回 %d 个整数，疑似查表回放。", len(group), minExp, maxExp, count),
			)
			break
		}
	}
}

func longestCommonNumericRun(a, b []int) (bestLen int, bestDistinct int) {
	if len(a) == 0 || len(b) == 0 {
		return 0, 0
	}
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	bestEndA := 0
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			if a[i-1] == b[j-1] {
				cur[j] = prev[j-1] + 1
				if cur[j] > bestLen {
					bestLen = cur[j]
					bestEndA = i
				}
			} else {
				cur[j] = 0
			}
		}
		prev, cur = cur, prev
	}
	if bestLen == 0 {
		return 0, 0
	}
	seen := make(map[int]struct{}, bestLen)
	for _, v := range a[bestEndA-bestLen : bestEndA] {
		seen[v] = struct{}{}
	}
	return bestLen, len(seen)
}

func isZeroTokenReported(m acMetrics) bool {
	return m.InputTokens == 0 && m.OutputTokens == 0 && (m.UsagePresent || m.OutputReported)
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func clipAntiCheat(s string, maxRunes int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= maxRunes {
		return string(r)
	}
	return string(r[:maxRunes]) + "…"
}

func decodeInto[T any](v any) (T, bool) {
	var zero T
	if v == nil {
		return zero, false
	}
	if direct, ok := v.(T); ok {
		return direct, true
	}
	if ptr, ok := v.(*T); ok && ptr != nil {
		return *ptr, true
	}
	b, err := json.Marshal(v)
	if err != nil {
		return zero, false
	}
	var out T
	if err := json.Unmarshal(b, &out); err != nil {
		return zero, false
	}
	return out, true
}

func antiCheatPenalizedParts(v any) map[string]bool {
	ac, ok := decodeInto[AntiCheatVerdict](v)
	if !ok || !ac.Detected {
		return nil
	}
	out := make(map[string]bool, len(ac.PenalizedParts))
	for _, p := range ac.PenalizedParts {
		out[p] = true
	}
	for _, sig := range ac.Signals {
		if sig.Section != "" {
			out[sig.Section] = true
		}
	}
	return out
}
