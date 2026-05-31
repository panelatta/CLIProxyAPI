package openai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/api/handlers"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	coreexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	sdkconfig "github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
)

type responsesRouteCacheExecutor struct {
	provider        string
	responsePayload []byte
	streamPayloads  [][]byte
	calls           int
	streamCalls     int
}

func (e *responsesRouteCacheExecutor) Identifier() string {
	if e.provider != "" {
		return e.provider
	}
	return "openai-platform"
}

func (e *responsesRouteCacheExecutor) Execute(context.Context, *coreauth.Auth, coreexecutor.Request, coreexecutor.Options) (coreexecutor.Response, error) {
	e.calls++
	return coreexecutor.Response{Payload: append([]byte(nil), e.responsePayload...)}, nil
}

func (e *responsesRouteCacheExecutor) ExecuteStream(context.Context, *coreauth.Auth, coreexecutor.Request, coreexecutor.Options) (*coreexecutor.StreamResult, error) {
	e.streamCalls++
	ch := make(chan coreexecutor.StreamChunk, len(e.streamPayloads))
	for _, payload := range e.streamPayloads {
		ch <- coreexecutor.StreamChunk{Payload: append([]byte(nil), payload...)}
	}
	close(ch)
	return &coreexecutor.StreamResult{Chunks: ch}, nil
}

func (e *responsesRouteCacheExecutor) Refresh(ctx context.Context, auth *coreauth.Auth) (*coreauth.Auth, error) {
	return auth, nil
}

func (e *responsesRouteCacheExecutor) CountTokens(context.Context, *coreauth.Auth, coreexecutor.Request, coreexecutor.Options) (coreexecutor.Response, error) {
	return coreexecutor.Response{}, errors.New("not implemented")
}

func (e *responsesRouteCacheExecutor) HttpRequest(context.Context, *coreauth.Auth, *http.Request) (*http.Response, error) {
	return nil, errors.New("not implemented")
}

func newResponsesRouteCacheTestHandler(t *testing.T, executor *responsesRouteCacheExecutor, auth *coreauth.Auth) *OpenAIResponsesAPIHandler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	manager := coreauth.NewManager(nil, nil, nil)
	manager.RegisterExecutor(executor)
	if _, err := manager.Register(context.Background(), auth); err != nil {
		t.Fatalf("Register auth: %v", err)
	}
	registry.GetGlobalRegistry().RegisterClient(auth.ID, auth.Provider, []*registry.ModelInfo{{ID: "test-model"}})
	t.Cleanup(func() {
		registry.GetGlobalRegistry().UnregisterClient(auth.ID)
	})

	base := handlers.NewBaseAPIHandlers(&sdkconfig.SDKConfig{}, manager)
	h := NewOpenAIResponsesAPIHandler(base)
	h.responseRoutes = newResponsesRouteStore(time.Minute)
	return h
}

func TestOpenAIResponsesNonStreamingRecordsRouteForPassthroughAuth(t *testing.T) {
	executor := &responsesRouteCacheExecutor{
		provider:        "openai-platform",
		responsePayload: []byte(`{"id":"resp_nonstream","object":"response","status":"completed"}`),
	}
	auth := &coreauth.Auth{
		ID:       "auth-route-nonstream",
		Provider: executor.Identifier(),
		Status:   coreauth.StatusActive,
		Attributes: map[string]string{
			"responses_passthrough": "true",
		},
	}
	h := newResponsesRouteCacheTestHandler(t, executor, auth)
	router := gin.New()
	router.POST("/v1/responses", h.Responses)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"test-model","input":"hi","background":true}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.Code, resp.Body.String())
	}
	entry, ok := h.responseRoutes.Get("resp_nonstream", "", time.Now())
	if !ok {
		t.Fatalf("response route was not cached")
	}
	if entry.AuthID != auth.ID {
		t.Fatalf("cached auth = %q, want %q", entry.AuthID, auth.ID)
	}
}

func TestOpenAIResponsesStreamingRecordsRouteForPassthroughAuth(t *testing.T) {
	executor := &responsesRouteCacheExecutor{
		provider: "openai-platform",
		streamPayloads: [][]byte{
			[]byte("event: response.created\ndata: {\"type\":\"response.created\",\"sequence_number\":0,\"response\":{\"id\":\"resp_stream\",\"status\":\"in_progress\"}}\n\n"),
			[]byte("event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_stream\",\"status\":\"completed\",\"output\":[]}}\n\n"),
		},
	}
	auth := &coreauth.Auth{
		ID:       "auth-route-stream",
		Provider: executor.Identifier(),
		Status:   coreauth.StatusActive,
		Attributes: map[string]string{
			"responses_passthrough": "true",
		},
	}
	h := newResponsesRouteCacheTestHandler(t, executor, auth)
	router := gin.New()
	router.POST("/v1/responses", h.Responses)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"test-model","input":"hi","background":true,"stream":true}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.Code, resp.Body.String())
	}
	entry, ok := h.responseRoutes.Get("resp_stream", "", time.Now())
	if !ok {
		t.Fatalf("streaming response route was not cached")
	}
	if entry.AuthID != auth.ID {
		t.Fatalf("cached auth = %q, want %q", entry.AuthID, auth.ID)
	}
}

func TestOpenAIResponsesDoesNotRecordRouteForNonPassthroughCompatAuth(t *testing.T) {
	executor := &responsesRouteCacheExecutor{
		provider:        "openai-compatible",
		responsePayload: []byte(`{"id":"resp_plain","object":"response","status":"completed"}`),
	}
	auth := &coreauth.Auth{
		ID:       "auth-route-plain",
		Provider: executor.Identifier(),
		Status:   coreauth.StatusActive,
	}
	h := newResponsesRouteCacheTestHandler(t, executor, auth)
	router := gin.New()
	router.POST("/v1/responses", h.Responses)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"test-model","input":"hi","background":true}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.Code, resp.Body.String())
	}
	if _, ok := h.responseRoutes.Get("resp_plain", "", time.Now()); ok {
		t.Fatalf("non-passthrough OpenAI-compatible auth should not be cached")
	}
}
