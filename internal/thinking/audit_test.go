package thinking

import (
	"encoding/base64"
	"testing"
)

func TestAuditSignatureMatchesDatedModel(t *testing.T) {
	sig := signModel("claude-opus-4-8-20260514")
	got := AuditSignature("claude-opus-4-8", sig)
	if got.Pass == nil || !*got.Pass || got.BoundModel != "claude-opus-4-8-20260514" {
		t.Fatalf("%+v", got)
	}
}

func TestAuditSignatureRejectsDifferentModel(t *testing.T) {
	sig := signModel("claude-sonnet-4-6")
	got := AuditSignature("claude-opus-4-8", sig)
	if got.Pass == nil || *got.Pass || got.Status != "fail" || got.BoundModel != "claude-sonnet-4-6" {
		t.Fatalf("%+v", got)
	}
}

func TestAuditSignatureRejectsOpus55Alias(t *testing.T) {
	sig := signModel("claude-opus-5-5")
	got := AuditSignature("claude-opus-5", sig)
	if got.Pass == nil || *got.Pass {
		t.Fatalf("%+v", got)
	}
}

func TestAuditSignatureDotsMatchHyphens(t *testing.T) {
	sig := signModel("claude-sonnet-4-5-20251001")
	got := AuditSignature("claude-sonnet-4.5", sig)
	if got.Pass == nil || !*got.Pass {
		t.Fatalf("%+v", got)
	}
}

func TestAuditSignatureUnreadable(t *testing.T) {
	got := AuditSignature("claude-sonnet-4-6", "not-a-signature")
	if got.Pass == nil || *got.Pass || got.BoundModel != "" || got.Reason != "签名里读不出模型名" {
		t.Fatalf("%+v", got)
	}
}

func signModel(model string) string {
	meta := append(protoString(6, model), protoString(8, "thinking")...)
	body := protoBytes(2, protoBytes(1, meta))
	return base64.StdEncoding.EncodeToString(body)
}

func protoBytes(field int, inner []byte) []byte {
	return append(append(protoVarint(field<<3|2), protoVarint(len(inner))...), inner...)
}

func protoString(field int, s string) []byte {
	return protoBytes(field, []byte(s))
}

func protoVarint(n int) []byte {
	var out []byte
	for n >= 0x80 {
		out = append(out, byte(n)|0x80)
		n >>= 7
	}
	return append(out, byte(n))
}
