package api

import "testing"

func TestBuildTotalFullPassAndDilutedFingerprint(t *testing.T) {
	pass := true
	v := Verdict{
		Fingerprint: map[string]any{"family": "claude", "top_model": "claude-opus-4-6", "used_outputs": 3},
		IQ:          map[string]any{"complete": true, "score": 100},
		Injection:   map[string]any{"score": 10},
		Hidden:      map[string]any{"status": "clean", "score": 10},
		Tools:       map[string]any{"pass": true},
		Web:         map[string]any{"status": "pass", "pass": true, "protocol": "messages"},
		Thinking:    map[string]any{"score": 10},
		Cache:       map[string]any{"pass": pass},
	}
	total := buildTotal(v, "claude-opus-4-6")
	if total.Score != 100 {
		t.Fatalf("full score %d %+v", total.Score, total.Parts)
	}
	v.Fingerprint = map[string]any{"family": "claude", "top_model": "claude-sonnet-4-5", "used_outputs": 3}
	total = buildTotal(v, "claude-opus-4-6")
	if total.Score != 70 {
		t.Fatalf("diluted score %d", total.Score)
	}
	v.Fingerprint = map[string]any{"family": "claude", "top_model": "claude-opus-4-6", "used_outputs": 0}
	total = buildTotal(v, "claude-opus-4-6")
	if total.Score != 70 {
		t.Fatalf("empty fingerprint score %d", total.Score)
	}
}

// A near-tie between two enrolled candidates must not be read as dilution:
// gpt-6.1-sol and gpt-6-astra sit close enough on real traffic that live draws
// pick the wrong one about one time in six.
func TestBuildTotalLowConfidenceTieIsNotDiluted(t *testing.T) {
	v := Verdict{
		Fingerprint: map[string]any{"family": "gpt", "top_model": "gpt-6-astra", "top_prob": 0.51, "used_outputs": 3, "low_confidence": true},
		IQ:          map[string]any{"complete": true, "score": 100},
		Injection:   map[string]any{"score": 10},
		Hidden:      map[string]any{"status": "clean", "score": 10},
		Tools:       map[string]any{"pass": true},
		Web:         map[string]any{"status": "pass", "pass": true, "protocol": "messages"},
		Thinking:    map[string]any{"score": 10},
		Cache:       map[string]any{"pass": true},
	}
	if total := buildTotal(v, "gpt-6.1-sol"); total.Score != 100 {
		t.Fatalf("low-confidence tie scored %d, want 100", total.Score)
	}
	v.Fingerprint = map[string]any{"family": "gpt", "top_model": "gpt-6-astra", "top_prob": 0.95, "used_outputs": 3, "low_confidence": false}
	if total := buildTotal(v, "gpt-6.1-sol"); total.Score != 68 {
		t.Fatalf("confident mismatch scored %d, want 68", total.Score)
	}
}

func TestBuildTotalChatWithoutCacheStill100(t *testing.T) {
	v := Verdict{
		Fingerprint: map[string]any{"family": "unknown", "used_outputs": 0},
		IQ:          map[string]any{"complete": true, "score": 100},
		Injection:   map[string]any{"score": 10},
		Hidden:      map[string]any{"status": "clean", "score": 10},
		Tools:       map[string]any{"pass": true},
		Web:         map[string]any{"status": "unknown", "protocol": "chat"},
	}
	total := buildTotal(v, "gemini-2.5-pro")
	fp := 0
	for _, part := range total.Parts {
		if part.Key == "fingerprint" {
			fp = part.Coefficient
			if part.Contribution != 0 || part.Points != 0 {
				t.Fatalf("unknown fingerprint scored %+v", part)
			}
		}
	}
	if fp == 0 || total.Score != 100-fp {
		t.Fatalf("renorm score %d fingerprint coeff %d %+v", total.Score, fp, total.Parts)
	}
}
