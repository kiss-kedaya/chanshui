package api

import (
	"strings"
	"testing"

	"github.com/kiss-kedaya/chanshui/internal/iq"
)

func TestBestQuestionSeedPrefersThinkingText(t *testing.T) {
	questions := []iq.Question{{ID: "Q6"}, {ID: "Q8"}}
	answers := map[string]QuestionResult{
		"Q6": {ID: "Q6", Prompt: "p6", Signature: strings.Repeat("s", 40000)},
		"Q8": {ID: "Q8", Prompt: "p8", ThinkingText: "real reasoning", Signature: "sig8"},
	}
	seed, ok := bestQuestionSeed(questions, answers)
	if !ok || seed.ID != "Q8" || seed.Metrics.ThinkingText != "real reasoning" {
		t.Fatalf("seed = %+v", seed)
	}
}

func TestBestQuestionSeedFallsBackToSignature(t *testing.T) {
	questions := []iq.Question{{ID: "Q1"}, {ID: "Q2"}}
	answers := map[string]QuestionResult{
		"Q1": {ID: "Q1", Signature: "short"},
		"Q2": {ID: "Q2", Signature: strings.Repeat("x", 100)},
	}
	seed, ok := bestQuestionSeed(questions, answers)
	if !ok || seed.ID != "Q2" {
		t.Fatalf("seed = %+v", seed)
	}
}
