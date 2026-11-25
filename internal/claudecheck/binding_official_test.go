package claudecheck

import (
	"context"
	"testing"
)

func TestBindingSkippedOnOfficialShape(t *testing.T) {
	c := &fakeCaller{result: Result{Status: 200, StopReason: "end_turn"}}
	v, _ := Run(context.Background(), c, "claude-opus-5", "sig", "prior", 30)
	if !v.Checks[0].Skipped || v.Checks[0].Pass {
		t.Fatalf("official-shape binding = %+v", v.Checks[0])
	}
}
