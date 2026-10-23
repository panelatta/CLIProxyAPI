package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDisabledAccessKeySaveAndReorder(t *testing.T) {
	raw := []byte("access:\n  api-keys:\n    - active-client\n    - api-key: paused-client\n      disabled: true\n")
	cfg, err := ParseConfigBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.APIKeyEntries[1].Disabled || len(cfg.APIKeys) != 2 {
		t.Fatal("disabled key was dropped or lost its state")
	}
	cfg.APIKeys = []string{"paused-client", "active-client"}
	cfg.NormalizeAccessAPIKeyEntries()
	if !cfg.APIKeyEntries[0].Disabled || cfg.APIKeyEntries[1].Disabled {
		t.Fatal("reordering changed the wrong key's state")
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err = SaveConfigPreserveComments(path, cfg, true); err != nil {
		t.Fatal(err)
	}
	reloaded, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reloaded.APIKeyEntries, cfg.APIKeyEntries) {
		t.Fatal("saving lost disabled state without optional name/model metadata")
	}
}
