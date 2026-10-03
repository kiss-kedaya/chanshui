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
	if summary != "上游响应流被提前中断（连接在传输中途结束），可重试（HTTP 200）；已尝试 4 次" {
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

// 5xx 必须说清是上游侧故障，避免用户误判成检测站坏了；次数只出现一次。
func TestFriendlyErrorBlamesUpstreamOn5xx(t *testing.T) {
	got := FriendlyError(500, "new_api_error request failed", 4)
	if !strings.Contains(got, "上游侧故障") || !strings.Contains(got, "非本站问题") {
		t.Fatalf("5xx 未标明上游侧故障: %q", got)
	}
	if strings.Contains(got, "本站或请求参数") {
		t.Fatalf("不该把责任推给本站: %q", got)
	}
	if n := strings.Count(got, "已尝试"); n != 1 {
		t.Fatalf("重试次数应只出现一次，出现 %d 次: %q", n, got)
	}
}

// 503 和 502 这类上游故障同样要归到上游侧。
func TestFriendlyErrorUpstreamAcrossStatuses(t *testing.T) {
	for _, status := range []int{500, 502, 503, 504} {
		got := FriendlyError(status, "upstream exploded", 3)
		if !strings.Contains(got, "上游侧故障") {
			t.Fatalf("HTTP %d 未标明上游侧: %q", status, got)
		}
	}
}

// 流在中途被掐断（HTTP 仍是 200）不能报成上游 5xx，要说成可重试的中断。
func TestFriendlyErrorStreamTruncationIsNotUpstream5xx(t *testing.T) {
	got := FriendlyError(200, "HTTP 200 read: SSE stream ended without message_stop or [DONE]", 4)
	if !strings.Contains(got, "中断") {
		t.Fatalf("断流未单独分类: %q", got)
	}
	if strings.Contains(got, "上游侧故障") {
		t.Fatalf("断流不该报成上游侧故障: %q", got)
	}
}
