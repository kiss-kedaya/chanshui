package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// Only public read endpoints use validators. Creation/retry responses contain
// private data and must never enter this path or a shared cache.
func replyConditional(w http.ResponseWriter, r *http.Request, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		reply(w, 500, map[string]string{"error": "报告暂时无法读取"})
		return
	}
	hash := sha256.Sum256(data)
	etag := `"` + hex.EncodeToString(hash[:16]) + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	inm := strings.TrimSpace(r.Header.Get("If-None-Match"))
	if inm == etag || inm == "W/"+etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
