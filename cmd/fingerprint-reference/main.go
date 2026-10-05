// fingerprint-reference builds a local reference using training rows only,
// then evaluates disjoint holdout triples with the production scorer.
package main

import (
	"chanzhushui/internal/fingerprint"
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

func main() {
	input := flag.String("capture", "", "capture JSON")
	output := flag.String("out", "", "optional reference output")
	flag.Parse()
	raw, err := os.ReadFile(*input)
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
	ref := fingerprint.LocalReference{ID: capture.Model, Family: "deepseek", FamilyName: "DeepSeek", Source: capture.BaseURL, CapturedAt: capture.StartedAt}
	if ref.ID == "deepseek-flash" {
		ref.Aliases = []string{"deepseek-v4.1-flash"}
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
	bank, err := fingerprint.LoadBank()
	if err != nil {
		panic(err)
	}
	if err = fingerprint.AddLocalReference(bank, ref); err != nil {
		panic(err)
	}
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
	if *output != "" {
		if len(ref.Samples) < 36 || len(holdout) < 12 || matches != len(results) {
			fmt.Fprintln(os.Stderr, "insufficient independent validation; reference not written")
			os.Exit(1)
		}
		bytes, e := json.MarshalIndent([]fingerprint.LocalReference{ref}, "", "  ")
		if e != nil {
			panic(e)
		}
		if err = os.WriteFile(*output, bytes, 0600); err != nil {
			panic(err)
		}
	}
}
