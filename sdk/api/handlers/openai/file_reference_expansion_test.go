package openai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	internalfiles "github.com/router-for-me/CLIProxyAPI/v6/internal/files"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/registry"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/api/handlers"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	coreexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
	sdkconfig "github.com/router-for-me/CLIProxyAPI/v6/sdk/config"
	"github.com/tidwall/gjson"
)

type capturePayloadExecutor struct {
	payload string
	calls   int
}

func (e *capturePayloadExecutor) Identifier() string { return "test-provider" }

func (e *capturePayloadExecutor) Execute(ctx context.Context, auth *coreauth.Auth, req coreexecutor.Request, opts coreexecutor.Options) (coreexecutor.Response, error) {
	e.calls++
	e.payload = string(req.Payload)
	return coreexecutor.Response{Payload: []byte(`{"ok":true}`)}, nil
}

func (e *capturePayloadExecutor) ExecuteStream(context.Context, *coreauth.Auth, coreexecutor.Request, coreexecutor.Options) (*coreexecutor.StreamResult, error) {
	return nil, errors.New("not implemented")
}

func (e *capturePayloadExecutor) Refresh(ctx context.Context, auth *coreauth.Auth) (*coreauth.Auth, error) {
	return auth, nil
}

func (e *capturePayloadExecutor) CountTokens(context.Context, *coreauth.Auth, coreexecutor.Request, coreexecutor.Options) (coreexecutor.Response, error) {
	return coreexecutor.Response{}, errors.New("not implemented")
}

func (e *capturePayloadExecutor) HttpRequest(context.Context, *coreauth.Auth, *http.Request) (*http.Response, error) {
	return nil, errors.New("not implemented")
}

func TestResponsesExpandsFileIDToInlineFileData(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := internalfiles.NewStore(t.TempDir())
	record, err := store.Create(internalfiles.CreateParams{
		Filename: "notes.txt",
		Purpose:  "assistants",
		MIMEType: "text/plain",
		Data:     []byte("hello"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	base, executor := newOpenAIHandlerTestBase(t, store)
	h := NewOpenAIResponsesAPIHandler(base)

	router := gin.New()
	router.POST("/v1/responses", h.Responses)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"test-model","input":[{"type":"message","role":"user","content":[{"type":"input_file","file_id":"`+record.ID+`"}]}]}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", resp.Code, http.StatusOK, resp.Body.String())
	}
	if executor.calls != 1 {
		t.Fatalf("executor calls = %d, want 1", executor.calls)
	}
	if got := gjson.Get(executor.payload, "input.0.content.0.file_data").String(); got != "data:text/plain;base64,aGVsbG8=" {
		t.Fatalf("file_data = %q, want %q", got, "data:text/plain;base64,aGVsbG8=")
	}
	if got := gjson.Get(executor.payload, "input.0.content.0.filename").String(); got != "notes.txt" {
		t.Fatalf("filename = %q, want %q", got, "notes.txt")
	}
	if gjson.Get(executor.payload, "input.0.content.0.file_id").Exists() {
		t.Fatalf("unexpected file_id in rewritten payload: %s", executor.payload)
	}
	if _, err := store.GetMetadata(record.ID); !errors.Is(err, internalfiles.ErrNotFound) {
		t.Fatalf("GetMetadata after successful responses request error = %v, want ErrNotFound", err)
	}
}

func TestResponsesRejectsUnknownFileID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := internalfiles.NewStore(t.TempDir())
	base, _ := newOpenAIHandlerTestBase(t, store)
	h := NewOpenAIResponsesAPIHandler(base)

	router := gin.New()
	router.POST("/v1/responses", h.Responses)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"test-model","input":[{"type":"message","role":"user","content":[{"type":"input_file","file_id":"file_cpa_missing"}]}]}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", resp.Code, http.StatusBadRequest, resp.Body.String())
	}
}

func TestChatCompletionsExpandsFileIDToInlineFileData(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := internalfiles.NewStore(t.TempDir())
	record, err := store.Create(internalfiles.CreateParams{
		Filename: "notes.txt",
		Purpose:  "assistants",
		MIMEType: "text/plain",
		Data:     []byte("hello"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	base, executor := newOpenAIHandlerTestBase(t, store)
	h := NewOpenAIAPIHandler(base)

	router := gin.New()
	router.POST("/v1/chat/completions", h.ChatCompletions)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"test-model","messages":[{"role":"user","content":[{"type":"file","file":{"file_id":"`+record.ID+`"}}]}]}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", resp.Code, http.StatusOK, resp.Body.String())
	}
	if got := gjson.Get(executor.payload, "messages.0.content.0.file.file_data").String(); got != "data:text/plain;base64,aGVsbG8=" {
		t.Fatalf("file_data = %q, want %q", got, "data:text/plain;base64,aGVsbG8=")
	}
	if got := gjson.Get(executor.payload, "messages.0.content.0.file.filename").String(); got != "notes.txt" {
		t.Fatalf("filename = %q, want %q", got, "notes.txt")
	}
	if gjson.Get(executor.payload, "messages.0.content.0.file.file_id").Exists() {
		t.Fatalf("unexpected file_id in rewritten payload: %s", executor.payload)
	}
	if _, err := store.GetMetadata(record.ID); !errors.Is(err, internalfiles.ErrNotFound) {
		t.Fatalf("GetMetadata after successful chat request error = %v, want ErrNotFound", err)
	}
}

func TestChatCompletionsConvertsResponsesFilePayloadToChatFilePayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := internalfiles.NewStore(t.TempDir())
	record, err := store.Create(internalfiles.CreateParams{
		Filename: "notes.txt",
		Purpose:  "assistants",
		MIMEType: "text/plain",
		Data:     []byte("hello"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	base, executor := newOpenAIHandlerTestBase(t, store)
	h := NewOpenAIAPIHandler(base)

	router := gin.New()
	router.POST("/v1/chat/completions", h.ChatCompletions)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"test-model","input":[{"type":"message","role":"user","content":[{"type":"input_file","file_id":"`+record.ID+`"}]}]}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", resp.Code, http.StatusOK, resp.Body.String())
	}
	if got := gjson.Get(executor.payload, "messages.0.content.0.type").String(); got != "file" {
		t.Fatalf("content type = %q, want %q", got, "file")
	}
	if got := gjson.Get(executor.payload, "messages.0.content.0.file.file_data").String(); got != "data:text/plain;base64,aGVsbG8=" {
		t.Fatalf("file_data = %q, want %q", got, "data:text/plain;base64,aGVsbG8=")
	}
	if _, err := store.GetMetadata(record.ID); !errors.Is(err, internalfiles.ErrNotFound) {
		t.Fatalf("GetMetadata after successful converted chat request error = %v, want ErrNotFound", err)
	}
}

func newOpenAIHandlerTestBase(t *testing.T, store *internalfiles.Store) (*handlers.BaseAPIHandler, *capturePayloadExecutor) {
	t.Helper()

	executor := &capturePayloadExecutor{}
	manager := coreauth.NewManager(nil, nil, nil)
	manager.RegisterExecutor(executor)
	authID := strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	auth := &coreauth.Auth{ID: authID, Provider: executor.Identifier(), Status: coreauth.StatusActive}
	if _, err := manager.Register(context.Background(), auth); err != nil {
		t.Fatalf("Register auth: %v", err)
	}
	registry.GetGlobalRegistry().RegisterClient(auth.ID, auth.Provider, []*registry.ModelInfo{{ID: "test-model"}})
	t.Cleanup(func() {
		registry.GetGlobalRegistry().UnregisterClient(auth.ID)
	})

	base := handlers.NewBaseAPIHandlers(&sdkconfig.SDKConfig{}, manager)
	base.UploadedFileStore = store
	return base, executor
}
