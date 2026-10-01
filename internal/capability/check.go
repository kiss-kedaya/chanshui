package capability

import (
	"bytes"
	"chanzhushui/internal/gateway"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

type Result struct {
	Status   string `json:"status"`
	Pass     *bool  `json:"pass"`
	Reason   string `json:"reason"`
	Protocol string `json:"protocol"`
	Evidence any    `json:"evidence,omitempty"`
	Attempts int    `json:"attempts"`
}

func Tools(ctx context.Context, c gateway.Client, model string, timeout int) (Result, error) {
	var nonceBytes [8]byte
	if _, e := rand.Read(nonceBytes[:]); e != nil {
		return Result{}, e
	}
	nonce := hex.EncodeToString(nonceBytes[:])
	r := Result{Status: "fail", Protocol: c.ProtocolFor(model)}
	schema := map[string]any{"type": "object", "properties": map[string]any{"nonce": map[string]string{"type": "string"}, "sum": map[string]string{"type": "integer"}}, "required": []string{"nonce", "sum"}, "additionalProperties": false}
	q := gateway.CallRequest{Model: model, Prompt: fmt.Sprintf("Call audit_echo exactly once with nonce %s and sum equal to 17+25. Do not print the arguments as prose. After the tool result arrives, reply only with the exact confirmation token from its result.", nonce), Tools: []gateway.ToolDefinition{{Name: "audit_echo", Description: "Return diagnostic values without side effects.", Parameters: schema}}, ForceTool: "audit_echo", MaxTokens: 512, TimeoutSeconds: timeout}
	m, e := c.CallMessages(ctx, q)
	r.Attempts = m.Attempts
	if e != nil {
		return r, e
	}
	if m.Err != "" {
		return r, fmt.Errorf("%s", m.Err)
	}
	pass := false
	for _, call := range m.ToolCalls {
		var args struct {
			Nonce string
			Sum   int
		}
		if len(m.ToolCalls) == 1 && call.Name == "audit_echo" && call.ID != "" && decodeToolArgs(call.Arguments, &args) == nil && args.Nonce == nonce && args.Sum == 42 {
			pass = true
		}
	}
	r.Pass = &pass
	r.Evidence = m.ToolCalls
	r.Reason = "未收到名称、参数均正确的结构化工具调用；普通文本不算工具调用。"
	if pass {
		confirmation := make([]byte, 12)
		if _, err := rand.Read(confirmation); err != nil {
			return r, err
		}
		token := "tool-confirm-" + hex.EncodeToString(confirmation)
		c.PinProtocol(m.Protocol)
		q.ForceTool = ""
		q.ToolReply = &gateway.ToolReply{Call: m.ToolCalls[0], Output: token}
		follow, err := c.CallMessages(ctx, q)
		r.Attempts += follow.Attempts
		if err != nil {
			return r, err
		}
		if follow.Err != "" {
			return r, fmt.Errorf("%s", follow.Err)
		}
		roundtrip := strings.TrimSpace(follow.Answer) == token && len(follow.ToolCalls) == 0
		r.Pass = &roundtrip
		r.Evidence = map[string]any{"calls": m.ToolCalls, "roundtrip": roundtrip, "confirmation_received": follow.Answer}
		if roundtrip {
			r.Status = "pass"
			r.Reason = "工具名称、随机参数、调用ID与结果回传均验证通过。"
		} else {
			r.Status = "fail"
			r.Reason = "工具调用格式正确，但对应结果回传未完成，不能算完整通过。"
		}
	}
	return r, nil
}
func Web(ctx context.Context, c gateway.Client, model string, timeout int) (Result, error) {
	r := Result{Status: "unknown", Protocol: c.ProtocolFor(model)}
	if r.Protocol == "chat" {
		r.Reason = "Chat Completions没有统一的服务端联网工具协议；不根据模型自述判断联网。"
		return r, nil
	}
	q := gateway.CallRequest{Model: model, Prompt: fmt.Sprintf("Use web search once to find the current latest release on github.com/golang/go/releases as of %s. Include the release tag and cite the source. Do not answer from memory.", time.Now().UTC().Format("2006-01-02")), WebSearch: true, MaxTokens: 1024, TimeoutSeconds: timeout}
	m, e := c.CallMessages(ctx, q)
	r.Attempts = m.Attempts
	if e != nil {
		return r, e
	}
	if m.Err != "" {
		return r, fmt.Errorf("%s", m.Err)
	}
	r.Evidence = map[string]any{"search_completed": m.WebSearchCompleted, "sources": m.SearchURLs}
	if m.WebSearchCompleted && len(m.SearchURLs) > 0 {
		pass := true
		r.Pass = &pass
		r.Status = "pass"
		r.Reason = "响应包含已执行搜索工具和来源证据；不是仅凭模型声称能联网。"
	} else {
		r.Reason = "未收到完整搜索工具与来源证据，联网能力未经验证。"
	}
	return r, nil
}

func decodeToolArgs(raw json.RawMessage, out any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if e := d.Decode(out); e != nil {
		return e
	}
	if d.Decode(new(any)) != io.EOF {
		return fmt.Errorf("extra tool data")
	}
	return nil
}
