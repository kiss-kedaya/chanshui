package api

import (
	"context"
	"github.com/kiss-kedaya/chanshui/internal/gateway"
	"time"
)

type Timing struct {
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at,omitempty"`
	TotalMs    int64  `json:"total_ms"`
	LastRunMs  int64  `json:"last_run_ms"`
	Runs       int    `json:"runs"`
}

func (s *Server) executionContext(q Request, expiresAt int64) (context.Context, context.CancelFunc) {
	// Budget each request's four attempts, including serial probes and a
	// conservative allowance for time spent waiting on the per-job semaphore.
	waves := 4
	if q.Concurrency > 0 {
		waves += (32 + q.Concurrency - 1) / q.Concurrency
	}
	budget := time.Duration(waves*(gateway.RetryCount+1)) * (time.Duration(q.Timeout)*time.Second + 2*time.Second)
	deadline := time.Now().Add(budget)
	if expiry := time.Unix(expiresAt, 0); expiry.Before(deadline) {
		deadline = expiry
	}
	return context.WithDeadline(s.ctx, deadline)
}
