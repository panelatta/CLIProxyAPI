package management

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
)

func apiKeyEntriesContext(method, target, body string) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	if body == "" {
		c.Request = httptest.NewRequest(method, target, nil)
	} else {
		c.Request = httptest.NewRequest(method, target, strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
	}
	return c, rec
}

func TestAPIKeysEndpointKeepsStringListAndIncludesEntries(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	h := &Handler{cfg: &config.Config{SDKConfig: config.SDKConfig{
		APIKeys: []string{"sk-a", "sk-b"},
		APIKeyEntries: []config.AccessAPIKeyEntry{
			{APIKey: "sk-a", Name: "Alice laptop"},
			{APIKey: "sk-b"},
		},
	}}}

	c, rec := apiKeyEntriesContext(http.MethodGet, "/v0/management/api-keys", "")
	h.GetAPIKeys(c)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var response struct {
		APIKeys []string                   `json:"api-keys"`
		Entries []config.AccessAPIKeyEntry `json:"api-key-entries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := strings.Join(response.APIKeys, ","); got != "sk-a,sk-b" {
		t.Fatalf("api-keys = %q, want sk-a,sk-b", got)
	}
	if len(response.Entries) != 2 || response.Entries[0].Name != "Alice laptop" {
		t.Fatalf("entries = %#v", response.Entries)
	}
}

func TestAPIKeyEntriesPutPatchDelete(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	path := writeTestConfigFile(t)
	h := &Handler{cfg: &config.Config{}, configFilePath: path}

	c, rec := apiKeyEntriesContext(http.MethodPut, "/v0/management/api-key-entries", `[{"api-key":"sk-a","name":"Alice laptop"},{"api-key":"sk-b","name":"CI runner"}]`)
	h.PutAPIKeyEntries(c)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := strings.Join(h.cfg.APIKeys, ","); got != "sk-a,sk-b" {
		t.Fatalf("PUT APIKeys = %q, want sk-a,sk-b", got)
	}
	if got := h.cfg.APIKeyEntries[0].Name; got != "Alice laptop" {
		t.Fatalf("PUT first name = %q", got)
	}

	c, rec = apiKeyEntriesContext(http.MethodPatch, "/v0/management/api-key-entries", `{"index":0,"value":{"api-key":"sk-a","name":"Alice desktop"}}`)
	h.PatchAPIKeyEntries(c)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := strings.Join(h.cfg.APIKeys, ","); got != "sk-a,sk-b" {
		t.Fatalf("PATCH changed APIKeys = %q", got)
	}
	if got := h.cfg.APIKeyEntries[0].Name; got != "Alice desktop" {
		t.Fatalf("PATCH first name = %q", got)
	}

	c, rec = apiKeyEntriesContext(http.MethodPatch, "/v0/management/api-key-entries", `{"new":{"api-key":"sk-c","name":"New device"}}`)
	h.PatchAPIKeyEntries(c)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH add status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := strings.Join(h.cfg.APIKeys, ","); got != "sk-a,sk-b,sk-c" {
		t.Fatalf("PATCH add APIKeys = %q", got)
	}

	c, rec = apiKeyEntriesContext(http.MethodDelete, "/v0/management/api-key-entries?index=1", "")
	h.DeleteAPIKeyEntries(c)
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := strings.Join(h.cfg.APIKeys, ","); got != "sk-a,sk-c" {
		t.Fatalf("DELETE APIKeys = %q, want sk-a,sk-c", got)
	}

	reloaded, err := config.LoadConfigOptional(path, false)
	if err != nil {
		t.Fatalf("reload saved config: %v", err)
	}
	if got := strings.Join(reloaded.APIKeys, ","); got != "sk-a,sk-c" {
		t.Fatalf("reloaded APIKeys = %q, want sk-a,sk-c", got)
	}
	if len(reloaded.APIKeyEntries) != 2 || reloaded.APIKeyEntries[1].Name != "New device" {
		t.Fatalf("reloaded entries = %#v", reloaded.APIKeyEntries)
	}
}

func TestAPIKeysPatchSynchronizesEntries(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	h := &Handler{cfg: &config.Config{SDKConfig: config.SDKConfig{
		APIKeys: []string{"sk-a", "sk-b"},
		APIKeyEntries: []config.AccessAPIKeyEntry{
			{APIKey: "sk-a", Name: "Old A"},
			{APIKey: "sk-b", Name: "Old B"},
		},
	}}, configFilePath: writeTestConfigFile(t)}

	c, rec := apiKeyEntriesContext(http.MethodPatch, "/v0/management/api-keys", `{"index":0,"value":"sk-c"}`)
	h.PatchAPIKeys(c)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := strings.Join(h.cfg.APIKeys, ","); got != "sk-c,sk-b" {
		t.Fatalf("APIKeys = %q, want sk-c,sk-b", got)
	}
	if len(h.cfg.APIKeyEntries) != 2 || h.cfg.APIKeyEntries[0].APIKey != "sk-c" || h.cfg.APIKeyEntries[0].Name != "" || h.cfg.APIKeyEntries[1].Name != "Old B" {
		t.Fatalf("APIKeyEntries not synchronized: %#v", h.cfg.APIKeyEntries)
	}
}

func TestPutConfigYAMLAceeptsAPIKeyEntryObjects(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	path := writeTestConfigFile(t)
	h := &Handler{cfg: &config.Config{}, configFilePath: path}
	body := "api-keys:\n  - api-key: sk-a\n    name: Alice laptop\n"
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/v0/management/config.yaml", strings.NewReader(body))

	h.PutConfigYAML(c)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(written), "name: Alice laptop") {
		t.Fatalf("written config lost name: %s", string(written))
	}
	if len(h.cfg.APIKeyEntries) != 1 || h.cfg.APIKeyEntries[0].Name != "Alice laptop" {
		t.Fatalf("handler cfg entries = %#v", h.cfg.APIKeyEntries)
	}
}
