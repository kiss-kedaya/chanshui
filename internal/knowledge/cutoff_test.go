package knowledge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"chanzhushui/internal/gateway"
)

func TestParseCutoff(t *testing.T) {
	for _, tc := range []struct{ text, want string }{
		{"2024-06", "2024-06"},
		{"I can only report 2025/1", "2025-01"},
		{"知识截止：2023年9月", "2023-09"},
		{"UNKNOWN", ""},
		{"截至 2024 年，没有月份", ""},
		{"2024-13", ""},
	} {
		if got := ParseCutoff(tc.text); got != tc.want {
			t.Errorf("ParseCutoff(%q) = %q, want %q", tc.text, got, tc.want)
		}
	}
}

func TestCheckIsInformationalAndHandlesUnknown(t *testing.T) {
	for _, body := range []string{"2024-06", "UNKNOWN"} {
		t.Run(body, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"content": []any{map[string]any{"type": "text", "text": body}},
					"usage":   map[string]int{"input_tokens": 8, "output_tokens": 2},
				})
			}))
			defer s.Close()
			v, err := Check(context.Background(), gateway.Client{BaseURL: s.URL, NoRetry: true}, "claude-opus-5", 3)
			if err != nil {
				t.Fatal(err)
			}
			if body == "2024-06" && (v.Status != "reported" || v.Cutoff != body) {
				t.Fatalf("reported cutoff = %+v", v)
			}
			if body == "UNKNOWN" && (v.Status != "unknown" || v.Cutoff != "") {
				t.Fatalf("unknown cutoff = %+v", v)
			}
		})
	}
}
