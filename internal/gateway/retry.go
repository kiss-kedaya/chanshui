package gateway

import (
	"context"
	"fmt"
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
		if (c.Mode == "" || c.Mode == "auto") && q.protocolOverride == "" {
			baseProtocol := c.ProtocolFor(q.Model)
			unsupportedPath := m.Status == 404 || m.Status == 405
			toolCompatibility := m.Status == 400 && (len(q.Tools) > 0 || q.WebSearch)
			if (baseProtocol == "responses" && unsupportedPath) || ((baseProtocol == "responses" || baseProtocol == "messages") && toolCompatibility) {
				q.protocolOverride = "chat"
			}
		}
		if err == nil && m.Err == "" {
			m.Ms = time.Since(start).Milliseconds()
			return m, nil
		}
		if ctx.Err() != nil {
			return m, ctx.Err()
		}
		if attempt < c.retries() {
			delay := c.RetryDelay
			if delay <= 0 {
				delay = 250 * time.Millisecond
			}
			select {
			case <-ctx.Done():
				return m, ctx.Err()
			case <-time.After(delay * time.Duration(1<<attempt)):
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
		message = "上游拒绝请求参数，请核对模型及接口兼容性"
	case status >= 500:
		message = "上游服务暂时异常"
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
	return fmt.Sprintf("%s%s；已尝试 %d 次", message, suffix, attempts)
}
