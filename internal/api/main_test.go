package api

import (
	"os"
	"testing"
	"time"

	"chanzhushui/internal/gateway"
)

// Production waits 2s/4s/8s after an upstream 5xx. Fixtures that return 5xx on
// purpose would otherwise spend ~14s per call and trip the tests' own deadlines.
func TestMain(m *testing.M) {
	gateway.Upstream5xxBackoff = time.Millisecond
	os.Exit(m.Run())
}
