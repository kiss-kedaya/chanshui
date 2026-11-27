package blacklist

import "testing"

func TestMatchHostExactAndSubdomains(t *testing.T) {
	cases := []struct {
		raw        string
		wantDomain string
	}{
		{"https://jaaak.cc/v1", "jaaak.cc"},
		{"HTTP://JAAAK.CC:443/v1/messages", "jaaak.cc"},
		{"https://api.jaaak.cc/v1", "jaaak.cc"},
		{"https://v2.api.jaaak.cc:8443/v1", "jaaak.cc"},
		{"https://a6api.vip/v1", "a6api.vip"},
		{"https://api.a6api.vip./v1", "a6api.vip"},
		{"https://cn.edge.a6api.vip/v1", "a6api.vip"},
		{"https://a2e0-103-151-173-91.ngrok-free.app/v1", "a2e0-103-151-173-91.ngrok-free.app"},
		{"https://api.a2e0-103-151-173-91.ngrok-free.app/v1", "a2e0-103-151-173-91.ngrok-free.app"},
	}
	for _, tc := range cases {
		entry, ok := MatchHost(tc.raw)
		if !ok || entry.Domain != tc.wantDomain {
			t.Fatalf("MatchHost(%q) = (%+v, %v), want domain %q", tc.raw, entry, ok, tc.wantDomain)
		}
	}

	// Excluded hosts (router.nerver.cc, official downgrades, suffix lookalikes,
	// unrelated ngrok tunnels, and genuine upstreams) must NOT match.
	for _, raw := range []string{
		"https://router.nerver.cc/v1",
		"https://nerver.cc/v1",
		"http://103.40.13.223:25664/v1",
		"https://gogo.aiqingxiang.xyz/v1",
		"https://notjaaak.cc/v1",
		"https://fakea6api.vip/v1",
		"https://other-tunnel.ngrok-free.app/v1",
		"https://ngrok-free.app/v1",
		"https://api.anthropic.com/v1",
		"",
	} {
		if entry, ok := MatchHost(raw); ok {
			t.Fatalf("unexpected match for %q: %+v", raw, entry)
		}
	}
}
