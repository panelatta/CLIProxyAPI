package configaccess

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	sdkaccess "github.com/router-for-me/CLIProxyAPI/v8/sdk/access"
)

func TestDisabledKeysAuthenticationAndReload(t *testing.T) {
	t.Cleanup(func() { sdkaccess.UnregisterProvider(sdkaccess.AccessProviderTypeConfigAPIKey) })
	cfg, err := config.ParseConfigBytes([]byte(`access:
  api-keys:
    - api-key: disabled-client
      name: Laptop
      disabled: true
    - active-client
`))
	if err != nil {
		t.Fatal(err)
	}
	manager := sdkaccess.NewManager()
	reload := func() {
		Register(&cfg.SDKConfig)
		manager.SetProviders(sdkaccess.RegisteredProviders())
	}
	assertAuth := func(key, source string, allowed bool) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		switch source {
		case "key", "auth_token":
			query := req.URL.Query()
			query.Set(source, key)
			req.URL.RawQuery = query.Encode()
		case "Authorization":
			req.Header.Set(source, "Bearer "+key)
		default:
			req.Header.Set(source, key)
		}
		result, authErr := manager.Authenticate(context.Background(), req)
		if allowed {
			if authErr != nil || result == nil || result.Principal != key {
				t.Fatalf("active key via %s rejected: %v", source, authErr)
			}
		} else if authErr == nil || result != nil {
			t.Fatalf("disabled or missing key via %s accepted", source)
		}
	}
	reload()
	for _, source := range []string{"Authorization", "X-Api-Key", "X-Goog-Api-Key", "key", "auth_token"} {
		assertAuth("disabled-client", source, false)
		assertAuth("active-client", source, true)
	}

	cfg.APIKeyEntries[1].Disabled = true
	reload()
	assertAuth("active-client", "Authorization", false)
	assertAuth("", "", false)
	assertAuth("unknown-client", "Authorization", false)

	cfg.APIKeyEntries[0].Disabled = false
	reload()
	assertAuth("disabled-client", "Authorization", true)
	assertAuth("active-client", "Authorization", false)
}

func TestDisabledDuplicateKeyCannotAuthenticate(t *testing.T) {
	t.Cleanup(func() { sdkaccess.UnregisterProvider(sdkaccess.AccessProviderTypeConfigAPIKey) })
	cfg := &config.SDKConfig{
		APIKeys: []string{"duplicate", "duplicate"},
		APIKeyEntries: []config.AccessAPIKeyEntry{
			{APIKey: "duplicate"},
			{APIKey: " duplicate ", Disabled: true},
		},
	}
	Register(cfg)
	manager := sdkaccess.NewManager()
	manager.SetProviders(sdkaccess.RegisteredProviders())
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer duplicate")
	if _, authErr := manager.Authenticate(context.Background(), req); authErr == nil {
		t.Fatal("enabled duplicate bypassed disabled state")
	}
}
