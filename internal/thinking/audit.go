package thinking

import (
	"encoding/base64"
	"strings"
	"unicode/utf8"
)

// SignatureAudit is a local read of the model name stored in a Claude thinking signature.
// It does not replay the signature and does not try to recover the sealed text.
type SignatureAudit struct {
	Status     string `json:"status"`
	Pass       *bool  `json:"pass"`
	BoundModel string `json:"bound_model"`
	Reason     string `json:"reason"`
}

// AuditSignature compares the claimed model with the model name embedded in the signature protobuf.
func AuditSignature(claimed, signature string) SignatureAudit {
	fail := false
	out := SignatureAudit{Status: "fail", Pass: &fail}
	bound := signatureModel(signature)
	out.BoundModel = bound
	if bound == "" {
		out.Reason = "签名里读不出模型名"
		return out
	}
	if !sameModel(claimed, bound) {
		out.Reason = "签名里的模型是 " + bound + "，与声称的 " + strings.TrimSpace(claimed) + " 不一致"
		return out
	}
	pass := true
	out.Pass = &pass
	out.Status = "pass"
	out.Reason = "签名里的模型与声称模型一致"
	return out
}

func sameModel(claimed, bound string) bool {
	c := canonicalModel(claimed)
	b := canonicalModel(bound)
	return c != "" && c == b
}

func canonicalModel(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, ".", "-")
	s = strings.Trim(s, "-")
	if len(s) > 9 {
		tail := s[len(s)-8:]
		if s[len(s)-9] == '-' && digits(tail) {
			s = s[:len(s)-9]
		}
	}
	return s
}

func digits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func signatureModel(signature string) string {
	raw := decodeSignature(signature)
	if len(raw) == 0 {
		return ""
	}
	var found []string
	collectStrings(raw, &found, 0)
	best := ""
	for _, s := range found {
		if !modelName(s) {
			continue
		}
		if best == "" || len(s) > len(best) {
			best = s
		}
	}
	return best
}

func decodeSignature(signature string) []byte {
	s := strings.Map(func(r rune) rune {
		if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
			return -1
		}
		return r
	}, strings.TrimSpace(signature))
	if s == "" {
		return nil
	}
	for _, enc := range []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	} {
		buf, err := enc.DecodeString(s)
		if err == nil && len(buf) > 0 {
			return buf
		}
	}
	return nil
}

func collectStrings(buf []byte, out *[]string, depth int) {
	if depth > 8 || len(buf) == 0 {
		return
	}
	i := 0
	var local []string
	for i < len(buf) {
		key, n := uvarint(buf[i:])
		if n <= 0 {
			return
		}
		i += n
		field := int(key >> 3)
		wt := int(key & 7)
		if field <= 0 {
			return
		}
		switch wt {
		case 0:
			_, n = uvarint(buf[i:])
			if n <= 0 {
				return
			}
			i += n
		case 1:
			if i+8 > len(buf) {
				return
			}
			i += 8
		case 5:
			if i+4 > len(buf) {
				return
			}
			i += 4
		case 2:
			l, n := uvarint(buf[i:])
			if n <= 0 || uint64(i+n)+l > uint64(len(buf)) {
				return
			}
			i += n
			chunk := buf[i : i+int(l)]
			i += int(l)
			if s, ok := asciiText(chunk); ok {
				local = append(local, s)
			}
			collectStrings(chunk, out, depth+1)
		default:
			return
		}
	}
	*out = append(*out, local...)
}

func uvarint(buf []byte) (uint64, int) {
	var x uint64
	for i, b := range buf {
		if i > 9 {
			return 0, 0
		}
		x |= uint64(b&0x7f) << (7 * uint(i))
		if b < 0x80 {
			return x, i + 1
		}
	}
	return 0, 0
}

func asciiText(buf []byte) (string, bool) {
	if len(buf) < 4 || len(buf) > 80 || !utf8.Valid(buf) {
		return "", false
	}
	for _, b := range buf {
		if b < 0x20 || b > 0x7e {
			return "", false
		}
	}
	return string(buf), true
}

func modelName(s string) bool {
	s = strings.ToLower(s)
	if !strings.HasPrefix(s, "claude-") || len(s) > 80 {
		return false
	}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '.' || r == '_' {
			continue
		}
		return false
	}
	return true
}
