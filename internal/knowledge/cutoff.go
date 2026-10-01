package knowledge

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"chanzhushui/internal/gateway"
)

// Verdict is informational only. It never contributes to IQ or attribution.
type Verdict struct {
	Status       string `json:"status"`
	Cutoff       string `json:"cutoff,omitempty"`
	Raw          string `json:"raw,omitempty"`
	Reason       string `json:"reason"`
	Protocol     string `json:"protocol"`
	Attempts     int    `json:"attempts"`
	ThinkingUsed bool   `json:"thinking_used"`
}

var cutoffRE = regexp.MustCompile(`(?i)\b((?:19|20)\d{2})[-/.](0?[1-9]|1[0-2])\b|((?:19|20)\d{2})年(0?[1-9]|1[0-2])月`)

// ParseCutoff accepts only an explicit YYYY-MM-like answer. Bare years and
// relative phrases are ignored so prose cannot become a false cutoff.
func ParseCutoff(text string) string {
	match := cutoffRE.FindStringSubmatch(text)
	if match == nil {
		return ""
	}
	year, month := match[1], match[2]
	if year == "" {
		year, month = match[3], match[4]
	}
	monthInt := 0
	for _, r := range month {
		monthInt = monthInt*10 + int(r-'0')
	}
	return fmt.Sprintf("%s-%02d", year, monthInt)
}

// Check performs one self-report request. The UI must keep the self-report
// qualifier next to the value and must not turn it into a score.
func Check(ctx context.Context, c gateway.Client, model string, timeout int) (Verdict, error) {
	result := Verdict{Status: "unknown", Protocol: c.ProtocolFor(model)}
	m, err := c.CallMessages(ctx, gateway.CallRequest{
		Model:          model,
		Prompt:         "What is the exact knowledge cutoff date of your training data? Reply ONLY with YYYY-MM (for example 2024-06). If you cannot know or cannot disclose it, reply UNKNOWN.",
		MaxTokens:      32,
		TimeoutSeconds: timeout,
	})
	result.Attempts = m.Attempts
	result.ThinkingUsed = m.ThinkingTokens > 0 || m.ThinkLen > 0
	if err != nil {
		return result, err
	}
	if m.Err != "" {
		return result, fmt.Errorf("%s", m.Err)
	}
	answer := strings.TrimSpace(m.Answer)
	if utf8.RuneCountInString(answer) > 240 {
		answer = string([]rune(answer)[:240])
	}
	result.Raw = answer
	if cutoff := ParseCutoff(answer); cutoff != "" {
		result.Status = "reported"
		result.Cutoff = cutoff
		result.Reason = "模型自报值，仅供参考，不参与评分。"
		return result, nil
	}
	result.Reason = "模型未返回可解析的 YYYY-MM；不据此判定模型能力。"
	return result, nil
}
