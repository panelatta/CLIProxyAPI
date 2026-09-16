package executor

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
	"github.com/tidwall/gjson"
)

func TestClaudeExecutor_AbliterationSearchCacheControl(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, explicit := range []bool{false, true} {
			t.Run(fmt.Sprintf("stream=%t/explicit=%t", stream, explicit), func(t *testing.T) {
				const model = "abliterated-model-large-v2"
				const baseURL = "https://api.abliteration.ai"
				var seenBody []byte
				transport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
					var errRead error
					seenBody, errRead = io.ReadAll(r.Body)
					if errRead != nil {
						return nil, errRead
					}
					contentType := "application/json"
					response := `{"id":"msg_1","type":"message","model":"abliterated-model-large-v2","role":"assistant","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`
					if stream {
						contentType = "text/event-stream"
						response = "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
					}
					return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(response)), Request: r}, nil
				})
				ctx := context.WithValue(t.Context(), "cliproxy.roundtripper", http.RoundTripper(transport))
				executor := NewClaudeExecutor(&config.Config{ClaudeKey: []config.ClaudeKey{{
					APIKey: "test-abliteration", BaseURL: baseURL, Cloak: &config.CloakConfig{Mode: "never"},
				}}})
				auth := &cliproxyauth.Auth{Attributes: map[string]string{"api_key": "test-abliteration", "base_url": baseURL}}
				payload := `{"model":"abliterated-model-large-v2","max_tokens":32,"messages":[{"role":"user","content":"search"}],"tools":[{"type":"web_search_20250305","name":"web_search","max_uses":1}]}`
				if explicit {
					payload = `{"model":"abliterated-model-large-v2","max_tokens":32,"system":[{"type":"text","text":"system","cache_control":{"type":"ephemeral"}}],"messages":[{"role":"user","content":[{"type":"text","text":"search","cache_control":{"type":"ephemeral"}}]}],"tools":[{"type":"web_search_20250305","name":"web_search","max_uses":1,"cache_control":{"type":"ephemeral"}}]}`
				}
				req := cliproxyexecutor.Request{Model: model, Payload: []byte(payload)}
				opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FormatClaude, Stream: stream}
				if stream {
					result, errStream := executor.ExecuteStream(ctx, auth, req, opts)
					if errStream != nil {
						t.Fatal(errStream)
					}
					for chunk := range result.Chunks {
						if chunk.Err != nil {
							t.Fatal(chunk.Err)
						}
					}
				} else if _, errExecute := executor.Execute(ctx, auth, req, opts); errExecute != nil {
					t.Fatal(errExecute)
				}
				if gjson.GetBytes(seenBody, "tools.0.cache_control").Exists() {
					t.Fatalf("search cache marker reached upstream: %s", seenBody)
				}
				if gjson.GetBytes(seenBody, "tools.0.max_uses").Int() != 1 || gjson.GetBytes(seenBody, "tools.0.type").String() != "web_search_20250305" {
					t.Fatalf("search tool changed: %s", seenBody)
				}
				if gjson.GetBytes(seenBody, "messages.0.content.0.cache_control.type").String() != "ephemeral" {
					t.Fatalf("message cache marker missing: %s", seenBody)
				}
				if explicit && gjson.GetBytes(seenBody, "system.0.cache_control.type").String() != "ephemeral" {
					t.Fatalf("system cache marker missing: %s", seenBody)
				}
			})
		}
	}
}
