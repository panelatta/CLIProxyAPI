package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeAPIKeyEntriesConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func requireAccessAPIKeys(t *testing.T, cfg *Config, keys []string, names []string) {
	t.Helper()
	if !reflect.DeepEqual(cfg.APIKeys, keys) {
		t.Fatalf("APIKeys = %#v, want %#v", cfg.APIKeys, keys)
	}
	if got, want := len(cfg.APIKeyEntries), len(keys); got != want {
		t.Fatalf("APIKeyEntries len = %d, want %d: %#v", got, want, cfg.APIKeyEntries)
	}
	for i := range keys {
		if cfg.APIKeyEntries[i].APIKey != keys[i] {
			t.Fatalf("APIKeyEntries[%d].APIKey = %q, want %q", i, cfg.APIKeyEntries[i].APIKey, keys[i])
		}
		if cfg.APIKeyEntries[i].Name != names[i] {
			t.Fatalf("APIKeyEntries[%d].Name = %q, want %q", i, cfg.APIKeyEntries[i].Name, names[i])
		}
	}
}

func TestAPIKeyEntriesLoadConfigOptional(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		yaml  string
		keys  []string
		names []string
	}{
		{
			name:  "strings",
			yaml:  "api-keys:\n  - sk-a\n  - sk-b\n",
			keys:  []string{"sk-a", "sk-b"},
			names: []string{"", ""},
		},
		{
			name:  "objects",
			yaml:  "api-keys:\n  - api-key: sk-a\n    name: Alice laptop\n  - api-key: sk-b\n    name: CI runner\n",
			keys:  []string{"sk-a", "sk-b"},
			names: []string{"Alice laptop", "CI runner"},
		},
		{
			name:  "mixed",
			yaml:  "api-keys:\n  - sk-a\n  - api-key: sk-b\n    name: CI runner\n",
			keys:  []string{"sk-a", "sk-b"},
			names: []string{"", "CI runner"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg, err := LoadConfigOptional(writeAPIKeyEntriesConfig(t, tt.yaml), false)
			if err != nil {
				t.Fatalf("LoadConfigOptional: %v", err)
			}
			requireAccessAPIKeys(t, cfg, tt.keys, tt.names)
		})
	}
}

func TestAPIKeyEntriesParseConfigBytesMatchesLoad(t *testing.T) {
	t.Parallel()

	data := []byte("api-keys:\n  - sk-a\n  - api-key: sk-b\n    name: CI runner\n")
	parsed, err := ParseConfigBytes(data)
	if err != nil {
		t.Fatalf("ParseConfigBytes: %v", err)
	}
	loaded, err := LoadConfigOptional(writeAPIKeyEntriesConfig(t, string(data)), false)
	if err != nil {
		t.Fatalf("LoadConfigOptional: %v", err)
	}

	if !reflect.DeepEqual(parsed.APIKeys, loaded.APIKeys) {
		t.Fatalf("parsed keys = %#v, loaded keys = %#v", parsed.APIKeys, loaded.APIKeys)
	}
	if !reflect.DeepEqual(parsed.APIKeyEntries, loaded.APIKeyEntries) {
		t.Fatalf("parsed entries = %#v, loaded entries = %#v", parsed.APIKeyEntries, loaded.APIKeyEntries)
	}
}

func TestSaveConfigPreserveCommentsAPIKeyEntriesStringYAMLWhenNoNames(t *testing.T) {
	t.Parallel()

	path := writeAPIKeyEntriesConfig(t, "# keep me\napi-keys:\n  - sk-old\n")
	cfg := &Config{SDKConfig: SDKConfig{
		APIKeys: []string{"sk-a", "sk-b"},
		APIKeyEntries: []AccessAPIKeyEntry{
			{APIKey: "sk-a"},
			{APIKey: "sk-b"},
		},
	}}
	if err := SaveConfigPreserveComments(path, cfg); err != nil {
		t.Fatalf("SaveConfigPreserveComments: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	out := string(data)
	if !strings.Contains(out, "# keep me") {
		t.Fatalf("saved config lost comment: %s", out)
	}
	if strings.Contains(out, "api-key:") {
		t.Fatalf("saved config used object api-key entries without names: %s", out)
	}
	if !strings.Contains(out, "- sk-a") || !strings.Contains(out, "- sk-b") {
		t.Fatalf("saved config missing string keys: %s", out)
	}
}

func TestSaveConfigPreserveCommentsAPIKeyEntriesObjectYAMLWhenNamed(t *testing.T) {
	t.Parallel()

	path := writeAPIKeyEntriesConfig(t, "# keep me\napi-keys:\n  - sk-old\n")
	cfg := &Config{SDKConfig: SDKConfig{
		APIKeys: []string{"sk-a", "sk-b"},
		APIKeyEntries: []AccessAPIKeyEntry{
			{APIKey: "sk-a", Name: "Alice laptop"},
			{APIKey: "sk-b"},
		},
	}}
	if err := SaveConfigPreserveComments(path, cfg); err != nil {
		t.Fatalf("SaveConfigPreserveComments: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	out := string(data)
	if !strings.Contains(out, "# keep me") || !strings.Contains(out, "api-key: sk-a") || !strings.Contains(out, "name: Alice laptop") || !strings.Contains(out, "api-key: sk-b") {
		t.Fatalf("saved config did not preserve comments and named entries: %s", out)
	}

	reloaded, err := LoadConfigOptional(path, false)
	if err != nil {
		t.Fatalf("reload saved config: %v", err)
	}
	requireAccessAPIKeys(t, reloaded, []string{"sk-a", "sk-b"}, []string{"Alice laptop", ""})
}
