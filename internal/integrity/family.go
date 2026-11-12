package integrity

import (
	"regexp"
	"strings"
)

var oFamily = regexp.MustCompile(`^o[134](?:[.-]|$)`)

func ModelFamily(model string) string {
	m := unqualifiedModel(model)
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
	case strings.HasPrefix(m, "kimi"):
		return "kimi"
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

func unqualifiedModel(model string) string {
	m := strings.ToLower(strings.TrimSpace(model))
	for _, prefix := range []string{"openai/", "anthropic/", "moonshotai/"} {
		m = strings.TrimPrefix(m, prefix)
	}
	return m
}

func CanonModel(model string) string {
	model = unqualifiedModel(model)
	if strings.EqualFold(strings.TrimSpace(model), "deepseek-flash") {
		model = "deepseek-v4.1-flash-1017"
	}
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(model)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Fingerprint attribution states. A different family or a different named
// Claude line is a confirmed substitution. Any other name mismatch may just be
// a name the library does not carry, so it stays suspicious and keeps part of
// the section's points.
const (
	FingerprintMatch      = "match"
	FingerprintSuspicious = "suspicious"
	FingerprintFake       = "fake"
)

func FingerprintState(requested, family, topModel string) string {
	claimed := CanonModel(requested)
	actual := CanonModel(topModel)
	want, got := ModelFamily(requested), FingerprintFamily(family)
	if want != "" && got != "" && got != "unknown" && want != got {
		return FingerprintFake
	}
	wantLine, gotLine := lineName(requested), lineName(topModel)
	if wantLine != "" && gotLine != "" && wantLine != gotLine {
		return FingerprintFake
	}
	if claimed != "" && actual != "" && claimed != actual {
		// A non-dated version claim is compatible with any dated baseline of
		// the same version; the report still shows which date matched.
		// shortcut: library membership is not consulted, so an unlisted
		// same-family name stays suspicious instead of fake.
		if claimed == "deepseekv41flash" && strings.HasPrefix(actual, "deepseekv41flash") {
			return FingerprintMatch
		}
		return FingerprintSuspicious
	}
	return FingerprintMatch
}

// Diluted is true only for a confirmed substitution.
func Diluted(requested, family, topModel string) bool {
	return FingerprintState(requested, family, topModel) == FingerprintFake
}

// PresentedScore is the total shown to visitors. The fingerprint share is
// rescaled to the current rule, so reports saved earlier follow it too.
// Coefficient is the whole section's share of the total.
func PresentedScore(score, contribution, coefficient int, state string) int {
	if coefficient <= 0 {
		return score
	}
	target := 0
	switch state {
	case FingerprintMatch:
		target = coefficient
	case FingerprintSuspicious:
		target = coefficient * 2 / 5
	}
	if target == contribution {
		return score
	}
	if next := score - contribution + target; next > 0 {
		return next
	}
	return 0
}
