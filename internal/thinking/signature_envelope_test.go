package thinking

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestStandardEnvelopeWithoutTimestamp(t *testing.T) {
	header := append(pbInt(1, 20), pbInt(3, 2)...)
	header = append(header, pbInt(7, 1)...)
	header = append(header, pbBytes(8, []byte("thinking"))...)
	cipher := make([]byte, 600)
	for i := range cipher {
		cipher[i] = byte(0x80 + i%64)
	}
	payload := pbBytes(1, header)
	payload = append(payload, pbBytes(2, make([]byte, 12))...)
	payload = append(payload, pbBytes(3, make([]byte, 12))...)
	payload = append(payload, pbBytes(4, make([]byte, 48))...)
	payload = append(payload, pbBytes(5, cipher)...)
	outer := append(pbInt(1, 4), pbBytes(2, payload)...)
	outer = append(outer, pbInt(3, 1)...)
	sig := base64.StdEncoding.EncodeToString(outer)

	a, class := signatureAnatomy(sig, time.Now())
	if !strings.HasPrefix(class, "标准信封") {
		t.Fatalf("class = %q", class)
	}
	if a.Version != 4 || a.Fields != 5 || a.CipherLen != 600 || a.HasTimestamp {
		t.Fatalf("anatomy = %+v", a)
	}
}

func TestOpaqueSignatureIsNotParseFailure(t *testing.T) {
	sig := base64.StdEncoding.EncodeToString([]byte{0x00, 0x01, 0x02, 0xff, 0x7e, 0x55})
	if _, class := signatureAnatomy(sig, time.Now()); !strings.HasPrefix(class, "未识别格式") {
		t.Fatalf("class = %q", class)
	}
}
