package cachecheck

import (
	"chanzhushui/internal/gateway"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strings"
)

const cacheSlack = 64

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

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// FromMetrics builds the cache report from three sequential prefix calls.
// The pass check is round-2 read+write versus round-3 read, within 64 tokens.
// The displayed rate is only round-3 read divided by round-3 total input.
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
	if len(r.Samples) > 1 && r.Samples[1].ReadReported && r.Samples[1].WriteReported {
		r.ThirdReadBaseline = r.Samples[1].Read + r.Samples[1].Write
	}
	if len(r.Samples) == 3 && r.Samples[2].Error == "" && r.Samples[2].ReadReported {
		r.ThirdReadRatio = rate(r.Samples[2].Read, r.Samples[2].Total)
	}
	var link *Transition
	for i := 0; i+1 < len(r.Samples) && i < 2; i++ {
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
	fieldsOK := len(r.Samples) == 3 && r.Samples[1].Error == "" && r.Samples[2].Error == "" && r.Samples[0].ReadReported && r.Samples[1].ReadReported && r.Samples[1].WriteReported && r.Samples[2].ReadReported
	if !fieldsOK || link == nil || link.Pass == nil {
		if len(r.Samples) == 3 && r.Samples[1].Error == "" && r.Samples[2].Error == "" && r.Samples[1].ReadReported && r.Samples[1].WriteReported && r.Samples[2].ReadReported && r.Samples[1].Read+r.Samples[1].Write == 0 {
			r.Reason = "第二轮缓存读和写都是 0，前缀没有进缓存。常见原因是前缀低于缓存门槛，或接口没有接住缓存断点。"
			return r
		}
		r.Reason = "读写字段不齐或上一轮缓存量为 0，无法验证衔接；缺失字段不按 0。"
		return r
	}
	grew := r.Samples[1].Read+r.Samples[1].Write > r.Samples[0].Read+cacheSlack
	if !grew {
		fail := false
		r.Pass = &fail
		r.Status = "mismatch"
		r.Reason = "第二轮缓存量没有比第一轮读取多出一段前缀，新内容没有进入缓存。"
		return r
	}
	r.Pass = link.Pass
	r.Status = "mismatch"
	r.Reason = "第二轮读加写与第三轮读相差超过 64 token。第三轮缓存率仍按该轮读取除以该轮总输入。"
	if *link.Pass {
		r.Status = "pass"
		r.Reason = "第二轮读加写与第三轮读相差不超过 64 token。第三轮缓存率是该轮读取除以该轮总输入。"
	}
	if r.RetriesObserved {
		r.Reason += " 本组发生重试，首次成功样本可能不是冷缓存；重试用量未计入样本命中率。"
	}
	return r
}

type Collection struct {
	Result  Result
	Metrics []gateway.Metrics
	Units   []int
}

const (
	cacheTailSame = "Reply with only OK."
	cacheTailLast = "Reply with only OK now."
)

func cacheSystemText() string {
	var b strings.Builder
	b.Grow(200 * 64)
	for i := 1; i <= 200; i++ {
		fmt.Fprintf(&b, "Cache prefix sentence %03d remains byte for byte identical.\n", i)
	}
	return b.String()
}

func cacheExtraText() string {
	var b strings.Builder
	for i := 1; i <= 20; i++ {
		fmt.Fprintf(&b, "Cache extension sentence %02d adds only the second block.\n", i)
	}
	return b.String()
}

// Collect sends three serial prefix probes. A is a stable system prefix, B is
// added on rounds 2 and 3, and only the short tail changes on round 3.
// Assistant replies are not replayed, so the cached prefix stays byte-identical.
func Collect(ctx context.Context, c gateway.Client, model string, timeout int) (Collection, error) {
	col := Collection{Metrics: make([]gateway.Metrics, 0, 3), Units: make([]int, 0, 3)}
	systemText := cacheSystemText()
	extraText := cacheExtraText()
	sum := sha256.Sum256([]byte(systemText))
	prefixHash := hex.EncodeToString(sum[:])
	protocol := c.ProtocolFor(model)
	var shapes []Sample
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
	hour := &gateway.CacheControl{TTL: "1h"}
	for i := 0; i < 3; i++ {
		tail := cacheTailSame
		if i == 2 {
			tail = cacheTailLast
		}
		q := gateway.CallRequest{
			Model:          model,
			Prompt:         tail,
			MaxTokens:      64,
			TimeoutSeconds: timeout,
			Validate:       validateSample,
			System:         []gateway.SystemBlock{{Text: systemText}},
		}
		visible := tail
		if protocol == "messages" {
			q.System[0].CacheControl = hour
		}
		if i > 0 {
			block := gateway.TextBlock{Text: extraText}
			if protocol == "messages" {
				block.CacheControl = hour
			}
			q.UserPrefix = []gateway.TextBlock{block}
			visible = extraText + tail
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
		col.Units = append(col.Units, len(systemText)+len(visible))
		hash := sha256.Sum256([]byte(systemText + "\n" + visible))
		shapes = append(shapes, Sample{PromptChars: len(visible), Messages: 1, RequestHash: hex.EncodeToString(hash[:])})
		if m.Err != "" {
			finish()
			col.Result.Reason = "缓存请求未完成，保留已完成样本；不把缺失样本当成0。"
			return col, fmt.Errorf("第%d次缓存请求：%s", i+1, m.Err)
		}
		if i == 0 {
			protocol = m.Protocol
			c.PinProtocol(m.Protocol)
		}
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
