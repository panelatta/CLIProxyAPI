package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigOpenAICompatibilityResponsesPassthrough(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := []byte(`openai-compatibility:
  - name: openai-platform
    base-url: https://api.openai.com/v1
    responses-passthrough: true
    api-key-entries:
      - api-key: sk-test
    models:
      - name: gpt-5.5
        alias: gpt-dev
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(cfg.OpenAICompatibility) != 1 {
		t.Fatalf("OpenAICompatibility len = %d, want 1", len(cfg.OpenAICompatibility))
	}
	if !cfg.OpenAICompatibility[0].ResponsesPassthrough {
		t.Fatalf("ResponsesPassthrough = false, want true")
	}
}
