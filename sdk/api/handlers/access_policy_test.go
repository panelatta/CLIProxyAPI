package handlers

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/registry"
)

func TestAccessProductsModelCatalog(t *testing.T) {
	registry.GetGlobalRegistry().RegisterClient("products-claude", "claude", []*registry.ModelInfo{{ID: "innocent-alias"}})
	registry.GetGlobalRegistry().RegisterClient("products-codex", "codex", []*registry.ModelInfo{{ID: "claude-disguised"}})
	t.Cleanup(func() {
		registry.GetGlobalRegistry().UnregisterClient("products-claude")
		registry.GetGlobalRegistry().UnregisterClient("products-codex")
	})
	h := &BaseAPIHandler{Cfg: &config.SDKConfig{APIKeys: []string{"client"}, APIKeyEntries: []config.AccessAPIKeyEntry{{APIKey: "client", AllowedProviders: []string{"claude"}}}}}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/v1/models", nil)
	c.Set("userApiKey", "client")
	for _, body := range []string{
		`{"data":[{"id":"innocent-alias"},{"id":"claude-disguised"}],"first_id":"innocent-alias","last_id":"claude-disguised"}`,
		`{"models":[{"name":"models/innocent-alias"},{"name":"models/claude-disguised"}]}`,
		`{"models":[{"slug":"innocent-alias"},{"slug":"claude-disguised"}]}`,
		`{"data":[{"id":"claude-fable-5-dd-saila-tneconni"},{"id":"claude-disguised"}]}`,
	} {
		filtered, err := h.filterAccessModelList(c, []byte(body))
		if err != nil {
			t.Fatal(err)
		}
		var data map[string]any
		if err = json.Unmarshal(filtered, &data); err != nil {
			t.Fatal(err)
		}
		field := "models"
		if data["data"] != nil {
			field = "data"
		}
		if len(data[field].([]any)) != 1 {
			t.Fatal(string(filtered))
		}
	}
	if h.pluginAccessPolicyError(context.WithValue(context.Background(), "gin", c)) == nil {
		t.Fatal("plugin executor bypass")
	}
}
