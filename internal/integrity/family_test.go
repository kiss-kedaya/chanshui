package integrity

import "testing"

func TestFingerprintState(t *testing.T) {
	for _, tc := range []struct{ requested, family, top, want string }{
		{"deepseek-v4.1-flash-1017", "deepseek", "deepseek-flash", FingerprintMatch},
		{"deepseek-flash", "deepseek", "deepseek-v4.1-flash-1017", FingerprintMatch},
		{"deepseek-v4.1-flash", "deepseek", "deepseek-v4.1-flash-0910", FingerprintMatch},
		{"deepseek-v4.1-flash-0910", "deepseek", "deepseek-flash", FingerprintSuspicious},
		{"deepseek-flash", "deepseek", "deepseek-v4.1-flash-0910", FingerprintSuspicious},
		{"deepseek-v4-flash", "deepseek", "deepseek-flash", FingerprintSuspicious},
		{"deepseek-flash", "deepseek", "deepseek-v4-pro", FingerprintSuspicious},
		{"claude-opus-4-6", "claude", "claude-opus-4-6", FingerprintMatch},
		{"claude-opus-4-6", "claude", "claude-opus-4-5", FingerprintSuspicious},
		{"claude-opus-4-6", "claude", "claude-sonnet-4-5", FingerprintFake},
		{"claude-fable-5-2", "claude", "claude-opus-5-5", FingerprintSuspicious},
		{"gpt-6-astra", "gpt", "gpt-5.6-luna", FingerprintSuspicious},
		{"gpt-6-sol", "claude", "claude-opus-4-6", FingerprintFake},
		{"gemini-2.5-pro", "unknown", "", FingerprintMatch},
		{"GPT-5.4", "openai", "gpt-5.4", FingerprintMatch},
	} {
		if got := FingerprintState(tc.requested, tc.family, tc.top); got != tc.want {
			t.Errorf("%s -> %s (%s) = %s, want %s", tc.requested, tc.family, tc.top, got, tc.want)
		}
	}
}

func TestPresentedScore(t *testing.T) {
	if got := PresentedScore(85, 32, 32, FingerprintFake); got != 53 {
		t.Fatalf("fake total %d", got)
	}
	if got := PresentedScore(85, 32, 32, FingerprintSuspicious); got != 65 {
		t.Fatalf("suspicious total %d", got)
	}
	if got := PresentedScore(85, 32, 32, FingerprintMatch); got != 85 {
		t.Fatalf("matching total %d", got)
	}
	if got := PresentedScore(87, 8, 21, FingerprintSuspicious); got != 87 {
		t.Fatalf("already partial total %d", got)
	}
	if got := PresentedScore(20, 32, 32, FingerprintFake); got != 0 {
		t.Fatalf("floor %d", got)
	}
}
