package helps

import (
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

func TestStripAbliterationSearchCacheControl(t *testing.T) {
	const body = `{"model":"abliterated-model-large-v2","tools":[{"type":"web_search_20250305","name":"web_search","max_uses":1,"cache_control":{"type":"ephemeral"}},{"type":"custom","name":"web_search","input_schema":{"type":"object"},"cache_control":{"type":"ephemeral"}},{"type":"web_search","name":"web_search","cache_control":{"type":"ephemeral","ttl":"1h"}}],"system":[{"type":"text","text":"system","cache_control":{"type":"ephemeral"}}],"messages":[{"role":"user","content":[{"type":"text","text":"search","cache_control":{"type":"ephemeral"}}]}]}`
	for _, tt := range []struct {
		name, endpoint, model string
		strip                 bool
	}{
		{"target", "https://api.abliteration.ai/v1/messages?beta=true", "abliterated-model-large-v2", true},
		{"host casing and port", "https://API.ABLITERATION.AI:443/v1/messages", "abliterated-model-large-v2", true},
		{"other model", "https://api.abliteration.ai/v1/messages", "abliterated-model-large-v3", false},
		{"other upstream", "https://api.anthropic.com/v1/messages", "abliterated-model-large-v2", false},
		{"host suffix", "https://api.abliteration.ai.example.com/v1/messages", "abliterated-model-large-v2", false},
		{"host in path", "https://example.com/api.abliteration.ai", "abliterated-model-large-v2", false},
		{"invalid URL", "://invalid", "abliterated-model-large-v2", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			input := strings.Replace(body, "abliterated-model-large-v2", tt.model, 1)
			out := StripAbliterationSearchCacheControl([]byte(input), tt.endpoint)
			if !tt.strip {
				if string(out) != input {
					t.Fatalf("unrelated request changed: %s", out)
				}
				return
			}
			for _, path := range []string{"tools.0.cache_control", "tools.2.cache_control"} {
				if gjson.GetBytes(out, path).Exists() {
					t.Fatalf("unsupported marker remains at %s: %s", path, out)
				}
			}
			for _, path := range []string{"tools.1", "system", "messages", "tools.0.max_uses", "tools.0.type", "tools.0.name"} {
				if gjson.GetBytes(out, path).Raw != gjson.Get(input, path).Raw {
					t.Fatalf("unrelated field changed at %s: %s", path, out)
				}
			}
			if again := StripAbliterationSearchCacheControl(out, tt.endpoint); string(again) != string(out) {
				t.Fatal("cleanup is not idempotent")
			}
		})
	}
}
