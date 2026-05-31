package openai

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/api/handlers"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	coreexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	sdkconfig "github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
)

type responsesRetrieveExecutor struct {
	provider string
	response *http.Response
	err      error
	calls    int
	gotAuth  string
	gotID    string
	gotQuery url.Values
}

func (e *responsesRetrieveExecutor) Identifier() string { return e.provider }

func (e *responsesRetrieveExecutor) Execute(context.Context, *coreauth.Auth, coreexecutor.Request, coreexecutor.Options) (coreexecutor.Response, error) {
	return coreexecutor.Response{}, errors.New("not implemented")
}

func (e *responsesRetrieveExecutor) ExecuteStream(context.Context, *coreauth.Auth, coreexecutor.Request, coreexecutor.Options) (*coreexecutor.StreamResult, error) {
	return nil, errors.New("not implemented")
}

func (e *responsesRetrieveExecutor) Refresh(ctx context.Context, auth *coreauth.Auth) (*coreauth.Auth, error) {
	return auth, nil
}

func (e *responsesRetrieveExecutor) CountTokens(context.Context, *coreauth.Auth, coreexecutor.Request, coreexecutor.Options) (coreexecutor.Response, error) {
	return coreexecutor.Response{}, errors.New("not implemented")
}

func (e *responsesRetrieveExecutor) HttpRequest(context.Context, *coreauth.Auth, *http.Request) (*http.Response, error) {
	return nil, errors.New("not implemented")
}

func (e *responsesRetrieveExecutor) ResponsesHTTPRequest(ctx context.Context, auth *coreauth.Auth, responseID string, query url.Values) (*http.Response, error) {
	e.calls++
	if auth != nil {
		e.gotAuth = auth.ID
	}
	e.gotID = responseID
	e.gotQuery = make(url.Values, len(query))
	for key, values := range query {
		e.gotQuery[key] = append([]string(nil), values...)
	}
	if e.err != nil {
		return nil, e.err
	}
	return e.response, nil
}

func newRetrieveTestHandler(t *testing.T, executor *responsesRetrieveExecutor, auths ...*coreauth.Auth) *OpenAIResponsesAPIHandler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	manager := coreauth.NewManager(nil, nil, nil)
	manager.RegisterExecutor(executor)
	for _, auth := range auths {
		if _, err := manager.Register(context.Background(), auth); err != nil {
			t.Fatalf("Register auth %s: %v", auth.ID, err)
		}
	}
	base := handlers.NewBaseAPIHandlers(&sdkconfig.SDKConfig{}, manager)
	h := NewOpenAIResponsesAPIHandler(base)
	h.responseRoutes = newResponsesRouteStore(time.Minute)
	return h
}

func httpTestResponse(status int, contentType, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": {contentType}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestOpenAIResponsesGetResponseCacheMiss(t *testing.T) {
	executor := &responsesRetrieveExecutor{provider: "test-provider"}
	h := newRetrieveTestHandler(t, executor, &coreauth.Auth{ID: "auth-b", Provider: "test-provider", Status: coreauth.StatusActive})
	router := gin.New()
	router.GET("/v1/responses/:response_id", h.GetResponse)

	req := httptest.NewRequest(http.MethodGet, "/v1/responses/resp_missing?stream=true&starting_after=42", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", resp.Code, resp.Body.String())
	}
	if executor.calls != 0 {
		t.Fatalf("executor calls = %d, want 0", executor.calls)
	}
	if !strings.Contains(resp.Body.String(), "response_not_found") {
		t.Fatalf("expected response_not_found body, got %s", resp.Body.String())
	}
}

func TestOpenAIResponsesGetResponseRejectsInvalidStartingAfter(t *testing.T) {
	executor := &responsesRetrieveExecutor{provider: "test-provider"}
	h := newRetrieveTestHandler(t, executor, &coreauth.Auth{ID: "auth-b", Provider: "test-provider", Status: coreauth.StatusActive})
	h.responseRoutes.Remember("resp_123", "auth-b", "", time.Now())
	router := gin.New()
	router.GET("/v1/responses/:response_id", h.GetResponse)

	req := httptest.NewRequest(http.MethodGet, "/v1/responses/resp_123?starting_after=abc", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", resp.Code, resp.Body.String())
	}
	if executor.calls != 0 {
		t.Fatalf("executor calls = %d, want 0", executor.calls)
	}
}

func TestOpenAIResponsesGetResponseUsesCachedAuthAndForwardsQuery(t *testing.T) {
	executor := &responsesRetrieveExecutor{
		provider: "test-provider",
		response: httpTestResponse(http.StatusOK, "application/json", `{"id":"resp_123","status":"completed"}`),
	}
	h := newRetrieveTestHandler(t, executor,
		&coreauth.Auth{ID: "auth-a", Provider: "test-provider", Status: coreauth.StatusActive},
		&coreauth.Auth{ID: "auth-b", Provider: "test-provider", Status: coreauth.StatusActive},
	)
	h.responseRoutes.Remember("resp_123", "auth-b", "", time.Now())
	router := gin.New()
	router.GET("/v1/responses/:response_id", h.GetResponse)

	req := httptest.NewRequest(http.MethodGet, "/v1/responses/resp_123?stream=true&starting_after=42", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.Code, resp.Body.String())
	}
	if executor.gotAuth != "auth-b" {
		t.Fatalf("auth = %q, want auth-b", executor.gotAuth)
	}
	if executor.gotID != "resp_123" {
		t.Fatalf("response id = %q, want resp_123", executor.gotID)
	}
	if executor.gotQuery.Get("stream") != "true" || executor.gotQuery.Get("starting_after") != "42" {
		t.Fatalf("query = %v", executor.gotQuery)
	}
}

func TestOpenAIResponsesGetResponseForwardsUpstream404Body(t *testing.T) {
	executor := &responsesRetrieveExecutor{
		provider: "test-provider",
		response: httpTestResponse(http.StatusNotFound, "application/json", `{"error":{"message":"Invalid URL"}}`),
	}
	h := newRetrieveTestHandler(t, executor, &coreauth.Auth{ID: "auth-b", Provider: "test-provider", Status: coreauth.StatusActive})
	h.responseRoutes.Remember("resp_123", "auth-b", "", time.Now())
	router := gin.New()
	router.GET("/v1/responses/:response_id", h.GetResponse)

	req := httptest.NewRequest(http.MethodGet, "/v1/responses/resp_123?stream=true&starting_after=42", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", resp.Code, resp.Body.String())
	}
	if strings.Contains(resp.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("404 must not be converted to SSE: %s", resp.Header().Get("Content-Type"))
	}
	if strings.TrimSpace(resp.Body.String()) != `{"error":{"message":"Invalid URL"}}` {
		t.Fatalf("body = %s", resp.Body.String())
	}
}

func TestOpenAIResponsesGetResponseStreamsUpstreamSSE(t *testing.T) {
	body := "event: response.created\ndata: {\"type\":\"response.created\",\"sequence_number\":43,\"response\":{\"id\":\"resp_123\"}}\n\n"
	executor := &responsesRetrieveExecutor{
		provider: "test-provider",
		response: httpTestResponse(http.StatusOK, "text/event-stream", body),
	}
	h := newRetrieveTestHandler(t, executor, &coreauth.Auth{ID: "auth-b", Provider: "test-provider", Status: coreauth.StatusActive})
	h.responseRoutes.Remember("resp_123", "auth-b", "", time.Now())
	router := gin.New()
	router.GET("/v1/responses/:response_id", h.GetResponse)

	req := httptest.NewRequest(http.MethodGet, "/v1/responses/resp_123?stream=true&starting_after=42", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.Code, resp.Body.String())
	}
	if got := resp.Header().Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}
	if resp.Body.String() != body {
		t.Fatalf("SSE body changed. got=%q want=%q", resp.Body.String(), body)
	}
}
