package inject

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"chanzhushui/internal/gateway"
)

// Point is one sampled (n, totals) observation from the regression.
type Point struct {
	N     int
	Input int
	CC    int // cache_creation 5m tokens reported for this point
	CR    int // cache_read tokens reported for this point
	Total int
}

// ProbeDiag carries the streaming probe diagnostics (not scored).
type ProbeDiag struct {
	SSEOK     bool
	SSEFrames int
	StreamErr string
}

// Result is the injection regression outcome.
type Result struct {
	Inj   int
	Slope float64
	// Points are ordered n = 1, 200, 500, 800, 1000.
	Points      []Point
	CacheHeader string
	Via         string
	Probe       *ProbeDiag
}

// sampleNs are the content lengths measured for the regression.
var sampleNs = []int{1, 200, 500, 800, 1000}

// maxAttempts bounds the per-point 429 retry loop.
const maxAttempts = 4

// BackoffDelay is the wait between 429 retries. Tests override it to keep
// runtimes sane; production value is 30s.
var BackoffDelay = 30 * time.Second

// retryDelay is a BackoffDelay value whose wait is negligible in tests.
var retryDelay = time.Millisecond

// streamProbeN is the content length for the one-off streaming probe.
const streamProbeN = 800

// streamProbeMaxTokens bounds the streaming probe reply.
const streamProbeMaxTokens = 32

// Measure runs the 5-point injection regression against gw/model, then one
// streaming probe. Totals per point are input_tokens + cache_creation(5m+1h)
// + cache_read, so mock upstreams that report inflated billing as
// cache_creation still land on the same line.
func Measure(ctx context.Context, gw gateway.Client, model string) (Result, error) {
	points := make([]Point, 0, len(sampleNs))
	var header, via string
	var firstErr error

	for _, n := range sampleNs {
		var m gateway.Metrics
		for attempt := 1; attempt <= maxAttempts; attempt++ {
			req := gateway.CallRequest{
				Model:     model,
				MaxTokens: 16,
				Prompt:    strings.Repeat("A", n),
				// TimeoutSeconds deliberately unset: the caller's ctx governs.
			}
			mm, err := gw.CallMessages(ctx, req)
			if err != nil {
				return Result{}, fmt.Errorf("n=%d call %d: %w", n, attempt, err)
			}
			m = mm
			if strings.Contains(m.Err, "429") {
				if attempt < maxAttempts {
					select {
					case <-time.After(BackoffDelay):
					case <-ctx.Done():
						return Result{}, fmt.Errorf("n=%d: ctx done during backoff: %w", n, ctx.Err())
					}
					continue
				}
				// retries exhausted: treat as a failed point
				if firstErr == nil {
					firstErr = fmt.Errorf("n=%d: %s", n, m.Err)
				}
				break
			}
			break
		}
		if m.Err != "" {
			if firstErr == nil {
				firstErr = fmt.Errorf("n=%d: %s", n, m.Err)
			}
			continue
		}
		total := m.InputTokens + m.CacheCreation5m + m.CacheCreation1h + m.CacheRead
		points = append(points, Point{
			N:     n,
			Input: m.InputTokens,
			CC:    m.CacheCreation5m + m.CacheCreation1h,
			CR:    m.CacheRead,
			Total: total,
		})
		if n == 500 {
			header, via = m.CacheHeader, m.Via
		}
	}

	if len(points) < 2 {
		if firstErr != nil {
			return Result{}, fmt.Errorf("inject measure: fewer than 2 usable points: %w", firstErr)
		}
		return Result{}, fmt.Errorf("inject measure: fewer than 2 usable points")
	}

	slope, intercept := linReg(points)
	inj := int(math.Round(intercept))

	probe := runStreamProbe(ctx, gw, model)

	return Result{
		Inj:         inj,
		Slope:       slope,
		Points:      points,
		CacheHeader: header,
		Via:         via,
		Probe:       probe,
	}, nil
}

// linReg returns the least-squares slope and intercept of Total on N.
func linReg(points []Point) (slope, intercept float64) {
	var n float64
	var sx, sy, sxx, sxy float64
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
	intercept = (sy - slope*sx) / n
	return slope, intercept
}

// runStreamProbe fires one streaming call at n=800 and reports the raw SSE
// frame count. Failure is diagnostic only: the probe never fails the overall
// measurement.
func runStreamProbe(ctx context.Context, gw gateway.Client, model string) *ProbeDiag {
	probe := &ProbeDiag{}
	req := gateway.CallRequest{
		Model:     model,
		MaxTokens: streamProbeMaxTokens,
		Prompt:    strings.Repeat("A", streamProbeN),
		Stream:    true,
	}
	m, err := gw.CallMessages(ctx, req)
	if err != nil {
		probe.StreamErr = err.Error()
		return probe
	}
	if m.Err != "" {
		probe.StreamErr = m.Err
		return probe
	}
	probe.SSEOK = true
	probe.SSEFrames = len(m.SSEFrames)
	return probe
}
