package executor

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
	"github.com/tidwall/gjson"
)

func TestOpenAICompatResponsesPassthroughDisabledUsesChatCompletions(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl_1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer server.Close()

	executor := NewOpenAICompatExecutor("openai-compatible", &config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": server.URL + "/v1",
		"api_key":  "test-key",
	}}
	_, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "gpt-5.5",
		Payload: []byte(`{"model":"alias","input":"hi","background":true,"store":true}`),
	}, cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("openai-response")})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if gotPath != "/v1/chat/completions" {
		t.Fatalf("path = %q, want /v1/chat/completions", gotPath)
	}
}

func TestOpenAICompatResponsesPassthroughExecutePostsResponses(t *testing.T) {
	var gotPath string
	var gotBody []byte
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_123","object":"response","status":"completed"}`))
	}))
	defer server.Close()

	executor := NewOpenAICompatExecutor("openai-platform", &config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url":              server.URL + "/v1",
		"api_key":               "test-key",
		"responses_passthrough": "true",
	}}
	resp, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "gpt-5.5",
		Payload: []byte(`{"model":"alias","input":"hi","background":true,"stream":true,"store":true}`),
	}, cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("openai-response"), OriginalRequest: []byte(`{"model":"alias","input":"hi","background":true,"stream":true,"store":true}`)})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if gotPath != "/v1/responses" {
		t.Fatalf("path = %q, want /v1/responses", gotPath)
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("Authorization = %q, want Bearer test-key", gotAuth)
	}
	if got := gjson.GetBytes(gotBody, "model").String(); got != "gpt-5.5" {
		t.Fatalf("body model = %q, want gpt-5.5; body=%s", got, gotBody)
	}
	for _, key := range []string{"background", "stream", "store"} {
		if !gjson.GetBytes(gotBody, key).Bool() {
			t.Fatalf("body %s should be true: %s", key, gotBody)
		}
	}
	if string(resp.Payload) != `{"id":"resp_123","object":"response","status":"completed"}` {
		t.Fatalf("payload = %s", resp.Payload)
	}
}

func TestOpenAICompatResponsesPassthroughExecuteStreamRawSSE(t *testing.T) {
	var gotPath string
	var gotAccept string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAccept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: response.created\ndata: {\"type\":\"response.created\",\"sequence_number\":0,\"response\":{\"id\":\"resp_123\"}}\n\n"))
	}))
	defer server.Close()

	executor := NewOpenAICompatExecutor("openai-platform", &config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url":              server.URL + "/v1",
		"api_key":               "test-key",
		"responses_passthrough": "true",
	}}
	result, err := executor.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "gpt-5.5",
		Payload: []byte(`{"model":"alias","input":"hi","background":true,"stream":true,"store":true}`),
	}, cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("openai-response"), Stream: true})
	if err != nil {
		t.Fatalf("ExecuteStream: %v", err)
	}
	var got strings.Builder
	for chunk := range result.Chunks {
		if chunk.Err != nil {
			t.Fatalf("unexpected stream error: %v", chunk.Err)
		}
		got.Write(chunk.Payload)
	}
	if gotPath != "/v1/responses" {
		t.Fatalf("path = %q, want /v1/responses", gotPath)
	}
	if gotAccept != "text/event-stream" {
		t.Fatalf("Accept = %q, want text/event-stream", gotAccept)
	}
	if !strings.Contains(got.String(), `"sequence_number":0`) || !strings.Contains(got.String(), `"id":"resp_123"`) {
		t.Fatalf("raw SSE was not preserved: %q", got.String())
	}
}

func TestResponsesPassthroughSSEFramerEmitsOnlyCompleteFrames(t *testing.T) {
	framer := &responsesPassthroughSSEFramer{}
	var got []string
	emit := func(chunk []byte) bool {
		got = append(got, string(chunk))
		return true
	}

	if !framer.Write([]byte("event: response.created\ndata: {\"type\""), emit) {
		t.Fatalf("unexpected canceled write")
	}
	if len(got) != 0 {
		t.Fatalf("incomplete frame emitted: %#v", got)
	}
	if !framer.Write([]byte(":\"response.created\",\"response\":{\"id\":\"resp_123\"}}\n\n"), emit) {
		t.Fatalf("unexpected canceled write")
	}
	if len(got) != 1 || !strings.Contains(got[0], `"id":"resp_123"`) {
		t.Fatalf("complete frame not emitted correctly: %#v", got)
	}
}

func TestOpenAICompatResponsesPassthroughErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"message":"Invalid URL"}}`))
	}))
	defer server.Close()

	executor := NewOpenAICompatExecutor("openai-platform", &config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url":              server.URL + "/v1",
		"api_key":               "test-key",
		"responses_passthrough": "true",
	}}
	_, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "gpt-5.5",
		Payload: []byte(`{"model":"alias","input":"hi","background":true,"store":true}`),
	}, cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("openai-response")})
	if err == nil {
		t.Fatalf("expected error")
	}
	status, ok := err.(interface{ StatusCode() int })
	if !ok || status.StatusCode() != http.StatusNotFound {
		t.Fatalf("status error = %v, want 404", err)
	}
	noRetry, ok := err.(interface{ NoAuthRetry() bool })
	if !ok || !noRetry.NoAuthRetry() {
		t.Fatalf("passthrough status error must prevent auth fallback: %T %[1]v", err)
	}
}

func TestOpenAICompatResponsesPassthroughStreamErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"message":"Invalid URL"}}`))
	}))
	defer server.Close()

	executor := NewOpenAICompatExecutor("openai-platform", &config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url":              server.URL + "/v1",
		"api_key":               "test-key",
		"responses_passthrough": "true",
	}}
	_, err := executor.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "gpt-5.5",
		Payload: []byte(`{"model":"alias","input":"hi","background":true,"store":true,"stream":true}`),
	}, cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("openai-response"), Stream: true})
	if err == nil {
		t.Fatalf("expected error")
	}
	noRetry, ok := err.(interface{ NoAuthRetry() bool })
	if !ok || !noRetry.NoAuthRetry() {
		t.Fatalf("passthrough stream status error must prevent auth fallback: %T %[1]v", err)
	}
}
