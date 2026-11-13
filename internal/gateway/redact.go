package gateway

import "regexp"

// Redact hides upstream hosts, IPs and bare domains in error text so reports
// never reveal someone else's gateway address.
var (
	urlHost  = regexp.MustCompile(`(?i)\b(https?://)[^\s/"'<>]+`)
	bareHost = regexp.MustCompile(`(?i)\b(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+(?:com|net|org|io|ai|dev|xyz|cn|co|me|app|cloud|top|cc|info|site|online|tech|pro|link|live|fun|space|store|shop|vip|work|asia|us|uk|de|jp|hk|sg|ru|fr|nl|ca|au|in|br|eu|moe|pw|tv)\b`)
	ipv4Host = regexp.MustCompile(`\b\d{1,3}(?:\.\d{1,3}){3}\b`)
)

// Redact replaces every host with *****.*** and keeps scheme, path and reason.
func Redact(text string) string {
	text = urlHost.ReplaceAllString(text, "${1}*****.***")
	text = bareHost.ReplaceAllString(text, "*****.***")
	return ipv4Host.ReplaceAllString(text, "*****.***")
}
