package usage

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	coreusage "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/usage"
)

func TestQuotaHistoryCaptureAndRestore(t *testing.T) {
	SetStatisticsEnabled(true)
	now := time.Date(2026, 10, 10, 10, 0, 0, 0, time.UTC)
	headers := http.Header{
		"X-Codex-Primary-Used-Percent":        {"23.5"},
		"X-Codex-Primary-Window-Minutes":      {"10080"},
		"X-Codex-Primary-Reset-After-Seconds": {"600"},
		"Authorization":                       {"must-not-be-persisted"},
	}
	stats := NewRequestStatistics()
	path := filepath.Join(t.TempDir(), "history.json")
	stop, err := stats.StartPersistence(path)
	if err != nil {
		t.Fatal(err)
	}
	stats.Record(context.Background(), coreusage.Record{
		APIKey: "test-key", Model: "test-model", AuthIndex: "account-a", AuthType: "oauth", Provider: "codex",
		RequestedAt: now, Latency: time.Second, ResponseHeaders: headers,
	})
	stop()
	stop()
	restored := NewRequestStatistics()
	stopRestored, err := restored.StartPersistence(path)
	if err != nil {
		t.Fatal(err)
	}
	defer stopRestored()
	snapshot := restored.Snapshot()
	detail := snapshot.APIs["test-key"].Models["test-model"].Details[0]
	if snapshot.TotalRequests != 1 || detail.AuthType != "oauth" || len(detail.QuotaWindows) != 1 {
		t.Fatalf("unexpected restored record: %+v", detail)
	}
	window := detail.QuotaWindows[0]
	if window.UsedPercent != 23.5 || window.WindowSeconds != 604800 || !window.ResetAt.Equal(now.Add(601*time.Second)) {
		t.Fatalf("window: %+v", window)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("unsafe history mode: %v", info.Mode())
	}
	if result := restored.MergeSnapshot(snapshot); result.Added != 0 {
		t.Fatal("restore must remain deduplicated")
	}
	detail.QuotaWindows[0].UsedPercent = 99
	if restored.Snapshot().APIs["test-key"].Models["test-model"].Details[0].QuotaWindows[0].UsedPercent != 23.5 {
		t.Fatal("snapshot quota windows alias the statistics store")
	}
}

func TestQuotaWindowsRejectMissingAndInvalidData(t *testing.T) {
	for _, value := range []string{"NaN", "+Inf", "-1", "101", "invalid"} {
		windows := ParseQuotaWindows("codex", http.Header{
			"X-Codex-Primary-Used-Percent": {value}, "X-Codex-Primary-Window-Minutes": {"300"}, "X-Codex-Primary-Reset-At": {"1792000000"},
		}, time.Now())
		if len(windows) != 0 {
			t.Fatalf("accepted %q", value)
		}
	}
	if len(ParseQuotaWindows("codex", http.Header{"X-Codex-Primary-Used-Percent": {"20"}}, time.Now())) != 0 {
		t.Fatal("missing window must remain unknown")
	}
	windows := ParseQuotaWindows("claude", http.Header{"Anthropic-Ratelimit-Unified-5h-Utilization": {"0.25"}, "Anthropic-Ratelimit-Unified-5h-Reset": {"1792000000"}}, time.Now())
	if len(windows) != 1 || windows[0].UsedPercent != 25 || windows[0].WindowSeconds != 18000 {
		t.Fatalf("claude: %+v", windows)
	}
	if len(ParseQuotaWindows("other", http.Header{"X-Codex-Primary-Used-Percent": {"20"}}, time.Now())) != 0 {
		t.Fatal("unsupported provider")
	}
}

func TestCorruptHistoryIsPreserved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	original := []byte(`{"version":`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRequestStatistics().StartPersistence(path); err == nil {
		t.Fatal("expected decode failure")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(original) {
		t.Fatal("corrupt history was overwritten")
	}
}

func TestAPIKeyRequestsDoNotRecordOAuthQuota(t *testing.T) {
	stats := NewRequestStatistics()
	stats.Record(context.Background(), coreusage.Record{APIKey: "test", Model: "model", AuthType: "api_key", Provider: "codex", ResponseHeaders: http.Header{
		"X-Codex-Primary-Used-Percent": {"20"}, "X-Codex-Primary-Window-Minutes": {"300"}, "X-Codex-Primary-Reset-At": {"1792000000"},
	}})
	if len(stats.Snapshot().APIs["test"].Models["model"].Details[0].QuotaWindows) != 0 {
		t.Fatal("API key must not be counted as OAuth")
	}
}
