package api

import (
	"fmt"
	"strings"

	"github.com/kiss-kedaya/chanshui/internal/fingerprint"
	"github.com/kiss-kedaya/chanshui/internal/integrity"
)

// familyHasNumericBaseline reports whether the numeric bank has at least one
// candidate in the claimed family. An alias whose exact id is absent can still
// be attributed when the family has dated baselines.
func familyHasNumericBaseline(bank *fingerprint.Bank, model string) bool {
	family := integrity.ModelFamily(model)
	if family == "" {
		return false
	}
	for _, candidate := range bank.Numeric.Models {
		if integrity.FingerprintFamily(candidate.Family) == family {
			return true
		}
	}
	return false
}

// behaviorFallback turns the behavioral bank into a direct attribution result
// when the numeric bank has no candidate for the claimed family. The
// behavioral features are coarse, so a close or weak top match stays low
// confidence instead of being presented as a confirmed substitution.
func behaviorFallback(model string, bank *fingerprint.Bank, b *fingerprint.BehaviorResult) map[string]any {
	base := map[string]any{
		"basis":           "behavioral",
		"evidence":        []any{},
		"behavioral_top3": []fingerprint.BehaviorMatch{},
		"used_outputs":    0,
		"requests":        0,
	}
	if b == nil || len(b.Top3) == 0 {
		base["family"] = "unknown"
		base["family_name"] = "行为库无可比候选"
		base["family_prob"] = 0
		base["top_model"] = ""
		base["top_prob"] = 0
		base["top_similarity"] = 0
		base["results"] = []any{}
		base["low_confidence"] = true
		base["note"] = "数字库没有该家族基线，行为库也没有可比候选；这项记 0 分。"
		if b != nil && b.Note != "" {
			base["behavior_note"] = b.Note
		}
		return base
	}

	top := b.Top3[0]
	low := behavioralLowConfidence(model, b.Top3)
	results := make([]map[string]any, 0, len(b.Top3))
	for _, match := range b.Top3 {
		results = append(results, map[string]any{
			"model":       match.ModelID,
			"display":     match.DisplayName,
			"probability": match.Similarity,
			"similarity":  match.Similarity,
		})
	}
	note := fmt.Sprintf(
		"数字库没有该家族基线，已用行为库 %d 个候选直接比对；最像 %s（相似度 %.1f%%）。",
		len(bank.Behavioral.Models), top.ModelID, top.Similarity*100,
	)
	if low {
		note += " 候选接近或相似度偏低，证据不足，不判掺水。"
	} else {
		note += " 行为归因不是数字基线证明。"
	}

	base["family"] = top.Family
	base["family_name"] = behaviorFamilyName(top.Family)
	base["family_prob"] = top.Similarity
	base["top_model"] = top.ModelID
	base["top_display"] = top.DisplayName
	base["top_prob"] = top.Similarity
	base["top_similarity"] = top.Similarity
	base["results"] = results
	base["behavioral_top3"] = b.Top3
	base["behavioral_total"] = len(bank.Behavioral.Models)
	base["behavior_note"] = b.Note
	base["used_outputs"] = 3
	base["requests"] = 3
	base["low_confidence"] = low
	base["note"] = note
	return base
}

func behavioralLowConfidence(model string, matches []fingerprint.BehaviorMatch) bool {
	if len(matches) == 0 {
		return true
	}
	top := matches[0]
	claimed := integrity.ModelFamily(model)
	actual := integrity.FingerprintFamily(top.Family)
	// A cross-family match is already meaningful at a lower similarity.
	if claimed != "" && actual != "" && actual != "unknown" && claimed != actual {
		return top.Similarity < 0.85
	}
	if top.Similarity < 0.88 {
		return true
	}
	if len(matches) > 1 && top.Similarity-matches[1].Similarity < 0.03 {
		return true
	}
	return false
}

func behaviorFamilyName(family string) string {
	switch strings.ToLower(strings.TrimSpace(family)) {
	case "openai", "gpt":
		return "GPT"
	case "anthropic", "claude":
		return "Claude"
	case "google", "gemini":
		return "Gemini"
	case "deepseek":
		return "DeepSeek"
	case "xai", "x-ai", "grok":
		return "Grok"
	case "zhipu", "z-ai", "glm":
		return "GLM"
	case "moonshot", "kimi":
		return "Kimi"
	default:
		return family
	}
}
