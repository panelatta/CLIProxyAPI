package executor

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

func TestOpenAICompatExecutorResponsesHTTPRequest(t *testing.T) {
	var gotPath string
	var gotQuery url.Values
	var gotAccept string
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		gotAccept = r.Header.Get("Accept")
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"ok\":true}\n\n"))
	}))
	defer server.Close()

	executor := NewOpenAICompatExecutor("openai-platform", &config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url":              server.URL + "/v1",
		"api_key":               "test-key",
		"responses_passthrough": "true",
	}}
	resp, err := executor.ResponsesHTTPRequest(context.Background(), auth, "resp_123", url.Values{"stream": {"true"}, "starting_after": {"42"}})
	if err != nil {
		t.Fatalf("ResponsesHTTPRequest: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if gotPath != "/v1/responses/resp_123" {
		t.Fatalf("path = %q, want /v1/responses/resp_123", gotPath)
	}
	if gotQuery.Get("stream") != "true" || gotQuery.Get("starting_after") != "42" {
		t.Fatalf("query = %v", gotQuery)
	}
	if gotAccept != "text/event-stream" {
		t.Fatalf("Accept = %q, want text/event-stream", gotAccept)
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("Authorization = %q, want Bearer test-key", gotAuth)
	}
	if string(body) != "data: {\"ok\":true}\n\n" {
		t.Fatalf("body = %q", body)
	}
}
