package gateway

import (
	"context"
	"fmt"
	"math/rand"
	"regexp"
	"strings"
	"time"
)

const RetryCount = 3

// IsClaude gates Anthropic-only configuration and visible reasoning evidence.
func IsClaude(model string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "claude-")
}
func Protocol(model string) string {
	m := strings.ToLower(model)
	if IsClaude(m) {
		return "messages"
	}
	if strings.HasPrefix(m, "gpt-") || strings.HasPrefix(m, "o1") || strings.HasPrefix(m, "o3") || strings.HasPrefix(m, "o4") {
		return "responses"
	}
	if strings.HasPrefix(m, "gemini-") || strings.HasPrefix(m, "deepseek-") || strings.HasPrefix(m, "grok-") || strings.HasPrefix(m, "glm-") {
		return "chat"
	}
	return "messages"
}
func (c *Client) CallMessages(ctx context.Context, q CallRequest) (Metrics, error) {
	start := time.Now()
	var m Metrics
	var err error
	for attempt := 0; attempt <= c.retries(); attempt++ {
		if ctx.Err() != nil {
			return m, ctx.Err()
		}
		protocol := c.ProtocolFor(q.Model)
		if q.protocolOverride != "" {
			protocol = q.protocolOverride
		}
		if protocol == "messages" {
			m, err = c.callMessagesOnce(ctx, q)
		} else {
			q.Thinking = nil
			m, err = c.callCompatibleOnce(ctx, q)
		}
		m.Attempts = attempt + 1
		if err == nil && m.Err == "" && q.Validate != nil {
			if validationErr := q.Validate(m); validationErr != nil {
				m.Err = validationErr.Error()
			}
		}
		if err == nil && m.Err == "" {
			m.Ms = time.Since(start).Milliseconds()
			return m, nil
		}
		if ctx.Err() != nil {
			return m, ctx.Err()
		}
		if q.StopOnInvalidSignature && m.Status == 400 && strings.Contains(strings.ToLower(m.Err), "signature") {
			break
		}
		adapted := false
		// Correct a stream-only endpoint on the same path before considering
		// tool compatibility. This also applies to explicitly selected protocols.
		if (m.Status == 400 || m.Status == 422) && !q.Stream && requiresStream(m.Err) {
			q.Stream = true
			adapted = true
		} else if (c.Mode == "" || c.Mode == "auto") && q.protocolOverride == "" {
			baseProtocol := c.ProtocolFor(q.Model)
			unsupportedPath := m.Status == 404 || m.Status == 405
			toolCompatibility := m.Status == 400 && (len(q.Tools) > 0 || q.WebSearch)
			if (baseProtocol == "responses" && unsupportedPath) || ((baseProtocol == "responses" || baseProtocol == "messages") && toolCompatibility) {
				q.protocolOverride = "chat"
				adapted = true
			}
		}
		// Some gateways reject forced tool selection even though they accept the
		// declared tool. One retry may proceed without the selector; this avoids
		// spending all four attempts on identical deterministic parameter errors.
		forceWasSet := q.ForceTool != ""
		if (m.Status == 400 || m.Status == 422) && forceWasSet && !adapted {
			q.ForceTool = ""
			adapted = true
		}
		// Retrying authentication, billing or unchanged parameter/path errors
		// only adds latency. Transient statuses retain their independent budget.
		if !adapted && PermanentRequestError(m.Status) {
			break
		}
		if !adapted && attempt < c.retries() {
			select {
			case <-ctx.Done():
				return m, ctx.Err()
			case <-time.After(c.backoff(attempt, m.Status)):
			}
		}
	}
	m.Ms = time.Since(start).Milliseconds()
	message := m.Err
	if err != nil {
		message = err.Error()
	}
	m.Err = FriendlyError(m.Status, message, m.Attempts)
	return m, nil
}

// PermanentRequestError identifies HTTP errors that need a changed request
// or credentials. Callers must apply supported compatibility changes first.
func PermanentRequestError(status int) bool {
	return status >= 400 && status < 500 && status != 408 && status != 409 && status != 425 && status != 429
}

func requiresStream(reason string) bool {
	lower := strings.ToLower(reason)
	return strings.Contains(lower, "stream") && (strings.Contains(lower, "streaming is required") ||
		strings.Contains(lower, "stream must be true") || strings.Contains(lower, "stream=true") ||
		strings.Contains(lower, `only accepts "stream": true`) || strings.Contains(lower, "only supports streaming"))
}

// Upstream5xxBackoff is the first wait after a 5xx. Some gateways answer an
// over-concurrency burst of long requests with 500 new_api_error; retrying
// 250ms later lands in the same burst, so 5xx waits longer: 2s, 4s, 8s.
var Upstream5xxBackoff = 2 * time.Second

// backoff returns the wait before the next attempt. RetryDelay, when set,
// replaces both bases (tests use it to stay fast).
func (c *Client) backoff(attempt, status int) time.Duration {
	base := 250 * time.Millisecond
	if status >= 500 {
		base = Upstream5xxBackoff
	}
	if c.RetryDelay > 0 {
		base = c.RetryDelay
	}
	d := base * time.Duration(1<<attempt)
	// Up to 25% jitter so parallel probes that failed together do not retry
	// in lockstep and re-create the same burst.
	if j := int64(d / 4); j > 0 {
		d += time.Duration(rand.Int63n(j))
	}
	return d
}

func (c *Client) retries() int {
	if c.NoRetry {
		return 0
	}
	return RetryCount
}

// FriendlyError never includes upstream raw payloads, addresses or credentials.
func FriendlyError(status int, reason string, attempts int) string {
	message := "上游请求未完成，请稍后重试"
	lower := strings.ToLower(reason)
	switch {
	case strings.Contains(lower, "context canceled"):
		message = "检测已取消"
	case strings.Contains(lower, "cloudflare") || strings.Contains(lower, "waf"):
		message = "上游安全策略拦截了当前出口地址（Cloudflare/WAF），不是 API Key 鉴权失败"
	case strings.Contains(lower, "sse stream ended") || strings.Contains(lower, "without message_stop"):
		// 上游把 HTTP 头先回了 200，随后把流掐断。这不是上游 5xx 故障，
		// 而是响应体没读完，重试通常能拿到完整结果，所以单独分类。
		message = "上游响应流被提前中断（连接在传输中途结束），可重试"
	case strings.Contains(lower, "不允许访问"):
		message = "目标地址不可用于公网检测"
	case status == 401 || status == 403:
		message = "上游拒绝鉴权，请检查密钥与模型权限"
	case status == 429:
		message = "上游请求频率或额度受限"
	case status == 402:
		message = "上游余额不足或账户计费不可用"
	case status == 404 || status == 405:
		message = "上游不支持此模型的 API 路径"
	case status == 400 || status == 422:
		switch {
		case strings.Contains(lower, "tool_choice"), strings.Contains(lower, "tool choice"), strings.Contains(lower, "tools"):
			message = "上游不接受当前工具调用参数，已停止重复发送相同参数"
		case strings.Contains(lower, "model"), strings.Contains(lower, "not found"):
			message = "上游不接受当前模型或接口路径"
		default:
			message = "上游拒绝请求参数，请核对模型及接口兼容性"
		}
	case status >= 500:
		// 5xx 是上游自己的故障，不是本站或请求参数的问题。明确写出来，
		// 免得用户把它当成检测站坏了。重试次数由下面的统一后缀补充。
		message = "上游服务异常，属上游侧故障，非本站问题"
	case strings.Contains(lower, "deadline") || strings.Contains(lower, "timeout"):
		message = "上游响应超时"
	case strings.Contains(lower, "empty response"):
		message = "上游返回空响应"
	case strings.Contains(lower, "max_tokens"):
		message = "上游输出被长度限制截断"
	case strings.Contains(lower, "json") || strings.Contains(lower, "stream"):
		message = "上游响应格式异常或流式响应中断"
	}
	suffix := ""
	if status > 0 {
		suffix = fmt.Sprintf("（HTTP %d）", status)
	}
	summary := message + suffix
	if status != 400 && status != 422 {
		summary = fmt.Sprintf("%s%s；已尝试 %d 次", message, suffix, attempts)
	}
	detail := safeDetail(reason)
	if detail == "" || detail == summary {
		return summary
	}
	return summary + "\n" + detail
}

var secretDetail = regexp.MustCompile(`(?i)sk-[A-Za-z0-9_-]{8,}|bearer\s+\S+|api[_-]?key["'\s:=]+\S+`)

func safeDetail(reason string) string {
	text := strings.Join(strings.Fields(strings.TrimSpace(reason)), " ")
	if text == "" {
		return ""
	}
	text = secretDetail.ReplaceAllString(text, "[已隐藏]")
	runes := []rune(text)
	if len(runes) > 280 {
		text = string(runes[:280]) + "…"
	}
	return text
}
