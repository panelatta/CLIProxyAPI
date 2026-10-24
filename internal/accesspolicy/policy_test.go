package accesspolicy

import (
	"reflect"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
)

func TestAccessProductsPolicy(t *testing.T) {
	cases := []struct {
		name      string
		allowed   []string
		providers []string
		want      []string
	}{
		{"default", nil, []string{"claude", "codex"}, []string{"claude", "codex"}},
		{"claude", []string{"claude"}, []string{"codex", "claude"}, []string{"claude"}},
		{"multiple", []string{"claude", "gemini"}, []string{"gemini", "codex", "claude"}, []string{"gemini", "claude"}},
		{"blank", []string{" "}, []string{"claude"}, nil},
		{"unknown", []string{"unknown"}, []string{"claude"}, nil},
		{"compatibility group", []string{"openai-compatibility"}, []string{"codex", "openai-compatible-example"}, []string{"openai-compatible-example"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			policy := ForKey(&config.SDKConfig{APIKeyEntries: []config.AccessAPIKeyEntry{{APIKey: "client", AllowedProviders: tc.allowed}}}, "client")
			got, err := policy.Filter(tc.providers)
			if (err != nil) != (tc.want == nil) || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, %v; want %v", got, err, tc.want)
			}
		})
	}
	// Conflicting duplicate definitions must deny rather than granting their union.
	policy := ForKey(&config.SDKConfig{APIKeyEntries: []config.AccessAPIKeyEntry{{APIKey: "client", AllowedProviders: []string{"claude"}}, {APIKey: "client", AllowedProviders: []string{"codex"}}}}, "client")
	if _, err := policy.Filter([]string{"claude", "codex"}); err == nil {
		t.Fatal("duplicate policy broadened access")
	}
}
