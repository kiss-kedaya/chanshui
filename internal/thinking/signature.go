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
	Version      int
	CipherLen    int
	Authentic    bool
	Forged       bool
	ForgeReason  string
}

// IsForgedPrefix reports whether a thinking signature starts with a known
// synthetic / local-replay marker instead of an Anthropic protobuf envelope.
func IsForgedPrefix(signature string) bool {
	lower := strings.ToLower(strings.TrimSpace(signature))
	for _, prefix := range []string{
		"thinking_local_",
		"local_thinking",
		"fake_sig",
		"mock_sig",
		"aws-quick-thinking",
		"quickwork",
	} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

// signatureAnatomy decodes one thinking signature and classifies it. The
// layout follows the documented double base64 protobuf envelope:
// outer.field2 -> container.field1 -> channel, with channel.field21 holding a
// Unix timestamp and channel.field2 the serving infrastructure.
func signatureAnatomy(signature string, now time.Time) (sigAnatomy, string) {
	trimmed := strings.TrimSpace(signature)
	if trimmed == "" {
		return sigAnatomy{}, "无签"
	}
	if IsForgedPrefix(trimmed) {
		return sigAnatomy{Forged: true, ForgeReason: "签名带本地回放/伪造前缀"}, "伪造签名（本地回放）"
	}
	raw := decodeSignature(trimmed)
	if len(raw) == 0 {
		return sigAnatomy{}, "未识别格式（不是 base64）"
	}
	if inner := decodeNestedBase64(raw); inner != nil {
		raw = inner
	}
	outer, ok := parseProtoFields(raw)
	if !ok {
		return sigAnatomy{Fields: 0}, fmt.Sprintf("未识别格式（%d 字节）", len(raw))
	}
	a := sigAnatomy{}
	outerHasField3 := false
	for _, field := range outer {
		if field.Num == 1 && field.Wire == 0 && field.Int < 100 {
			a.Version = int(field.Int)
		}
		if field.Num == 3 && field.Wire == 0 {
			outerHasField3 = true
		}
	}
	container := lastBytes(outer, 2)
	if container == nil {
		return a, fmt.Sprintf("未识别格式（无 payload，%d 字节）", len(raw))
	}
	middle, ok := parseProtoFields(container)
	if !ok {
		return a, fmt.Sprintf("未识别格式（payload 不可解析，%d 字节）", len(container))
	}
	middleHasEnvelope := hasBytesField(middle, 2) && hasBytesField(middle, 3) && hasBytesField(middle, 4) && hasBytesField(middle, 5)
	headerHasRole := false
	if hdr := lastBytes(middle, 1); hdr != nil {
		var hdrStrings []string
		collectSignatureStrings(hdr, &hdrStrings, 0)
		for _, s := range hdrStrings {
			ls := strings.ToLower(s)
			if ls == "thinking" || ls == "narration" {
				headerHasRole = true
				break
			}
		}
	}
	fields := middle
	followedChannel := false
	// Documented layout wraps the real channel in payload.field1; real
	// envelopes (header/nonce/nonce/MAC/cipher) keep those five fields at the
	// payload level, so only follow field1 when it looks like a full channel.
	if ch := lastBytes(middle, 1); ch != nil {
		if cf, ok := parseProtoFields(ch); ok {
			if len(cf) >= 6 || hasIntField(cf, 21) || hasIntField(cf, 2) {
				fields = cf
				followedChannel = true
			}
		}
	}
	a.Fields = len(fields)
	for _, field := range fields {
		if field.Wire == 2 && len(field.Data) > a.CipherLen && len(field.Data) >= 200 {
			a.CipherLen = len(field.Data)
		}
	}
	if a.CipherLen == 0 {
		for _, field := range middle {
			if field.Wire == 2 && len(field.Data) > a.CipherLen {
				a.CipherLen = len(field.Data)
			}
		}
	}
	present := map[int]bool{}
	channelHasModelName := false
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
		case f.Wire == 2:
			if s, ok := signatureText(f.Data); ok && signatureModelName(s) {
				channelHasModelName = true
			}
		}
	}
	a.Complete = true
	for _, want := range realSignatureFields {
		if !present[want] {
			a.Complete = false
			break
		}
	}

	// Forged protobuf envelope detection:
	// 1. Hand-crafted envelopes (e.g. router.nerver.cc) omit Anthropic's outer
	//    version=4 header while setting outer.field3=1, mixing v4 nonce/MAC/cipher
	//    fields with legacy channel field21 timestamps or plaintext model names.
	// 2. Authentic v4 envelopes never embed a plaintext channel timestamp (f21)
	//    or plaintext claude-* model string inside a channel alongside v4 fields.
	if (a.Version != 4 && a.Version != 2 && outerHasField3) ||
		(followedChannel && middleHasEnvelope && (a.HasTimestamp || channelHasModelName)) ||
		(a.Version == 4 && a.HasTimestamp) ||
		(a.Version == 0 && a.HasTimestamp && channelHasModelName) {
		a.Forged = true
		a.Authentic = false
		a.ForgeReason = fmt.Sprintf("仿 protobuf 假信封（outer v=%d, field3=%v, f21=%v, 明文模型=%v）", a.Version, outerHasField3, a.HasTimestamp, channelHasModelName)
		return a, "伪造信封（仿 protobuf）"
	}

	if !a.HasTimestamp {
		if a.Version == 4 && a.CipherLen >= 200 {
			a.Authentic = true
			return a, fmt.Sprintf("标准信封（无时间戳，密文 %d 字节）", a.CipherLen)
		}
		if a.Version == 2 && headerHasRole {
			a.Authentic = true
			return a, "空信封"
		}
		if a.CipherLen >= 200 {
			a.Forged = true
			a.ForgeReason = fmt.Sprintf("缺少外层版本 4 头（outer v=%d，密文 %d 字节）", a.Version, a.CipherLen)
			return a, "伪造信封（缺少版本头）"
		}
		if len(fields) <= 4 {
			return a, "空信封"
		}
		return a, fmt.Sprintf("未识别格式（%d 字段，密文 %d 字节）", len(fields), a.CipherLen)
	}
	skew := now.Unix() - a.Timestamp
	if math.Abs(float64(skew)) > sigSkewSeconds {
		return a, fmt.Sprintf("历史/缓存签(偏 %ds)", skew)
	}
	if !a.Complete {
		return a, "半成品(现时间戳)"
	}
	a.Authentic = true
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

func hasIntField(fields []protoField, num int) bool {
	for _, field := range fields {
		if field.Num == num && field.Wire == 0 {
			return true
		}
	}
	return false
}

func hasBytesField(fields []protoField, num int) bool {
	for _, field := range fields {
		if field.Num == num && field.Wire == 2 {
			return true
		}
	}
	return false
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
