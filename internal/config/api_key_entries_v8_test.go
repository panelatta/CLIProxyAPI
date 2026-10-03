package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestNamedAccessKeysSurviveV8MigrationAndSave(t *testing.T) {
	legacy := []byte(`api-keys:
  - api-key: client-one
    name: Laptop
    force-model: pinned-model
  - client-two
codex-api-key:
  - api-key: upstream-secret
    base-url: https://example.com/v1
`)
	before, err := ParseConfigBytes(legacy)
	if err != nil {
		t.Fatal(err)
	}
	migrated, _, err := NormalizeConfigLayout(legacy, true)
	if err != nil {
		t.Fatal(err)
	}
	if err = ValidateV8Config(migrated); err != nil {
		t.Fatal(err)
	}
	after, err := ParseConfigBytes(migrated)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before.APIKeyEntries, after.APIKeyEntries) || !reflect.DeepEqual(before.CodexKey, after.CodexKey) {
		t.Fatal("migration lost client metadata or upstream credentials")
	}
	var direct Config
	if err = yaml.Unmarshal(migrated, &direct); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before.APIKeyEntries, direct.APIKeyEntries) {
		t.Fatal("direct YAML decode lost metadata")
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err = os.WriteFile(path, migrated, 0600); err != nil {
		t.Fatal(err)
	}
	after.RequestRetry = 4
	if err = SaveConfigPreserveComments(path, after, true); err != nil {
		t.Fatal(err)
	}
	saved, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before.APIKeyEntries, saved.APIKeyEntries) || !reflect.DeepEqual(before.CodexKey, saved.CodexKey) {
		t.Fatal("save lost client metadata or upstream credentials")
	}
	if saved.RequestRetry != 4 {
		t.Fatal("save did not persist changed setting")
	}
}

func TestNamedAccessKeysV8PresenceOverridesLegacy(t *testing.T) {
	cfg, err := ParseConfigBytes([]byte("api-keys: [legacy]\naccess:\n  api-keys:\n    - api-key: current\n      name: Device\n      force-model: pinned\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := []AccessAPIKeyEntry{{APIKey: "current", Name: "Device", ForceModel: "pinned"}}
	if !reflect.DeepEqual(cfg.APIKeyEntries, want) {
		t.Fatal("v8 client key precedence or metadata incorrect")
	}
}
