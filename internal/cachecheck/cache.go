package cachecheck

import (
	"chanzhushui/internal/gateway"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

const cacheSlack = 64
const cacheTurns = 6

type Sample struct {
	Index          int      `json:"index"`
	Read           int      `json:"read"`
	Write          int      `json:"write"`
	Input          int      `json:"input"`
	Output         int      `json:"output"`
	Total          int      `json:"total"`
	ReadReported   bool     `json:"read_reported"`
	WriteReported  bool     `json:"write_reported"`
	InputReported  bool     `json:"input_reported"`
	OutputReported bool     `json:"output_reported"`
	Attempts       int      `json:"attempts"`
	HitRate        *float64 `json:"hit_rate"`
	PromptChars    int      `json:"prompt_chars"`
	Messages       int      `json:"messages"`
	RequestHash    string   `json:"request_sha256,omitempty"`
	Error          string   `json:"error,omitempty"`
}
type Transition struct {
	From     int   `json:"from"`
	To       int   `json:"to"`
	Expected int   `json:"expected_read"`
	Actual   int   `json:"actual_read"`
	Delta    int   `json:"delta"`
	Pass     *bool `json:"pass"`
}
type Result struct {
	Status            string       `json:"status"`
	Pass              *bool        `json:"pass"`
	Reason            string       `json:"reason"`
	First             *Sample      `json:"first"`
	Second            *Sample      `json:"second"`
	Third             *Sample      `json:"third"`
	Samples           []Sample     `json:"samples"`
	Transitions       []Transition `json:"transitions"`
	Expected          int          `json:"expected_read"`
	Actual            int          `json:"actual_read"`
	Delta             int          `json:"delta"`
	Protocol          string       `json:"protocol"`
	ThirdReadRatio    *float64     `json:"third_read_ratio"`
	ThirdReadBaseline int          `json:"third_read_baseline"`
	TotalRead         int          `json:"total_read"`
	TotalOutput       int          `json:"total_output"`
	PrefixHash        string       `json:"prefix_sha256"`
	RetriesObserved   bool         `json:"retries_observed"`
}

func chainRate(read, baseline int) *float64 {
	if baseline <= 0 || read < 0 {
		return nil
	}
	v := math.Round(float64(read)/float64(baseline)*10000) / 100
	return &v
}
func sample(m gateway.Metrics, index int) Sample {
	total := m.InputTokens + m.CacheRead + m.CacheCreation5m + m.CacheCreation1h
	s := Sample{Index: index, Read: m.CacheRead, Write: m.CacheCreation5m + m.CacheCreation1h, Input: m.InputTokens, Output: m.OutputTokens, Total: total, ReadReported: m.CacheReadReported, WriteReported: m.CacheWriteReported, InputReported: m.UsagePresent, Attempts: m.Attempts}
	s.OutputReported = m.OutputReported
	return s
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// FromMetrics checks a serial conversation. When every sample reports cache
// read and none reports cache write, the last-round rate is read divided by
// that round's total input. A positive last read passes. A zero last read is
// 0% and does not pass. Missing write is never treated as zero in a mixed
// chain. When writes are reported, the pass condition stays the last read
// versus the previous read+write, within 64 tokens, and that ratio is 100%
// only when the two sides match.
func FromMetrics(metrics []gateway.Metrics, prefixHash, protocol string, retriesObserved bool) Result {
	r := Result{Status: "unknown", Protocol: protocol, PrefixHash: prefixHash, RetriesObserved: retriesObserved, Samples: []Sample{}, Transitions: []Transition{}}
	for i, m := range metrics {
		s := sample(m, i+1)
		if m.Err != "" {
			s.Error = m.Err
		}
		r.Samples = append(r.Samples, s)
	}
	if len(r.Samples) == 0 {
		r.Reason = "没有完成缓存请求。"
		return r
	}
	for i := 1; i < len(r.Samples); i++ {
		prev := r.Samples[i-1]
		if prev.Error != "" || r.Samples[i].Error != "" || !prev.ReadReported || !prev.WriteReported || !r.Samples[i].ReadReported {
			continue
		}
		r.Samples[i].HitRate = chainRate(r.Samples[i].Read, prev.Read+prev.Write)
	}
	r.First = &r.Samples[0]
	if len(r.Samples) > 1 {
		r.Second = &r.Samples[1]
	}
	if len(r.Samples) > 2 {
		r.Third = &r.Samples[2]
	}
	for _, s := range r.Samples {
		r.TotalRead += s.Read
		r.TotalOutput += s.Output
	}
	if len(r.Samples) < 2 {
		r.Reason = "缓存对话不足两轮，无法验证衔接。"
		return r
	}
	if readOnlySamples(r.Samples) {
		return finishReadOnly(r)
	}
	prev := r.Samples[len(r.Samples)-2]
	last := r.Samples[len(r.Samples)-1]
	if prev.ReadReported && prev.WriteReported {
		r.ThirdReadBaseline = prev.Read + prev.Write
	}
	if last.Error == "" && last.ReadReported && prev.ReadReported && prev.WriteReported {
		r.ThirdReadRatio = chainRate(last.Read, prev.Read+prev.Write)
	}
	var link *Transition
	for i := 0; i+1 < len(r.Samples); i++ {
		a, b := r.Samples[i], r.Samples[i+1]
		tr := Transition{From: i + 1, To: i + 2, Expected: a.Read + a.Write, Actual: b.Read, Delta: b.Read - a.Read - a.Write}
		if a.Error == "" && b.Error == "" && a.ReadReported && a.WriteReported && b.ReadReported && tr.Expected > 0 {
			pass := absInt(tr.Delta) <= cacheSlack
			tr.Pass = &pass
		}
		r.Transitions = append(r.Transitions, tr)
		link = &r.Transitions[len(r.Transitions)-1]
	}
	if link != nil {
		r.Expected = link.Expected
		r.Actual = link.Actual
		r.Delta = link.Delta
	}
	fieldsOK := prev.Error == "" && last.Error == "" && r.Samples[0].ReadReported && prev.ReadReported && prev.WriteReported && last.ReadReported
	if !fieldsOK || link == nil || link.Pass == nil {
		if prev.Error == "" && last.Error == "" && prev.ReadReported && prev.WriteReported && last.ReadReported && prev.Read+prev.Write == 0 {
			r.Reason = "上一轮缓存读和写都是 0，前缀没有进缓存。常见原因是前缀低于缓存门槛，或接口没有接住缓存断点。"
			return r
		}
		r.Reason = "读写字段不齐或上一轮缓存量为 0，无法验证衔接；缺失字段不按 0。"
		return r
	}
	grew := len(r.Samples) > 1 && r.Samples[1].Read+r.Samples[1].Write > r.Samples[0].Read+cacheSlack
	if !grew {
		fail := false
		r.Pass = &fail
		r.Status = "mismatch"
		r.Reason = "第二轮没有比第一轮多出一段缓存，新对话没有进入缓存。"
		return r
	}
	r.Pass = link.Pass
	r.Status = "mismatch"
	r.Reason = "上一轮读加写与末轮读相差超过 64 token。末轮缓存率是末轮读取除以上一轮读加写。"
	if *link.Pass {
		r.Status = "pass"
		r.Reason = "上一轮读加写与末轮读相差不超过 64 token。末轮缓存率是末轮读取除以上一轮读加写，相等为 100%。"
	}
	if r.RetriesObserved {
		r.Reason += " 本组发生重试，首次成功样本可能不是冷缓存；重试用量未计入样本命中率。"
	}
	return r
}

func readOnlySamples(samples []Sample) bool {
	if len(samples) < 2 {
		return false
	}
	for _, s := range samples {
		if s.Error != "" || !s.ReadReported || s.WriteReported {
			return false
		}
	}
	return true
}

func finishReadOnly(r Result) Result {
	for i := range r.Samples {
		s := &r.Samples[i]
		if s.Total > 0 {
			s.HitRate = chainRate(s.Read, s.Total)
			continue
		}
		zero := 0.0
		s.HitRate = &zero
	}
	r.First = &r.Samples[0]
	if len(r.Samples) > 1 {
		r.Second = &r.Samples[1]
	}
	if len(r.Samples) > 2 {
		r.Third = &r.Samples[2]
	}
	last := r.Samples[len(r.Samples)-1]
	if last.Total > 0 {
		r.ThirdReadRatio = chainRate(last.Read, last.Total)
	} else {
		zero := 0.0
		r.ThirdReadRatio = &zero
	}
	pass := last.Read > 0
	r.Pass = &pass
	if pass {
		r.Status = "pass"
		r.Reason = "上游没有回传缓存写入。末轮缓存率是末轮读取除以末轮总输入。"
	} else {
		r.Status = "mismatch"
		r.Reason = "上游没有回传缓存写入，末轮读取为 0，缓存率按 0% 记，未通过。"
	}
	if r.RetriesObserved {
		r.Reason += " 检测发生过重试，首次成功回包可能不再进缓存；缓存率未因重试被改写。"
	}
	return r
}

type Collection struct {
	Result  Result
	Metrics []gateway.Metrics
	Units   []int
}

func prefixSentences(model string) int {
	// Sonnet and Opus cache from 1024 tokens. Haiku caches from 2048.
	// One prefix above both thresholds keeps the six-turn chain comparable.
	_ = model
	return 170
}

func cacheSystemText(n int) string {
	var b strings.Builder
	b.Grow(n * 64)
	b.WriteString("Stable agent brief. Keep this document byte-for-byte identical across turns.\n")
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "Cache prefix sentence %03d remains byte for byte identical.\n", i)
	}
	return b.String()
}

func cacheTurnPrompt(i int) string {
	return fmt.Sprintf("Continue the same document. Do not rewrite earlier turns. Reply with only noted-%d.", i+1)
}

func replayAnswer(answer string) string {
	answer = strings.TrimSpace(answer)
	if i := strings.IndexByte(answer, '\n'); i >= 0 {
		answer = strings.TrimSpace(answer[:i])
	}
	if answer == "" {
		return "noted"
	}
	if utf8.RuneCountInString(answer) <= 80 {
		return answer
	}
	runes := []rune(answer)
	return string(runes[:80])
}

// Collect runs six serial chat turns. The system document stays byte-identical.
// Each later turn replays the earlier user and assistant lines, the same way an
// agent continues a thread. Anthropic breakpoints sit on the system block, the
// previous user line, and the new user line. No one-hour TTL is sent.
func Collect(ctx context.Context, c gateway.Client, model string, timeout int) (Collection, error) {
	col := Collection{Metrics: make([]gateway.Metrics, 0, cacheTurns), Units: make([]int, 0, cacheTurns)}
	systemText := cacheSystemText(prefixSentences(model))
	sum := sha256.Sum256([]byte(systemText))
	prefixHash := hex.EncodeToString(sum[:])
	protocol := c.ProtocolFor(model)
	var shapes []Sample
	var history []gateway.ConversationMessage
	finish := func() {
		col.Result = FromMetrics(col.Metrics, prefixHash, protocol, hasRetry(col.Metrics))
		for i, shape := range shapes {
			if i >= len(col.Result.Samples) {
				break
			}
			col.Result.Samples[i].PromptChars = shape.PromptChars
			col.Result.Samples[i].Messages = shape.Messages
			col.Result.Samples[i].RequestHash = shape.RequestHash
		}
		if len(col.Result.Samples) > 0 {
			col.Result.First = &col.Result.Samples[0]
		}
		if len(col.Result.Samples) > 1 {
			col.Result.Second = &col.Result.Samples[1]
		}
		if len(col.Result.Samples) > 2 {
			col.Result.Third = &col.Result.Samples[2]
		}
	}
	mark := &gateway.CacheControl{}
	for i := 0; i < cacheTurns; i++ {
		prompt := cacheTurnPrompt(i)
		hist := append([]gateway.ConversationMessage(nil), history...)
		q := gateway.CallRequest{
			Model:          model,
			Prompt:         prompt,
			MaxTokens:      64,
			TimeoutSeconds: timeout,
			Validate:       validateSample,
			System:         []gateway.SystemBlock{{Text: systemText}},
			History:        hist,
		}
		if protocol == "messages" {
			q.System[0].CacheControl = mark
			q.PromptCacheControl = mark
			for j := len(q.History) - 1; j >= 0; j-- {
				if q.History[j].Role == "user" {
					q.History[j].CacheControl = mark
					break
				}
			}
		}
		m, e := c.CallMessages(ctx, q)
		if e != nil {
			m.Err = e.Error()
		}
		if m.Err == "" {
			if e = validateSample(m); e != nil {
				m.Err = e.Error()
			}
		}
		col.Metrics = append(col.Metrics, m)
		visible := systemText + "\n" + prompt
		for _, item := range hist {
			visible += "\n" + item.Content
		}
		hash := sha256.Sum256([]byte(visible))
		shapes = append(shapes, Sample{PromptChars: len(visible), Messages: len(hist) + 1, RequestHash: hex.EncodeToString(hash[:])})
		col.Units = append(col.Units, len(visible))
		if m.Err != "" {
			finish()
			col.Result.Reason = "缓存请求未完成，保留已完成样本；不把缺失样本当成0。"
			return col, fmt.Errorf("第%d次缓存请求：%s", i+1, m.Err)
		}
		if i == 0 {
			protocol = m.Protocol
			c.PinProtocol(m.Protocol)
		}
		history = append(history, gateway.ConversationMessage{Role: "user", Content: prompt}, gateway.ConversationMessage{Role: "assistant", Content: replayAnswer(m.Answer)})
	}
	finish()
	return col, nil
}

func Measure(ctx context.Context, c gateway.Client, model string, timeout int) (Result, error) {
	collection, err := Collect(ctx, c, model, timeout)
	return collection.Result, err
}

func validateSample(m gateway.Metrics) error {
	if !m.UsagePresent || m.InputTokens < 0 || m.CacheRead < 0 || m.CacheCreation5m < 0 || m.CacheCreation1h < 0 || m.OutputTokens < 0 {
		return fmt.Errorf("上游缺少有效 token 用量")
	}
	if strings.TrimSpace(m.Answer) == "" || m.StopReason == "max_tokens" {
		return fmt.Errorf("上游确认文本为空或被截断")
	}
	return nil
}

func hasRetry(metrics []gateway.Metrics) bool {
	for _, m := range metrics {
		if m.Attempts > 1 {
			return true
		}
	}
	return false
}
