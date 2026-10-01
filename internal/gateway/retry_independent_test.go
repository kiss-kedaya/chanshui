package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestConcurrentRequestsEachReceiveThreeRetries(t *testing.T) {
	var mu sync.Mutex
	calls := map[string]int{}
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Messages []struct{ Content string } }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		id := body.Messages[0].Content
		mu.Lock()
		calls[id]++
		n := calls[id]
		mu.Unlock()
		if n <= 3 {
			w.WriteHeader(502)
			return
		}
		fmt.Fprint(w, `{"content":[{"type":"text","text":"OK"}],"usage":{"input_tokens":1}}`)
	}))
	defer up.Close()
	c := Client{BaseURL: up.URL, RetryDelay: time.Millisecond}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			m, err := c.CallMessages(context.Background(), CallRequest{Model: "claude-fixture", Prompt: fmt.Sprintf("probe-%d", i)})
			if err != nil || m.Err != "" || m.Answer != "OK" || m.Attempts != 4 {
				t.Errorf("probe %d: %+v %v", i, m, err)
			}
		}(i)
	}
	wg.Wait()
	for id, n := range calls {
		if n != 4 {
			t.Errorf("%s: %d requests", id, n)
		}
	}
	if len(calls) != 12 {
		t.Fatal("missing concurrent probes")
	}
}

func TestInvalidSuccessfulResponseSharesRequestRetryBudget(t *testing.T) {
	calls := 0
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		fmt.Fprint(w, `{"content":[],"usage":{"input_tokens":1}}`)
	}))
	defer up.Close()
	c := Client{BaseURL: up.URL, RetryDelay: time.Millisecond}
	m, err := c.CallMessages(context.Background(), CallRequest{Model: "claude-fixture", Validate: func(m Metrics) error { return fmt.Errorf("empty response") }})
	if err != nil || calls != 4 || m.Attempts != 4 || m.Err == "" {
		t.Fatalf("calls=%d m=%+v err=%v", calls, m, err)
	}
}
