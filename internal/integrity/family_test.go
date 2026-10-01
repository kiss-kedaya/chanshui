package integrity

import "testing"

func TestDilutedMatchesRequestedModel(t *testing.T) {
	if Diluted("claude-opus-4-6", "claude", "claude-opus-4-6") {
		t.Fatal("matching opus marked diluted")
	}
	if !Diluted("claude-opus-4-6", "claude", "claude-opus-4-5") {
		t.Fatal("different opus version is still another model")
	}
	if !Diluted("claude-opus-4-6", "claude", "claude-sonnet-4-5") {
		t.Fatal("opus routed to sonnet")
	}
	if !Diluted("gpt-6-astra", "gpt", "gpt-5.6-luna") {
		t.Fatal("astra routed to luna")
	}
	if !Diluted("gpt-6-sol", "claude", "claude-opus-4-6") {
		t.Fatal("family mismatch")
	}
	if Diluted("gemini-2.5-pro", "unknown", "") {
		t.Fatal("missing baseline is not dilution")
	}
	if Diluted("GPT-5.4", "openai", "gpt-5.4") {
		t.Fatal("same model with different punctuation")
	}
}

func TestPresentedScoreDropsOnlyDilutedFingerprint(t *testing.T) {
	if got := PresentedScore(85, 32, true); got != 53 {
		t.Fatalf("diluted total %d", got)
	}
	if got := PresentedScore(85, 32, false); got != 85 {
		t.Fatalf("matching total %d", got)
	}
	if got := PresentedScore(20, 32, true); got != 0 {
		t.Fatalf("floor %d", got)
	}
}
