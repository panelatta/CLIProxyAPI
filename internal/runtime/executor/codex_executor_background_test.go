package executor

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MoeForge/moego/ojson"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	_ "github.com/router-for-me/CLIProxyAPI/v8/internal/translator"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v8/sdk/translator"
)

func TestCodexExecutorExecuteStripsBackgroundBeforeUpstream(t *testing.T) {
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = body
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"object\":\"response\",\"created_at\":0,\"status\":\"completed\",\"background\":false,\"error\":null}}\n\n"))
	}))
	defer server.Close()

	executor := NewCodexExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": server.URL,
		"api_key":  "test",
	}}

	_, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "gpt-5.5",
		Payload: []byte(`{"model":"gpt-5.5","input":"hello","background":true,"store":true}`),
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai-response"),
		Stream:       false,
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	assertCodexBackgroundCompatibilityBody(t, gotBody)
}

func TestCodexExecutorExecuteStreamStripsBackgroundBeforeUpstream(t *testing.T) {
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = body
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"object\":\"response\",\"created_at\":0,\"status\":\"completed\",\"background\":false,\"error\":null}}\n\n"))
	}))
	defer server.Close()

	executor := NewCodexExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": server.URL,
		"api_key":  "test",
	}}

	result, err := executor.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "gpt-5.5",
		Payload: []byte(`{"model":"gpt-5.5","input":"hello","background":true,"store":true,"stream":true}`),
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai-response"),
		Stream:       true,
	})
	if err != nil {
		t.Fatalf("ExecuteStream error: %v", err)
	}
	for chunk := range result.Chunks {
		if chunk.Err != nil {
			t.Fatalf("stream chunk error: %v", chunk.Err)
		}
	}
	assertCodexBackgroundCompatibilityBody(t, gotBody)
}

func assertCodexBackgroundCompatibilityBody(t *testing.T, body []byte) {
	t.Helper()
	if ojson.GetBytes(body, "background").Exists() {
		t.Fatalf("background should not be forwarded to Codex upstream: %s", string(body))
	}
	if ojson.GetBytes(body, "store").Bool() {
		t.Fatalf("store=true should not be forwarded to Codex upstream: %s", string(body))
	}
}

func TestLocalImageRoutesApplyPayloadRulesLast(t *testing.T) {
	for _, provider := range []string{"codex", "openai-compatible"} {
		t.Run(provider, func(t *testing.T) {
			var body []byte
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ = io.ReadAll(r.Body)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"data":[{"b64_json":"aW1hZ2U="}]}`))
			}))
			defer server.Close()
			cfg := &config.Config{Payload: config.PayloadConfig{
				Override: []config.PayloadRule{{Models: []config.PayloadModelRule{{Name: "image-route"}}, Params: map[string]any{"model": "configured-image", "size": "512x512"}}},
				Filter:   []config.PayloadFilterRule{{Models: []config.PayloadModelRule{{Name: "image-route"}}, Params: []string{"metadata"}}},
			}}
			auth := &cliproxyauth.Auth{Attributes: map[string]string{"api_key": "test-key", "base_url": server.URL}}
			req := cliproxyexecutor.Request{Model: "image-route", Payload: []byte(`{"model":"alias","prompt":"test","metadata":{"internal":true}}`)}
			opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FormatOpenAI, Alt: "images/generations"}
			var err error
			if provider == "codex" {
				_, err = NewCodexExecutor(cfg).Execute(context.Background(), auth, req, opts)
			} else {
				_, err = NewOpenAICompatExecutor(provider, cfg).Execute(context.Background(), auth, req, opts)
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := ojson.GetBytes(body, "model").String(); got != "configured-image" {
				t.Fatalf("configured model overwritten: %s", body)
			}
			if got := ojson.GetBytes(body, "size").String(); got != "512x512" {
				t.Fatalf("configured size missing: %s", body)
			}
			if ojson.GetBytes(body, "metadata").Exists() {
				t.Fatalf("filtered metadata restored: %s", body)
			}
		})
	}
}
