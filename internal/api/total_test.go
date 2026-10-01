package api

import "testing"

func TestBuildTotalFullPassAndDilutedFingerprint(t *testing.T) {
	pass := true
	v := Verdict{
		Fingerprint: map[string]any{"family": "claude", "top_model": "claude-opus-4-6"},
		IQ:          map[string]any{"complete": true, "score": 100},
		Injection:   map[string]any{"score": 10},
		Tools:       map[string]any{"pass": true},
		Web:         map[string]any{"status": "pass", "pass": true, "protocol": "messages"},
		Thinking:    map[string]any{"score": 10},
		Cache:       map[string]any{"pass": pass},
	}
	total := buildTotal(v, "claude-opus-4-6")
	if total.Score != 100 {
		t.Fatalf("full score %d %+v", total.Score, total.Parts)
	}
	v.Fingerprint = map[string]any{"family": "claude", "top_model": "claude-sonnet-4-5"}
	total = buildTotal(v, "claude-opus-4-6")
	if total.Score != 70 {
		t.Fatalf("diluted score %d", total.Score)
	}
}

func TestBuildTotalChatWithoutCacheStill100(t *testing.T) {
	v := Verdict{
		Fingerprint: map[string]any{"family": "unknown"},
		IQ:          map[string]any{"complete": true, "score": 100},
		Injection:   map[string]any{"score": 10},
		Tools:       map[string]any{"pass": true},
		Web:         map[string]any{"status": "unknown", "protocol": "chat"},
	}
	total := buildTotal(v, "gemini-2.5-pro")
	if total.Score != 100 {
		t.Fatalf("renorm score %d %+v", total.Score, total.Parts)
	}
}
