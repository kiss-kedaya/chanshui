package claudecheck

import (
	"context"
	"fmt"
	"strings"
)

// Result mirrors the fields the deep checks read from a Messages response.
type Result struct {
	Status     int
	Err        string
	Answer     string
	StopReason string
	Transforms []string
	// HasInputTransformations reports whether the upstream sent the field at
	// all; official Anthropic does not, so absence cannot mean "dropped".
	HasInputTransformations bool
	InputTokens             int
	OutputTokens            int
	Signature               string
}

// Caller posts one Messages body with an optional beta header.
type Caller interface {
	PostMessagesRaw(ctx context.Context, body map[string]any, beta string, timeoutSeconds int) (Result, error)
}

// Check is one deep-check outcome.
type Check struct {
	Name    string `json:"name"`
	Title   string `json:"title"`
	Pass    bool   `json:"pass"`
	Skipped bool   `json:"skipped,omitempty"`
	Status  int    `json:"status,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

// Verdict is the deep Claude audit.
type Verdict struct {
	Checks []Check `json:"checks"`
	Note   string  `json:"note"`
	Error  string  `json:"error,omitempty"`
}

const bindingBeta = "thinking-binding-controls-2026-08-01"
const compactEdit = "compact_20260112"
const compactTriggerTokens = 50000

// Run executes the beta/gated Claude checks: thinking block binding and
// context compaction. Both are expensive or beta-only, so they run only when
// the caller opts into the deep audit.
func Run(ctx context.Context, c Caller, model, signature, priorUser string, timeoutSeconds int) (Verdict, error) {
	v := Verdict{Note: "深度体检跑渠道指纹（hi 用量、行为注入、现签/复签、system 透传）加 beta 项（thinking 绑定、上下文压缩）。签名解码和协议严格性在常规检测里。"}
	v.Checks = append(v.Checks, hiUsage(ctx, c, model, timeoutSeconds))
	v.Checks = append(v.Checks, behaviorInjection(ctx, c, model, timeoutSeconds))
	v.Checks = append(v.Checks, signatureFresh(ctx, c, model, timeoutSeconds))
	v.Checks = append(v.Checks, systemPassthrough(ctx, c, model, timeoutSeconds))
	v.Checks = append(v.Checks, binding(ctx, c, model, signature, priorUser, timeoutSeconds))
	v.Checks = append(v.Checks, compact(ctx, c, model, timeoutSeconds))
	return v, nil
}

func binding(ctx context.Context, c Caller, model, signature, priorUser string, timeoutSeconds int) Check {
	check := Check{Name: "thinking_binding", Title: "thinking block 绑定"}
	if strings.TrimSpace(signature) == "" {
		check.Skipped = true
		check.Detail = "没有可用签名，跳过绑定校验"
		return check
	}
	if strings.TrimSpace(priorUser) == "" {
		priorUser = "请仔细分析后，必须调用 get_weather 查询北京天气。"
	}
	body := map[string]any{
		"model":      model,
		"max_tokens": 4096,
		"system":     "你是天气助手 v2（system 已变更）。",
		"thinking": map[string]any{
			"type":          "adaptive",
			"block_binding": map[string]string{"prefix_mismatch_behavior": "drop_block"},
		},
		"output_config": map[string]string{"effort": "high"},
		"tools": []any{map[string]any{
			"name":        "get_weather",
			"description": "Get the current weather for a city.",
			"input_schema": map[string]any{
				"type":       "object",
				"properties": map[string]any{"city": map[string]string{"type": "string"}},
				"required":   []string{"city"},
			},
		}},
		"messages": []any{
			map[string]any{"role": "user", "content": priorUser},
			map[string]any{"role": "assistant", "content": []any{
				map[string]any{"type": "thinking", "thinking": "", "signature": signature},
				map[string]any{"type": "tool_use", "id": "toolu_bdrk_01EbDQrcMUtxWVRDawp6yUfx", "name": "get_weather", "input": map[string]string{"city": "北京"}},
			}},
			map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "tool_result", "tool_use_id": "toolu_bdrk_01EbDQrcMUtxWVRDawp6yUfx", "content": "sunny, 26C"},
			}},
		},
	}
	res, err := c.PostMessagesRaw(ctx, body, bindingBeta, timeoutSeconds)
	if err != nil {
		check.Detail = err.Error()
		return check
	}
	check.Status = res.Status
	switch {
	case res.Status != 200 && unstable(res.Status):
		check.Skipped = true
		check.Detail = "上游不可用/限流（HTTP " + fmt.Sprint(res.Status) + "），按不定处理"
	case res.Status != 200:
		check.Detail = "HTTP " + fmt.Sprint(res.Status) + " " + clip(res.Err, 160)
	case contains(res.Transforms, "thinking_dropped"):
		check.Pass = true
		check.Detail = "前缀变更后思维链被丢弃（thinking_dropped）"
	case !res.HasInputTransformations:
		check.Skipped = true
		check.Detail = "响应没有 input_transformations 字段（官方直连口径），这项无法用该字段判定"
	default:
		check.Detail = "响应有 input_transformations 但没有 thinking_dropped：" + strings.Join(res.Transforms, ",")
	}
	return check
}

func compact(ctx context.Context, c Caller, model string, timeoutSeconds int) Check {
	check := Check{Name: "context_compact", Title: "上下文压缩"}
	body := map[string]any{
		"model":      model,
		"max_tokens": 2048,
		"system":     "你是一个智能助理",
		"messages": []any{map[string]any{
			"role":    "user",
			"content": compactCorpus(compactTriggerTokens),
		}},
		"context_management": map[string]any{
			"edits": []any{map[string]any{
				"type":                   compactEdit,
				"trigger":                map[string]any{"type": "input_tokens", "value": compactTriggerTokens},
				"pause_after_compaction": true,
			}},
		},
	}
	res, err := c.PostMessagesRaw(ctx, body, "", timeoutSeconds)
	if err != nil {
		check.Detail = err.Error()
		return check
	}
	check.Status = res.Status
	switch {
	case res.Status != 200 && unstable(res.Status):
		check.Skipped = true
		check.Detail = "上游不可用/限流（HTTP " + fmt.Sprint(res.Status) + "），按不定处理"
	case res.Status != 200:
		check.Detail = "HTTP " + fmt.Sprint(res.Status) + " " + clip(res.Err, 160)
	case strings.EqualFold(res.StopReason, "compaction"):
		check.Pass = true
		check.Detail = fmt.Sprintf("stop_reason=compaction（输入 %d tokens）", res.InputTokens)
	default:
		check.Detail = "stop_reason=" + res.StopReason
	}
	return check
}

// compactCorpus builds a record list long enough to cross the trigger. About
// four characters per token keeps the estimate within ~10% of the real count.
func compactCorpus(tokens int) string {
	target := tokens * 4
	var b strings.Builder
	b.Grow(target + 256)
	line := 0
	for b.Len() < target {
		line++
		fmt.Fprintf(&b, "Record %d: the quick brown fox jumps over the lazy dog near warehouse %d, shipment status pending, priority level %d.\n", line, line%97, line%5)
	}
	return b.String()
}

// unstable marks statuses that say nothing about the channel: outages, rate
// limits and timeouts. The fingerprint spec calls these 不定 instead of fail.
func unstable(status int) bool {
	return status >= 500 || status == 429 || status == 408
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}
