package executor

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
)

const claudeDisabledWebSearchBody = `{"type":"error","error":{"type":"permission_error","message":"Web search is disabled for this organization."}}`

func TestClassifyClaudeUpstreamError_DisabledWebSearch(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		scoped bool
	}{
		{"disabled search", 403, claudeDisabledWebSearchBody, true},
		{"case and punctuation", 403, `{"error":{"type":"permission_error","message":"  WEB SEARCH IS DISABLED FOR THIS ORGANIZATION  "}}`, true},
		{"revoked key", 403, `{"error":{"type":"permission_error","message":"API key has been revoked."}}`, false},
		{"disabled organization", 403, `{"error":{"type":"permission_error","message":"This organization has been disabled."}}`, false},
		{"missing scope", 403, `{"error":{"type":"permission_error","message":"OAuth token does not meet scope requirement user:inference"}}`, false},
		{"unknown permission", 403, `{"error":{"type":"permission_error","message":"Permission denied"}}`, false},
		{"authentication error", 403, strings.ReplaceAll(claudeDisabledWebSearchBody, "permission_error", "authentication_error"), false},
		{"missing type", 403, `{"error":{"message":"Web search is disabled for this organization."}}`, false},
		{"unstructured body", 403, "Web search is disabled for this organization.", false},
		{"invalid JSON", 403, claudeDisabledWebSearchBody + "garbage", false},
		{"quoted in credential failure", 403, `{"error":{"type":"permission_error","message":"API key revoked. Web search is disabled for this organization."}}`, false},
		{"unauthorized", 401, claudeDisabledWebSearchBody, false},
		{"rate limit", 429, claudeDisabledWebSearchBody, false},
		{"server error", 500, claudeDisabledWebSearchBody, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := classifyClaudeUpstreamError(tt.status, nil, []byte(tt.body))
			var scoped cliproxyexecutor.RequestScopedError
			got := errors.As(err, &scoped) && scoped.IsRequestScoped()
			if got != tt.scoped {
				t.Fatalf("request scoped = %t, want %t; error = %T %v", got, tt.scoped, err, err)
			}
			var status cliproxyexecutor.StatusError
			if !errors.As(err, &status) || status.StatusCode() != tt.status || err.Error() != tt.body {
				t.Fatalf("upstream status/body changed: %v", err)
			}
		})
	}
}

func TestClaudeExecutor_WebSearchRefusalCooldown(t *testing.T) {
	for _, mode := range []string{"execute", "stream", "count"} {
		for _, disabledSearch := range []bool{true, false} {
			name := mode + "/revoked-key"
			if disabledSearch {
				name = mode + "/disabled-search"
			}
			t.Run(name, func(t *testing.T) {
				body := claudeDisabledWebSearchBody
				if !disabledSearch {
					body = `{"type":"error","error":{"type":"permission_error","message":"API key has been revoked."}}`
				}
				var searchAttempts, normalAttempts atomic.Int32
				transport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
					payload, errRead := io.ReadAll(r.Body)
					if errRead != nil {
						return nil, errRead
					}
					response := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Request: r}
					if strings.Contains(string(payload), "web_search") {
						searchAttempts.Add(1)
						response.StatusCode = http.StatusForbidden
						response.Body = io.NopCloser(strings.NewReader(body))
						return response, nil
					}
					normalAttempts.Add(1)
					response.Body = io.NopCloser(strings.NewReader(`{"id":"msg-ok","type":"message","model":"claude-opus-5","role":"assistant","content":[{"type":"text","text":"OK"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
					return response, nil
				})
				ctx := context.WithValue(t.Context(), "cliproxy.roundtripper", http.RoundTripper(transport))

				manager := cliproxyauth.NewManager(nil, nil, nil)
				manager.SetRetryConfig(0, 0, 0)
				manager.RegisterExecutor(NewClaudeExecutor(&config.Config{}))
				auth := &cliproxyauth.Auth{
					ID: uuid.NewString(), Provider: "claude",
					Attributes: map[string]string{"api_key": "sk-ant-api03-test-key", "base_url": "https://api.anthropic.com"},
					Metadata:   map[string]any{"disable_cooling": false},
				}
				const model = "claude-opus-5"
				reg := registry.GetGlobalRegistry()
				reg.RegisterClient(auth.ID, auth.Provider, []*registry.ModelInfo{{ID: model}})
				t.Cleanup(func() { reg.UnregisterClient(auth.ID) })
				if _, errRegister := manager.Register(t.Context(), auth); errRegister != nil {
					t.Fatal(errRegister)
				}
				request := cliproxyexecutor.Request{Model: model, Payload: []byte(`{"model":"claude-opus-5","max_tokens":16,"messages":[{"role":"user","content":"Search for example.com"}],"tools":[{"type":"web_search_20250305","name":"web_search"}]}`)}
				opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FormatClaude}
				before := time.Now()
				var errRun error
				switch mode {
				case "execute":
					_, errRun = manager.Execute(ctx, []string{"claude"}, request, opts)
				case "stream":
					opts.Stream = true
					_, errRun = manager.ExecuteStream(ctx, []string{"claude"}, request, opts)
				case "count":
					_, errRun = manager.ExecuteCount(ctx, []string{"claude"}, request, opts)
				}
				var status cliproxyexecutor.StatusError
				if !errors.As(errRun, &status) || status.StatusCode() != http.StatusForbidden || errRun.Error() != body {
					t.Fatalf("expected original upstream 403, got %T %v", errRun, errRun)
				}
				if got := searchAttempts.Load(); got != 1 {
					t.Fatalf("search attempts = %d, want 1", got)
				}
				updated, ok := manager.GetByID(auth.ID)
				if !ok {
					t.Fatal("credential disappeared")
				}
				state := updated.ModelStates[model]
				if disabledSearch {
					if updated.Unavailable || !updated.NextRetryAfter.IsZero() ||
						(state != nil && (state.Unavailable || !state.NextRetryAfter.IsZero())) {
						t.Fatalf("search refusal cooled credential: auth=%+v model=%+v", updated, state)
					}
					request.Payload = []byte(`{"model":"claude-opus-5","max_tokens":16,"messages":[{"role":"user","content":"Reply OK"}]}`)
					_, errNormal := manager.Execute(ctx, []string{"claude"}, request, cliproxyexecutor.Options{SourceFormat: sdktranslator.FormatClaude})
					if errNormal != nil || normalAttempts.Load() != 1 {
						t.Fatalf("ordinary request did not reach the same credential: attempts=%d error=%v", normalAttempts.Load(), errNormal)
					}
				} else if state == nil || !state.Unavailable || state.NextRetryAfter.Before(before.Add(30*time.Minute)) {
					t.Fatalf("revoked key lost its 30-minute cooldown: %+v", state)
				}
			})
		}
	}
}
