package config

import "testing"

func TestV8UsageHistoryConfiguration(t *testing.T) {
	cfg, err := ParseConfigBytes([]byte("config-version: 8\nobservability:\n  usage:\n    usage-statistics-enabled: true\n    usage-history-file: logs/history.json\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.UsageStatisticsEnabled || cfg.UsageHistoryFile != "logs/history.json" {
		t.Fatal("usage history config was not decoded")
	}
}
