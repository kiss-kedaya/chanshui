package thinking

import (
	"context"
	"errors"
)

// Verdict is the outcome of one summarized-thinking passthrough probe.
type Verdict struct {
	Pass           bool   `json:"pass"`
	Reason         string `json:"reason"`
	ThinkLen       int    `json:"think_len"`
	SigLen         int    `json:"sig_len"`
	ThinkingTokens int    `json:"thinking_tokens"`
	Ms             int64  `json:"ms"`
	Err            string `json:"err,omitempty"`
}

// Check sends one messages call with adaptive thinking and display=summarized,
// then judges whether the upstream returned the thinking text (not just the
// signature). Three verdict shapes:
//
//   - text + signature present  -> pass ("思考文本正常回传")
//   - signature only            -> fail ("思考被清空，只回签名")
//   - no thinking block at all  -> fail ("无思考块")
func Check(ctx context.Context, c Caller, model string) (Verdict, error) {
	req := CallRequest{
		Model:     model,
		MaxTokens: 2048,
		Messages: []Message{
			{
				Role:    "user",
				Content: "What is 13*17? Answer with the number only.",
			},
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

	v := Verdict{
		ThinkLen:       m.ThinkLen,
		SigLen:         m.SigLen,
		ThinkingTokens: m.ThinkingTokens,
		Ms:             m.Ms,
	}

	switch {
	case m.ThinkLen > 0 && m.SigLen > 0:
		v.Pass = true
		v.Reason = "思考文本正常回传"
	case m.SigLen > 0 || m.ThinkingTokens > 0:
		// The upstream billed/reporting thinking tokens but stripped the text:
		// only the signature came back. ThinkingTokens>0 with empty text is the
		// same failure shape as signature-only.
		v.Pass = false
		v.Reason = "思考被清空，只回签名"
	default:
		v.Pass = false
		v.Reason = "无思考块"
	}
	return v, nil
}

// ErrNoCaller guards against a nil Caller being passed in.
var ErrNoCaller = errors.New("thinking: nil caller")
