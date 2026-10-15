package integrity

import "testing"

func TestProviderAliasesDoNotImplyDilution(t *testing.T) {
	for _, tc := range []struct{ requested, family, top string }{
		{"anthropic/claude-fable-5", "claude", "claude-fable-5"},
		{"openai/gpt-6.1-sol", "gpt", "gpt-6.1-sol"},
		{"moonshotai/kimi-k3", "kimi", "kimi-k3"},
	} {
		if Diluted(tc.requested, tc.family, tc.top) {
			t.Errorf("matching alias marked diluted: %s", tc.requested)
		}
		if !Diluted(tc.requested, tc.family, tc.top+"-different") {
			t.Errorf("different version accepted: %s", tc.requested)
		}
		if ModelFamily(tc.requested) != tc.family {
			t.Errorf("wrong family: %s", tc.requested)
		}
	}
}
