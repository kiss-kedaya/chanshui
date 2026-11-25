package thinking

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const probePrompt = "求最小正整数 n，使 n 除以 17 余 5，除以 19 余 7，除以 23 余 11。请推导并核验，最后给出数值。"
const identityPrompt = "Who are you? Answer in one short sentence."

// Verdict is the outcome of one summarized-thinking and signature audit.
type Verdict struct {
	Pass             bool   `json:"pass"`
	Score            int    `json:"score"`
	MaxScore         int    `json:"max_score"`
	SummaryScore     int    `json:"summary_score"`
	SignatureScore   int    `json:"signature_score"`
	ReplayScore      int    `json:"replay_score"`
	TamperScore      int    `json:"tamper_score"`
	TemperatureScore int    `json:"temperature_score,omitempty"`
	Reason           string `json:"reason"`
	ThinkLen         int    `json:"think_len"`
	SigLen           int    `json:"sig_len"`
	ThinkingTokens   int    `json:"thinking_tokens"`
	Ms               int64  `json:"ms"`
	ThinkingText     string `json:"thinking_text"`
	Signature        string `json:"signature"`
	Answer           string `json:"answer"`
	Err              string `json:"err,omitempty"`

	SignaturePresent    bool              `json:"signature_present"`
	SignedModel         string            `json:"signed_model,omitempty"`
	SignedModelMatch    *bool             `json:"signed_model_match,omitempty"`
	SigClass            string            `json:"sig_class,omitempty"`
	SigFieldCount       int               `json:"sig_field_count,omitempty"`
	SigInfra            string            `json:"sig_infra,omitempty"`
	SigTimestamp        int64             `json:"sig_timestamp,omitempty"`
	SigAgeSeconds       int64             `json:"sig_age_seconds,omitempty"`
	SigFresh            *bool             `json:"sig_fresh,omitempty"`
	Strictness          []StrictnessCheck `json:"strictness,omitempty"`
	Source              string            `json:"source,omitempty"`
	OriginalReplay      bool              `json:"original_replay"`
	TamperRejected      bool              `json:"tamper_rejected"`
	TamperStatus        int               `json:"tamper_status,omitempty"`
	TamperError         string            `json:"tamper_error,omitempty"`
	SelfReport          string            `json:"self_report,omitempty"`
	FamilyConsistent    bool              `json:"family_consistent"`
	TemperatureRejected bool              `json:"temperature_rejected,omitempty"`
	TemperatureError    string            `json:"temperature_error,omitempty"`
	AwSB                *bool             `json:"aws_b,omitempty"`
	AwSBDetail          string            `json:"aws_b_detail,omitempty"`
}

// Seed is an existing question response that already carries a thinking
// signature. Reusing it avoids asking the model to think twice.
type Seed struct {
	ID      string
	Prompt  string
	Metrics Metrics
}

// Check asks for summarized thinking, then:
//   - scores the summary (5)
//   - replays the returned signature with adaptive thinking; a valid signature
//     must be accepted (1) and a tampered one must be rejected with 400 (2)
//   - for opus-5-5, sends temperature and expects the documented 400 (2)
//
// The replay must carry thinking=adaptive. Without it the upstream can skip
// signature validation and accept tampered signatures.
func Check(ctx context.Context, c Caller, model string) (Verdict, error) {
	if c == nil {
		return Verdict{}, ErrNoCaller
	}
	m, err := c.CallMessages(ctx, CallRequest{
		Model:     model,
		MaxTokens: 4096,
		Messages:  []Message{{Role: "user", Content: probePrompt}},
		Thinking:  &ThinkingConfig{Type: "adaptive", Display: "summarized"},
	})
	if err != nil {
		return Verdict{Err: err.Error()}, err
	}
	if m.Err != "" || m.Status != 200 {
		return Verdict{Err: m.Err}, fmt.Errorf("thinking request failed: HTTP %d %s", m.Status, m.Err)
	}
	return audit(ctx, c, model, "", probePrompt, m)
}

// CheckWithSeed audits a signature that a question already returned.
func CheckWithSeed(ctx context.Context, c Caller, model string, seed Seed) (Verdict, error) {
	if c == nil {
		return Verdict{}, ErrNoCaller
	}
	if strings.TrimSpace(seed.Metrics.Signature) == "" {
		return Verdict{}, fmt.Errorf("thinking: seed has no signature")
	}
	return audit(ctx, c, model, seed.ID, seed.Prompt, seed.Metrics)
}

func audit(ctx context.Context, c Caller, model, source, priorUser string, m Metrics) (Verdict, error) {
	v := Verdict{
		ThinkingText: m.ThinkingText, Signature: m.Signature, Answer: m.Answer,
		ThinkLen: m.ThinkLen, SigLen: m.SigLen, ThinkingTokens: m.ThinkingTokens, Ms: m.Ms,
		MaxScore: 10, Source: source,
	}
	opus55 := strings.Contains(strings.ToLower(model), "opus-5-5")
	if opus55 {
		v.MaxScore = 12
	}
	if strings.TrimSpace(m.ThinkingText) != "" {
		v.SummaryScore = 5
	}
	if m.Signature != "" {
		v.SignaturePresent = true
		v.SignatureScore = 2
		v.SignedModel = signatureModel(m.Signature)
		if anatomy, class := signatureAnatomy(m.Signature, time.Now()); class != "" {
			v.SigClass = class
			v.SigFieldCount = anatomy.Fields
			v.SigInfra = anatomy.Infra
			if anatomy.HasTimestamp && anatomy.Timestamp > 0 {
				v.SigTimestamp = anatomy.Timestamp
				v.SigAgeSeconds = time.Now().Unix() - anatomy.Timestamp
				fresh := strings.HasPrefix(class, "真签")
				v.SigFresh = &fresh
			}
		}
		if v.SignedModel != "" {
			match := signatureModelMatches(model, v.SignedModel)
			v.SignedModelMatch = &match
		}
		original, origErr := c.CallMessages(ctx, CallRequest{
			Model: model, MaxTokens: 2048,
			Replay:                 &Replay{PriorUser: priorUser, Signature: m.Signature, FollowUp: identityPrompt},
			StopOnInvalidSignature: true,
		})
		if origErr == nil && original.Status == 200 && original.Err == "" {
			v.OriginalReplay = true
			v.ReplayScore = 1
			v.SelfReport = clipText(original.Answer, 200)
			v.FamilyConsistent = mentionsClaudeFamily(original.Answer)
		}
		tampered, tamperErr := c.CallMessages(ctx, CallRequest{
			Model: model, MaxTokens: 2048,
			Replay:                 &Replay{PriorUser: priorUser, Signature: tamperSignature(m.Signature), FollowUp: identityPrompt},
			StopOnInvalidSignature: true,
		})
		v.TamperStatus = tampered.Status
		v.TamperError = clipText(tampered.Err, 300)
		if tamperErr == nil && tampered.Status == 400 && strings.Contains(strings.ToLower(tampered.Err), "signature") {
			v.TamperRejected = true
			if v.OriginalReplay {
				v.TamperScore = 2
			}
		}
		if tampered.Status == 400 {
			awsB, detail := bedrockRejection(tampered.Err)
			v.AwSB = &awsB
			v.AwSBDetail = detail
		}
	}
	if opus55 {
		temp := 0.1
		tr, tempErr := c.CallMessages(ctx, CallRequest{
			Model: model, MaxTokens: 8, Temperature: &temp,
			Messages: []Message{{Role: "user", Content: "Reply OK"}},
		})
		v.TemperatureError = clipText(tr.Err, 200)
		if tempErr == nil && tr.Status == 400 &&
			strings.Contains(strings.ToLower(tr.Err), "temperature") &&
			strings.Contains(strings.ToLower(tr.Err), "deprecated") {
			v.TemperatureRejected = true
			v.TemperatureScore = 2
		}
	}
	if p, ok := c.(StrictnessProber); ok {
		if checks := p.ProbeStrictness(ctx, model); len(checks) > 0 {
			v.Strictness = checks
		}
	}
	v.Score = v.SummaryScore + v.SignatureScore + v.ReplayScore + v.TamperScore + v.TemperatureScore
	if v.SignedModelMatch != nil && !*v.SignedModelMatch {
		v.Score -= 2
		if v.Score < 0 {
			v.Score = 0
		}
	}
	v.Pass = v.Score == v.MaxScore
	v.Reason = thinkingReason(v)
	if extra := v.evidenceSuffix(); extra != "" {
		v.Reason = strings.TrimRight(v.Reason, "。") + "；" + extra + "。"
	}
	return v, nil
}

// evidenceSuffix summarizes the signature anatomy and the protocol strictness
// checks without changing the score.
func (v Verdict) evidenceSuffix() string {
	parts := []string{}
	if v.SigClass != "" {
		part := "签名结构 " + v.SigClass
		if v.SigFieldCount > 0 {
			part += fmt.Sprintf("（%d 字段", v.SigFieldCount)
			if v.SigInfra != "" {
				part += "，" + v.SigInfra
			}
			part += "）"
		}
		parts = append(parts, part)
	}
	if v.AwSB != nil {
		if *v.AwSB {
			parts = append(parts, "AWS-B 坏签 400（Bedrock 关键字）")
		} else {
			parts = append(parts, "拒签但无 Bedrock 标记")
		}
	}
	if len(v.Strictness) > 0 {
		passed := 0
		failed := []string{}
		for _, check := range v.Strictness {
			if check.Pass {
				passed++
			} else {
				failed = append(failed, check.Name)
			}
		}
		part := fmt.Sprintf("协议严格性 %d/%d", passed, len(v.Strictness))
		if len(failed) > 0 {
			part += "（未拒绝：" + strings.Join(failed, "、") + "）"
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, "；")
}

func thinkingReason(v Verdict) string {
	source := ""
	if v.Source != "" && v.SignaturePresent {
		source = "复用 " + v.Source + " 的签名；"
	}
	mismatch := ""
	if v.SignedModelMatch != nil && !*v.SignedModelMatch {
		mismatch = fmt.Sprintf("签名型号 %s 与请求不符（-2）；", v.SignedModel)
	}
	if !v.SignaturePresent {
		if v.SummaryScore > 0 {
			return "没有签名；只拿到思维摘要。"
		}
		return "没有思维摘要，也没有签名。"
	}
	if !v.OriginalReplay {
		return source + mismatch + fmt.Sprintf("签名 %d/2；原始签名续轮失败；篡改拒绝 %d/2。", v.SignatureScore, v.TamperScore)
	}
	if !v.TamperRejected {
		return source + mismatch + "原始签名可续轮，但篡改签名仍被接受：签名未透传或未校验。"
	}
	parts := []string{
		fmt.Sprintf("摘要 %d/5", v.SummaryScore),
		fmt.Sprintf("签名 %d/2", v.SignatureScore),
		fmt.Sprintf("原始续轮 %d/1", v.ReplayScore),
		fmt.Sprintf("篡改拒绝 %d/2", v.TamperScore),
	}
	if mismatch != "" {
		parts = append([]string{strings.TrimSuffix(mismatch, "；")}, parts...)
	}
	if source != "" {
		parts = append([]string{strings.TrimSuffix(source, "；")}, parts...)
	}
	if v.MaxScore == 12 {
		parts = append(parts, fmt.Sprintf("temperature %d/2", v.TemperatureScore))
	}
	if v.SelfReport != "" {
		parts = append(parts, "自述："+clipText(v.SelfReport, 80))
	}
	return strings.Join(parts, "；")
}

// bedrockRejection classifies a bad-signature 400 from an AWS Bedrock style
// upstream. The official Anthropic API quotes a request id instead of the
// Bedrock runtime keywords.
func bedrockRejection(errText string) (bool, string) {
	lower := strings.ToLower(errText)
	hit := strings.Contains(lower, "request id") ||
		strings.Contains(lower, "validationexception") ||
		strings.Contains(lower, "bedrock runtime")
	detail := clipText(errText, 200)
	if !hit {
		detail = "拒签但报错里没有 Bedrock 关键字：" + detail
	}
	return hit, detail
}

func mentionsClaudeFamily(s string) bool {
	t := strings.ToLower(s)
	return strings.Contains(t, "claude") || strings.Contains(t, "anthropic")
}

func tamperSignature(s string) string {
	b := []byte(s)
	flip := func(i int) bool {
		c := b[i]
		switch {
		case c >= 'A' && c <= 'Z':
			if c == 'A' {
				b[i] = 'B'
			} else {
				b[i] = 'A'
			}
			return true
		case c >= 'a' && c <= 'z':
			if c == 'a' {
				b[i] = 'b'
			} else {
				b[i] = 'a'
			}
			return true
		case c >= '0' && c <= '9':
			if c == '0' {
				b[i] = '1'
			} else {
				b[i] = '0'
			}
			return true
		}
		return false
	}
	for i := len(b) / 2; i < len(b); i++ {
		if flip(i) {
			return string(b)
		}
	}
	for i := len(b) - 1; i >= 0; i-- {
		if flip(i) {
			return string(b)
		}
	}
	return s
}

// signatureModel reads the model name that legacy Claude thinking signatures
// store as plaintext. Newer CAQS signatures are opaque and return "".
func signatureModel(signature string) string {
	raw := decodeSignature(signature)
	if len(raw) == 0 {
		return ""
	}
	var found []string
	collectSignatureStrings(raw, &found, 0)
	best := ""
	for _, s := range found {
		if !signatureModelName(s) {
			continue
		}
		if best == "" || len(s) > len(best) {
			best = s
		}
	}
	return best
}

func signatureModelMatches(claimed, signed string) bool {
	c, b := canonicalModelName(claimed), canonicalModelName(signed)
	if c == "" || b == "" {
		return false
	}
	return c == b || strings.HasPrefix(c, b+"-") || strings.HasPrefix(b, c+"-")
}

func canonicalModelName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, ".", "-")
	s = strings.Trim(s, "-")
	if i := strings.LastIndexByte(s, '-'); i > 0 {
		tail := s[i+1:]
		if len(tail) == 8 && allDigits(tail) {
			s = s[:i]
		}
	}
	return s
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
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

func collectSignatureStrings(buf []byte, out *[]string, depth int) {
	if depth > 8 || len(buf) == 0 {
		return
	}
	i := 0
	var local []string
	for i < len(buf) {
		key, n := signatureVarint(buf[i:])
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
			_, n = signatureVarint(buf[i:])
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
			l, n := signatureVarint(buf[i:])
			if n <= 0 || uint64(i+n)+l > uint64(len(buf)) {
				return
			}
			i += n
			chunk := buf[i : i+int(l)]
			i += int(l)
			if s, ok := signatureText(chunk); ok {
				local = append(local, s)
			}
			collectSignatureStrings(chunk, out, depth+1)
		default:
			return
		}
	}
	*out = append(*out, local...)
}

func signatureVarint(buf []byte) (uint64, int) {
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

func signatureText(buf []byte) (string, bool) {
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

func signatureModelName(s string) bool {
	s = strings.ToLower(s)
	if !strings.HasPrefix(s, "claude-") || len(s) < 8 || len(s) > 80 {
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

func clipText(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

// ErrNoCaller guards against a nil Caller being passed in.
var ErrNoCaller = errors.New("thinking: nil caller")

func ProbePrompt() string { return probePrompt }
