package usage

import (
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// QuotaWindow is an upstream observation, never a per-request billing amount.
type QuotaWindow struct {
	ID            string    `json:"id"`
	UsedPercent   float64   `json:"used_percent"`
	WindowSeconds int64     `json:"window_seconds"`
	ResetAt       time.Time `json:"reset_at"`
}

type OAuthAccount struct {
	AuthIndex  string        `json:"auth_index"`
	Provider   string        `json:"provider"`
	Name       string        `json:"name"`
	ObservedAt time.Time     `json:"observed_at"`
	Windows    []QuotaWindow `json:"windows"`
}

// ParseQuotaWindows only accepts supported subscription windows. Credits and
// request/token rate limits must not be presented as subscription percentages.
func ParseQuotaWindows(provider string, headers http.Header, observedAt time.Time) []QuotaWindow {
	values := make(map[string]string, len(headers))
	for key, value := range headers {
		if len(value) > 0 {
			values[strings.ToLower(key)] = strings.TrimSpace(value[0])
		}
	}
	var windows []QuotaWindow
	for key, raw := range values {
		var id, prefix string
		var seconds int64
		multiplier := 1.0
		switch strings.ToLower(provider) {
		case "codex":
			if !strings.HasPrefix(key, "x-codex-") || !strings.HasSuffix(key, "-used-percent") {
				continue
			}
			prefix = strings.TrimSuffix(key, "used-percent")
			id = strings.TrimSuffix(strings.TrimPrefix(prefix, "x-codex-"), "-")
			minutes, err := strconv.ParseInt(values[prefix+"window-minutes"], 10, 64)
			if err != nil || minutes <= 0 || minutes > 366*24*60 {
				continue
			}
			seconds = minutes * 60
		case "claude":
			if !strings.HasPrefix(key, "anthropic-ratelimit-unified-") || !strings.HasSuffix(key, "-utilization") {
				continue
			}
			prefix = strings.TrimSuffix(key, "utilization")
			id = strings.TrimSuffix(strings.TrimPrefix(prefix, "anthropic-ratelimit-unified-"), "-")
			switch {
			case id == "5h":
				seconds = 5 * 3600
			case id == "7d" || strings.HasPrefix(id, "7d-"):
				seconds = 7 * 24 * 3600
			default:
				continue
			}
			multiplier = 100
		default:
			continue
		}
		used, err := strconv.ParseFloat(raw, 64)
		used *= multiplier
		if err != nil || math.IsNaN(used) || math.IsInf(used, 0) || used < 0 || used > 100 || len(id) > 256 {
			continue
		}
		resetRaw := values[prefix+"reset-at"]
		if provider == "claude" {
			resetRaw = values[prefix+"reset"]
		}
		reset, _ := strconv.ParseInt(resetRaw, 10, 64)
		if reset <= 0 && !observedAt.IsZero() {
			if after, errAfter := strconv.ParseInt(values[prefix+"reset-after-seconds"], 10, 64); errAfter == nil && after >= 0 && after <= 366*24*3600 {
				reset = observedAt.Unix() + after
			}
		}
		if reset <= 0 || reset > 253402300799 {
			continue
		}
		windows = append(windows, QuotaWindow{ID: id, UsedPercent: used, WindowSeconds: seconds, ResetAt: time.Unix(reset, 0).UTC()})
	}
	sort.Slice(windows, func(i, j int) bool { return windows[i].ID < windows[j].ID })
	return windows
}
