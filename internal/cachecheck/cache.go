package cachecheck

import (
	"chanzhushui/internal/gateway"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strings"
)

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

func rate(read, total int) *float64 {
	if total <= 0 || read < 0 {
		return nil
	}
	v := math.Round(float64(read)/float64(total)*10000) / 100
	return &v
}
func sample(m gateway.Metrics, index int) Sample {
	total := m.InputTokens + m.CacheRead + m.CacheCreation5m + m.CacheCreation1h
	s := Sample{Index: index, Read: m.CacheRead, Write: m.CacheCreation5m + m.CacheCreation1h, Input: m.InputTokens, Output: m.OutputTokens, Total: total, ReadReported: m.CacheReadReported, WriteReported: m.CacheWriteReported, InputReported: m.UsagePresent, Attempts: m.Attempts}
	s.OutputReported = m.OutputReported
	if m.UsagePresent && m.CacheReadReported && (m.Protocol != "messages" || m.CacheWriteReported) {
		s.HitRate = rate(s.Read, s.Total)
	}
	return s
}

// FromMetrics builds the cache report from already-collected sequential calls.
func FromMetrics(metrics []gateway.Metrics, prefixHash, protocol string, retriesObserved bool) Result {
	r := Result{Status: "unknown", Protocol: protocol, PrefixHash: prefixHash, RetriesObserved: retriesObserved, Samples: []Sample{}, Transitions: []Transition{}}
	for i, m := range metrics {
		s := sample(m, i+1)
		r.Samples = append(r.Samples, s)
		if m.Err != "" {
			s.Error = m.Err
			r.Samples[len(r.Samples)-1] = s
		}
	}
	if len(r.Samples) == 0 {
		r.Reason = "没有完成缓存请求。"
		return r
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
	if len(r.Samples) == 3 && r.Samples[1].Error == "" && r.Samples[2].Error == "" && r.Samples[1].ReadReported && r.Samples[1].WriteReported && r.Samples[2].ReadReported {
		denom := r.Samples[1].Read + r.Samples[1].Write
		r.ThirdReadBaseline = denom
		if denom > 0 {
			r.ThirdReadRatio = rate(r.Samples[2].Read, denom)
		}
	}
	allKnown, allPass := len(r.Samples) == 3, true
	// A read-only warm hit is not proof that this endpoint can write a
	// cache. Require a positive write in the first two rounds before calling
	// the read/write chain a pass; reported zero is evidence, not a write.
	if len(r.Samples) == 3 &&
		(!r.Samples[0].WriteReported || !r.Samples[1].WriteReported ||
			r.Samples[0].Write <= 0 || r.Samples[1].Write <= 0) {
		allKnown = false
	}
	for i := 0; i+1 < len(r.Samples) && i < 2; i++ {
		a, b := r.Samples[i], r.Samples[i+1]
		tr := Transition{From: i + 1, To: i + 2, Expected: a.Read + a.Write, Actual: b.Read, Delta: b.Read - a.Read - a.Write}
		if a.Error == "" && b.Error == "" && a.ReadReported && a.WriteReported && b.ReadReported && tr.Expected > 0 {
			pass := tr.Delta == 0
			tr.Pass = &pass
			allPass = allPass && pass
		} else {
			allKnown = false
		}
		r.Transitions = append(r.Transitions, tr)
	}
	if len(r.Transitions) > 0 {
		r.Expected = r.Transitions[0].Expected
		r.Actual = r.Transitions[0].Actual
		r.Delta = r.Transitions[0].Delta
	}
	if !allKnown {
		r.Reason = "缓存读写字段不齐或上一轮缓存量为0，无法完整验证衔接；缺失字段不按0处理。"
		return r
	}
	r.Pass = &allPass
	r.Status = "mismatch"
	r.Reason = "缓存衔接未全部对齐；查看每轮读、写、输出和第三轮读取比例。"
	if allPass {
		r.Status = "pass"
		r.Reason = "三轮请求的缓存读写衔接已完成；第三轮仅展示相对上一轮缓存量的读取比例。"
	}
	if r.RetriesObserved {
		r.Reason += " 本组发生重试，首次成功样本可能不是冷缓存；重试用量未计入样本命中率。"
	}
	return r
}

// Collection records one growing conversation for the cache check only.
type Collection struct {
	Result  Result
	Metrics []gateway.Metrics
	Units   []int
}

// Collect issues three continuations. Every user input gets a cache breakpoint,
// so the following call can read the previous prefix and write its new suffix.
func Collect(ctx context.Context, c gateway.Client, model string, timeout int) (Collection, error) {
	col := Collection{Metrics: make([]gateway.Metrics, 0, 3), Units: make([]int, 0, 3)}
	var nonce [16]byte
	if _, e := rand.Read(nonce[:]); e != nil {
		return col, e
	}
	var history []gateway.ConversationMessage
	var shapes []Sample
	protocol := c.ProtocolFor(model)
	var prefixHash string
	totalUnits := 0
	finish := func() {
		col.Result = FromMetrics(col.Metrics, prefixHash, protocol, hasRetry(col.Metrics))
		for i, shape := range shapes {
			col.Result.Samples[i].PromptChars = shape.PromptChars
			col.Result.Samples[i].Messages = shape.Messages
			col.Result.Samples[i].RequestHash = shape.RequestHash
		}
	}
	// Each round waits for the previous response before starting. A few hundred
	// tokens is enough to see a cache write; the rounds must not run together.
	for i, units := range []int{240, 200, 160} {
		prompt := strings.Repeat("A ", units) + "\nReply only OK."
		if i == 0 {
			prompt = "Audit " + hex.EncodeToString(nonce[:]) + "\n" + prompt
			hash := sha256.Sum256([]byte(prompt))
			prefixHash = hex.EncodeToString(hash[:])
		}
		q := gateway.CallRequest{Model: model, History: history, Prompt: prompt, MaxTokens: 64, TimeoutSeconds: timeout, Validate: validateSample}
		if protocol == "messages" {
			q.PromptCacheControl = &gateway.CacheControl{TTL: "1h"}
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
		// The independent variable is the complete visible user text in bytes.
		// This includes the nonce and fixed suffix, preventing them from being
		// misreported as hidden injected tokens.
		totalUnits += len(prompt)
		col.Units = append(col.Units, totalUnits)
		var request strings.Builder
		for _, msg := range history {
			fmt.Fprintf(&request, "%s:%s\n", msg.Role, msg.Content)
		}
		request.WriteString("user:" + prompt)
		hash := sha256.Sum256([]byte(request.String()))
		shapes = append(shapes, Sample{PromptChars: len(prompt), Messages: len(history) + 1, RequestHash: hex.EncodeToString(hash[:])})
		if m.Err != "" {
			finish()
			col.Result.Reason = "缓存请求未完成，保留已完成样本；不把缺失样本当成0。"
			return col, fmt.Errorf("第%d次缓存请求：%s", i+1, m.Err)
		}
		if i == 0 {
			protocol = m.Protocol
			c.PinProtocol(m.Protocol)
		}
		history = append(history, gateway.ConversationMessage{Role: "user", Content: prompt, CacheControl: q.PromptCacheControl}, gateway.ConversationMessage{Role: "assistant", Content: m.Answer})
	}
	finish()
	return col, nil
}

// Measure is the cache check entry point. Injection sampling is intentionally
// implemented independently in internal/inject.
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
