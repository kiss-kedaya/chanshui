package blacklist

import (
	"net"
	"net/url"
	"strings"
)

// Entry describes one confirmed site that actively cheats against chanshui.dev
// probes (local replay, forged signatures, static fingerprint tables, or
// selective hijacking). Any detection request targeting a matching domain or
// its subdomains is short-circuited to 0 score without sending upstream traffic.
type Entry struct {
	Domain      string `json:"domain"`
	Reason      string `json:"reason"`
	ReasonEn    string `json:"reason_en"`
	ConfirmedAt string `json:"confirmed_at"`
}

// blockedEntries lists the 3 confirmed domains whose apex and subdomains
// (2nd/3rd-level and deeper) are immediately scored 0 with zero upstream requests.
// Upstream official model downgrades and router.nerver.cc are excluded.
var blockedEntries = []Entry{
	{
		Domain:      "jaaak.cc",
		Reason:      "针对性劫持推理与识图测试，返回零 token 本地预设答案洗白掺水上游",
		ReasonEn:    "Selectively hijacks reasoning and vision probes, returning zero-token local preset answers to disguise a diluted upstream.",
		ConfirmedAt: "2026-11-27",
	},
	{
		Domain:      "a6api.vip",
		Reason:      "数字指纹固定回放同一组 300 整数数组冒充 Opus，底层推理大幅缩水",
		ReasonEn:    "Replays the same fixed 300-integer array to impersonate Opus while actual reasoning performance collapses.",
		ConfirmedAt: "2026-11-27",
	},
	{
		Domain:      "a2e0-103-151-173-91.ngrok-free.app",
		Reason:      "全套本地参考答案回放服务，自述回放身份并伪造签名、指纹切片与识图直出",
		ReasonEn:    "Full local reference-answer replay service that self-identifies as a replay endpoint and fakes signatures, fingerprint slices, and vision answers.",
		ConfirmedAt: "2026-11-27",
	},
}

// NormalizeHost extracts a lowercase hostname without port or trailing dot.
func NormalizeHost(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return ""
	}
	host := u.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
}

// MatchHost checks whether raw (URL or bare host) matches any blocked domain
// or any of its subdomains (2nd/3rd-level and deeper).
func MatchHost(raw string) (Entry, bool) {
	host := NormalizeHost(raw)
	if host == "" {
		return Entry{}, false
	}
	for _, e := range blockedEntries {
		target := strings.ToLower(e.Domain)
		if host == target || strings.HasSuffix(host, "."+target) {
			return e, true
		}
	}
	return Entry{}, false
}
