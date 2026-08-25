package watcher

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
)

func TestSetConfigSnapshotPreservesAPIKeyForceModel(t *testing.T) {
	cfg := &config.Config{}
	cfg.APIKeys = []string{"sk-a"}
	cfg.APIKeyEntries = []config.AccessAPIKeyEntry{{APIKey: "sk-a", ForceModel: "gpt-5.6-luna"}}

	w := &Watcher{}
	w.SetConfig(cfg)
	cfg.APIKeyEntries[0].ForceModel = "gpt-5.6-sol"

	if w.oldConfigSnapshot == nil || len(w.oldConfigSnapshot.APIKeyEntries) != 1 {
		t.Fatalf("old config snapshot entries = %#v", w.oldConfigSnapshot)
	}
	if got := w.oldConfigSnapshot.APIKeyEntries[0].ForceModel; got != "gpt-5.6-luna" {
		t.Fatalf("snapshot ForceModel = %q, want gpt-5.6-luna", got)
	}
}
