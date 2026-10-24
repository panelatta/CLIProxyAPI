package configaccess

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	sdkaccess "github.com/router-for-me/CLIProxyAPI/v8/sdk/access"
)

func TestAccessProductsDirectRoutes(t *testing.T) {
	Register(&config.SDKConfig{APIKeys: []string{"client"}, APIKeyEntries: []config.AccessAPIKeyEntry{{APIKey: "client", AllowedProviders: []string{"claude"}}}})
	t.Cleanup(func() { sdkaccess.UnregisterProvider(sdkaccess.AccessProviderTypeConfigAPIKey) })
	manager := sdkaccess.NewManager()
	manager.SetProviders(sdkaccess.RegisteredProviders())
	for _, path := range []string{"/v1/realtime", "/v1/realtime/client_secrets", "/v1/live", "/v1/alpha/search", "/backend-api/codex/alpha/search", "/openai/v1/videos", "/api/provider/codex/chat", "/plugin/unknown"} {
		req := httptest.NewRequest("POST", path, nil)
		req.Header.Set("Authorization", "Bearer client")
		_, err := manager.Authenticate(context.Background(), req)
		if err == nil || err.HTTPStatusCode() != 403 {
			t.Fatalf("%s: %v", path, err)
		}
	}
	for _, header := range []string{"Authorization", "X-Api-Key", "X-Goog-Api-Key"} {
		req := httptest.NewRequest("POST", "/v1/messages", nil)
		req.Header.Set(header, "client")
		if _, err := manager.Authenticate(context.Background(), req); err != nil {
			t.Fatal(err)
		}
	}
}
