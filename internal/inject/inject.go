package inject

import (
	"chanzhushui/internal/cachecheck"
	"chanzhushui/internal/gateway"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strings"
	"time"
)

type Point struct {
	N     int `json:"n"`
	Input int `json:"input"`
	CC    int `json:"cc"`
	CR    int `json:"cr"`
	Total int `json:"total"`
}
type ProbeDiag struct {
	SSEOK     bool   `json:"sse_ok"`
	SSEFrames int    `json:"sse_frames"`
	StreamErr string `json:"stream_error,omitempty"`
}
type Result struct {
	Inj         int                `json:"inj"`
	Slope       float64            `json:"slope"`
	Points      []Point            `json:"points"`
	CacheHeader string             `json:"cache_header"`
	Via         string             `json:"via"`
	Probe       *ProbeDiag         `json:"probe,omitempty"`
	R2          *float64           `json:"r2"`
	MaxResidual *float64           `json:"max_residual"`
	Reliable    bool               `json:"reliable"`
	Score       *int               `json:"score"`
	MaxScore    int                `json:"max_score"`
	Note        string             `json:"note"`
	Cache       *cachecheck.Result `json:"cache,omitempty"`
}

var sampleNs = []int{1, 800}

const maxAttempts = 3

var BackoffDelay = 2 * time.Second
var retryDelay = time.Millisecond

// InjectionScore is separate from the IQ score. Boundaries are inclusive at 1000.
func InjectionScore(n int) int {
	switch {
	case n < 50:
		return 10
	case n <= 1000:
		return 8
	case n <= 3000:
		return 6
	case n <= 6000:
		return 4
	case n <= 10000:
		return 2
	default:
		return 0
	}
}
func Measure(ctx context.Context, gw gateway.Client, model string) (Result, error) {
	lines := 64
	if gateway.IsClaude(model) {
		lines = 160
	}
	var prefix strings.Builder
	fmt.Fprintf(&prefix, "Cache and injection audit reference.\n")
	for i := 0; i < lines; i++ {
		fmt.Fprintf(&prefix, "Record %04d: inventory verification values %d %d %d, stable repeated reference.\n", i, i*17+19, i*31+43, i*7+11)
	}
	hash := sha256.Sum256([]byte(prefix.String()))
	prefixHash := hex.EncodeToString(hash[:])
	system := gateway.SystemBlock{Text: prefix.String()}
	if gw.ProtocolFor(model) == "messages" {
		system.CacheControl = &gateway.CacheControl{TTL: "1h"}
	}
	// Three sequential calls serve both measurements: n=1 and n=800 estimate
	// the injected offset, while the repeated n=800 call measures warm reads.
	metrics := make([]gateway.Metrics, 0, 3)
	all := make([]Point, 0, 2)
	history := []gateway.ConversationMessage{}
	var lastErr error
	for i, n := range []int{sampleNs[0], sampleNs[1], sampleNs[1]} {
		m, e := gw.CallMessages(ctx, gateway.CallRequest{Model: model, MaxTokens: 16, Prompt: strings.Repeat("A", n), History: history, System: []gateway.SystemBlock{system}})
		if e != nil {
			lastErr = fmt.Errorf("n=%d: %w", n, e)
			break
		}
		if m.Err != "" {
			lastErr = fmt.Errorf("n=%d: %s", n, m.Err)
			break
		}
		if m.InputTokens < 0 || m.CacheRead < 0 || m.CacheCreation5m < 0 || m.CacheCreation1h < 0 || !m.UsagePresent {
			lastErr = fmt.Errorf("n=%d: invalid or missing token usage", n)
			break
		}
		metrics = append(metrics, m)
		if i < 2 {
			all = append(all, Point{N: n, Input: m.InputTokens, CC: m.CacheCreation5m + m.CacheCreation1h, CR: m.CacheRead, Total: m.InputTokens + m.CacheCreation5m + m.CacheCreation1h + m.CacheRead})
		}
		if i == 0 {
			gw.PinProtocol(m.Protocol)
		}
		if strings.TrimSpace(m.Answer) != "" {
			history = append(history, gateway.ConversationMessage{Role: "assistant", Content: m.Answer})
		}
	}
	r := Result{MaxScore: 10, Points: all, Note: "三次串行请求合并完成注入大小与缓存读写验证：前两轮估计注入偏移，第三轮读取比例只相对上一轮缓存读写量计算。"}
	if len(metrics) < 3 {
		return r, fmt.Errorf("注入/缓存采样不足：%d/3 有效；%v", len(metrics), lastErr)
	}
	for _, m := range metrics {
		if m.CacheHeader != "" {
			r.CacheHeader = m.CacheHeader
		}
		r.Via = m.Via
	}
	cacheResult := cachecheck.FromMetrics(metrics, prefixHash, metrics[0].Protocol, anyRetry(metrics))
	r.Cache = &cacheResult
	slope, intercept := linReg(r.Points)
	r.Slope = slope
	r.Inj = int(math.Round(math.Max(0, intercept)))
	// Two points have zero residual by construction, so R2 and residual remain null.
	r.Reliable = slope > 0 && intercept >= -2
	if r.Reliable {
		score := InjectionScore(r.Inj)
		r.Score = &score
	} else {
		r.Note += " 两次采样用量异常，暂不计分。"
	}
	return r, nil
}

func anyRetry(metrics []gateway.Metrics) bool {
	for _, m := range metrics {
		if m.Attempts > 1 {
			return true
		}
	}
	return false
}
func linReg(points []Point) (slope, intercept float64) {
	var n, sx, sy, sxx, sxy float64
	for _, p := range points {
		x, y := float64(p.N), float64(p.Total)
		n++
		sx += x
		sy += y
		sxx += x * x
		sxy += x * y
	}
	den := n*sxx - sx*sx
	if den == 0 {
		return 0, sy / n
	}
	slope = (n*sxy - sx*sy) / den
	return slope, (sy - slope*sx) / n
}
