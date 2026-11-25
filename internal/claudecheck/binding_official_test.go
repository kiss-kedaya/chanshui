package claudecheck

import (
	"context"
	"testing"
)

func TestBindingSkippedOnOfficialShape(t *testing.T) {
	c := &fakeCaller{result: Result{Status: 200, StopReason: "end_turn"}}
	v, _ := Run(context.Background(), c, "claude-opus-5", "sig", "prior", 30)
	binding := findCheck(v.Checks, "thinking_binding")
	if !binding.Skipped || binding.Pass {
		t.Fatalf("official-shape binding = %+v", binding)
	}
}
