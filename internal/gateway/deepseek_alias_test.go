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

func TestDeepSeekAliasOnlyRewritesOfficialHost(t *testing.T) {
	for _, host := range []string{"api.deepseek.com", "fixture.invalid"} {
		c := Client{BaseURL: "https://" + host, NoRetry: true, HTTP: &http.Client{Transport: aliasTransport(func(r *http.Request) (*http.Response, error) {
			var body struct{ Model string }
			json.NewDecoder(r.Body).Decode(&body)
			want := "deepseek-v4.1-flash"
			if host == "api.deepseek.com" {
				want = "deepseek-flash"
			}
			if body.Model != want {
				t.Errorf("host %s model %s want %s", host, body.Model, want)
			}
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"OK"},"finish_reason":"stop"}]}`))}, nil
		})}}
		m, e := c.CallMessages(context.Background(), CallRequest{Model: "deepseek-v4.1-flash"})
		if e != nil || m.Err != "" {
			t.Fatal(m, e)
		}
	}
}
