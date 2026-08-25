package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	codexlive "github.com/router-for-me/CLIProxyAPI/v7/internal/client/codex/live"
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

func TestAccessModelOverrideRealtimeQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name         string
		path         string
		clientSecret bool
		wantModel    string
	}{
		{
			name:      "api key rewrites direct realtime model",
			path:      "/v1/realtime?model=gpt-realtime&intent=voice",
			wantModel: "gpt-5.6-luna",
		},
		{
			name:      "api key supplies omitted direct realtime model",
			path:      "/v1/realtime?intent=voice",
			wantModel: "gpt-5.6-luna",
		},
		{
			name:         "client secret leaves requested model for scope validation",
			path:         "/v1/realtime?model=gpt-realtime-other",
			clientSecret: true,
			wantModel:    "gpt-realtime-other",
		},
		{
			name:      "sideband call leaves model query unchanged",
			path:      "/v1/realtime?call_id=call-123&model=gpt-realtime",
			wantModel: "gpt-realtime",
		},
		{
			name:      "ordinary get leaves model query unchanged",
			path:      "/v1/models?model=gpt-realtime",
			wantModel: "gpt-realtime",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := accessModelOverrideTestServer()
			engine := gin.New()
			engine.Use(func(c *gin.Context) {
				c.Set("userApiKey", "sk-mengzhe")
				if tt.clientSecret {
					c.Set(codexlive.ClientSecretSessionContextKey, json.RawMessage(`{"type":"realtime","model":"gpt-5.6-luna"}`))
				}
				c.Next()
			})
			engine.Use(server.accessModelOverrideMiddleware())
			queryHandler := func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{
					"model":  c.Query("model"),
					"intent": c.Query("intent"),
				})
			}
			engine.GET("/v1/realtime", queryHandler)
			engine.GET("/v1/models", queryHandler)

			request := httptest.NewRequest(http.MethodGet, tt.path, nil)
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
			var response struct {
				Model  string `json:"model"`
				Intent string `json:"intent"`
			}
			if errDecode := json.Unmarshal(recorder.Body.Bytes(), &response); errDecode != nil {
				t.Fatalf("decode response: %v", errDecode)
			}
			if response.Model != tt.wantModel {
				t.Fatalf("model = %q, want %q", response.Model, tt.wantModel)
			}
			if strings.Contains(tt.path, "intent=voice") && response.Intent != "voice" {
				t.Fatalf("intent = %q, want voice", response.Intent)
			}
		})
	}
}

func TestAccessModelOverrideLeavesFileRequestUntouched(t *testing.T) {
	gin.SetMode(gin.TestMode)

	server := accessModelOverrideTestServer()
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("userApiKey", "sk-mengzhe")
		c.Next()
	})
	engine.Use(server.accessModelOverrideMiddleware())
	engine.POST("/v1/files", func(c *gin.Context) {
		payload, errRead := io.ReadAll(c.Request.Body)
		if errRead != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": errRead.Error()})
			return
		}
		c.Data(http.StatusOK, "application/octet-stream", payload)
	})

	request := httptest.NewRequest(http.MethodPost, "/v1/files", strings.NewReader("file-bytes"))
	request.Header.Set("Content-Type", "application/octet-stream")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Body.String() != "file-bytes" {
		t.Fatalf("body = %q, want file-bytes", recorder.Body.String())
	}
}

func accessModelOverrideTestServer() *Server {
	return &Server{cfg: &config.Config{SDKConfig: config.SDKConfig{
		APIKeys: []string{"sk-mengzhe"},
		APIKeyEntries: []config.AccessAPIKeyEntry{
			{APIKey: "sk-mengzhe", Name: "mengzhe", ForceModel: "gpt-5.6-luna"},
		},
	}}}
}
