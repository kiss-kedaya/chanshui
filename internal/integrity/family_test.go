package integrity

import "testing"

func TestDilutedMatchesRequestedFamilyAndClaudeLine(t *testing.T) {
	if Diluted("claude-opus-4-6", "claude", "claude-opus-4-6") {
		t.Fatal("matching opus marked diluted")
	}
	if !Diluted("claude-opus-4-6", "claude", "claude-sonnet-4-5") {
		t.Fatal("opus routed to sonnet")
	}
	if !Diluted("gpt-6-sol", "claude", "claude-opus-4-6") {
		t.Fatal("family mismatch")
	}
	if Diluted("gemini-2.5-pro", "unknown", "") {
		t.Fatal("missing baseline is not dilution")
	}
}
