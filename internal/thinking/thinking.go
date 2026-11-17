package thinking

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const probePrompt = "求最小正整数 n，使 n 除以 17 余 5，除以 19 余 7，除以 23 余 11。请推导并核验，最后给出数值。"
const identityPrompt = "Who are you? Answer in one short sentence."

// Verdict is the outcome of one summarized-thinking and signature audit.
type Verdict struct {
	Pass             bool   `json:"pass"`
	Score            int    `json:"score"`
	MaxScore         int    `json:"max_score"`
	SummaryScore     int    `json:"summary_score"`
	SignatureScore   int    `json:"signature_score"`
	ReplayScore      int    `json:"replay_score"`
	TamperScore      int    `json:"tamper_score"`
	TemperatureScore int    `json:"temperature_score,omitempty"`
	Reason           string `json:"reason"`
	ThinkLen         int    `json:"think_len"`
	SigLen           int    `json:"sig_len"`
	ThinkingTokens   int    `json:"thinking_tokens"`
	Ms               int64  `json:"ms"`
	ThinkingText     string `json:"thinking_text"`
	Signature        string `json:"signature"`
	Answer           string `json:"answer"`
	Err              string `json:"err,omitempty"`

	SignaturePresent    bool   `json:"signature_present"`
	OriginalReplay      bool   `json:"original_replay"`
	TamperRejected      bool   `json:"tamper_rejected"`
	TamperStatus        int    `json:"tamper_status,omitempty"`
	TamperError         string `json:"tamper_error,omitempty"`
	SelfReport          string `json:"self_report,omitempty"`
	FamilyConsistent    bool   `json:"family_consistent"`
	TemperatureRejected bool   `json:"temperature_rejected,omitempty"`
	TemperatureError    string `json:"temperature_error,omitempty"`
}

// Check asks for summarized thinking, then:
//   - scores the summary (5)
//   - replays the returned signature with adaptive thinking; a valid signature
//     must be accepted (1) and a tampered one must be rejected with 400 (2)
//   - for opus-5-5, sends temperature and expects the documented 400 (2)
//
// The replay must carry thinking=adaptive. Without it the upstream can skip
// signature validation and accept tampered signatures.
func Check(ctx context.Context, c Caller, model string) (Verdict, error) {
	if c == nil {
		return Verdict{}, ErrNoCaller
	}
	m, err := c.CallMessages(ctx, CallRequest{
		Model:     model,
		MaxTokens: 4096,
		Messages:  []Message{{Role: "user", Content: probePrompt}},
		Thinking:  &ThinkingConfig{Type: "adaptive", Display: "summarized"},
	})
	if err != nil {
		return Verdict{Err: err.Error()}, err
	}
	if m.Err != "" || m.Status != 200 {
		return Verdict{Err: m.Err}, fmt.Errorf("thinking request failed: HTTP %d %s", m.Status, m.Err)
	}
	v := Verdict{
		ThinkingText: m.ThinkingText, Signature: m.Signature, Answer: m.Answer,
		ThinkLen: m.ThinkLen, SigLen: m.SigLen, ThinkingTokens: m.ThinkingTokens, Ms: m.Ms,
		MaxScore: 10,
	}
	opus55 := strings.Contains(strings.ToLower(model), "opus-5-5")
	if opus55 {
		v.MaxScore = 12
	}
	if strings.TrimSpace(m.ThinkingText) != "" {
		v.SummaryScore = 5
	}
	if m.Signature != "" {
		v.SignaturePresent = true
		v.SignatureScore = 2
		original, origErr := c.CallMessages(ctx, CallRequest{
			Model: model, MaxTokens: 2048,
			Replay:                 &Replay{PriorUser: probePrompt, Signature: m.Signature, FollowUp: identityPrompt},
			StopOnInvalidSignature: true,
		})
		if origErr == nil && original.Status == 200 && original.Err == "" {
			v.OriginalReplay = true
			v.ReplayScore = 1
			v.SelfReport = clipText(original.Answer, 200)
			v.FamilyConsistent = mentionsClaudeFamily(original.Answer)
		}
		tampered, tamperErr := c.CallMessages(ctx, CallRequest{
			Model: model, MaxTokens: 2048,
			Replay:                 &Replay{PriorUser: probePrompt, Signature: tamperSignature(m.Signature), FollowUp: identityPrompt},
			StopOnInvalidSignature: true,
		})
		v.TamperStatus = tampered.Status
		v.TamperError = clipText(tampered.Err, 300)
		if tamperErr == nil && tampered.Status == 400 && strings.Contains(strings.ToLower(tampered.Err), "signature") {
			v.TamperRejected = true
			if v.OriginalReplay {
				v.TamperScore = 2
			}
		}
	}
	if opus55 {
		temp := 0.1
		tr, tempErr := c.CallMessages(ctx, CallRequest{
			Model: model, MaxTokens: 8, Temperature: &temp,
			Messages: []Message{{Role: "user", Content: "Reply OK"}},
		})
		v.TemperatureError = clipText(tr.Err, 200)
		if tempErr == nil && tr.Status == 400 &&
			strings.Contains(strings.ToLower(tr.Err), "temperature") &&
			strings.Contains(strings.ToLower(tr.Err), "deprecated") {
			v.TemperatureRejected = true
			v.TemperatureScore = 2
		}
	}
	v.Score = v.SummaryScore + v.SignatureScore + v.ReplayScore + v.TamperScore + v.TemperatureScore
	v.Pass = v.Score == v.MaxScore
	v.Reason = thinkingReason(v)
	return v, nil
}

func thinkingReason(v Verdict) string {
	if !v.SignaturePresent {
		if v.SummaryScore > 0 {
			return "没有签名；只拿到思维摘要。"
		}
		return "没有思维摘要，也没有签名。"
	}
	if !v.OriginalReplay {
		return fmt.Sprintf("签名 %d/2；原始签名续轮失败；篡改拒绝 %d/2。", v.SignatureScore, v.TamperScore)
	}
	if !v.TamperRejected {
		return "原始签名可续轮，但篡改签名仍被接受：签名未透传或未校验。"
	}
	parts := []string{
		fmt.Sprintf("摘要 %d/5", v.SummaryScore),
		fmt.Sprintf("签名 %d/2", v.SignatureScore),
		fmt.Sprintf("原始续轮 %d/1", v.ReplayScore),
		fmt.Sprintf("篡改拒绝 %d/2", v.TamperScore),
	}
	if v.MaxScore == 12 {
		parts = append(parts, fmt.Sprintf("temperature %d/2", v.TemperatureScore))
	}
	if v.SelfReport != "" {
		parts = append(parts, "自述："+clipText(v.SelfReport, 80))
	}
	return strings.Join(parts, "；")
}

func mentionsClaudeFamily(s string) bool {
	t := strings.ToLower(s)
	return strings.Contains(t, "claude") || strings.Contains(t, "anthropic")
}

func tamperSignature(s string) string {
	b := []byte(s)
	flip := func(i int) bool {
		c := b[i]
		switch {
		case c >= 'A' && c <= 'Z':
			if c == 'A' {
				b[i] = 'B'
			} else {
				b[i] = 'A'
			}
			return true
		case c >= 'a' && c <= 'z':
			if c == 'a' {
				b[i] = 'b'
			} else {
				b[i] = 'a'
			}
			return true
		case c >= '0' && c <= '9':
			if c == '0' {
				b[i] = '1'
			} else {
				b[i] = '0'
			}
			return true
		}
		return false
	}
	for i := len(b) / 2; i < len(b); i++ {
		if flip(i) {
			return string(b)
		}
	}
	for i := len(b) - 1; i >= 0; i-- {
		if flip(i) {
			return string(b)
		}
	}
	return s
}

func clipText(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

// ErrNoCaller guards against a nil Caller being passed in.
var ErrNoCaller = errors.New("thinking: nil caller")

func ProbePrompt() string { return probePrompt }
