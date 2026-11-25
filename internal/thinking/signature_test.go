package thinking

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func pbVarint(v uint64) []byte {
	out := []byte{}
	for v >= 0x80 {
		out = append(out, byte(v)|0x80)
		v >>= 7
	}
	return append(out, byte(v))
}

func pbKey(field, wire int) []byte { return pbVarint(uint64(field<<3 | wire)) }

func pbInt(field int, v uint64) []byte { return append(pbKey(field, 0), pbVarint(v)...) }

func pbBytes(field int, data []byte) []byte {
	out := append(pbKey(field, 2), pbVarint(uint64(len(data)))...)
	return append(out, data...)
}

// buildSignature assembles the documented envelope:
// outer.field2 -> container.field1 -> channel.
func buildSignature(ts int64, fields []int) string {
	channel := []byte{}
	for _, f := range fields {
		if f == 21 {
			continue
		}
		channel = append(channel, pbInt(f, uint64(f))...)
	}
	if ts > 0 {
		channel = append(channel, pbInt(21, uint64(ts))...)
	}
	container := pbBytes(1, channel)
	outer := pbBytes(2, container)
	return base64.StdEncoding.EncodeToString(outer)
}

func TestSignatureAnatomy(t *testing.T) {
	now := time.Now()
	full := []int{1, 3, 5, 6, 7, 8, 11, 14, 17, 19, 21, 22, 24, 27}

	fresh, class := signatureAnatomy(buildSignature(now.Unix(), full), now)
	if class != "真签(现生成)" {
		t.Fatalf("fresh class = %q", class)
	}
	if fresh.Fields != len(full) || fresh.Timestamp != now.Unix() || !fresh.HasTimestamp || !fresh.Complete {
		t.Fatalf("fresh anatomy = %+v", fresh)
	}

	if _, class = signatureAnatomy(buildSignature(now.Unix()-3600, full), now); class[:len("历史")] != "历史" {
		t.Fatalf("stale class = %q", class)
	}
	if _, class = signatureAnatomy(buildSignature(now.Unix(), []int{21}), now); class != "半成品(现时间戳)" {
		t.Fatalf("half class = %q", class)
	}
	if _, class = signatureAnatomy(buildSignature(0, []int{1, 2, 3}), now); class != "空信封" {
		t.Fatalf("envelope class = %q", class)
	}
	if _, class = signatureAnatomy("!!!not-base64!!!", now); !strings.HasPrefix(class, "未识别格式") {
		t.Fatalf("garbage class = %q", class)
	}
	if _, class = signatureAnatomy("  ", now); class != "无签" {
		t.Fatalf("empty class = %q", class)
	}
}
