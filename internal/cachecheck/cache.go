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
	Index         int      `json:"index"`
	Read          int      `json:"read"`
	Write         int      `json:"write"`
	Input         int      `json:"input"`
	Output        int      `json:"output"`
	Total         int      `json:"total"`
	ReadReported  bool     `json:"read_reported"`
	WriteReported bool     `json:"write_reported"`
	Attempts      int      `json:"attempts"`
	HitRate       *float64 `json:"hit_rate"`
	Error         string   `json:"error,omitempty"`
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
	Status          string       `json:"status"`
	Pass            *bool        `json:"pass"`
	Reason          string       `json:"reason"`
	First           *Sample      `json:"first"`
	Second          *Sample      `json:"second"`
	Third           *Sample      `json:"third"`
	Samples         []Sample     `json:"samples"`
	Transitions     []Transition `json:"transitions"`
	Expected        int          `json:"expected_read"`
	Actual          int          `json:"actual_read"`
	Delta           int          `json:"delta"`
	Protocol        string       `json:"protocol"`
	ThirdReadRatio  *float64     `json:"third_read_ratio"`
	TotalRead       int          `json:"total_read"`
	TotalOutput     int          `json:"total_output"`
	PrefixHash      string       `json:"prefix_sha256"`
	RetriesObserved bool         `json:"retries_observed"`
}

func rate(read, total int) *float64 {
	if total <= 0 || read < 0 || read > total {
		return nil
	}
	v := math.Round(float64(read)/float64(total)*10000) / 100
	return &v
}
func sample(m gateway.Metrics, index int) Sample {
	total := m.InputTokens + m.CacheRead + m.CacheCreation5m + m.CacheCreation1h
	s := Sample{Index: index, Read: m.CacheRead, Write: m.CacheCreation5m + m.CacheCreation1h, Input: m.InputTokens, Output: m.OutputTokens, Total: total, ReadReported: m.CacheReadReported, WriteReported: m.CacheWriteReported, Attempts: m.Attempts}
	return s
}

// FromMetrics builds the cache report from already-collected sequential calls.
// It lets the injection probe reuse the same three upstream requests.
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
	if len(r.Samples) == 3 && r.Samples[1].ReadReported && r.Samples[1].WriteReported && r.Samples[2].ReadReported {
		denom := r.Samples[1].Read + r.Samples[1].Write
		if denom > 0 {
			r.ThirdReadRatio = rate(r.Samples[2].Read, denom)
		}
	}
	allKnown, allPass := len(r.Samples) == 3, true
	for i := 0; i+1 < len(r.Samples) && i < 2; i++ {
		a, b := r.Samples[i], r.Samples[i+1]
		tr := Transition{From: i + 1, To: i + 2, Expected: a.Read + a.Write, Actual: b.Read, Delta: b.Read - a.Read - a.Write}
		if a.ReadReported && a.WriteReported && b.ReadReported && tr.Expected > 0 {
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
		r.Reason = "三个请求已完成，但缓存读写字段不齐或缓存量为0，无法完整验证衔接；0＝0不算通过。"
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
func Measure(ctx context.Context, c gateway.Client, model string, timeout int) (Result, error) {
	r := Result{Status: "unknown", Protocol: c.ProtocolFor(model), Samples: []Sample{}, Transitions: []Transition{}}
	metrics := make([]gateway.Metrics, 0, 3)
	var nonce [16]byte
	if _, e := rand.Read(nonce[:]); e != nil {
		return r, e
	}
	var prefix strings.Builder
	fmt.Fprintf(&prefix, "Cache audit %s. Fixed reference data.\n", hex.EncodeToString(nonce[:]))
	lines := 64
	if gateway.IsClaude(model) {
		lines = 160
	}
	for i := 0; i < lines; i++ {
		fmt.Fprintf(&prefix, "Record %04d: inventory verification values %d %d %d, stable repeated reference.\n", i, i*17+19, i*31+43, i*7+11)
	}
	hash := sha256.Sum256([]byte(prefix.String()))
	r.PrefixHash = hex.EncodeToString(hash[:])
	system := gateway.SystemBlock{Text: prefix.String()}
	if c.ProtocolFor(model) == "messages" {
		system.CacheControl = &gateway.CacheControl{TTL: "1h"}
	}
	q := gateway.CallRequest{Model: model, System: []gateway.SystemBlock{system}, Prompt: "Reply OK.", MaxTokens: 16, Stream: true, TimeoutSeconds: timeout}
	for i := 1; i <= 3; i++ {
		m, e := c.CallMessages(ctx, q)
		s := sample(m, i)
		if e != nil {
			s.Error = e.Error()
		} else {
			s.Error = m.Err
		}
		r.Samples = append(r.Samples, s)
		if m.Attempts > 1 {
			r.RetriesObserved = true
		}
		if s.Error != "" {
			r.Reason = "缓存请求未完成，保留已完成样本；不把缺失样本当成0。"
			return r, fmt.Errorf("第%d次缓存请求：%s", i, s.Error)
		}
		metrics = append(metrics, m)
		if i == 1 {
			r.Protocol = m.Protocol
			c.PinProtocol(m.Protocol)
		}
	}
	return FromMetrics(metrics, r.PrefixHash, r.Protocol, r.RetriesObserved), nil
}
