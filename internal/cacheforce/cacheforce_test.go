package cacheforce

import (
	"context"
	"strings"
	"testing"
)

type fakeCaller struct{ forced bool }

func hasBlock(body map[string]any) (string, bool) {
	msgs, _ := body["messages"].([]any)
	if len(msgs) == 0 {
		return "", false
	}
	first, _ := msgs[0].(map[string]any)
	blocks, ok := first["content"].([]any)
	if !ok || len(blocks) == 0 {
		return "", false
	}
	block, _ := blocks[0].(map[string]any)
	cc, ok := block["cache_control"].(map[string]any)
	if !ok {
		return "", false
	}
	ttl, _ := cc["ttl"].(string)
	return ttl, true
}

func (f fakeCaller) PostMessagesRaw(ctx context.Context, body map[string]any, beta string, timeoutSeconds int) (Result, error) {
	if f.forced {
		if _, ok := hasBlock(body); !ok {
			return Result{Status: 200, InputTokens: 2, CacheRead: 7000}, nil
		}
		return Result{Status: 200, InputTokens: 2, CacheRead: 7000}, nil
	}
	ttl, ok := hasBlock(body)
	if !ok {
		return Result{Status: 200, InputTokens: 8000}, nil
	}
	if ttl == "1h" {
		return Result{Status: 200, InputTokens: 2, CacheCreation1h: 7000}, nil
	}
	return Result{Status: 200, InputTokens: 2, CacheCreation5m: 7000}, nil
}

func TestForcedWriteDetected(t *testing.T) {
	v := Run(context.Background(), fakeCaller{forced: true}, "claude-opus-5", 10)
	if v.Forced == nil || !*v.Forced {
		t.Fatalf("verdict = %+v", v)
	}
}

func TestPassthroughKeepsTtl(t *testing.T) {
	v := Run(context.Background(), fakeCaller{}, "claude-opus-5", 10)
	if v.Forced == nil || *v.Forced {
		t.Fatalf("verdict = %+v", v)
	}
	if !strings.Contains(v.Conclusion, "5 分钟档生效") || !strings.Contains(v.Conclusion, "1 小时档生效") {
		t.Fatalf("conclusion = %q", v.Conclusion)
	}
}
