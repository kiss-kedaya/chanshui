package gateway

import (
	"strings"
	"testing"
)

func TestFriendlyErrorKeepsSafeDetail(t *testing.T) {
	got := FriendlyError(200, "SSE stream ended without message_stop sk-abcdefghijklmnopqrstuvwxyz", 4)
	summary, detail, ok := strings.Cut(got, "\n")
	if !ok {
		t.Fatalf("missing detail: %q", got)
	}
	if summary != "上游响应格式异常或流式响应中断（HTTP 200）；已尝试 4 次" {
		t.Fatalf("summary = %q", summary)
	}
	if !strings.Contains(detail, "SSE stream ended") || strings.Contains(detail, "sk-abc") {
		t.Fatalf("detail = %q", detail)
	}
	if strings.Contains(got, "abcdefghijklmnopqrstuvwxyz") {
		t.Fatalf("secret leaked: %q", got)
	}
}

func TestFriendlyErrorOmitsEmptyDetail(t *testing.T) {
	got := FriendlyError(400, "   ", 4)
	if strings.Contains(got, "\n") || strings.Contains(got, "已尝试") {
		t.Fatalf("got %q", got)
	}
}
