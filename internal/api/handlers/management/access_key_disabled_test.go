package management

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
)

func TestConfigV8AccessKeyDisabledRoundTrip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("access: {api-keys: [client]}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{cfg: cfg, configFilePath: path}
	reloads := make(chan *config.Config, 2)
	h.SetConfigReloadHook(func(_ context.Context, next *config.Config) { reloads <- next })
	router := gin.New()
	router.PUT("/v8/management/config/*path", h.ConfigV8)
	router.GET("/v8/management/config/*path", h.ConfigV8)
	for _, disabled := range []bool{true, false} {
		entries := []config.AccessAPIKeyEntry{{APIKey: "client", Name: "Laptop", ForceModel: "pinned", Disabled: disabled}}
		body, errMarshal := json.Marshal(entries)
		if errMarshal != nil {
			t.Fatal(errMarshal)
		}
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/v8/management/config/access/api-keys", strings.NewReader(string(body))))
		if recorder.Code != http.StatusOK {
			t.Fatalf("PUT: status=%d body=%s", recorder.Code, recorder.Body.String())
		}
		select {
		case next := <-reloads:
			if len(next.APIKeys) != 1 || len(next.APIKeyEntries) != 1 || next.APIKeyEntries[0] != entries[0] {
				t.Fatal("runtime reload lost the key or its settings")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("no runtime reload after management save")
		}
		recorder = httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v8/management/config/access/api-keys", nil))
		var saved []config.AccessAPIKeyEntry
		if recorder.Code != http.StatusOK || json.Unmarshal(recorder.Body.Bytes(), &saved) != nil || len(saved) != 1 || saved[0] != entries[0] {
			t.Fatalf("GET lost key settings: status=%d body=%s", recorder.Code, recorder.Body.String())
		}
	}
}
