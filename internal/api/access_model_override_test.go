package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
)

func TestAccessModelOverrideMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name      string
		apiKey    string
		body      string
		mediaType string
		wantModel string
	}{
		{
			name:      "configured key is rewritten",
			apiKey:    "sk-mengzhe",
			body:      `{"model":"gpt-5.6-sol","reasoning":{"effort":"xhigh"}}`,
			mediaType: "application/json",
			wantModel: "gpt-5.6-luna",
		},
		{
			name:      "other key is unchanged",
			apiKey:    "sk-other",
			body:      `{"model":"gpt-5.6-sol"}`,
			mediaType: "application/json",
			wantModel: "gpt-5.6-sol",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := &Server{cfg: &config.Config{SDKConfig: config.SDKConfig{
				APIKeys: []string{"sk-mengzhe", "sk-other"},
				APIKeyEntries: []config.AccessAPIKeyEntry{
					{APIKey: "sk-mengzhe", Name: "mengzhe", ForceModel: "gpt-5.6-luna"},
					{APIKey: "sk-other", Name: "other"},
				},
			}}}
			engine := gin.New()
			engine.Use(func(c *gin.Context) {
				c.Set("userApiKey", tt.apiKey)
				c.Next()
			})
			engine.Use(server.accessModelOverrideMiddleware())
			engine.POST("/v1/responses", func(c *gin.Context) {
				var payload struct {
					Model     string `json:"model"`
					Reasoning struct {
						Effort string `json:"effort"`
					} `json:"reasoning"`
				}
				if errBind := c.ShouldBindJSON(&payload); errBind != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": errBind.Error()})
					return
				}
				c.JSON(http.StatusOK, payload)
			})

			request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(tt.body))
			request.Header.Set("Content-Type", tt.mediaType)
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
			var response struct {
				Model     string `json:"model"`
				Reasoning struct {
					Effort string `json:"effort"`
				} `json:"reasoning"`
			}
			if errDecode := json.Unmarshal(recorder.Body.Bytes(), &response); errDecode != nil {
				t.Fatalf("decode response: %v", errDecode)
			}
			if response.Model != tt.wantModel {
				t.Fatalf("model = %q, want %q", response.Model, tt.wantModel)
			}
			if tt.apiKey == "sk-mengzhe" && response.Reasoning.Effort != "xhigh" {
				t.Fatalf("reasoning effort = %q, want xhigh", response.Reasoning.Effort)
			}
		})
	}
}
