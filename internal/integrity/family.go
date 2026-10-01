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

// Diluted is true only when attribution contradicts the requested model.
// Missing baselines stay false: unknown is not treated as a routed model.
func Diluted(requested, family, topModel string) bool {
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
