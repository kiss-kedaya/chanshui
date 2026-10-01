package integrity

import (
	"regexp"
	"strings"
)

var oFamily = regexp.MustCompile(`^o[134](?:[.-]|$)`)

func ModelFamily(model string) string {
	m := strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.HasPrefix(m, "claude"):
		return "claude"
	case strings.HasPrefix(m, "gpt") || oFamily.MatchString(m) || strings.Contains(m, "chatgpt"):
		return "gpt"
	case strings.HasPrefix(m, "gemini"):
		return "gemini"
	case strings.HasPrefix(m, "deepseek"):
		return "deepseek"
	case strings.HasPrefix(m, "grok"):
		return "grok"
	case strings.HasPrefix(m, "glm"):
		return "glm"
	default:
		return ""
	}
}

func FingerprintFamily(family string) string {
	f := strings.ToLower(strings.TrimSpace(family))
	switch f {
	case "openai", "chatgpt":
		return "gpt"
	case "anthropic":
		return "claude"
	case "xai", "x-ai":
		return "grok"
	case "zhipu", "z-ai":
		return "glm"
	case "google":
		return "gemini"
	default:
		return f
	}
}

func lineName(value string) string {
	v := strings.ToLower(value)
	for _, item := range []string{"opus", "sonnet", "haiku"} {
		if strings.Contains(v, item) {
			return item
		}
	}
	return ""
}

func CanonModel(model string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(model)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Diluted is true when the attributed model is not the model that was requested.
// A different product in the same family still counts. Missing baselines stay false.
func Diluted(requested, family, topModel string) bool {
	claimed := CanonModel(requested)
	actual := CanonModel(topModel)
	if claimed != "" && actual != "" && claimed != actual {
		return true
	}
	want := ModelFamily(requested)
	got := FingerprintFamily(family)
	if want == "" || got == "" || got == "unknown" {
		return false
	}
	if want != got {
		return true
	}
	wantLine, gotLine := lineName(requested), lineName(topModel)
	return wantLine != "" && gotLine != "" && wantLine != gotLine
}

// PresentedScore is the total shown to visitors. A diluted fingerprint contributes nothing,
// including on reports saved before that rule existed.
func PresentedScore(score, fingerprintContribution int, diluted bool) int {
	if !diluted || fingerprintContribution <= 0 || score <= 0 {
		return score
	}
	next := score - fingerprintContribution
	if next < 0 {
		return 0
	}
	return next
}
