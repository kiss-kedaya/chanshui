package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

func (c *Client) ListModels(ctx context.Context) ([]string, error) {
	var last string
	status := 0
	for attempt := 0; attempt <= c.retries(); attempt++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		base := apiBase(c.BaseURL)
		req, e := http.NewRequestWithContext(ctx, "GET", base+"/models", nil)
		if e != nil {
			return nil, e
		}
		req.Header.Set("x-api-key", c.Key)
		req.Header.Set("Authorization", "Bearer "+c.Key)
		req.Header.Set("anthropic-version", "2023-06-01")
		req.Header.Set("User-Agent", c.ua())
		res, e := c.http().Do(req)
		if e != nil {
			last = e.Error()
		} else {
			status = res.StatusCode
			var data struct{ Data []struct{ ID string } }
			e = json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(&data)
			res.Body.Close()
			if status == 200 && e == nil && data.Data != nil {
				ids := make([]string, 0, len(data.Data))
				for _, m := range data.Data {
					ids = append(ids, m.ID)
				}
				return ids, nil
			}
			last = "invalid model list JSON"
		}
		if attempt < c.retries() {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(1<<attempt) * 250 * time.Millisecond):
			}
		}
	}
	return nil, fmt.Errorf("%s", FriendlyError(status, last, c.retries()+1))
}
