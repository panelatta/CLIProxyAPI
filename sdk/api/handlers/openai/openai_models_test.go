package openai

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/registry"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/api/handlers"
	"github.com/tidwall/gjson"
)

func TestOpenAIModelsPreservesExtendedMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)

	reg := registry.GetGlobalRegistry()
	clientID := "test-openai-models-preserves-extended-metadata"
	t.Cleanup(func() {
		reg.UnregisterClient(clientID)
	})

	reg.RegisterClient(clientID, "openai", []*registry.ModelInfo{{
		ID:                  "test-model",
		OwnedBy:             "openai",
		DisplayName:         "Test Model",
		Description:         "Extended metadata test model",
		ContextLength:       123456,
		MaxCompletionTokens: 4096,
		SupportedParameters: []string{"tools"},
		Thinking: &registry.ThinkingSupport{
			Levels: []string{"low", "high"},
		},
	}})

	handler := NewOpenAIAPIHandler(&handlers.BaseAPIHandler{})
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)

	handler.OpenAIModels(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	body := recorder.Body.String()
	if got := gjson.Get(body, "data.#(id==\"test-model\").display_name").String(); got != "Test Model" {
		t.Fatalf("display_name = %q, want %q; body=%s", got, "Test Model", body)
	}
	if got := gjson.Get(body, "data.#(id==\"test-model\").name").String(); got != "Test Model" {
		t.Fatalf("name = %q, want %q; body=%s", got, "Test Model", body)
	}
	if got := gjson.Get(body, "data.#(id==\"test-model\").supported_parameters.0").String(); got != "tools" {
		t.Fatalf("supported_parameters[0] = %q, want %q; body=%s", got, "tools", body)
	}
	if got := gjson.Get(body, "data.#(id==\"test-model\").thinking.levels.1").String(); got != "high" {
		t.Fatalf("thinking.levels[1] = %q, want %q; body=%s", got, "high", body)
	}
	if got := gjson.Get(body, "data.#(id==\"test-model\").info.meta.capabilities.builtin_tools").Bool(); !got {
		t.Fatalf("info.meta.capabilities.builtin_tools = %v, want true; body=%s", got, body)
	}
}
