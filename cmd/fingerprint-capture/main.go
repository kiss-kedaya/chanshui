// fingerprint-capture collects independent probes; API keys are read from stdin
// or CHANSHUI_CAPTURE_KEY and are never written into the capture artifact.
package main

import (
	"bufio"
	"chanzhushui/internal/fingerprint"
	"chanzhushui/internal/gateway"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type record struct {
	Index          int    `json:"index"`
	Split          string `json:"split"`
	Prompt         string `json:"prompt"`
	Expected       int    `json:"expected_count"`
	Answer         string `json:"answer"`
	Parsed         int    `json:"parsed_count"`
	Usable         bool   `json:"usable"`
	Status         int    `json:"http_status"`
	Attempts       int    `json:"attempts"`
	Ms             int64  `json:"ms"`
	StopReason     string `json:"stop_reason"`
	OutputTokens   int    `json:"output_tokens"`
	ThinkingTokens int    `json:"thinking_tokens"`
	Error          string `json:"error,omitempty"`
	Protocol       string `json:"protocol,omitempty"`
}

// pacedTransport only enforces an explicitly supplied upstream request quota.
// It paces every HTTP attempt, including retries, rather than letting bursts
// turn an advertised per-minute limit into repeated 429 responses.
type pacedTransport struct {
	base     http.RoundTripper
	interval time.Duration
	mu       sync.Mutex
	next     time.Time
}

func (p *pacedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	p.mu.Lock()
	now := time.Now()
	slot := p.next
	if slot.Before(now) {
		slot = now
	}
	p.next = slot.Add(p.interval)
	p.mu.Unlock()
	if wait := time.Until(slot); wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-timer.C:
		}
	}
	return p.base.RoundTrip(req)
}

func main() {
	base := flag.String("base-url", "https://redacted.invalid", "upstream base URL")
	model := flag.String("model", "deepseek-v4.1-flash", "model label to sample")
	out := flag.String("out", ".local/deepseek-capture.json", "output artifact (must not exist)")
	train := flag.Int("train", 36, "training samples")
	holdout := flag.Int("holdout", 12, "independent validation samples")
	concurrency := flag.Int("concurrency", 3, "parallel upstream requests")
	seed := flag.Int64("seed", 20261005, "reproducible independent challenge seed")
	maxTokens := flag.Int("max-tokens", 16384, "output budget including reasoning")
	timeout := flag.Int("timeout", 180, "per-attempt timeout in seconds")
	resume := flag.Bool("resume", false, "continue an existing matching capture; completed rows are retained")
	retryInvalid := flag.Bool("retry-invalid", false, "retry failed rows when resuming")
	rotateInvalid := flag.Bool("rotate-invalid", false, "when retrying invalid rows, draw a fresh challenge instead of repeating one the model refused")
	requestsPerMinute := flag.Int("requests-per-minute", 0, "honor an explicit upstream request quota; 0 disables pacing")
	protocol := flag.String("protocol", "chat", "chat, auto, anthropic, or openai; use the endpoint contract")
	flag.Parse()
	if *protocol != "chat" && *protocol != "auto" && *protocol != "anthropic" && *protocol != "openai" {
		panic("invalid protocol")
	}
	if *train < 0 || *holdout < 0 || *train+*holdout < 1 || *concurrency < 1 || *timeout < 1 || *requestsPerMinute < 0 {
		panic("invalid sample counts or concurrency")
	}
	if !*resume {
		file, err := os.OpenFile(*out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			panic(err)
		}
		file.Close()
	}
	key := strings.TrimSpace(os.Getenv("CHANSHUI_CAPTURE_KEY"))
	if key == "" {
		key, _ = bufio.NewReader(os.Stdin).ReadString('\n')
		key = strings.TrimSpace(key)
	}
	if key == "" {
		panic("API key required on stdin or CHANSHUI_CAPTURE_KEY")
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy = nil
	defer tr.CloseIdleConnections()
	var transport http.RoundTripper = tr
	if *requestsPerMinute > 0 {
		transport = &pacedTransport{base: tr, interval: time.Minute / time.Duration(*requestsPerMinute)}
	}
	client := gateway.Client{BaseURL: *base, Key: key, Mode: *protocol, HTTP: &http.Client{Transport: transport}}
	rng := rand.New(rand.NewSource(*seed))
	var challenges []fingerprint.Challenge
	for len(challenges) < *train+*holdout {
		challenges = append(challenges, fingerprint.GenerateChallenges(min(41, *train+*holdout-len(challenges)), rng)...)
	}
	rows := make([]record, len(challenges))
	var mu sync.Mutex
	started := time.Now().UTC().Format(time.RFC3339)
	if *resume {
		raw, err := os.ReadFile(*out)
		if err != nil {
			panic(err)
		}
		var previous struct {
			Model     string   `json:"model"`
			Base      string   `json:"base_url"`
			Started   string   `json:"started_at"`
			Seed      int64    `json:"seed"`
			MaxTokens int      `json:"max_tokens"`
			Train     int      `json:"training_requested"`
			Holdout   int      `json:"holdout_requested"`
			Records   []record `json:"records"`
		}
		if err = json.Unmarshal(raw, &previous); err != nil {
			panic(err)
		}
		if previous.Model != *model || previous.Base != *base || previous.Seed != *seed || previous.MaxTokens != *maxTokens || previous.Train != *train || previous.Holdout != *holdout || len(previous.Records) != len(rows) {
			panic("resume parameters differ from capture")
		}
		rows = previous.Records
		started = previous.Started
		if *retryInvalid {
			// A provider refusal is a property of the prompt, so repeating it can
			// never yield a sample. -rotate-invalid draws a fresh challenge for the
			// failed slot instead; the artifact still records the prompt actually asked.
			rotate := rand.New(rand.NewSource(time.Now().UnixNano()))
			for i := range rows {
				if rows[i].Index != 0 && !rows[i].Usable {
					if *rotateInvalid {
						challenges[i] = fingerprint.GenerateChallenges(1, rotate)[0]
					}
					rows[i] = record{}
				}
			}
		}
	}
	save := func() {
		data := map[string]any{"schema_version": 1, "model": *model, "base_url": *base, "protocol": client.ProtocolFor(*model), "started_at": started, "updated_at": time.Now().UTC().Format(time.RFC3339), "seed": *seed, "training_requested": *train, "holdout_requested": *holdout, "max_tokens": *maxTokens, "timeout_seconds": *timeout, "temperature": "upstream default", "identity": "endpoint-labelled baseline, not independently verified model identity", "records": rows}
		raw, e := json.MarshalIndent(data, "", "  ")
		if e != nil {
			panic(e)
		}
		if e = os.WriteFile(*out+".tmp", raw, 0600); e != nil {
			panic(e)
		}
		if e = os.Rename(*out+".tmp", *out); e != nil {
			panic(e)
		}
	}
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < min(*concurrency, len(challenges)); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				ch := challenges[i]
				m, e := client.CallMessages(context.Background(), gateway.CallRequest{Model: *model, Prompt: ch.Prompt, MaxTokens: *maxTokens, Stream: true, TimeoutSeconds: *timeout})
				errorText := m.Err
				if e != nil {
					errorText = e.Error()
				}
				errorText = strings.ReplaceAll(errorText, key, "[redacted]")
				split := "train"
				if i >= *train {
					split = "holdout"
				}
				row := record{Index: i + 1, Split: split, Prompt: ch.Prompt, Expected: ch.ExpectedCount, Answer: strings.ReplaceAll(m.Answer, key, "[redacted]"), Parsed: len(fingerprint.ParseNumbers(m.Answer)), Usable: errorText == "" && m.StopReason != "max_tokens" && fingerprint.Usable(m.Answer, ch.ExpectedCount, 80), Status: m.Status, Attempts: m.Attempts, Ms: m.Ms, Error: errorText}
				row.StopReason, row.OutputTokens, row.ThinkingTokens = m.StopReason, m.OutputTokens, m.ThinkingTokens
				row.Protocol = m.Protocol
				mu.Lock()
				rows[i] = row
				save()
				fmt.Printf("sample %d/%d split=%s usable=%v numbers=%d attempts=%d ms=%d error=%s\n", i+1, len(challenges), split, row.Usable, row.Parsed, row.Attempts, row.Ms, row.Error)
				mu.Unlock()
			}
		}()
	}
	for i := range challenges {
		mu.Lock()
		complete := rows[i].Index != 0 && (rows[i].Usable || !*retryInvalid)
		mu.Unlock()
		if !complete {
			jobs <- i
		}
	}
	close(jobs)
	wg.Wait()
}
