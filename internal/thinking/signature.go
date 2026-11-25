package thinking

import (
	"encoding/base64"
	"fmt"
	"math"
	"strings"
	"time"
)

// sigSkewSeconds mirrors the probe scripts: signatures generated within five
// minutes of the response count as fresh; anything older is a cached or
// replayed signature.
const sigSkewSeconds = 300

// realSignatureFields are the protobuf fields a freshly generated Claude
// thinking signature carries. Their absence marks a half-built envelope.
var realSignatureFields = []int{1, 3, 5, 6, 7, 8, 11, 14, 17, 19, 21, 22, 24, 27}

type protoField struct {
	Num  int
	Wire int
	Int  uint64
	Data []byte
}

type sigAnatomy struct {
	Fields       int
	Infra        string
	Timestamp    int64
	HasTimestamp bool
	Complete     bool
}

// signatureAnatomy decodes one thinking signature and classifies it. The
// layout follows the documented double base64 protobuf envelope:
// outer.field2 -> container.field1 -> channel, with channel.field21 holding a
// Unix timestamp and channel.field2 the serving infrastructure.
func signatureAnatomy(signature string, now time.Time) (sigAnatomy, string) {
	if strings.TrimSpace(signature) == "" {
		return sigAnatomy{}, "无签"
	}
	raw := decodeSignature(signature)
	if len(raw) == 0 {
		return sigAnatomy{}, "解析失败"
	}
	if inner := decodeNestedBase64(raw); inner != nil {
		raw = inner
	}
	outer, ok := parseProtoFields(raw)
	if !ok {
		return sigAnatomy{}, "解析失败"
	}
	container := lastBytes(outer, 2)
	if container == nil {
		return sigAnatomy{}, "解析失败"
	}
	middle, ok := parseProtoFields(container)
	if !ok {
		return sigAnatomy{}, "解析失败"
	}
	channel := lastBytes(middle, 1)
	if channel == nil {
		return sigAnatomy{}, "解析失败"
	}
	fields, ok := parseProtoFields(channel)
	if !ok {
		return sigAnatomy{}, "解析失败"
	}
	a := sigAnatomy{Fields: len(fields)}
	present := map[int]bool{}
	for _, f := range fields {
		present[f.Num] = true
		switch {
		case f.Num == 2 && f.Wire == 0:
			switch f.Int {
			case 1:
				a.Infra = "infra_aws"
			case 2:
				a.Infra = "infra_google"
			default:
				a.Infra = fmt.Sprintf("infra_%d", f.Int)
			}
		case f.Num == 21 && f.Wire == 0:
			a.Timestamp = int64(f.Int)
			a.HasTimestamp = true
		}
	}
	a.Complete = true
	for _, want := range realSignatureFields {
		if !present[want] {
			a.Complete = false
			break
		}
	}
	if !a.HasTimestamp {
		if len(fields) <= 4 {
			return a, "空信封"
		}
		return a, "半成品(缺时间戳)"
	}
	skew := now.Unix() - a.Timestamp
	if math.Abs(float64(skew)) > sigSkewSeconds {
		return a, fmt.Sprintf("历史/缓存签(偏 %ds)", skew)
	}
	if !a.Complete {
		return a, "半成品(现时间戳)"
	}
	return a, "真签(现生成)"
}

func decodeNestedBase64(raw []byte) []byte {
	if len(raw) <= 16 {
		return nil
	}
	for _, b := range raw {
		if strings.IndexByte("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/=", b) < 0 {
			return nil
		}
	}
	inner, err := base64.StdEncoding.DecodeString(string(raw))
	if err != nil || len(inner) == 0 {
		return nil
	}
	for _, b := range inner {
		if b >= 0x20 && b < 0x7f {
			return nil
		}
	}
	return inner
}

func parseProtoFields(buf []byte) ([]protoField, bool) {
	out := make([]protoField, 0, 16)
	i := 0
	for i < len(buf) {
		key, n := signatureVarint(buf[i:])
		if n <= 0 {
			return out, len(out) > 0
		}
		i += n
		num, wire := int(key>>3), int(key&7)
		if num <= 0 {
			return out, false
		}
		switch wire {
		case 0:
			v, m := signatureVarint(buf[i:])
			if m <= 0 {
				return out, false
			}
			i += m
			out = append(out, protoField{Num: num, Wire: wire, Int: v})
		case 1:
			if i+8 > len(buf) {
				return out, false
			}
			i += 8
			out = append(out, protoField{Num: num, Wire: wire})
		case 5:
			if i+4 > len(buf) {
				return out, false
			}
			i += 4
			out = append(out, protoField{Num: num, Wire: wire})
		case 2:
			l, m := signatureVarint(buf[i:])
			if m <= 0 || uint64(i+m)+l > uint64(len(buf)) {
				return out, false
			}
			i += m
			out = append(out, protoField{Num: num, Wire: wire, Data: buf[i : i+int(l)]})
			i += int(l)
		default:
			return out, false
		}
	}
	return out, len(out) > 0
}

func lastBytes(fields []protoField, num int) []byte {
	var out []byte
	for _, f := range fields {
		if f.Num == num && f.Wire == 2 {
			out = f.Data
		}
	}
	return out
}
