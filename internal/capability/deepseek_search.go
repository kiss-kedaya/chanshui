package capability

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/kiss-kedaya/chanshui/internal/gateway"
)

const deepSeekSearchQuery = "DeepSeek V4 模型 发布"

type deepSeekSearchResponse struct {
	Results []struct {
		RerankScore float64 `json:"rerank_score"`
		Title       string  `json:"title"`
		URL         string  `json:"url"`
		Snippet     string  `json:"snippet"`
		Summary     string  `json:"summary"`
		Content     string  `json:"content"`
		SiteName    string  `json:"site_name"`
	} `json:"results"`
}

// DeepSeekSearch exercises the documented raw Search endpoint
// POST /v1/tools/search. It validates the JSON result structure rather than
// trusting a model's claim that it can search.
func DeepSeekSearch(ctx context.Context, c gateway.Client, timeout int) (Result, error) {
	endpoint, err := deepSeekSearchEndpoint(c.BaseURL)
	if err != nil {
		return Result{}, err
	}
	r := Result{Status: "unknown", Protocol: "deepseek_search", Attempts: 1}
	body, err := json.Marshal(map[string]any{
		"query":        deepSeekSearchQuery,
		"count":        3,
		"need_content": false,
	})
	if err != nil {
		return Result{}, err
	}
	limit := timeout
	if limit <= 0 || limit > 30 {
		limit = 30
	}
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(limit)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Key)
	req.Header.Set("Content-Type", "application/json")
	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		r.Reason = "Search 接口暂时不可用（网络错误）。"
		return r, nil
	}
	defer resp.Body.Close()
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	switch resp.StatusCode {
	case http.StatusNotFound, http.StatusMethodNotAllowed:
		r.Status = "unsupported"
		r.Reason = "当前网关未提供 /v1/tools/search。"
		return r, nil
	case http.StatusUnauthorized, http.StatusForbidden:
		fail := false
		r.Status = "fail"
		r.Pass = &fail
		r.Reason = fmt.Sprintf("Search 接口鉴权失败（HTTP %d）。", resp.StatusCode)
		return r, nil
	case http.StatusTooManyRequests:
		r.Reason = "Search 接口暂时受限（HTTP 429）。"
		return r, nil
	}
	if resp.StatusCode >= 500 {
		r.Reason = fmt.Sprintf("Search 接口暂时不可用（HTTP %d）。", resp.StatusCode)
		return r, nil
	}
	if resp.StatusCode != http.StatusOK {
		fail := false
		r.Status = "fail"
		r.Pass = &fail
		r.Reason = fmt.Sprintf("Search 接口返回 HTTP %d。", resp.StatusCode)
		return r, nil
	}
	if readErr != nil {
		fail := false
		r.Status = "fail"
		r.Pass = &fail
		r.Reason = "Search 接口响应读取失败。"
		return r, nil
	}
	var parsed deepSeekSearchResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		fail := false
		r.Status = "fail"
		r.Pass = &fail
		r.Reason = "Search 接口返回了无法解析的 JSON。"
		return r, nil
	}
	sources := make([]string, 0, 3)
	titles := make([]string, 0, 3)
	valid := 0
	topScore := 0.0
	for _, item := range parsed.Results {
		if item.RerankScore > topScore {
			topScore = item.RerankScore
		}
		if strings.TrimSpace(item.Title) == "" || !validSearchURL(item.URL) {
			continue
		}
		if strings.TrimSpace(item.Snippet) == "" && strings.TrimSpace(item.Summary) == "" {
			continue
		}
		valid++
		if len(sources) < 3 {
			sources = append(sources, item.URL)
			titles = append(titles, item.Title)
		}
	}
	pass := valid > 0
	r.Pass = &pass
	r.Evidence = map[string]any{
		"endpoint":     "/v1/tools/search",
		"query":        deepSeekSearchQuery,
		"result_count": len(parsed.Results),
		"valid_count":  valid,
		"top_score":    topScore,
		"sources":      sources,
		"titles":       titles,
	}
	if pass {
		r.Status = "pass"
		r.Reason = fmt.Sprintf("DeepSeek Search API 返回 %d 条有效结果，含标题、摘要和来源链接。", valid)
	} else {
		r.Status = "fail"
		r.Reason = "Search 接口返回 200，但没有有效的标题/摘要/来源结果。"
	}
	return r, nil
}

func deepSeekSearchEndpoint(base string) (string, error) {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("invalid base url")
	}
	for _, suffix := range []string{"/chat/completions", "/messages", "/responses", "/models", "/tools/search", "/tools/fetch"} {
		if strings.HasSuffix(base, suffix) {
			base = strings.TrimRight(strings.TrimSuffix(base, suffix), "/")
			break
		}
	}
	if strings.HasSuffix(base, "/v1") {
		return base + "/tools/search", nil
	}
	return base + "/v1/tools/search", nil
}

func validSearchURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && u.Host != "" && (u.Scheme == "http" || u.Scheme == "https")
}
