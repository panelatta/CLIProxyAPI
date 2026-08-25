package diff

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
)

func TestBuildConfigChangeDetailsAPIKeyForceModelOnly(t *testing.T) {
	oldCfg := &config.Config{}
	oldCfg.APIKeys = []string{"sk-a"}
	oldCfg.APIKeyEntries = []config.AccessAPIKeyEntry{{APIKey: "sk-a", Name: "workstation", ForceModel: "gpt-5.6-luna"}}
	newCfg := oldCfg.CloneForRuntime()
	newCfg.APIKeyEntries[0].ForceModel = "gpt-5.6-sol"

	details := BuildConfigChangeDetails(oldCfg, newCfg)
	for _, detail := range details {
		if detail == "api-keys: force-model policies updated" {
			return
		}
	}
	t.Fatalf("force-model change not reported: %v", details)
}
