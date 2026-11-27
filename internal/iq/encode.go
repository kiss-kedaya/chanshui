package iq

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/rand"
	"net/url"
	"strings"
)

// Encoding identifies the lightweight tool-free ASCII encoding applied to an
// IQ prompt. All encoded bytes are strictly printable 7-bit ASCII so frontier
// models can decode them directly in-context without calling external tools,
// while defeating static plaintext keyword/regex matching by relay proxies.
type Encoding string

const (
	EncodingURL    Encoding = "url"
	EncodingHex    Encoding = "hex"
	EncodingBase64 Encoding = "base64"
)

// EncodingTag returns the localized display tag for an encoding mode.
func EncodingTag(mode Encoding) string {
	switch mode {
	case EncodingURL:
		return "URL编码"
	case EncodingHex:
		return "十六进制编码"
	case EncodingBase64:
		return "Base64编码"
	default:
		return string(mode)
	}
}

// BalancedEncodings returns a shuffled slice of length n containing an equal
// mix of URL, Hex, and Base64 encodings (e.g. 3/3/3 for the 9-item battery).
func BalancedEncodings(r *rand.Rand, n int) []Encoding {
	cycle := []Encoding{EncodingURL, EncodingHex, EncodingBase64}
	out := make([]Encoding, n)
	for i := range out {
		out[i] = cycle[i%len(cycle)]
	}
	if r != nil {
		r.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	}
	return out
}

// EncodeURL percent-encodes ASCII whitespace, punctuation, and digits (%30-%39)
// while preserving ASCII letters and line breaks. Every line decodes losslessly
// via standard url.PathUnescape while remaining effortlessly readable without
// external tools.
func EncodeURL(ascii string) string {
	var b strings.Builder
	for i := 0; i < len(ascii); i++ {
		c := ascii[i]
		if c == '\n' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", c)
	}
	return b.String()
}

// wrapASCIILines splits ASCII text along explicit newlines and word boundaries
// so every line has at most maxBytes bytes (never cutting inside a word unless
// a single token exceeds maxBytes).
func wrapASCIILines(ascii string, maxBytes int) []string {
	var out []string
	for _, rawLine := range strings.Split(ascii, "\n") {
		rawLine = strings.TrimSpace(rawLine)
		if rawLine == "" {
			continue
		}
		if len(rawLine) <= maxBytes {
			out = append(out, rawLine)
			continue
		}
		words := strings.Fields(rawLine)
		cur := ""
		for _, w := range words {
			if cur == "" {
				cur = w
				continue
			}
			if len(cur)+1+len(w) <= maxBytes {
				cur += " " + w
			} else {
				out = append(out, cur)
				cur = w
			}
		}
		if cur != "" {
			out = append(out, cur)
		}
	}
	return out
}

// EncodeHexLines encodes each short ASCII line as uppercase space-separated
// two-digit hex bytes (<= 32 bytes per line) for easy tool-free mental decoding.
func EncodeHexLines(ascii string) string {
	lines := wrapASCIILines(ascii, 32)
	encoded := make([]string, len(lines))
	for i, line := range lines {
		parts := make([]string, len(line))
		for j := 0; j < len(line); j++ {
			parts[j] = fmt.Sprintf("%02X", line[j])
		}
		encoded[i] = strings.Join(parts, " ")
	}
	return strings.Join(encoded, "\n")
}

// EncodeBase64Lines encodes each short ASCII line (<= 24 bytes) independently
// with standard Base64 so bit boundaries never drift across long paragraphs.
func EncodeBase64Lines(ascii string) string {
	lines := wrapASCIILines(ascii, 24)
	encoded := make([]string, len(lines))
	for i, line := range lines {
		encoded[i] = base64.StdEncoding.EncodeToString([]byte(line))
	}
	return strings.Join(encoded, "\n")
}

// FormatEncodedPrompt builds the final model-facing prompt by applying the
// selected encoding mode to setupASCII (optional framing) and coreASCII (exact
// parameters/constraints/question). All encoded payloads are pure ASCII.
func FormatEncodedPrompt(setupASCII, coreASCII string, mode Encoding, tail string) string {
	setupASCII = strings.TrimSpace(setupASCII)
	coreASCII = strings.TrimSpace(coreASCII)
	if tail == "" {
		tail = answerTail
	}
	var b strings.Builder
	switch mode {
	case EncodingHex:
		if setupASCII != "" {
			b.WriteString("以下题面背景为 URL 编码（ASCII），关键条件为十六进制 ASCII 编码（每行空格分隔字节）。请勿使用工具，直接心算解码后作答：\n")
			b.WriteString("[URL]\n")
			b.WriteString(EncodeURL(setupASCII))
			b.WriteString("\n[/URL]\n")
		} else {
			b.WriteString("以下题面为十六进制 ASCII 编码（每行独立，空格分隔字节）。请勿使用工具，直接心算解码后作答：\n")
		}
		b.WriteString("[HEX]\n")
		b.WriteString(EncodeHexLines(coreASCII))
		b.WriteString("\n[/HEX]\n")
	case EncodingBase64:
		if setupASCII != "" {
			b.WriteString("以下题面背景为 URL 编码（ASCII），关键条件每行为独立短 ASCII Base64 编码。请勿使用工具，直接心算解码后作答：\n")
			b.WriteString("[URL]\n")
			b.WriteString(EncodeURL(setupASCII))
			b.WriteString("\n[/URL]\n")
		} else {
			b.WriteString("以下题面每行为独立短 ASCII Base64 编码。请勿使用工具，直接心算逐行解码后作答：\n")
		}
		b.WriteString("[BASE64]\n")
		b.WriteString(EncodeBase64Lines(coreASCII))
		b.WriteString("\n[/BASE64]\n")
	default: // EncodingURL
		combined := coreASCII
		if setupASCII != "" {
			combined = setupASCII + "\n" + coreASCII
		}
		b.WriteString("以下题面采用 URL 编码（ASCII）。请勿使用工具，直接心算解码后作答：\n")
		b.WriteString("[URL]\n")
		b.WriteString(EncodeURL(combined))
		b.WriteString("\n[/URL]\n")
	}
	b.WriteString(tail)
	return b.String()
}

// DecodePrompt decodes every [URL], [HEX], and [BASE64] block inside a prompt
// produced by FormatEncodedPrompt back into plain ASCII text.
func DecodePrompt(prompt string) (string, error) {
	lines := strings.Split(prompt, "\n")
	var sections []string
	mode := ""
	var buf []string
	flush := func() error {
		if mode == "" {
			return nil
		}
		var decoded []string
		for _, ln := range buf {
			ln = strings.TrimSpace(ln)
			if ln == "" {
				continue
			}
			switch mode {
			case "URL":
				s, err := url.PathUnescape(ln)
				if err != nil {
					return fmt.Errorf("decode URL line %q: %w", ln, err)
				}
				decoded = append(decoded, s)
			case "HEX":
				compact := strings.ReplaceAll(ln, " ", "")
				raw, err := hex.DecodeString(compact)
				if err != nil {
					return fmt.Errorf("decode HEX line %q: %w", ln, err)
				}
				decoded = append(decoded, string(raw))
			case "BASE64":
				raw, err := base64.StdEncoding.DecodeString(ln)
				if err != nil {
					return fmt.Errorf("decode BASE64 line %q: %w", ln, err)
				}
				decoded = append(decoded, string(raw))
			}
		}
		sections = append(sections, strings.Join(decoded, "\n"))
		mode = ""
		buf = nil
		return nil
	}
	for _, ln := range lines {
		trim := strings.TrimSpace(ln)
		switch trim {
		case "[URL]":
			mode = "URL"
			buf = nil
		case "[HEX]":
			mode = "HEX"
			buf = nil
		case "[BASE64]":
			mode = "BASE64"
			buf = nil
		case "[/URL]", "[/HEX]", "[/BASE64]":
			if err := flush(); err != nil {
				return "", err
			}
		default:
			if mode != "" {
				buf = append(buf, ln)
			}
		}
	}
	if len(sections) == 0 {
		return "", fmt.Errorf("no encoded block found in prompt")
	}
	return strings.Join(sections, "\n"), nil
}

// isPrintableASCII reports whether s consists exclusively of printable 7-bit
// ASCII bytes (0x20..0x7E) and newlines.
func isPrintableASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '\n' && (c < 0x20 || c > 0x7E) {
			return false
		}
	}
	return true
}
