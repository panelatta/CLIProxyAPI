package management

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	sdkconfig "github.com/router-for-me/CLIProxyAPI/v6/sdk/config"
)

func TestExportRequestCallChainGroupsResponseChain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logDir := t.TempDir()

	firstLog := `=== REQUEST INFO ===
Version: test
URL: /v1/responses
Method: POST
Downstream Transport: http
Upstream Transport: http
Timestamp: 2026-04-26T12:00:00Z

=== HEADERS ===
Content-Type: application/json

=== REQUEST BODY ===
{"model":"gpt-test","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}]}

=== API REQUEST 1 ===
Timestamp: 2026-04-26T12:00:01Z
Upstream URL: https://chatgpt.com/backend-api/codex/responses
HTTP Method: POST
Auth: provider=codex, auth_id=codex-a, type=oauth

Headers:
Content-Type: application/json

Body:
{"model":"gpt-test","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}]}

=== API RESPONSE 1 ===
Timestamp: 2026-04-26T12:00:02Z

Status: 200
Headers:
Content-Type: text/event-stream

Body:
data: {"type":"response.output_item.done","item":{"type":"function_call","call_id":"call_1","name":"shell","arguments":"{\"cmd\":\"ls\"}"}}

=== RESPONSE ===
Status: 200
Content-Type: text/event-stream

data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-test","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"done"}]}]}}
`
	secondLog := `=== REQUEST INFO ===
Version: test
URL: /v1/responses
Method: POST
Downstream Transport: http
Upstream Transport: http
Timestamp: 2026-04-26T12:01:00Z

=== HEADERS ===
Content-Type: application/json

=== REQUEST BODY ===
{"model":"gpt-test","previous_response_id":"resp_1","input":[{"type":"function_call_output","call_id":"call_1","output":"file list"}]}

=== API REQUEST 1 ===
Timestamp: 2026-04-26T12:01:01Z
Upstream URL: https://chatgpt.com/backend-api/codex/responses
HTTP Method: POST
Auth: provider=codex, auth_id=codex-a, type=oauth

Headers:
Content-Type: application/json

Body:
{"model":"gpt-test","previous_response_id":"resp_1","input":[{"type":"function_call_output","call_id":"call_1","output":"file list"}]}

=== API RESPONSE 1 ===
Timestamp: 2026-04-26T12:01:02Z

Status: 200
Headers:
Content-Type: application/json

Body:
{"id":"resp_2","model":"gpt-test","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"next"}]}]}

=== RESPONSE ===
Status: 200
Content-Type: application/json

{"id":"resp_2","model":"gpt-test","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"next"}]}]}
`

	if err := os.WriteFile(filepath.Join(logDir, "v1-responses-2026-04-26T120000-reqone.log"), []byte(firstLog), 0o644); err != nil {
		t.Fatalf("write first log: %v", err)
	}
	if err := os.WriteFile(filepath.Join(logDir, "v1-responses-2026-04-26T120100-reqtwo.log"), []byte(secondLog), 0o644); err != nil {
		t.Fatalf("write second log: %v", err)
	}

	handler := NewHandlerWithoutConfigFilePath(&config.Config{
		SDKConfig:     sdkconfig.SDKConfig{RequestLog: true},
		LoggingToFile: true,
	}, nil)
	handler.SetLogDirectory(logDir)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v0/management/request-call-chain/export?limit=10", nil)

	handler.ExportRequestCallChain(ctx)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Disposition"); got == "" {
		t.Fatal("expected attachment disposition")
	}

	var payload callChainExportPayload
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v\n%s", err, rec.Body.String())
	}
	if payload.SessionCount != 1 {
		t.Fatalf("session count = %d, want 1", payload.SessionCount)
	}
	if payload.RequestCount != 2 {
		t.Fatalf("request count = %d, want 2", payload.RequestCount)
	}
	session := payload.Sessions[0]
	if session.ID != "response-chain:resp_1" && session.ID != "response-chain:resp_2" {
		t.Fatalf("session ID = %q, want response-chain", session.ID)
	}
	if len(session.Requests) != 2 {
		t.Fatalf("session requests = %d, want 2", len(session.Requests))
	}
	if got := session.Requests[0].UserInputs[0].Text; got != "hello" {
		t.Fatalf("first user input = %q, want hello", got)
	}
	if len(session.Requests[0].ToolCalls) == 0 || session.Requests[0].ToolCalls[0].CallID != "call_1" {
		t.Fatalf("expected tool call call_1, got %+v", session.Requests[0].ToolCalls)
	}
	if len(session.Requests[1].ToolResults) == 0 || session.Requests[1].ToolResults[0].CallID != "call_1" {
		t.Fatalf("expected tool result call_1, got %+v", session.Requests[1].ToolResults)
	}
}

func TestExportRequestCallChainSummaryModeOmitsHTTPBodies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logDir := t.TempDir()

	longInput := strings.Repeat("hello ", 120)
	logBody := `=== REQUEST INFO ===
URL: /v1/responses
Method: POST
Timestamp: 2026-04-26T12:00:00Z

=== HEADERS ===
Content-Type: application/json
Authorization: Bearer secret

=== REQUEST BODY ===
{"model":"gpt-test","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"` + longInput + `"}]}]}

=== API REQUEST 1 ===
Timestamp: 2026-04-26T12:00:01Z
Upstream URL: https://example.test/v1/responses
HTTP Method: POST
Auth: provider=test

Headers:
Authorization: Bearer upstream-secret

Body:
{"model":"gpt-test","input":"` + longInput + `"}

=== API RESPONSE 1 ===
Timestamp: 2026-04-26T12:00:02Z

Status: 200
Headers:
Content-Type: application/json

Body:
{"id":"resp_1","model":"gpt-test","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}]}

=== RESPONSE ===
Status: 200
Content-Type: application/json

{"id":"resp_1","model":"gpt-test","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}]}
`
	if err := os.WriteFile(filepath.Join(logDir, "v1-responses-2026-04-26T120000-summary.log"), []byte(logBody), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	handler := NewHandlerWithoutConfigFilePath(&config.Config{
		SDKConfig:     sdkconfig.SDKConfig{RequestLog: true},
		LoggingToFile: true,
	}, nil)
	handler.SetLogDirectory(logDir)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v0/management/request-call-chain/export?summary=true&include_raw=true", nil)

	handler.ExportRequestCallChain(ctx)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload callChainExportPayload
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if !payload.Filters.Summary {
		t.Fatal("expected summary filter to be true")
	}
	req := payload.Sessions[0].Requests[0]
	if req.HTTP.DownstreamRequest.Body != "" || len(req.HTTP.DownstreamRequest.Headers) != 0 {
		t.Fatalf("summary should omit downstream request body and headers: %+v", req.HTTP.DownstreamRequest)
	}
	if len(req.HTTP.UpstreamRequests) != 1 || req.HTTP.UpstreamRequests[0].Body != "" || len(req.HTTP.UpstreamRequests[0].Headers) != 0 {
		t.Fatalf("summary should omit upstream request body and headers: %+v", req.HTTP.UpstreamRequests)
	}
	if req.RawSections != nil {
		t.Fatalf("summary should omit raw sections: %+v", req.RawSections)
	}
	if req.Summary == nil || req.Summary.DownstreamRequestBytes == 0 || req.Summary.UpstreamRequestBytes == 0 {
		t.Fatalf("summary stats missing body byte counts: %+v", req.Summary)
	}
	if len(req.UserInputs) != 1 || len([]rune(req.UserInputs[0].Text)) > callChainSummaryTextLimit+3 {
		t.Fatalf("expected truncated user input, got %+v", req.UserInputs)
	}
}

func TestExportRequestCallChainFiltersSessionID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logDir := t.TempDir()

	logBody := `=== REQUEST INFO ===
URL: /v1/responses
Method: POST
Timestamp: 2026-04-26T12:00:00Z

=== HEADERS ===
X-Session-ID: sess-123

=== REQUEST BODY ===
{"model":"gpt-test","input":"hello"}

=== RESPONSE ===
Status: 200

{"id":"resp_1","output_text":"ok"}
`
	if err := os.WriteFile(filepath.Join(logDir, "v1-responses-2026-04-26T120000-abc123.log"), []byte(logBody), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	handler := NewHandlerWithoutConfigFilePath(&config.Config{
		SDKConfig:     sdkconfig.SDKConfig{RequestLog: true},
		LoggingToFile: true,
	}, nil)
	handler.SetLogDirectory(logDir)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v0/management/request-call-chain/export?session_id=sess-123", nil)

	handler.ExportRequestCallChain(ctx)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload callChainExportPayload
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.SessionCount != 1 || payload.Sessions[0].ID != "sess-123" {
		t.Fatalf("unexpected sessions: %+v", payload.Sessions)
	}
}
