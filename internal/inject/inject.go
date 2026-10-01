package inject

import (
	"chanzhushui/internal/gateway"
	"context"
	"fmt"
	"math"
	"strings"
	"time"
)

type Point struct {
	N     int `json:"n"`
	Input int `json:"input"`
	Total int `json:"total"`
}

type ProbeDiag struct {
	SSEOK     bool   `json:"sse_ok"`
	SSEFrames int    `json:"sse_frames"`
	StreamErr string `json:"stream_error,omitempty"`
}

type Result struct {
	Inj         int        `json:"inj"`
	Slope       float64    `json:"slope"`
	Points      []Point    `json:"points"`
	CacheHeader string     `json:"cache_header"`
	Via         string     `json:"via"`
	Probe       *ProbeDiag `json:"probe,omitempty"`
	R2          *float64   `json:"r2"`
	MaxResidual *float64   `json:"max_residual"`
	Reliable    bool       `json:"reliable"`
	Score       *int       `json:"score"`
	MaxScore    int        `json:"max_score"`
	Note        string     `json:"note"`
}

var BackoffDelay = 2 * time.Second
var retryDelay = time.Millisecond

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

// Measure deliberately does not use cachecheck. Injection is two independent
// prompts so cache reads/writes cannot distort the offset estimate. Cache
// validation is a separate three-turn probe in internal/cachecheck.
func Measure(ctx context.Context, gw gateway.Client, model string, timeout ...int) (Result, error) {
	seconds := 0
	if len(timeout) > 0 {
		seconds = timeout[0]
	}
	points := make([]Point, 0, 2)
	var lastErr error
	for _, units := range []int{1, 800} {
		// A separated corpus keeps the two samples on a stable tokenizer path.
		// Repeating "A" without separators is often merged into a few BPE tokens.
		prompt := strings.TrimSpace(strings.Repeat("x ", units))
		m, err := gw.CallMessages(ctx, gateway.CallRequest{
			Model: model, Prompt: prompt,
			MaxTokens: 16, TimeoutSeconds: seconds, Validate: func(m gateway.Metrics) error {
				if !m.UsagePresent {
					return fmt.Errorf("missing token usage")
				}
				if m.StopReason == "max_tokens" || m.StopReason == "length" {
					return fmt.Errorf("response truncated")
				}
				if m.ThinkingTokens > 0 || m.ThinkLen > 0 {
					return fmt.Errorf("reasoning output included in sample")
				}
				if strings.TrimSpace(m.Answer) == "" {
					return fmt.Errorf("empty response")
				}
				return nil
			},
		})
		if err != nil {
			lastErr = fmt.Errorf("units=%d: %w", units, err)
			break
		}
		if m.Err != "" {
			lastErr = fmt.Errorf("units=%d: %s", units, m.Err)
			break
		}
		points = append(points, Point{N: len(prompt), Input: m.InputTokens, Total: m.InputTokens + m.CacheCreation5m + m.CacheCreation1h + m.CacheRead})
		gw.PinProtocol(m.Protocol)
	}
	r := Result{Points: points, MaxScore: 10, Note: "两次独立小请求估计协议固定开销；不复用缓存验证对话，按上游实际输入总量拟合近似值。"}
	if len(points) != 2 {
		return r, fmt.Errorf("注入采样不足：%d/2 有效；%v", len(points), lastErr)
	}
	r.Slope, r.Inj = fit(points)
	r.Inj = int(math.Round(math.Max(0, float64(r.Inj))))
	// Two points cannot establish a residual, but an implausible token/character
	// slope or offset still indicates a malformed usage report.
	r.Reliable = r.Slope > 0 && r.Slope <= 4 && r.Inj >= 0 && r.Inj <= 100000
	if r.Reliable {
		score := InjectionScore(r.Inj)
		r.Score = &score
	} else {
		r.Note += " 用量异常，暂不计分。"
	}
	return r, nil
}

func fit(points []Point) (float64, int) {
	if len(points) != 2 || points[1].N == points[0].N {
		return 0, 0
	}
	x1, x2 := float64(points[0].N), float64(points[1].N)
	y1, y2 := float64(points[0].Total), float64(points[1].Total)
	slope := (y2 - y1) / (x2 - x1)
	intercept := y1 - slope*x1
	return slope, int(math.Round(intercept))
}
