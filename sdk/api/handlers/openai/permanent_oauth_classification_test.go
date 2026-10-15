package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/registry"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/api/handlers"
	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	coreexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	sdkconfig "github.com/router-for-me/CLIProxyAPI/v8/sdk/config"
)

const reproOAuthMessage = `token refresh failed with status 401: {"error":{"message":"Refresh credential has already been consumed; sign in again.","type":"invalid_request_error","code":"refresh_token_reused"}}`
const reproCompletedResponse = `{"id":"resp-fixture","object":"response","status":"completed","output":[]}`

type reproOAuthExecutor struct {
	calls        atomic.Int32
	refreshCalls atomic.Int32
	reject       bool
	bootstrap    bool
}

func (*reproOAuthExecutor) Identifier() string { return "codex" }
func (e *reproOAuthExecutor) Execute(ctx context.Context, auth *coreauth.Auth, _ coreexecutor.Request, _ coreexecutor.Options) (coreexecutor.Response, error) {
	coreexecutor.MarkUpstreamAttempt(ctx)
	e.calls.Add(1)
	if e.reject && auth.Metadata["access_token"] == "rejected" {
		return coreexecutor.Response{}, &coreauth.Error{HTTPStatus: 401, Message: "Encountered invalidated oauth token for user, failing request"}
	}
	return coreexecutor.Response{Payload: []byte(reproCompletedResponse)}, nil
}
func (e *reproOAuthExecutor) ExecuteStream(ctx context.Context, auth *coreauth.Auth, _ coreexecutor.Request, _ coreexecutor.Options) (*coreexecutor.StreamResult, error) {
	coreexecutor.MarkUpstreamAttempt(ctx)
	e.calls.Add(1)
	if e.reject && auth.Metadata["access_token"] == "rejected" {
		err := &coreauth.Error{HTTPStatus: 401, Message: "Encountered invalidated oauth token for user, failing request"}
		if !e.bootstrap {
			return nil, err
		}
		chunks := make(chan coreexecutor.StreamChunk, 1)
		chunks <- coreexecutor.StreamChunk{Err: err}
		close(chunks)
		return &coreexecutor.StreamResult{Chunks: chunks}, nil
	}
	chunks := make(chan coreexecutor.StreamChunk, 1)
	chunks <- coreexecutor.StreamChunk{Payload: []byte("event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":" + reproCompletedResponse + "}\n\n")}
	close(chunks)
	return &coreexecutor.StreamResult{Chunks: chunks}, nil
}
func (e *reproOAuthExecutor) Refresh(context.Context, *coreauth.Auth) (*coreauth.Auth, error) {
	e.refreshCalls.Add(1)
	if e.reject {
		return nil, errors.New(`token refresh failed with status 401: {"error":{"message":"Your session has ended. Please log in again.","type":"invalid_request_error","code":"refresh_token_invalidated"}}`)
	}
	return nil, errors.New(reproOAuthMessage)
}
func (*reproOAuthExecutor) CountTokens(context.Context, *coreauth.Auth, coreexecutor.Request, coreexecutor.Options) (coreexecutor.Response, error) {
	return coreexecutor.Response{}, errors.New("unused fixture method")
}
func (*reproOAuthExecutor) HttpRequest(context.Context, *coreauth.Auth, *http.Request) (*http.Response, error) {
	return nil, errors.New("network is not used in this reproduction")
}

func TestReproPermanentOAuthFailureClassification(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			model := fmt.Sprintf("repro-permanent-oauth-%t", stream)
			failedID := "failed-" + model
			healthyID := "healthy-" + model
			executor := &reproOAuthExecutor{}
			manager := coreauth.NewManager(nil, nil, nil)
			manager.RegisterExecutor(executor)
			defer manager.StopAutoRefresh()

			// This is the terminal state produced by refreshAuth when a 401
			// refresh failure has no remaining valid access token. The upstream
			// TestManager_RefreshAuthUnauthorizedFailureStopsAutoRefreshRetry
			// independently verifies that transition and unscheduling behavior.
			failed := &coreauth.Auth{
				ID: failedID, Provider: "codex", Status: coreauth.StatusError,
				Unavailable: true, StatusMessage: "unauthorized",
				LastError: &coreauth.Error{Code: "unauthorized", Message: reproOAuthMessage,
					Retryable: false, HTTPStatus: http.StatusUnauthorized},
				Metadata: map[string]any{"access_token": "expired-fixture",
					"expired": time.Now().Add(-time.Hour).Format(time.RFC3339)},
			}
			if _, errRegister := manager.Register(context.Background(), failed); errRegister != nil {
				t.Fatal(errRegister)
			}
			registry.GetGlobalRegistry().RegisterClient(failedID, "codex", []*registry.ModelInfo{{ID: model}})
			t.Cleanup(func() {
				registry.GetGlobalRegistry().UnregisterClient(failedID)
				registry.GetGlobalRegistry().UnregisterClient(healthyID)
			})
			base := handlers.NewBaseAPIHandlers(&sdkconfig.SDKConfig{}, manager)
			router := gin.New()
			router.POST("/v1/responses", NewOpenAIResponsesAPIHandler(base).Responses)
			request := func() *httptest.ResponseRecorder {
				body := fmt.Sprintf(`{"model":%q,"input":"fixture","stream":%t}`, model, stream)
				req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("User-Agent", "codex_cli_rs/0.153.4")
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, req)
				return recorder
			}

			for attempt := 1; attempt <= 3; attempt++ {
				got := request()
				var payload struct {
					Error struct {
						Code, Type, Message string
						Retryable           *bool `json:"retryable"`
					} `json:"error"`
				}
				if errUnmarshal := json.Unmarshal(got.Body.Bytes(), &payload); errUnmarshal != nil {
					t.Fatalf("decode HTTP %d body %q: %v", got.Code, got.Body.String(), errUnmarshal)
				}
				t.Logf("attempt=%d HTTP=%d code=%q type=%q retryable=%v executor_calls=%d body=%s", attempt,
					got.Code, payload.Error.Code, payload.Error.Type, payload.Error.Retryable, executor.calls.Load(), got.Body.String())

				if got.Code != http.StatusForbidden {
					t.Errorf("attempt %d: HTTP status = %d, want %d", attempt, got.Code, http.StatusForbidden)
				}
				if payload.Error.Type != "authentication_error" {
					t.Errorf("attempt %d: Error.Type = %q, want %q", attempt, payload.Error.Type, "authentication_error")
				}
				if payload.Error.Code != "upstream_authentication_required" {
					t.Errorf("attempt %d: Error.Code = %q, want %q", attempt, payload.Error.Code, "upstream_authentication_required")
				}
				if payload.Error.Retryable == nil || *payload.Error.Retryable {
					t.Errorf("attempt %d: Error.Retryable = %v, want explicit false", attempt, payload.Error.Retryable)
				}
				if !strings.Contains(payload.Error.Message, "refresh_token_reused") {
					t.Errorf("attempt %d: terminal OAuth cause missing from response: %s", attempt, got.Body.String())
				}
				if !strings.Contains(payload.Error.Message, "[REDACTED]") {
					t.Errorf("attempt %d: sensitive token details not redacted in message: %s", attempt, payload.Error.Message)
				}
			}
			if executor.calls.Load() != 0 {
				t.Errorf("terminal auth reached executor %d times", executor.calls.Load())
			}

			// A healthy candidate must make this same route succeed, proving the
			// earlier errors are not missing model registration or a bad handler.
			healthy := &coreauth.Auth{ID: healthyID, Provider: "codex", Status: coreauth.StatusActive}
			if _, errRegisterHealthy := manager.Register(context.Background(), healthy); errRegisterHealthy != nil {
				t.Fatal(errRegisterHealthy)
			}
			registry.GetGlobalRegistry().RegisterClient(healthyID, "codex", []*registry.ModelInfo{{ID: model}})
			control := request()
			if control.Code != http.StatusOK || executor.calls.Load() != 1 {
				t.Errorf("healthy control HTTP=%d executor_calls=%d body=%s", control.Code, executor.calls.Load(), control.Body.String())
			}
			t.Logf("healthy control HTTP=%d executor_calls=%d", control.Code, executor.calls.Load())
		})
	}
}

func TestResponsesInvalidatedOAuthFirstAndSubsequentRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, mode := range []string{"nonstream", "stream", "bootstrap", "websocket", "websocket-bootstrap"} {
		for _, backup := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/backup=%t", mode, backup), func(t *testing.T) {
				model := "invalidated-" + strings.ReplaceAll(t.Name(), "/", "-")
				manager := coreauth.NewManager(nil, &coreauth.FillFirstSelector{}, nil)
				defer manager.StopAutoRefresh()
				executor := &reproOAuthExecutor{reject: true, bootstrap: strings.Contains(mode, "bootstrap")}
				manager.RegisterExecutor(executor)
				register := func(id, token string) {
					_, err := manager.Register(context.Background(), &coreauth.Auth{ID: id, Provider: "codex", Status: coreauth.StatusActive, Metadata: map[string]any{"access_token": token, "refresh_token": "fixture-refresh", "expired": time.Now().Add(time.Hour).Format(time.RFC3339)}})
					if err != nil {
						t.Fatal(err)
					}
					registry.GetGlobalRegistry().RegisterClient(id, "codex", []*registry.ModelInfo{{ID: model}})
					t.Cleanup(func() { registry.GetGlobalRegistry().UnregisterClient(id) })
				}
				register("aa-"+model, "rejected")
				if backup {
					register("bb-"+model, "healthy")
				}
				router := gin.New()
				handler := NewOpenAIResponsesAPIHandler(handlers.NewBaseAPIHandlers(&sdkconfig.SDKConfig{}, manager))
				router.POST("/v1/responses", handler.Responses)
				router.GET("/v1/responses", handler.ResponsesWebsocket)
				server := httptest.NewServer(router)
				defer server.Close()
				for attempt := 0; attempt < 3; attempt++ {
					rec := httptest.NewRecorder()
					req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(fmt.Sprintf(`{"model":%q,"input":"fixture","stream":%t}`, model, mode != "nonstream")))
					if strings.HasPrefix(mode, "websocket") {
						conn, _, errDial := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/v1/responses", nil)
						if errDial != nil {
							t.Fatal(errDial)
						}
						if errWrite := conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf(`{"type":"response.create","model":%q,"input":[{"role":"user","content":"fixture"}]}`, model))); errWrite != nil {
							t.Fatal(errWrite)
						}
						_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
						_, payload, errRead := conn.ReadMessage()
						_ = conn.Close()
						if errRead != nil {
							t.Fatalf("OAuth error was hidden or request did not finish: %v", errRead)
						}
						var frame struct {
							Status int    `json:"status"`
							Type   string `json:"type"`
						}
						if errDecode := json.Unmarshal(payload, &frame); errDecode != nil {
							t.Fatal(errDecode)
						}
						if backup {
							if frame.Type != "response.completed" {
								t.Fatalf("backup response: %s", payload)
							}
						} else {
							rec.Code = frame.Status
						}
						rec.Body.Write(payload)
					} else {
						router.ServeHTTP(rec, req)
					}
					if backup {
						if rec.Code != 200 {
							t.Fatalf("backup failed: HTTP %d %s", rec.Code, rec.Body.String())
						}
					} else {
						if rec.Code != http.StatusForbidden {
							t.Fatalf("attempt %d: HTTP %d %s", attempt, rec.Code, rec.Body.String())
						}
						for _, want := range []string{`"code":"upstream_authentication_required"`, `"retryable":false`, "refresh_token_invalidated", "signing in again"} {
							if !strings.Contains(rec.Body.String(), want) {
								t.Errorf("attempt %d: missing %s in %s", attempt, want, rec.Body.String())
							}
						}
						if executor.calls.Load() != 1 {
							t.Errorf("rejected credential executed %d times", executor.calls.Load())
						}
					}
				}
				if executor.refreshCalls.Load() != 1 {
					t.Errorf("refresh called %d times", executor.refreshCalls.Load())
				}
			})
		}
	}
}
