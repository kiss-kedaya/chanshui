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
	flag.Parse()
	if *train < 1 || *holdout < 0 || *concurrency < 1 || *concurrency > 8 {
		panic("invalid sample counts or concurrency")
	}
	file, err := os.OpenFile(*out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		panic(err)
	}
	file.Close()
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
	client := gateway.Client{BaseURL: *base, Key: key, Mode: "chat", HTTP: &http.Client{Transport: tr}}
	rng := rand.New(rand.NewSource(*seed))
	var challenges []fingerprint.Challenge
	for len(challenges) < *train+*holdout {
		challenges = append(challenges, fingerprint.GenerateChallenges(min(41, *train+*holdout-len(challenges)), rng)...)
	}
	rows := make([]record, len(challenges))
	var mu sync.Mutex
	started := time.Now().UTC().Format(time.RFC3339)
	save := func() {
		data := map[string]any{"schema_version": 1, "model": *model, "base_url": *base, "protocol": "chat", "started_at": started, "updated_at": time.Now().UTC().Format(time.RFC3339), "seed": *seed, "training_requested": *train, "holdout_requested": *holdout, "max_tokens": *maxTokens, "temperature": "upstream default", "identity": "endpoint-labelled baseline, not independently verified model identity", "records": rows}
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
	for w := 0; w < *concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				ch := challenges[i]
				m, e := client.CallMessages(context.Background(), gateway.CallRequest{Model: *model, Prompt: ch.Prompt, MaxTokens: *maxTokens, Stream: true, TimeoutSeconds: 180})
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
				mu.Lock()
				rows[i] = row
				save()
				fmt.Printf("sample %d/%d split=%s usable=%v numbers=%d attempts=%d ms=%d error=%s\n", i+1, len(challenges), split, row.Usable, row.Parsed, row.Attempts, row.Ms, row.Error)
				mu.Unlock()
			}
		}()
	}
	for i := range challenges {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
}
