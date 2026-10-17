package api

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/kiss-kedaya/chanzhushui/internal/fingerprint"
	"io"
	"log"
	"net/http"
	"regexp"
	"time"
)

const bankStateKey = "modeltrace-bank-v1"
const latestBankCommitURL = "https://api.github.com/repos/xqy2006/ModelTrace/commits/main"

var commitPattern = regexp.MustCompile("^[a-f0-9]{40}$")

func (s *Server) currentBank() *fingerprint.Bank {
	s.bankMu.RLock()
	defer s.bankMu.RUnlock()
	return s.bank
}
func (s *Server) loadCachedBank() {
	raw, e := s.store.LoadRuntimeState(s.ctx, bankStateKey)
	if e != nil {
		return
	}
	var b fingerprint.Bank
	if json.Unmarshal(raw, &b) == nil && fingerprint.MergeLocalReferences(&b) == nil {
		s.bank = &b
	}
}
func (s *Server) publishBank(ctx context.Context, b *fingerprint.Bank) error {
	// Never append local centroids into a snapshot already used by jobs.
	copyRaw, e := json.Marshal(b)
	if e != nil {
		return e
	}
	var next fingerprint.Bank
	if e = json.Unmarshal(copyRaw, &next); e != nil {
		return e
	}
	b = &next
	if e := fingerprint.MergeLocalReferences(b); e != nil {
		return e
	}
	raw, e := json.Marshal(b)
	if e != nil {
		return e
	}
	if e = s.store.SaveRuntimeState(ctx, bankStateKey, raw); e != nil {
		return e
	}
	s.bankMu.Lock()
	s.bank = b
	s.bankMu.Unlock()
	return nil
}
func fetchSyncJSON(ctx context.Context, c *http.Client, url string, out any) error {
	req, e := http.NewRequestWithContext(ctx, "GET", url, nil)
	if e != nil {
		return e
	}
	req.Header.Set("User-Agent", "chanshui-model-bank-sync")
	resp, e := c.Do(req)
	if e != nil {
		return e
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("bank source HTTP %d", resp.StatusCode)
	}
	raw, e := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if e != nil {
		return e
	}
	return json.Unmarshal(raw, out)
}
func (s *Server) syncBank(ctx context.Context, c *http.Client) error {
	var head struct {
		SHA string `json:"sha"`
	}
	if e := fetchSyncJSON(ctx, c, latestBankCommitURL, &head); e != nil {
		return e
	}
	if !commitPattern.MatchString(head.SHA) {
		return fmt.Errorf("invalid source commit")
	}
	current := s.currentBank()
	if current.Source["commit"] == head.SHA {
		return nil
	}
	var remote struct {
		fingerprint.NumericBank
		BuiltAt string `json:"built_at"`
	}
	url := "https://raw.githubusercontent.com/xqy2006/ModelTrace/" + head.SHA + "/data/unified_bank.json"
	if e := fetchSyncJSON(ctx, c, url, &remote); e != nil {
		return e
	}
	next := &fingerprint.Bank{Numeric: remote.NumericBank, Behavioral: current.Behavioral, Source: map[string]string{"repository": "xqy2006/ModelTrace", "commit": head.SHA, "path": "data/unified_bank.json", "built_at": remote.BuiltAt, "synced_at": time.Now().UTC().Format(time.RFC3339)}}
	return s.publishBank(ctx, next)
}
func (s *Server) startBankSync() {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		tr := http.DefaultTransport.(*http.Transport).Clone()
		tr.Proxy = nil
		defer tr.CloseIdleConnections()
		client := &http.Client{Transport: tr, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return fmt.Errorf("sync redirect blocked") }}
		for {
			ctx, cancel := context.WithTimeout(s.ctx, 65*time.Second)
			e := s.syncBank(ctx, client)
			cancel()
			delay := 6 * time.Hour
			if e != nil {
				if s.ctx.Err() != nil {
					return
				}
				log.Print("model bank sync unavailable; retaining last verified bank")
				delay = 5 * time.Minute
			} else {
				log.Print("model bank background sync checked")
			}
			timer := time.NewTimer(delay)
			select {
			case <-s.ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}()
}
