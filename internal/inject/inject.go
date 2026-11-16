package inject

import (
	"context"
	"fmt"
	"github.com/kiss-kedaya/chanshui/internal/gateway"
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
	Mode        string     `json:"mode,omitempty"`
	Threshold   int        `json:"threshold,omitempty"`
	Nominal     int        `json:"nominal,omitempty"`
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
	if gateway.IsClaude(model) {
		return measureClaudeThreshold(ctx, gw, model, seconds)
	}
	points := make([]Point, 0, 2)
	var lastErr error
	for _, units := range []int{1, 800} {
		// A separated corpus keeps the two samples on a stable tokenizer path.
		// Repeating "A" without separators is often merged into a few BPE tokens.
		prompt := strings.TrimSpace(strings.Repeat("x ", units))
		m, err := gw.CallMessages(ctx, gateway.CallRequest{
			Model: model, Prompt: prompt,
			MaxTokens: 64, TimeoutSeconds: seconds, Validate: func(m gateway.Metrics) error {
				total := m.InputTokens + m.CacheCreation5m + m.CacheCreation1h + m.CacheRead
				if !m.UsagePresent || m.InputTokens < 0 || total <= 0 {
					return fmt.Errorf("missing token usage")
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
	r := Result{Points: points, MaxScore: 10, Note: "两次独立采样估计固定开销。"}
	if len(points) != 2 {
		return r, fmt.Errorf("注入采样不足：%d/2 有效；%v", len(points), lastErr)
	}
	if points[0].Input == 0 && points[1].Input == 0 {
		r.Reliable = false
		r.Note += " 输入全部计为缓存，无法拟合。"
		return r, nil
	}
	var raw int
	r.Slope, raw = fit(points)
	r.Reliable = r.Slope > 0 && r.Slope <= 4 && raw >= 0 && raw <= 100000
	if r.Reliable {
		r.Inj = raw
		score := InjectionScore(r.Inj)
		r.Score = &score
	} else {
		r.Note += " 样本异常，不计分。"
	}
	return r, nil
}

// Claude routes can add a hidden system prefix and deduct it from usage, so
// the old two-sample intercept sees zero. The cache write threshold is the
// billable prefix needed to reach the provider minimum: 0.8/5m first writes at
// 512; 1.2/1h first writes near 368 because about 144 hidden tokens are not
// billed. Measure the first writing prefix and subtract it from the nominal.
func measureClaudeThreshold(ctx context.Context, gw gateway.Client, model string, seconds int) (Result, error) {
	nominal := claudeNominal(model)
	r := Result{MaxScore: 10, Mode: "cache_threshold", Nominal: nominal, Note: "Claude 按缓存首写阈值反推隐藏前缀。"}
	points := []Point{}
	probe := func(reps int) (int, error) {
		nonce := fmt.Sprintf("%x", time.Now().UnixNano())
		text := "cache threshold probe " + nonce + " " + strings.TrimSpace(strings.Repeat("cache threshold probe ", reps))
		m, err := gw.CallMessages(ctx, gateway.CallRequest{
			Model: model, Prompt: "Reply OK", MaxTokens: 4, TimeoutSeconds: seconds,
			System: []gateway.SystemBlock{{Text: text, CacheControl: &gateway.CacheControl{}}},
		})
		if err != nil {
			return 0, err
		}
		if m.Err != "" {
			return 0, fmt.Errorf("%s", m.Err)
		}
		write := m.CacheCreation5m + m.CacheCreation1h
		points = append(points, Point{N: len(text), Input: m.InputTokens, Total: m.InputTokens + write + m.CacheRead})
		return write, nil
	}
	lo, hi := 0, 200
	if nominal >= 2048 {
		hi = 600
	}
	wrote, err := probe(hi)
	if err != nil {
		return r, err
	}
	for wrote == 0 && hi < 2400 {
		lo, hi = hi, hi*2
		wrote, err = probe(hi)
		if err != nil {
			return r, err
		}
	}
	if wrote == 0 {
		return r, fmt.Errorf("Claude 缓存首写未建立：前缀已到 %d 字符", hi)
	}
	threshold := wrote
	for lo+1 < hi {
		mid := (lo + hi) / 2
		w, e := probe(mid)
		if e != nil {
			return r, e
		}
		if w > 0 {
			hi, threshold = mid, w
		} else {
			lo = mid
		}
	}
	r.Points = points
	r.Threshold = threshold
	hidden := nominal - threshold
	if hidden < 8 {
		hidden = 0
	}
	r.Inj = hidden
	score := InjectionScore(hidden)
	r.Score = &score
	r.Reliable = true
	r.Note = fmt.Sprintf("Claude 按缓存首写阈值反推：首写 %d token，名义门槛 %d，隐藏 %d。", threshold, nominal, hidden)
	return r, nil
}

func claudeNominal(model string) int {
	m := strings.ToLower(model)
	if strings.Contains(m, "haiku") {
		return 2048
	}
	if strings.Contains(m, "-4-") || strings.Contains(m, "-4.") {
		return 1024
	}
	return 512
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
