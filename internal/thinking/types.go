package thinking

import "context"

// Local mirror of the gateway client's request/response types.
// The real gateway package (internal/gateway) lives in another worktree;
// the integrator adapts these shapes onto it. Do not import gateway here.

// ThinkingConfig mirrors gateway.ThinkingConfig.
type ThinkingConfig struct {
	Type    string `json:"type"`
	Display string `json:"display,omitempty"`
}

// Message mirrors gateway.Message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// CallRequest mirrors gateway.CallRequest.
type CallRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	System    string          `json:"system,omitempty"`
	Messages  []Message       `json:"messages"`
	Thinking  *ThinkingConfig `json:"thinking,omitempty"`
}

// Metrics mirrors gateway.Metrics (subset of fields needed for the
// thinking check plus the shared bookkeeping fields).
type Metrics struct {
	Status         int
	Ms             int64
	Err            string
	Answer         string
	Blocks         string
	ThinkLen       int
	SigLen         int
	ThinkingText   string
	Signature      string
	InputTokens    int
	OutputTokens   int
	ThinkingTokens int
}

// Caller mirrors the gateway client surface used by Check.
type Caller interface {
	CallMessages(ctx context.Context, req CallRequest) (Metrics, error)
}
