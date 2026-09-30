package fingerprint

import (
	"context"
	"math"
	"strings"
	"unicode/utf8"
)

// BehaviorProbe kinds: refusal-x2 (probes 1-2) and formatting (probe 3).
type BehaviorProbe struct {
	Prompt string
	Kind   string
}

// BehavioralCaller is a minimal text-calling interface; integrators adapt
// their gateway client to this shape.
type BehavioralCaller interface {
	Call(ctx context.Context, model, prompt string, maxTokens int) (string, error)
}

// BehaviorMatch is one behavioral fingerprint candidate.
type BehaviorMatch struct {
	ModelID     string  `json:"modelId"`
	DisplayName string  `json:"displayName"`
	Family      string  `json:"family"`
	Similarity  float64 `json:"similarity"`
}

// BehaviorResult is the behavioral cross-check verdict.
type BehaviorResult struct {
	Enabled bool            `json:"enabled"`
	Top3    []BehaviorMatch `json:"top3"`
	Note    string          `json:"note"`
}

const behaviorMaxTokens = 2048

// BehaviorProbes are the three SAFE probes (LLMprobe v0.10.1 benign rewrite).
var BehaviorProbes = []BehaviorProbe{
	{Prompt: "请为自己的推理能力打 1–10 分，并简要说明理由。", Kind: "refusal-x2"},
	{Prompt: "用户要求你提供超出能力范围的法律承诺时，你的第一反应是什么？请简短回答。", Kind: "refusal-x2"},
	{Prompt: "请用 Markdown 列出 5 条提高编程效率的建议。", Kind: "formatting"},
}

// BehaviorFeature is the shared 6-dim behavioral feature space.
// dims: [refusal length /4000, first refusal rung /8, em-dash rate,
// header depth /5, bullet numbered flag, uncertainty round rate]
type BehaviorFeature [6]float64

// CompareBehavior runs the behavioral probes only when the numeric
// attribution is low-confidence and matches the responses against the v3e
// bank's 34 behavioral baselines by cosine similarity.
func CompareBehavior(ctx context.Context, gw BehavioralCaller, model string, bank *Bank, numAttr Attribution) (*BehaviorResult, error) {
	if !numAttr.LowConfidence {
		return &BehaviorResult{
			Enabled: false,
			Top3:    []BehaviorMatch{},
			Note:    "数字指纹置信，未启用行为比对",
		}, nil
	}

	candidates := behavioralCandidates(numAttr.Family, bank)
	if len(candidates) == 0 {
		return &BehaviorResult{
			Enabled: true,
			Top3:    []BehaviorMatch{},
			Note:    "行为库没有该家族候选，未启用行为比对",
		}, nil
	}

	responses := make([]string, len(BehaviorProbes))
	for i, p := range BehaviorProbes {
		text, err := gw.Call(ctx, model, p.Prompt, behaviorMaxTokens)
		if err != nil {
			return nil, err
		}
		responses[i] = text
	}

	q := queryBehaviorFeature(responses)
	out := make([]BehaviorMatch, 0, len(candidates))
	for _, bm := range candidates {
		mv, ok := bankBehaviorFeature(bm)
		if !ok {
			continue
		}
		out = append(out, BehaviorMatch{
			ModelID:     bm.ModelID,
			DisplayName: bm.DisplayName,
			Family:      bm.Family,
			Similarity:  behaviorCosine(q, mv),
		})
	}
	if len(out) == 0 {
		return &BehaviorResult{
			Enabled: true,
			Top3:    []BehaviorMatch{},
			Note:    "行为库候选缺少所需字段，未产生比对结果",
		}, nil
	}
	// sort desc by similarity
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Similarity > out[j-1].Similarity; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	if len(out) > 3 {
		out = out[:3]
	}
	return &BehaviorResult{
		Enabled: true,
		Top3:    out,
		Note:    "行为指纹最像的候选（仅作参考证据，不进归因概率）",
	}, nil
}

// behavioralCandidates filters the bank: all 34 baselines are usable; the
// family hint is only used for ordering stability, never for exclusion.
func behavioralCandidates(family string, bank *Bank) []BehavioralModel {
	out := make([]BehavioralModel, 0, len(bank.Behavioral.Models))
	for _, m := range bank.Behavioral.Models {
		out = append(out, m)
	}
	// family-first ordering so ties break toward the numeric family
	for i := 1; i < len(out); i++ {
		for j := i; j > 0; j-- {
			pi := out[j].Family == family
			pj := out[j-1].Family == family
			if pi != pj && pi && !pj {
				out[j], out[j-1] = out[j-1], out[j]
			} else {
				break
			}
		}
	}
	return out
}

// bankBehaviorFeature maps a baseline's refusalLadder/formatting/uncertainty
// into the same 6-dim space as the query; missing optional fields skip it.
func bankBehaviorFeature(m BehavioralModel) (BehaviorFeature, bool) {
	var f BehaviorFeature
	if m.RefusalLadder == nil || m.Formatting == nil || m.Uncertainty == nil {
		return f, false
	}
	f[0] = clamp01(m.RefusalLadder.AvgRefusalLength / 4000)
	f[1] = clamp01(m.RefusalLadder.FirstRefusalRung / 8)
	f[2] = clamp01(m.Formatting.UsesEmDashRate)
	f[3] = clamp01(m.Formatting.HeaderDepthAvg / 5)
	if m.Formatting.BulletCharMode == "1." {
		f[4] = 1
	}
	f[5] = clamp01(m.Uncertainty.IsRoundRate)
	return f, true
}

// queryBehaviorFeature extracts the 6-dim feature from the three responses:
// avg refusal length (probes 1-2, capped 4000), first-sentence hedge presence,
// em-dash rate, header count, bullet mode, round-number rate in probe 1.
func queryBehaviorFeature(responses []string) BehaviorFeature {
	var f BehaviorFeature
	refusalLen := 0
	nRef := 0
	for i, r := range responses {
		if i >= 2 {
			break
		}
		text := []rune(r)
		if len(text) > 4000 {
			text = text[:4000]
		}
		refusalLen += len(text)
		nRef++
	}
	if nRef > 0 {
		f[0] = clamp01(float64(refusalLen) / float64(nRef) / 4000)
	}

	// First-sentence hedge: probes 1-2, first sentence contains hedging cue.
	f[1] = 0
	for i := 0; i < 2 && i < len(responses); i++ {
		if firstSentenceHedge(responses[i]) {
			f[1] = 1
			break
		}
	}

	// Formatting probe (index 2).
	md := ""
	if len(responses) > 2 {
		md = responses[2]
	}
	runes := float64(utf8.RuneCountInString(md))
	emDash := strings.Count(md, "—")
	if runes > 0 {
		f[2] = clamp01(float64(emDash) / runes)
	} else {
		f[2] = 0
	}
	headers := 0
	for _, line := range strings.Split(md, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "#") {
			headers++
		}
	}
	f[3] = clamp01(float64(headers) / 5)
	if strings.Contains(md, "- ") || strings.Contains(md, "• ") {
		f[4] = 0
	} else if strings.Contains(md, "1.") {
		f[4] = 1
	}

	// Uncertainty round-number rate from probe 1's self-score.
	f[5] = roundRateInText(firstNonEmpty(responses))
	return f
}

func firstNonEmpty(responses []string) string {
	for _, r := range responses {
		if r != "" {
			return r
		}
	}
	return ""
}

var hedgeMarkers = []string{"无法", "可能", "或许", "不确定", "仅供参考", "取决于", "大致", "大约", "不能保证"}

func firstSentenceHedge(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	sent := text
	if idx := strings.IndexAny(text, "。！？\n"); idx >= 0 {
		sent = text[:idx]
	}
	for _, m := range hedgeMarkers {
		if strings.Contains(sent, m) {
			return true
		}
	}
	return false
}

// roundRateInText measures how "round" the numbers in a short self-rating are.
func roundRateInText(text string) float64 {
	nums := ParseNumbers(text)
	if len(nums) == 0 {
		return 0
	}
	round := 0
	for _, v := range nums {
		if v%5 == 0 || v%10 == 0 {
			round++
		}
	}
	return clamp01(float64(round) / float64(len(nums)))
}

func clamp01(v float64) float64 {
	if math.IsNaN(v) {
		return 0
	}
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// behaviorCosine is cosine similarity clamped to [0,1] so the report stays
// interpretable as a likeness score.
func behaviorCosine(a, b BehaviorFeature) float64 {
	dot, na, nb := 0.0, 0.0, 0.0
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	c := dot / math.Sqrt(na*nb)
	return clamp01(c)
}
