package gateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type aliasTransport func(*http.Request) (*http.Response, error)

func (f aliasTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// The station must forward exactly the model name the user typed. The old
// api.deepseek.com -> deepseek-flash rewrite hid the real request name and made
// reports disagree with the submission.
func TestModelNameIsForwardedVerbatim(t *testing.T) {
	for _, host := range []string{"api.deepseek.com", "fixture.invalid"} {
		for _, model := range []string{"deepseek-v4.1-flash-1017", "deepseek-v4.1-flash", "deepseek-flash"} {
			c := Client{BaseURL: "https://" + host, NoRetry: true, HTTP: &http.Client{Transport: aliasTransport(func(r *http.Request) (*http.Response, error) {
				var body struct{ Model string }
				json.NewDecoder(r.Body).Decode(&body)
				if body.Model != model {
					t.Errorf("host %s sent model %q, want %q", host, body.Model, model)
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"OK"},"finish_reason":"stop"}]}`))}, nil
			})}}
			m, e := c.CallMessages(context.Background(), CallRequest{Model: model})
			if e != nil || m.Err != "" {
				t.Fatal(m, e)
			}
		}
	}
}
