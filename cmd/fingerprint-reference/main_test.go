package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestReadCaptureKeepsTrainingAndHoldoutSeparate(t *testing.T) {
	for _, model := range []string{"grok-4.6", "grok-4.7", "deepseek-flash", "anthropic/claude-fable-5", "openai/gpt-6.1-sol", "moonshotai/kimi-k3"} {
		t.Run(model, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "capture.json")
			body := map[string]any{"model": model, "base_url": "https://fixture.invalid", "records": []any{
				map[string]any{"split": "train", "answer": "1,2,3", "usable": true},
				map[string]any{"split": "holdout", "answer": "4,5,6", "expected_count": 3, "usable": true},
				map[string]any{"split": "train", "answer": "7,8,9", "usable": false},
			}}
			raw, _ := json.Marshal(body)
			if err := os.WriteFile(p, raw, 0600); err != nil {
				t.Fatal(err)
			}
			ref, holdout := readCapture(p)
			if len(ref.Samples) != 1 || len(holdout) != 1 || ref.Samples[0][0] != 1 || holdout[0].Text != "4,5,6" {
				t.Fatal("split or invalid filtering failed")
			}
			if model == "deepseek-flash" {
				if ref.ID != "deepseek-v4.1-flash" || ref.Family != "deepseek" {
					t.Fatal(ref)
				}
			} else if model == "anthropic/claude-fable-5" || model == "openai/gpt-6.1-sol" || model == "moonshotai/kimi-k3" {
				if len(ref.Aliases) != 1 || ref.Aliases[0] != model || ref.ID == model || ref.Family == "" {
					t.Fatal("provider alias lost", ref)
				}
			} else if ref.ID != model || ref.Family != "grok" || ref.FamilyName != "Grok" {
				t.Fatal(ref)
			}
		})
	}
}
