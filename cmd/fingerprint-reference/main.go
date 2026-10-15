// fingerprint-reference builds a local reference using training rows only,
// then evaluates disjoint holdout triples with the production scorer.
package main

import (
	"chanzhushui/internal/fingerprint"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
)

func readCapture(path string) (fingerprint.LocalReference, []fingerprint.Output) {
	raw, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	var capture struct {
		Model     string `json:"model"`
		BaseURL   string `json:"base_url"`
		StartedAt string `json:"started_at"`
		Records   []struct {
			Split    string `json:"split"`
			Answer   string `json:"answer"`
			Expected int    `json:"expected_count"`
			Usable   bool   `json:"usable"`
		} `json:"records"`
	}
	if err = json.Unmarshal(raw, &capture); err != nil {
		panic(err)
	}
	requestModel := capture.Model
	if prefix, name, ok := strings.Cut(capture.Model, "/"); ok {
		switch prefix {
		case "anthropic", "openai", "moonshotai":
			capture.Model = name
		}
	}
	family, familyName := "", ""
	switch {
	case strings.HasPrefix(capture.Model, "gpt-"):
		family, familyName = "gpt", "GPT"
	case strings.HasPrefix(capture.Model, "claude-"):
		family, familyName = "claude", "Claude"
	case strings.HasPrefix(capture.Model, "kimi-"):
		family, familyName = "kimi", "Kimi"
	case strings.HasPrefix(capture.Model, "deepseek-"):
		family, familyName = "deepseek", "DeepSeek"
	case strings.HasPrefix(capture.Model, "grok-"):
		family, familyName = "grok", "Grok"
	default:
		panic("unsupported local reference family")
	}
	ref := fingerprint.LocalReference{ID: capture.Model, Family: family, FamilyName: familyName, Source: capture.BaseURL, CapturedAt: capture.StartedAt}
	if ref.ID == "deepseek-flash" {
		ref.ID = "deepseek-v4.1-flash"
		ref.Aliases = []string{"deepseek-flash"}
	}
	if requestModel != capture.Model {
		ref.Aliases = append(ref.Aliases, requestModel)
	}
	var holdout []fingerprint.Output
	for _, r := range capture.Records {
		if !r.Usable {
			continue
		}
		if r.Split == "train" {
			ref.Samples = append(ref.Samples, fingerprint.ParseNumbers(r.Answer))
		}
		if r.Split == "holdout" {
			holdout = append(holdout, fingerprint.Output{Text: r.Answer, ExpectedCount: r.Expected})
		}
	}
	return ref, holdout
}

func main() {
	input := flag.String("capture", "", "comma-separated capture JSON files; all are validated jointly")
	output := flag.String("out", "", "optional merged reference output")
	flag.Parse()
	bank, err := fingerprint.LoadBank()
	if err != nil {
		panic(err)
	}
	var refs []fingerprint.LocalReference
	var holdouts [][]fingerprint.Output
	for _, path := range strings.Split(*input, ",") {
		ref, holdout := readCapture(strings.TrimSpace(path))
		if err = fingerprint.AddLocalReference(bank, ref); err != nil {
			panic(err)
		}
		refs = append(refs, ref)
		holdouts = append(holdouts, holdout)
	}
	passed := true
	for index, ref := range refs {
		holdout := holdouts[index]
		matches := 0
		var results []any
		for i := 0; i+3 <= len(holdout); i += 3 {
			a := fingerprint.Attribute(holdout[i:i+3], bank)
			if a.TopModel == ref.ID {
				matches++
			}
			results = append(results, map[string]any{"holdout_group": i/3 + 1, "top_model": a.TopModel, "relative_candidate_weight": a.TopProb, "results": a.Results[:min(3, len(a.Results))]})
		}
		summary := map[string]any{"model": ref.ID, "training_samples": len(ref.Samples), "holdout_samples": len(holdout), "holdout_triples": len(results), "matching_triples": matches, "calibration": "existing bank temperature; not recalibrated across providers or all models", "results": results}
		json.NewEncoder(os.Stdout).Encode(summary)
		if len(ref.Samples) < 36 || len(holdout) < 12 || matches != len(results) {
			passed = false
		}
	}
	if *output != "" {
		if !passed {
			fmt.Fprintln(os.Stderr, "insufficient joint validation; references not written")
			os.Exit(1)
		}
		var merged []fingerprint.LocalReference
		if raw, e := os.ReadFile(*output); e == nil {
			if e = json.Unmarshal(raw, &merged); e != nil {
				panic(e)
			}
		} else if !os.IsNotExist(e) {
			panic(e)
		}
		for _, ref := range refs {
			found := false
			for i := range merged {
				if merged[i].ID == ref.ID {
					merged[i] = ref
					found = true
					break
				}
			}
			if !found {
				merged = append(merged, ref)
			}
		}
		bytes, e := json.MarshalIndent(merged, "", "  ")
		if e != nil {
			panic(e)
		}
		if err = os.WriteFile(*output, bytes, 0600); err != nil {
			panic(err)
		}
	}
}
