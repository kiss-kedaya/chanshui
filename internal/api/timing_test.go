package api

import (
	"context"
	"testing"
	"time"
)

func TestExecutionBudgetPreservesSerialRequestRetries(t *testing.T) {
	s := &Server{ctx: context.Background()}
	for _, concurrency := range []int{0, 1, 16} {
		ctx, cancel := s.executionContext(Request{Timeout: 360, Concurrency: concurrency}, time.Now().Add(24*time.Hour).Unix())
		deadline, ok := ctx.Deadline()
		cancel()
		if !ok || time.Until(deadline) < 4*4*360*time.Second {
			t.Fatalf("concurrency=%d insufficient deadline: %v", concurrency, deadline)
		}
	}
	expiry := time.Now().Add(time.Hour).Unix()
	ctx, cancel := s.executionContext(Request{Timeout: 600, Concurrency: 1}, expiry)
	defer cancel()
	deadline, _ := ctx.Deadline()
	if deadline.Unix() != expiry {
		t.Fatal("job must not survive report TTL")
	}
}
