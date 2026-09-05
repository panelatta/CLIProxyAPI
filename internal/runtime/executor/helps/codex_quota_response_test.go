package helps

import (
	"net/http"
	"testing"
	"time"
)

func TestCodexQuotaResponseHeadersReplacesSnapshotAndConvertsReset(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-Codex-Secondary-Used-Percent", "99")
	headers.Set("X-Codex-Credits-Balance", "10")
	headers.Set("X-Request-Id", "keep")
	payload := []byte(`{"type":"codex.rate_limits","rate_limits":{"primary":{"used_percent":11,"window_minutes":10080,"reset_after_seconds":60},"secondary":null}}`)
	got := CodexQuotaResponseHeaders(headers, payload, time.Unix(1700000000, 0))
	for key, want := range map[string]string{
		"X-Codex-Primary-Used-Percent":   "11",
		"X-Codex-Primary-Window-Minutes": "10080",
		"X-Codex-Primary-Reset-At":       "1700000060",
		"X-Codex-Secondary-Used-Percent": "",
		"X-Codex-Credits-Balance":        "",
		"X-Request-Id":                   "keep",
	} {
		if got.Get(key) != want {
			t.Errorf("%s = %q, want %q", key, got.Get(key), want)
		}
	}
}

func TestCodexQuotaResponseHeadersPreservesAbsoluteReset(t *testing.T) {
	payload := []byte(`{"type":"codex.rate_limits","rate_limits":{"primary":{"used_percent":0,"window_minutes":300,"reset_after_seconds":60,"reset_at":1800000000}},"credits":{"has_credits":false,"unlimited":false,"balance":"0"}}`)
	got := CodexQuotaResponseHeaders(nil, payload, time.Unix(1700000000, 0))
	if got.Get("X-Codex-Primary-Reset-At") != "1800000000" || got.Get("X-Codex-Credits-Has-Credits") != "false" {
		t.Fatalf("unexpected snapshot: %v", got)
	}
}

func TestCodexQuotaResponseHeadersIgnoresNonSnapshots(t *testing.T) {
	for _, payload := range []string{`{"type":"response.created"}`, `{"type":"codex.rate_limits"}`, `{"type":"error","headers":{"x-codex-primary-used-percent":"99"}}`} {
		got := CodexQuotaResponseHeaders(http.Header{"X-Codex-Primary-Used-Percent": {"11"}}, []byte(payload), time.Unix(1700000000, 0))
		if got.Get("X-Codex-Primary-Used-Percent") != "11" {
			t.Fatalf("non-snapshot replaced headers: %v", got)
		}
	}
}

func TestCodexQuotaResponseHeadersKeepsAccountQuotaWhenAdditionalLimitsExist(t *testing.T) {
	payload := []byte(`{"type":"codex.rate_limits","rate_limits":{"primary":{"used_percent":11,"window_minutes":10080,"reset_at":1788847375}},"additional_rate_limits":{"GPT-5.3-Codex-Spark":{"primary":{"used_percent":0,"window_minutes":300,"reset_at":1788847375}}},"code_review_rate_limits":{"primary":{"used_percent":4,"window_minutes":300,"reset_at":1788847375}}}`)
	got := CodexQuotaResponseHeaders(nil, payload, time.Unix(1700000000, 0))
	if len(got) != 3 || got.Get("X-Codex-Primary-Used-Percent") != "11" || got.Get("X-Codex-Primary-Window-Minutes") != "10080" {
		t.Fatalf("account quota was polluted by an additional limit family: %v", got)
	}
}
