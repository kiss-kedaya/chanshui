package inject

import (
	"chanzhushui/internal/cachecheck"
	"chanzhushui/internal/gateway"
	"context"
	"fmt"
	"math"
	"time"
)

type Point struct {
	N             int `json:"n"`
	Round         int `json:"round"`
	Input         int `json:"input"`
	CC            int `json:"cc"`
	CR            int `json:"cr"`
	Total         int `json:"total"`
	OutputBefore  int `json:"output_before"`
	AdjustedTotal int `json:"adjusted_total"`
}
type ProbeDiag struct {
	SSEOK     bool   `json:"sse_ok"`
	SSEFrames int    `json:"sse_frames"`
	StreamErr string `json:"stream_error,omitempty"`
}
type Result struct {
	Inj           int                `json:"inj"`
	Slope         float64            `json:"slope"`
	RoundOverhead float64            `json:"round_overhead"`
	Points        []Point            `json:"points"`
	CacheHeader   string             `json:"cache_header"`
	Via           string             `json:"via"`
	Probe         *ProbeDiag         `json:"probe,omitempty"`
	R2            *float64           `json:"r2"`
	MaxResidual   *float64           `json:"max_residual"`
	Reliable      bool               `json:"reliable"`
	Score         *int               `json:"score"`
	MaxScore      int                `json:"max_score"`
	Note          string             `json:"note"`
	Cache         *cachecheck.Result `json:"cache,omitempty"`
}

// Kept for compatibility with older package tests; request retry timing now
// belongs to gateway.Client and is applied independently to every probe.
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

// Measure reuses all three cache requests; optional timeout preserves callers
// that already supply a configured HTTP client.
func Measure(ctx context.Context, gw gateway.Client, model string, timeout ...int) (Result, error) {
	seconds := 0
	if len(timeout) > 0 {
		seconds = timeout[0]
	}
	col, err := cachecheck.Collect(ctx, gw, model, seconds)
	r := Result{MaxScore: 10, Points: make([]Point, 0, len(col.Metrics)), Cache: &col.Result,
		Note: "注入与缓存共用三次递增对话。按累计样本文本回归，扣除前轮输出并修正每轮新增包装；近似值仍包含首轮协议包装和短指令，不能当作隐藏提示词的精确数量。"}
	outputBefore := 0
	visibleOutput := true
	for i, m := range col.Metrics {
		if m.CacheHeader != "" {
			r.CacheHeader = m.CacheHeader
		}
		r.Via = m.Via
		if m.Err != "" || !m.UsagePresent {
			continue
		}
		total := m.InputTokens + m.CacheRead + m.CacheCreation5m + m.CacheCreation1h
		r.Points = append(r.Points, Point{N: col.Units[i], Round: i + 1, Input: m.InputTokens, CC: m.CacheCreation5m + m.CacheCreation1h,
			CR: m.CacheRead, Total: total, OutputBefore: outputBefore, AdjustedTotal: total - outputBefore})
		outputBefore += m.OutputTokens
		visibleOutput = visibleOutput && m.OutputReported && m.ThinkingTokens == 0 && m.ThinkLen == 0
	}
	if err != nil {
		return r, err
	}
	if len(r.Points) != 3 {
		return r, fmt.Errorf("注入/缓存采样不足：%d/3 有效", len(r.Points))
	}
	var intercept float64
	r.Slope, r.RoundOverhead, intercept = fitConversation(r.Points)
	r.Inj = int(math.Round(math.Max(0, intercept)))
	// Three fitted points cannot independently establish linearity. Keep R2
	// and residual null, and reject implausible slopes/framing corrections.
	r.Reliable = visibleOutput && r.Slope > 0 && r.Slope <= 4 && intercept >= -16 && r.RoundOverhead >= -4 && r.RoundOverhead <= 128
	if r.Reliable {
		score := InjectionScore(r.Inj)
		r.Score = &score
	} else {
		r.Note += " 用量不符合稳定的文本/包装模型，或包含未回传推理用量，暂不计分。"
	}
	return r, nil
}

// y_i = slope*x_i + roundOverhead*i + intercept, after removing prior output.
// Unequal appended lengths make the three unknowns identifiable. Keeping the
// entire test prefix in x is critical: it must never become injected overhead.
func fitConversation(points []Point) (slope, roundOverhead, intercept float64) {
	if len(points) != 3 {
		return
	}
	a, b, c := points[0], points[1], points[2]
	dx1, dx2 := float64(b.N-a.N), float64(c.N-b.N)
	dy1, dy2 := float64(b.AdjustedTotal-a.AdjustedTotal), float64(c.AdjustedTotal-b.AdjustedTotal)
	if dx1 <= 0 || dx2 <= 0 || dx1 == dx2 {
		return
	}
	slope = (dy1 - dy2) / (dx1 - dx2)
	// Usage is integer-valued. The large unequal intervals make the tokenizer
	// slope stable to roughly 1e-3; quantizing it removes one-token floor noise
	// from the intercept instead of turning rounding noise into fake injection.
	slope = math.Round(slope*1000) / 1000
	roundOverhead = dy1 - slope*dx1
	intercept = float64(a.AdjustedTotal) - slope*float64(a.N)
	return
}
