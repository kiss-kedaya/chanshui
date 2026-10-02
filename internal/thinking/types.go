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
	// Replay is set only by signature audit. Thinking stays nil.
	Replay *Replay `json:"replay,omitempty"`
	// StopOnInvalidSignature stops the transport on the expected forged-signature 400.
	StopOnInvalidSignature bool   `json:"-"`
	Stream                 bool   `json:"stream,omitempty"`
	Effort                 string `json:"effort,omitempty"`
}

// Replay is one assistant thinking block followed by a short user turn.
type Replay struct {
	PriorUser string `json:"prior_user"`
	Signature string `json:"signature"`
	FollowUp  string `json:"follow_up"`
}

// Metrics mirrors gateway.Metrics (subset of fields needed for the
// thinking check plus the shared bookkeeping fields).
type Metrics struct {
	Status          int
	Ms              int64
	Err             string
	Answer          string
	Blocks          string
	ThinkLen        int
	SigLen          int
	ThinkingText    string
	Signature       string
	InputTokens     int
	OutputTokens    int
	ThinkingTokens  int
	StopReason      string
	RefusalCategory string
}

// Caller mirrors the gateway client surface used by Check.
type Caller interface {
	CallMessages(ctx context.Context, req CallRequest) (Metrics, error)
}
