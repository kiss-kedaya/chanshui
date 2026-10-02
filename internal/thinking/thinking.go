package thinking

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const probePrompt = "求最小正整数 n，使 n 除以 17 余 5，除以 19 余 7，除以 23 余 11。请推导并核验，最后给出数值。"

// Verdict is the outcome of one summarized-thinking passthrough probe.
type Verdict struct {
	Pass           bool   `json:"pass"`
	Score          int    `json:"score"`
	SummaryScore   int    `json:"summary_score"`
	SignatureScore int    `json:"signature_score"`
	Reason         string `json:"reason"`
	ThinkLen       int    `json:"think_len"`
	SigLen         int    `json:"sig_len"`
	ThinkingTokens int    `json:"thinking_tokens"`
	Ms             int64  `json:"ms"`
	ThinkingText   string `json:"thinking_text"`
	Signature      string `json:"signature"`
	Answer         string `json:"answer"`
	Err            string `json:"err,omitempty"`
}

// Check sends one messages call with adaptive thinking and display=summarized,
// then judges whether the upstream returned the thinking text (not just the
// signature). Three verdict shapes:
//
//   - text + signature present  -> pass ("思考文本正常回传")
//   - signature only            -> fail ("未回传思考文本，仅收到签名")
//   - no thinking block at all  -> fail ("无思考块")
func Check(ctx context.Context, c Caller, model string) (Verdict, error) {
	if c == nil {
		return Verdict{}, ErrNoCaller
	}
	req := CallRequest{
		Model:     model,
		MaxTokens: 4096,
		Messages: []Message{
			{Role: "user", Content: probePrompt},
		},
		Thinking: &ThinkingConfig{
			Type:    "adaptive",
			Display: "summarized",
		},
	}

	m, err := c.CallMessages(ctx, req)
	if err != nil {
		return Verdict{
			Err: err.Error(),
		}, err
	}

	if m.Err != "" || m.Status != 200 {
		return Verdict{Err: m.Err}, fmt.Errorf("thinking request failed: HTTP %d %s", m.Status, m.Err)
	}
	v := Verdict{
		ThinkingText: m.ThinkingText, Signature: m.Signature, Answer: m.Answer,
		ThinkLen:       m.ThinkLen,
		SigLen:         m.SigLen,
		ThinkingTokens: m.ThinkingTokens,
		Ms:             m.Ms,
	}

	if strings.TrimSpace(m.ThinkingText) != "" {
		v.SummaryScore = 5
	}
	v.Score = v.SummaryScore
	v.Pass = v.Score == 5
	if v.Pass {
		v.Reason = "思维摘要已回传"
	} else {
		v.Reason = "没有思维摘要"
	}
	return v, nil
}

// ErrNoCaller guards against a nil Caller being passed in.
var ErrNoCaller = errors.New("thinking: nil caller")

func ProbePrompt() string { return probePrompt }
