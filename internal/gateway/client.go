package gateway

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const DefaultUA = "claude-cli/2.1.284 (external, claude-desktop-3p, agent-sdk/0.3.284)"

type ThinkingConfig struct {
	Type         string `json:"type"`
	Display      string `json:"display"`
	BudgetTokens int    `json:"budget_tokens"`
}

type CacheControl struct {
	TTL string
}

type SystemBlock struct {
	Text         string
	CacheControl *CacheControl
}

type CallRequest struct {
	Model     string
	MaxTokens int
	Prompt    string
	System    []SystemBlock
	Thinking  *ThinkingConfig
	Stream    bool
	// TimeoutSeconds overrides the per-request timeout when > 0. When 0 the
	// request runs under the context deadline only.
	TimeoutSeconds int
}

type Metrics struct {
	Status       int
	Ms           int64
	Err          string
	Answer       string
	Blocks       []string
	StopReason   string
	InputTokens  int
	OutputTokens int
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
	SSEFrames []string
}

type Client struct {
	BaseURL string
	Key     string
	HTTP    *http.Client
	UA      string
}

type wireSystemBlock struct {
	Type         string          `json:"type"`
	Text         string          `json:"text"`
	CacheControl *map[string]any `json:"cache_control,omitempty"`
}

type wireThinking struct {
	Type         string `json:"type"`
	Display      string `json:"display"`
	BudgetTokens int    `json:"budget_tokens,omitempty"`
}

type wireRequest struct {
	Model     string            `json:"model"`
	MaxTokens int               `json:"max_tokens"`
	Stream    bool              `json:"stream"`
	System    []wireSystemBlock `json:"system,omitempty"`
	Thinking  *wireThinking     `json:"thinking,omitempty"`
	Messages  []wireMessage     `json:"messages"`
}

type wireMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
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
func (c *Client) CallMessages(ctx context.Context, req CallRequest) (Metrics, error) {
	base := strings.TrimRight(c.BaseURL, "/")
	wire := wireRequest{
		Model:     req.Model,
		MaxTokens: req.MaxTokens,
		Stream:    req.Stream,
		Messages:  []wireMessage{{Role: "user", Content: req.Prompt}},
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

	httpReq, err := http.NewRequestWithContext(callCtx, http.MethodPost, base+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return Metrics{}, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("x-api-key", c.Key)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("content-type", "application/json")
	httpReq.Header.Set("user-agent", c.ua())

	start := time.Now()
	httpResp, err := c.http().Do(httpReq)
	if err != nil {
		return Metrics{Ms: time.Since(start).Milliseconds()}, fmt.Errorf("do request: %w", err)
	}
	defer httpResp.Body.Close()

	m := Metrics{
		Status: httpResp.StatusCode,
		Ms:     time.Since(start).Milliseconds(),
		Via:    "direct",
	}
	m.CacheHeader = firstNonEmpty(httpResp.Header.Get("x-cache"), httpResp.Header.Get("cf-cache-status"))

	readErr := func() error {
		if req.Stream && httpResp.StatusCode == 200 {
			return c.readStream(httpResp.Body, &m)
		}
		raw, err := io.ReadAll(httpResp.Body)
		if err != nil {
			return err
		}
		_ = parseMessagesBody(raw, &m)
		return nil
	}()
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

// readStream consumes an SSE body: every `data: ` payload is kept verbatim in
// Metrics.SSEFrames until the [DONE] sentinel or EOF. The [DONE] sentinel
// itself is not recorded. Non-`data:` lines (event names, blank separators,
// comments) are skipped.
func (c *Client) readStream(body io.Reader, m *Metrics) error {
	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data: ") && line != "data:" {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		if payload == "[DONE]" {
			break
		}
		m.SSEFrames = append(m.SSEFrames, payload)
	}
	return sc.Err()
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
	Content    []contentBlock `json:"content"`
	Model      string         `json:"model"`
	StopReason string         `json:"stop_reason"`
	Usage      usageBlock     `json:"usage"`
}

type contentBlock struct {
	Type      string `json:"type"`
	Text      string `json:"text"`
	Thinking  string `json:"thinking"`
	Signature string `json:"signature"`
}

type usageBlock struct {
	InputTokens   int `json:"input_tokens"`
	OutputTokens  int `json:"output_tokens"`
	CacheCreation *struct {
		Ephemeral5m int `json:"ephemeral_5m_input_tokens"`
		Ephemeral1h int `json:"ephemeral_1h_input_tokens"`
	} `json:"cache_creation"`
	CacheReadInputTokens int `json:"cache_read_input_tokens"`
	OutputTokensDetails  *struct {
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
			m.ThinkLen = len(m.ThinkingText)
			m.SigLen = len(m.Signature)
		case "text":
			m.Answer += b.Text
		}
	}
	m.StopReason = resp.StopReason
	m.InputTokens = resp.Usage.InputTokens
	m.OutputTokens = resp.Usage.OutputTokens
	if resp.Usage.CacheCreation != nil {
		m.CacheCreation5m = resp.Usage.CacheCreation.Ephemeral5m
		m.CacheCreation1h = resp.Usage.CacheCreation.Ephemeral1h
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
