package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

const DefaultUA = "claude-cli/2.1.284 (external, claude-desktop-3p, agent-sdk/0.3.284)"

type ThinkingConfig struct {
	Type         string `json:"type"`
	Display      string `json:"display,omitempty"`
	BudgetTokens int    `json:"budget_tokens"`
}

type CacheControl struct {
	TTL string
}

type SystemBlock struct {
	Text         string
	CacheControl *CacheControl
}

type TextBlock struct {
	Text         string
	CacheControl *CacheControl
}

type CallRequest struct {
	// SessionID pins gateway-side session affinity without changing the
	// message prefix. Gateways use metadata.user_id for sticky routing.
	SessionID              string
	Validate               func(Metrics) error
	ToolReply              *ToolReply
	Tools                  []ToolDefinition
	ForceTool              string
	WebSearch              bool
	protocolOverride       string
	Effort                 string
	Model                  string
	MaxTokens              int
	Prompt                 string
	PromptCacheControl     *CacheControl
	UserPrefix             []TextBlock
	History                []ConversationMessage
	System                 []SystemBlock
	Thinking               *ThinkingConfig
	PriorUser              string
	PriorSignature         string
	StopOnInvalidSignature bool
	Stream                 bool
	// Betas are extra anthropic-beta flags the caller asked for. They are
	// merged with the flags this client adds itself, never replaced.
	Betas []string
	// TimeoutSeconds overrides the per-request timeout when > 0. When 0 the
	// request runs under the context deadline only.
	TimeoutSeconds int
}

// thinkingDisplayUpdatesBeta gates thinking.display="updates". Without it the
// API accepts only the generally available display values and returns 400.
const thinkingDisplayUpdatesBeta = "thinking-display-updates-2026-08-18"

// betaHeader builds one comma-separated anthropic-beta value. Caller flags are
// kept as given; flags required by the request body are appended when missing.
// Duplicates are dropped, order is preserved.
func betaHeader(req CallRequest) string {
	var flags []string
	seen := map[string]bool{}
	add := func(raw string) {
		for _, f := range strings.Split(raw, ",") {
			f = strings.TrimSpace(f)
			if f == "" || seen[f] {
				continue
			}
			seen[f] = true
			flags = append(flags, f)
		}
	}
	for _, b := range req.Betas {
		add(b)
	}
	if req.PriorSignature != "" {
		add("interleaved-thinking-2025-05-14")
	}
	if req.Thinking != nil && req.Thinking.Display == "updates" {
		add(thinkingDisplayUpdatesBeta)
	}
	return strings.Join(flags, ",")
}

type ConversationMessage struct {
	Role         string
	Content      string
	CacheControl *CacheControl
}

type Metrics struct {
	ToolCalls          []ToolCall
	WebSearchInvoked   bool
	WebSearchCompleted bool
	SearchURLs         []string
	Attempts           int
	Protocol           string
	CacheReadReported  bool
	CacheWriteReported bool
	Status             int
	Ms                 int64
	Err                string
	Answer             string
	Blocks             []string
	StopReason         string
	RefusalCategory    string
	InputTokens        int
	OutputTokens       int
	OutputReported     bool
	// ThinkingTokens comes from usage.output_tokens_details.thinking_tokens.
	ThinkingTokens  int
	CacheCreation5m int
	CacheCreation1h int
	CacheRead       int
	ThinkLen        int
	SigLen          int
	ThinkingText    string
	Signature       string
	// CacheHeader is the first non-empty of x-cache / cf-cache-status.
	CacheHeader string
	// Via is "direct" for plain HTTP transport, "proxy" once a proxy
	// transport is installed on Client.HTTP.
	Via string
	// SSEFrames holds raw `data: ` payload lines when Stream is true.
	UsagePresent   bool
	StreamComplete bool
	FirstByteMs    int64
	SSEFrames      []string
}

type Client struct {
	Mode       string
	NoRetry    bool
	RetryDelay time.Duration
	BaseURL    string
	Key        string
	HTTP       *http.Client
	UA         string
}

func apiBase(raw string) string {
	base := strings.TrimRight(strings.TrimSpace(raw), "/")
	for _, suffix := range []string{"/chat/completions", "/messages", "/responses", "/models"} {
		if strings.HasSuffix(base, suffix) {
			base = strings.TrimRight(strings.TrimSuffix(base, suffix), "/")
			break
		}
	}
	if !strings.HasSuffix(base, "/v1") {
		base += "/v1"
	}
	return base
}

type wireSystemBlock struct {
	Type         string          `json:"type"`
	Text         string          `json:"text"`
	CacheControl *map[string]any `json:"cache_control,omitempty"`
}

type wireThinking struct {
	Type         string `json:"type"`
	Display      string `json:"display,omitempty"`
	BudgetTokens int    `json:"budget_tokens,omitempty"`
}

type wireRequest struct {
	Metadata     map[string]any    `json:"metadata,omitempty"`
	Tools        []any             `json:"tools,omitempty"`
	ToolChoice   any               `json:"tool_choice,omitempty"`
	OutputConfig map[string]string `json:"output_config,omitempty"`
	Model        string            `json:"model"`
	MaxTokens    int               `json:"max_tokens"`
	Stream       bool              `json:"stream"`
	System       []wireSystemBlock `json:"system,omitempty"`
	Thinking     *wireThinking     `json:"thinking,omitempty"`
	Messages     []wireMessage     `json:"messages"`
}

type wireMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

func (c *Client) ua() string {
	if c.UA != "" {
		return c.UA
	}
	return DefaultUA
}

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

// CallMessages POSTs one /v1/messages request and returns full response
// metrics. HTTP-level errors and API error bodies land in Metrics.Err, not in
// the returned Go error; the Go error is reserved for transport failures.
// 429 handling is the caller's job.
func (c *Client) callMessagesOnce(ctx context.Context, req CallRequest) (Metrics, error) {
	base := apiBase(c.BaseURL)
	wire := wireRequest{
		Model:     req.Model,
		MaxTokens: req.MaxTokens,
		Stream:    req.Stream,
		Messages:  make([]wireMessage, 0, len(req.History)+1),
	}
	for _, message := range req.History {
		wire.Messages = append(wire.Messages, wireMessage{Role: message.Role, Content: messageContent(message.Content, message.CacheControl)})
	}
	if req.PriorSignature != "" {
		wire.Messages = append(wire.Messages,
			wireMessage{Role: "user", Content: req.PriorUser},
			wireMessage{Role: "assistant", Content: []any{
				map[string]any{"type": "thinking", "thinking": "", "signature": req.PriorSignature},
				map[string]any{"type": "text", "text": "Done."},
			}},
		)
	}
	if req.SessionID != "" {
		wire.Metadata = map[string]any{"user_id": "session_" + req.SessionID}
	}
	wire.Messages = append(wire.Messages, wireMessage{Role: "user", Content: userContent(req.UserPrefix, req.Prompt, req.PromptCacheControl)})
	if req.ToolReply != nil {
		reply := req.ToolReply
		wire.Messages = append(wire.Messages, wireMessage{Role: "assistant", Content: []any{map[string]any{"type": "tool_use", "id": reply.Call.ID, "name": reply.Call.Name, "input": reply.Call.Arguments}}}, wireMessage{Role: "user", Content: []any{map[string]any{"type": "tool_result", "tool_use_id": reply.Call.ID, "content": reply.Output}}})
	}
	if req.Effort != "" {
		wire.OutputConfig = map[string]string{"effort": req.Effort}
	}
	for _, b := range req.System {
		sb := wireSystemBlock{Type: "text", Text: b.Text}
		if b.CacheControl != nil {
			cc := map[string]any{"type": "ephemeral"}
			if b.CacheControl.TTL != "" {
				cc["ttl"] = b.CacheControl.TTL
			}
			sb.CacheControl = &cc
		}
		wire.System = append(wire.System, sb)
	}
	if req.Thinking != nil {
		wire.Thinking = &wireThinking{
			Type:         req.Thinking.Type,
			Display:      req.Thinking.Display,
			BudgetTokens: req.Thinking.BudgetTokens,
		}
	}
	wire.Tools, wire.ToolChoice = toolWire(req, "messages")
	body, err := json.Marshal(wire)
	if err != nil {
		return Metrics{}, fmt.Errorf("marshal request: %w", err)
	}

	callCtx := ctx
	if req.TimeoutSeconds > 0 {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(ctx, time.Duration(req.TimeoutSeconds)*time.Second)
		defer cancel()
	}

	httpReq, err := http.NewRequestWithContext(callCtx, http.MethodPost, base+"/messages", bytes.NewReader(body))
	if err != nil {
		return Metrics{}, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("x-api-key", c.Key)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("content-type", "application/json")
	httpReq.Header.Set("user-agent", c.ua())
	if beta := betaHeader(req); beta != "" {
		httpReq.Header.Set("anthropic-beta", beta)
	}

	start := time.Now()
	httpResp, err := c.http().Do(httpReq)
	if err != nil {
		return Metrics{Ms: time.Since(start).Milliseconds()}, fmt.Errorf("do request: %w", err)
	}
	defer httpResp.Body.Close()

	m := Metrics{
		Status: httpResp.StatusCode, Protocol: "messages",
		Ms:  time.Since(start).Milliseconds(),
		Via: "direct",
	}
	m.CacheHeader = firstNonEmpty(httpResp.Header.Get("x-cache"), httpResp.Header.Get("cf-cache-status"))

	readErr := func() error {
		if req.Stream && httpResp.StatusCode == 200 && strings.Contains(httpResp.Header.Get("Content-Type"), "text/event-stream") {
			return c.readStream(httpResp.Body, &m)
		}
		raw, err := io.ReadAll(io.LimitReader(httpResp.Body, 8<<20))
		if err != nil {
			return err
		}
		return parseMessagesBody(raw, &m)
	}()
	if httpResp.StatusCode == http.StatusForbidden && strings.Contains(strings.ToLower(httpResp.Header.Get("content-type")), "text/html") && strings.Contains(strings.ToLower(httpResp.Header.Get("server")), "cloudflare") {
		m.Err = "HTTP 403 Cloudflare WAF blocked the source address"
	}
	m.FirstByteMs = m.Ms
	m.Ms = time.Since(start).Milliseconds()
	if readErr != nil {
		if m.Err == "" {
			m.Err = "HTTP " + itoa(httpResp.StatusCode) + " read: " + trunc(readErr.Error(), 200)
		}
		return m, nil
	}
	if m.Err == "" && httpResp.StatusCode != 200 {
		m.Err = "HTTP " + itoa(httpResp.StatusCode)
	}
	return m, nil
}

func messageContent(text string, cache *CacheControl) any {
	if cache == nil {
		return text
	}
	return []any{textPart(text, cache)}
}

func userContent(prefix []TextBlock, prompt string, promptCache *CacheControl) any {
	if len(prefix) == 0 {
		return messageContent(prompt, promptCache)
	}
	blocks := make([]any, 0, len(prefix)+1)
	for _, block := range prefix {
		blocks = append(blocks, textPart(block.Text, block.CacheControl))
	}
	blocks = append(blocks, textPart(prompt, promptCache))
	return blocks
}

func textPart(text string, cache *CacheControl) map[string]any {
	part := map[string]any{"type": "text", "text": text}
	if cache == nil {
		return part
	}
	control := map[string]any{"type": "ephemeral"}
	if cache.TTL != "" {
		control["ttl"] = cache.TTL
	}
	part["cache_control"] = control
	return part
}

func userPlain(q CallRequest) string {
	var b strings.Builder
	for _, block := range q.UserPrefix {
		b.WriteString(block.Text)
	}
	b.WriteString(q.Prompt)
	return b.String()
}

// readStream consumes an SSE body: every `data: ` payload is kept verbatim in
// Metrics.SSEFrames until the [DONE] sentinel or EOF. The [DONE] sentinel
// itself is not recorded. Non-`data:` lines (event names, blank separators,
// comments) are skipped.
func (c *Client) readStream(body io.Reader, m *Metrics) error {
	return parseStream(body, m)
}

type apiErrorBody struct {
	Type  string         `json:"type"`
	Error *apiErrorInner `json:"error"`
}

type apiErrorInner struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type messagesResponse struct {
	Content     []contentBlock `json:"content"`
	Model       string         `json:"model"`
	StopReason  string         `json:"stop_reason"`
	StopDetails *struct {
		Type     string `json:"type"`
		Category string `json:"category"`
	} `json:"stop_details"`
	Usage usageBlock `json:"usage"`
}

type contentBlock struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	Signature string          `json:"signature"`
}

type usageBlock struct {
	InputTokens   int `json:"input_tokens"`
	OutputTokens  int `json:"output_tokens"`
	CacheCreation *struct {
		Ephemeral5m int `json:"ephemeral_5m_input_tokens"`
		Ephemeral1h int `json:"ephemeral_1h_input_tokens"`
	} `json:"cache_creation"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	OutputTokensDetails      *struct {
		ThinkingTokens int `json:"thinking_tokens"`
	} `json:"output_tokens_details"`
}

// parseMessagesBody fills Metrics from a Messages API JSON body, handling
// both success payloads and {"error":{...}} error bodies.
func parseMessagesBody(raw []byte, m *Metrics) error {
	var apiErr apiErrorBody
	if json.Unmarshal(raw, &apiErr) == nil && apiErr.Error != nil {
		m.Err = fmt.Sprintf("HTTP %s %s %s", itoa(m.Status), apiErr.Error.Type, trunc(apiErr.Error.Message, 200))
	}
	extractToolEvidence(raw, "messages", m)
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return err
	}
	if _, ok := obj["content"]; !ok && m.Err == "" {
		return fmt.Errorf("invalid Messages response: content missing")
	}
	_, m.UsagePresent = obj["usage"]
	var usageFields map[string]json.RawMessage
	json.Unmarshal(obj["usage"], &usageFields)
	_, m.CacheReadReported = usageFields["cache_read_input_tokens"]
	_, m.CacheWriteReported = usageFields["cache_creation_input_tokens"]
	if string(usageFields["cache_creation_input_tokens"]) == "null" {
		m.CacheWriteReported = false
	}
	if string(usageFields["cache_read_input_tokens"]) == "null" {
		m.CacheReadReported = false
	}
	if string(usageFields["input_tokens"]) == "null" || usageFields["input_tokens"] == nil {
		m.UsagePresent = false
	}
	m.OutputReported = usageFields["output_tokens"] != nil && string(usageFields["output_tokens"]) != "null"
	var creationFields map[string]json.RawMessage
	if json.Unmarshal(usageFields["cache_creation"], &creationFields) == nil {
		for _, field := range []string{"ephemeral_5m_input_tokens", "ephemeral_1h_input_tokens"} {
			if v := creationFields[field]; v != nil && string(v) != "null" {
				m.CacheWriteReported = true
			}
		}
	}
	var resp messagesResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		if m.Err != "" {
			return nil
		}
		return err
	}
	for _, b := range resp.Content {
		m.Blocks = append(m.Blocks, b.Type)
		switch b.Type {
		case "thinking":
			m.ThinkingText += b.Thinking
			m.Signature += b.Signature
			m.ThinkLen = utf8.RuneCountInString(m.ThinkingText)
			m.SigLen = len(m.Signature)
		case "text":
			m.Answer += b.Text
		}
	}
	m.StopReason = resp.StopReason
	if resp.StopDetails != nil {
		if resp.StopDetails.Category != "" {
			m.RefusalCategory = resp.StopDetails.Category
		} else if resp.StopDetails.Type == "refusal" {
			m.RefusalCategory = "refusal"
		}
	}
	m.InputTokens = resp.Usage.InputTokens
	m.OutputTokens = resp.Usage.OutputTokens
	if resp.Usage.CacheCreation != nil {
		m.CacheCreation5m = resp.Usage.CacheCreation.Ephemeral5m
		m.CacheCreation1h = resp.Usage.CacheCreation.Ephemeral1h
	}
	if resp.Usage.CacheCreationInputTokens > m.CacheCreation5m+m.CacheCreation1h {
		m.CacheCreation5m = resp.Usage.CacheCreationInputTokens - m.CacheCreation1h
	}
	m.CacheRead = resp.Usage.CacheReadInputTokens
	if resp.Usage.OutputTokensDetails != nil {
		m.ThinkingTokens = resp.Usage.OutputTokensDetails.ThinkingTokens
	}
	return nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func trunc(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}
