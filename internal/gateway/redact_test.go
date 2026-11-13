package gateway

import (
	"strings"
	"testing"
)

func TestRedactHidesUpstreamHost(t *testing.T) {
	in := `do request: Post "https://az.aizlapi.com/v1/messages": net/http: request canceled (Client.Timeout exceeded while awaiting headers)`
	out := Redact(in)
	if strings.Contains(out, "aizlapi") || strings.Contains(out, "az.") {
		t.Fatalf("host leaked: %s", out)
	}
	if !strings.Contains(out, `https://*****.***/v1/messages`) {
		t.Fatalf("path or mask missing: %s", out)
	}
}

func TestRedactHidesBareHostAndIP(t *testing.T) {
	out := Redact("upstream api.example.com and 10.1.2.3 failed")
	if strings.Contains(out, "example.com") || strings.Contains(out, "10.1.2.3") {
		t.Fatalf("leaked: %s", out)
	}
	if strings.Count(out, "*****.***") != 2 {
		t.Fatalf("want two masks: %s", out)
	}
}
func TestFriendlyErrorRedactsHostInDetail(t *testing.T) {
	got := FriendlyError(0, `do request: Post "https://az.aizlapi.com/v1/messages": net/http: request canceled (Client.Timeout exceeded while awaiting headers)`, 4)
	if strings.Contains(got, "aizlapi") {
		t.Fatalf("friendly detail leaked host: %s", got)
	}
	if !strings.Contains(got, "https://*****.***/v1/messages") || !strings.Contains(got, "已尝试 4 次") {
		t.Fatalf("unexpected detail: %s", got)
	}
}
