package configaccess

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	sdkaccess "github.com/router-for-me/CLIProxyAPI/v7/sdk/access"
)

func TestRegisterUsesAPIKeysFromObjectYAML(t *testing.T) {
	t.Parallel()

	cfg, err := config.ParseConfigBytes([]byte("api-keys:\n  - api-key: sk-a\n    name: Alice laptop\n"))
	if err != nil {
		t.Fatalf("ParseConfigBytes: %v", err)
	}
	Register(&cfg.SDKConfig)
	t.Cleanup(func() { sdkaccess.UnregisterProvider(sdkaccess.AccessProviderTypeConfigAPIKey) })

	manager := sdkaccess.NewManager()
	manager.SetProviders(sdkaccess.RegisteredProviders())
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer sk-a")

	result, authErr := manager.Authenticate(context.Background(), req)
	if authErr != nil {
		t.Fatalf("Authenticate returned auth error: %v", authErr)
	}
	if result == nil {
		t.Fatal("Authenticate returned nil result")
	}
	if result.Principal != "sk-a" {
		t.Fatalf("Principal = %q, want sk-a", result.Principal)
	}
	if result.Principal == "Alice laptop" {
		t.Fatalf("Principal used display name: %q", result.Principal)
	}
}
