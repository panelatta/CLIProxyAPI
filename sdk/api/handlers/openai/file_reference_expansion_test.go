package openai

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	internalfiles "github.com/router-for-me/CLIProxyAPI/v8/internal/files"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/registry"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/api/handlers"
	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	coreexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	sdkconfig "github.com/router-for-me/CLIProxyAPI/v8/sdk/config"
	"github.com/tidwall/gjson"
)

func TestMixedUploadedTextAndPDFPreserveOrderAndBinaryTransport(t *testing.T) {
	store := internalfiles.NewStore(t.TempDir())
	textRecord, err := store.Create(internalfiles.CreateParams{
		Filename: "notes.txt", MIMEType: "text/plain", Data: []byte("校验码 42\nsecond line"),
	})
	if err != nil {
		t.Fatal(err)
	}
	pdfData := []byte("%PDF-1.4\nopaque binary document\x00")
	pdfRecord, err := store.Create(internalfiles.CreateParams{
		Filename: "report.pdf", MIMEType: "application/pdf", Data: pdfData,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, responses := range []bool{true, false} {
		var expanded []byte
		var ids []string
		var errExpand error
		var prefix, textType, binaryPath string
		if responses {
			expanded, ids, errExpand = expandResponsesFileReferences([]byte(`{"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"Read both files"},{"type":"input_file","file_id":"`+textRecord.ID+`"},{"type":"input_file","file_id":"`+pdfRecord.ID+`"}]}]}`), store)
			prefix, textType, binaryPath = "input.0.content", "input_text", "2.file_data"
		} else {
			expanded, ids, errExpand = expandChatCompletionsFileReferences([]byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"Read both files"},{"type":"file","file":{"file_id":"`+textRecord.ID+`"}},{"type":"file","file":{"file_id":"`+pdfRecord.ID+`"}}]}]}`), store)
			prefix, textType, binaryPath = "messages.0.content", "text", "2.file.file_data"
		}
		if errExpand != nil || len(ids) != 2 {
			t.Fatalf("ids=%v err=%v", ids, errExpand)
		}
		parts := gjson.GetBytes(expanded, prefix)
		if parts.Get("0.text").String() != "Read both files" || parts.Get("1.type").String() != textType ||
			parts.Get("1.text").String() != "Contents of uploaded file \"notes.txt\":\n校验码 42\nsecond line" {
			t.Fatalf("text content lost or reordered: %s", parts.Raw)
		}
		if got := parts.Get(binaryPath).String(); got != "data:application/pdf;base64,"+base64.StdEncoding.EncodeToString(pdfData) {
			t.Fatalf("binary file changed: %q", got)
		}
		if strings.Contains(string(expanded), "file_id") || parts.Get("1.file").Exists() || parts.Get("1.file_data").Exists() {
			t.Fatalf("stale file reference: %s", expanded)
		}
	}
}

func TestInvalidUploadedTextReturnsErrorWithoutConsumingFile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := internalfiles.NewStore(t.TempDir())
	record, err := store.Create(internalfiles.CreateParams{
		Filename: "invalid.txt", MIMEType: "text/plain", Data: []byte{0xff},
	})
	if err != nil {
		t.Fatal(err)
	}
	base, executor := newOpenAIHandlerTestBase(t, store)
	h := NewOpenAIResponsesAPIHandler(base)
	router := gin.New()
	router.POST("/v1/responses", h.Responses)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"test-model","input":[{"role":"user","content":[{"type":"input_file","file_id":"`+record.ID+`"}]}]}`))
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest || executor.calls != 0 || !strings.Contains(resp.Body.String(), "UTF-8") {
		t.Fatalf("status=%d calls=%d body=%s", resp.Code, executor.calls, resp.Body.String())
	}
	if _, errMetadata := store.GetMetadata(record.ID); errMetadata != nil {
		t.Fatalf("rejected file was consumed: %v", errMetadata)
	}
}

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

func TestResponsesExpandsUploadedTextFile(t *testing.T) {
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
	if got := gjson.Get(executor.payload, "input.0.content.0.type").String(); got != "input_text" {
		t.Fatalf("content type = %q, want input_text", got)
	}
	if got := gjson.Get(executor.payload, "input.0.content.0.text").String(); got != "Contents of uploaded file \"notes.txt\":\nhello" {
		t.Fatalf("unexpected uploaded text: %q", got)
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

func TestChatCompletionsExpandsUploadedTextFile(t *testing.T) {
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
	if got := gjson.Get(executor.payload, "messages.0.content.0.text").String(); got != "Contents of uploaded file \"notes.txt\":\nhello" {
		t.Fatalf("unexpected uploaded text: %q", got)
	}
	if got := gjson.Get(executor.payload, "messages.0.content.0.type").String(); got != "text" {
		t.Fatalf("content type = %q, want text", got)
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
	if got := gjson.Get(executor.payload, "messages.0.content.0.type").String(); got != "text" {
		t.Fatalf("content type = %q, want %q", got, "text")
	}
	if got := gjson.Get(executor.payload, "messages.0.content.0.text").String(); got != "Contents of uploaded file \"notes.txt\":\nhello" {
		t.Fatalf("unexpected uploaded text: %q", got)
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
