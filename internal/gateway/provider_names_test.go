package gateway

import "testing"

func TestProviderQualifiedProtocol(t *testing.T) {
	for model, want := range map[string]string{
		"anthropic/claude-fable-5": "messages",
		"openai/gpt-6.1-sol":       "responses",
		"moonshotai/kimi-k3":       "chat",
		"kimi-k3":                  "chat",
	} {
		if got := Protocol(model); got != want {
			t.Errorf("%s: got %s want %s", model, got, want)
		}
	}
}
