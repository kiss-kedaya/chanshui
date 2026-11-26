package cacheforce

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Caller posts one Messages body and returns the usage fields.
type Caller interface {
	PostMessagesRaw(ctx context.Context, body map[string]any, beta string, timeoutSeconds int) (Result, error)
}

// Result mirrors the gateway response subset.
type Result struct {
	Status          int
	Err             string
	Answer          string
	InputTokens     int
	CacheCreation5m int
	CacheCreation1h int
	CacheRead       int
}

// Step is one request's usage.
type Step struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Status  int    `json:"status"`
	Input   int    `json:"input"`
	Write5m int    `json:"write_5m"`
	Write1h int    `json:"write_1h"`
	Read    int    `json:"read"`
}

// Verdict reports whether the upstream writes cache without cache_control.
type Verdict struct {
	Steps  []Step `json:"steps"`
	Forced *bool  `json:"forced,omitempty"`
	// Tier5m and Tier1h say whether an explicit cache_control wrote to the
	// tier it asked for. Only measured once the upstream is known not to force
	// cache writes, so both stay nil otherwise.
	Tier5m     *bool  `json:"tier_5m,omitempty"`
	Tier1h     *bool  `json:"tier_1h,omitempty"`
	Conclusion string `json:"conclusion"`
	Note       string `json:"note"`
	Error      string `json:"error,omitempty"`
}

const bodyTokens = 8000
const cacheBeta5m = ""
const cacheBeta1h = "extended-cache-ttl-2025-04-11"

// Run measures forced cache writes. Each stage sends the same unique body
// twice: the first request shows whether the upstream writes cache on its own,
// the replay shows whether it reads back. Three stages: no cache_control, 5m
// ephemeral, and 1h ephemeral.
func Run(ctx context.Context, c Caller, model string, timeoutSeconds int) Verdict {
	v := Verdict{Note: "每步换唯一 nonce，正文约 8000 token，max_tokens=16。无 cache_control 时 input 很小却出现大额 cache write/read = 强制写缓存；正常透传应是大额 input、write=0、read=0。"}
	stages := []struct {
		key   string
		label string
		beta  string
		block map[string]any
	}{
		{"plain", "无 cache_control", "", nil},
		{"5m", "5 分钟缓存", cacheBeta5m, map[string]any{"type": "ephemeral"}},
		{"1h", "1 小时缓存", cacheBeta1h, map[string]any{"type": "ephemeral", "ttl": "1h"}},
	}
	for _, stage := range stages {
		text := corpus(fmt.Sprintf("%s-%d", stage.key, nowNano()))
		for attempt := 1; attempt <= 2; attempt++ {
			body := map[string]any{"model": model, "max_tokens": 16, "stream": true}
			if stage.block == nil {
				body["messages"] = []any{map[string]any{"role": "user", "content": text}}
			} else {
				body["messages"] = []any{map[string]any{"role": "user", "content": []any{
					map[string]any{"type": "text", "text": text, "cache_control": stage.block},
				}}}
			}
			res, err := c.PostMessagesRaw(ctx, body, stage.beta, timeoutSeconds)
			if err != nil {
				v.Error = err.Error()
				return v
			}
			v.Steps = append(v.Steps, Step{
				Key: stage.key, Label: stage.label, Status: res.Status,
				Input: res.InputTokens, Write5m: res.CacheCreation5m, Write1h: res.CacheCreation1h, Read: res.CacheRead,
			})
			if res.Status != 200 {
				v.Conclusion = fmt.Sprintf("%s 返回 HTTP %d，样本不定", stage.label, res.Status)
				return v
			}
		}
	}
	plain := v.Steps[0]
	if plain.Input <= 16 && (plain.Write5m+plain.Write1h+plain.Read) >= bodyTokens/4 {
		forced := true
		v.Forced = &forced
		v.Conclusion = "强制写缓存：没带 cache_control 也被记成 cache write/read"
	} else if plain.Write5m+plain.Write1h+plain.Read == 0 && plain.Input >= bodyTokens/2 {
		forced := false
		v.Forced = &forced
		v.Conclusion = "透传：第 1 步不写不读"
	} else {
		v.Conclusion = fmt.Sprintf("样本介于两者之间：input=%d write5m=%d write1h=%d read=%d", plain.Input, plain.Write5m, plain.Write1h, plain.Read)
	}
	if v.Forced != nil && !*v.Forced {
		five := v.Steps[2]
		hour := v.Steps[4]
		tier5m := five.Write5m > 0 && five.Write1h == 0
		tier1h := hour.Write1h > 0
		v.Tier5m, v.Tier1h = &tier5m, &tier1h
		if tier5m {
			v.Conclusion += "；5 分钟档生效"
		} else {
			v.Conclusion += "；5 分钟档没有按参数写入"
		}
		if tier1h {
			v.Conclusion += "；1 小时档生效"
		} else {
			v.Conclusion += "；1 小时档没有按参数写入"
		}
	}
	return v
}

func corpus(nonce string) string {
	target := bodyTokens * 4
	var b strings.Builder
	b.Grow(target + 256)
	b.WriteString("nonce=" + nonce + "\n")
	n := 0
	for b.Len() < target {
		n++
		fmt.Fprintf(&b, "Record %d [%s]: the quick brown fox jumps over the lazy dog near warehouse %d, shipment status pending, priority level %d.\n", n, nonce, n%97, n%5)
	}
	return b.String()
}

func nowNano() int64 {
	return time.Now().UnixNano()
}
