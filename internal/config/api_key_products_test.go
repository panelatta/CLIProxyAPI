package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestAccessProductsRoundTrip(t *testing.T) {
	raw := []byte("access:\n  api-keys:\n    - open\n    - api-key: restricted\n      allowed-providers: [' Claude ', codex, claude]\n")
	cfg, err := ParseConfigBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"claude", "codex"}
	if !reflect.DeepEqual(cfg.APIKeyEntries[1].AllowedProviders, want) {
		t.Fatal(cfg.APIKeyEntries)
	}
	cfg.APIKeys = []string{"restricted", "open"}
	cfg.NormalizeAccessAPIKeyEntries()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := SaveConfigPreserveComments(path, cfg, true); err != nil {
		t.Fatal(err)
	}
	saved, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(saved.APIKeyEntries, cfg.APIKeyEntries) {
		t.Fatal("policy lost on save/reorder")
	}
	cloned := cfg.CloneForRuntime()
	cloned.APIKeyEntries[0].AllowedProviders[0] = "gemini"
	if cfg.APIKeyEntries[0].AllowedProviders[0] != "claude" {
		t.Fatal("shared policy slice")
	}
}

func TestAccessProductsDuplicateIntersection(t *testing.T) {
	for _, raw := range []string{
		"access: {api-keys: [{api-key: client, allowed-providers: [claude]}, {api-key: client}]}",
		"access: {api-keys: [{api-key: client}, {api-key: client, allowed-providers: [claude]}]}",
	} {
		cfg, err := ParseConfigBytes([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range cfg.APIKeyEntries {
			if !reflect.DeepEqual(entry.AllowedProviders, []string{"claude"}) {
				t.Fatal(entry)
			}
		}
	}
	cfg, err := ParseConfigBytes([]byte("access: {api-keys: [{api-key: client, allowed-providers: [claude]}, {api-key: client, allowed-providers: [codex]}]}"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range cfg.APIKeyEntries {
		if len(entry.AllowedProviders) == 0 {
			t.Fatal("conflicting restrictions must not allow all")
		}
	}
}
