package claudecheck

import (
	"context"
	"strings"
	"testing"
)

type downCaller struct{ status int }

func (d downCaller) PostMessagesRaw(ctx context.Context, body map[string]any, beta string, timeoutSeconds int) (Result, error) {
	return Result{Status: d.status}, nil
}

func TestOutageIsUndeterminedNotFailed(t *testing.T) {
	v, _ := Run(context.Background(), downCaller{status: 503}, "claude-opus-5", "sig", "prior", 10)
	for _, name := range []string{"hi_usage", "injection", "signature_fresh", "system_passthrough", "thinking_binding", "context_compact"} {
		check := findCheck(v.Checks, name)
		if !check.Skipped || check.Pass {
			t.Fatalf("%s should be skipped on 503: %+v", name, check)
		}
	}
}

func TestBehaviorInjectionSkipsWhenAllRejected(t *testing.T) {
	c := &fakeCaller{result: Result{Status: 500}}
	check := behaviorInjection(context.Background(), c, "claude-opus-5", 5)
	if !check.Skipped || check.Pass || !strings.Contains(check.Detail, "按不定处理") {
		t.Fatalf("check = %+v", check)
	}
}
