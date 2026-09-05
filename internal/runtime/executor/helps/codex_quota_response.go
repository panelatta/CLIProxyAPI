package helps

import (
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// CodexQuotaResponseHeaders applies a quota snapshot while response headers are
// still owned by the executor. Never call it after publishing a StreamResult:
// downstream readers may already be using its headers. Late events remain on
// the websocket stream; HTTP clients receive the bootstrap snapshot only.
func CodexQuotaResponseHeaders(headers http.Header, payload []byte, observedAt time.Time) http.Header {
	if codexQuotaEventKind(payload) != codexQuotaEventRateLimits {
		return headers
	}
	quota := ParseCodexQuotaEventHeaders(payload)
	if len(quota) == 0 {
		return headers
	}
	if headers == nil {
		headers = make(http.Header)
	}
	// An event replaces the snapshot, including windows that became null. Do
	// not leave stale handshake windows or credits alongside the new snapshot.
	for key := range headers {
		if strings.HasPrefix(strings.ToLower(key), "x-codex-") && isCodexQuotaHeaderName(key) {
			delete(headers, key)
		}
	}
	for key, values := range quota {
		headers[key] = values
		const suffix = "-Reset-After-Seconds"
		if !strings.HasSuffix(key, suffix) {
			continue
		}
		resetAtKey := strings.TrimSuffix(key, suffix) + "-Reset-At"
		if quota.Get(resetAtKey) != "" {
			continue
		}
		// Codex CLI reads absolute reset timestamps from HTTP headers.
		seconds, err := strconv.ParseInt(quota.Get(key), 10, 64)
		now := observedAt.Unix()
		if err == nil && seconds >= 0 && now >= 0 && seconds <= math.MaxInt64-now {
			headers.Set(resetAtKey, strconv.FormatInt(now+seconds, 10))
		}
	}
	return headers
}
