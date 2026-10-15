package api

import (
	"bytes"
	"chanzhushui/internal/fingerprint"
	"chanzhushui/internal/history"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type syncTransport func(*http.Request) (*http.Response, error)

func TestUpstreamOnlyCachedBankKeepsLocalReference(t *testing.T) {
	db, err := history.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	raw, err := os.ReadFile("../fingerprint/bank.generated.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.SaveRuntimeState(context.Background(), bankStateKey, raw); err != nil {
		t.Fatal(err)
	}
	s, err := New(db, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !s.supported("deepseek-flash") || !s.supported("deepseek-v4.1-flash") {
		t.Fatal("cached upstream bank erased local models")
	}
	var upstream fingerprint.Bank
	if err = json.Unmarshal(raw, &upstream); err != nil {
		t.Fatal(err)
	}
	count := len(upstream.Numeric.Models)
	if err = s.publishBank(context.Background(), &upstream); err != nil {
		t.Fatal(err)
	}
	if len(upstream.Numeric.Models) != count {
		t.Fatal("publish mutated caller snapshot")
	}
	if s.currentBank().NumericModel("deepseek-v4.1-flash") == nil {
		t.Fatal("published bank erased local models")
	}
	for _, model := range []string{"grok-4.6", "grok-4.7", "gpt-6.1-sol", "openai/gpt-6.1-sol", "claude-fable-5", "anthropic/claude-fable-5"} {
		if !s.supported(model) || s.currentBank().NumericModel(model) == nil {
			t.Fatalf("published bank erased %s", model)
		}
	}
}

func (f syncTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestAutomaticSyncValidatedSwapAndPersistence(t *testing.T) {
	db, e := history.Open(filepath.Join(t.TempDir(), "sync.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	s, e := New(db, Options{})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	old := s.currentBank()
	sha := strings.Repeat("a", 40)
	count := 0
	client := &http.Client{Transport: syncTransport(func(r *http.Request) (*http.Response, error) {
		count++
		var v any
		if strings.Contains(r.URL.Path, "/commits/") {
			v = map[string]string{"sha": sha}
		} else {
			if !strings.Contains(r.URL.Path, sha+"/data/unified_bank.json") {
				t.Error(r.URL.Path)
			}
			v = old.Numeric
		}
		raw, _ := json.Marshal(v)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(raw)), Header: http.Header{}}, nil
	})}
	if e = s.syncBank(context.Background(), client); e != nil {
		t.Fatal(e)
	}
	if s.currentBank() == old || old.Source["commit"] == sha || s.currentBank().Source["commit"] != sha {
		t.Fatal("bank snapshot mutation")
	}
	if count != 2 {
		t.Fatal(count)
	}
	if e = s.syncBank(context.Background(), client); e != nil || count != 3 {
		t.Fatal("unchanged commit redownloaded", e, count)
	}
	raw, _ := json.Marshal(s.currentBank())
	var invalid fingerprint.Bank
	json.Unmarshal(raw, &invalid)
	invalid.Numeric.Robust.OrderedBlocks.Centroids[0] = []float64{1}
	if e = s.publishBank(context.Background(), &invalid); e == nil {
		t.Fatal("invalid matrix accepted")
	}
	if s.currentBank().Source["commit"] != sha {
		t.Fatal("bad update replaced verified bank")
	}
	next, e := New(db, Options{})
	if e != nil {
		t.Fatal(e)
	}
	defer next.Close()
	if next.currentBank().Source["commit"] != sha {
		t.Fatal("verified cache not loaded")
	}
	if next.currentBank().NumericModel("deepseek-v4.1-flash") == nil {
		t.Fatal("local reference lost after restart")
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				_ = s.currentBank()
				_ = s.supported("claude-sonnet-5-5")
			}
		}()
	}
	if e = s.publishBank(context.Background(), old); e != nil {
		t.Fatal(e)
	}
	wg.Wait()
}
func TestFeedbackStoredPrivatelyAndRateLimited(t *testing.T) {
	s, _, _, _ := testSetup(t)
	payload := map[string]string{"kind": "method", "contact": "fixture-contact@example.invalid", "message": "建议补充模型测试对照样本。"}
	w := post(t, s, "/api/feedback", payload)
	if w.Code != 201 || strings.Contains(w.Body.String(), payload["contact"]) {
		t.Fatal(w.Code, w.Body.String())
	}
	rows, e := s.store.ListFeedback(context.Background())
	if e != nil || len(rows) != 1 || rows[0].Contact != payload["contact"] {
		t.Fatal(rows, e)
	}
	if r := getJSON(t, s.Handler(), "/api/feedback"); r.Code != 405 {
		t.Fatal("private feedback exposed", r.Code)
	}
	for i := 0; i < 4; i++ {
		if r := post(t, s, "/api/feedback", payload); r.Code != 201 {
			t.Fatal(r.Code)
		}
	}
	if r := post(t, s, "/api/feedback", payload); r.Code != 429 {
		t.Fatal("rate limit absent", r.Code)
	}
	payload["message"] = "密钥 sk-should-not-be-persisted"
	if r := post(t, s, "/api/feedback", payload); r.Code != 400 {
		t.Fatal("secret accepted")
	}
}
