package openai

import (
	"bytes"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	internalfiles "github.com/router-for-me/CLIProxyAPI/v6/internal/files"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/api/handlers"
	sdkconfig "github.com/router-for-me/CLIProxyAPI/v6/sdk/config"
)

func TestOpenAIFilesCreateReturnsObject(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := internalfiles.NewStore(t.TempDir())
	base := handlers.NewBaseAPIHandlers(&sdkconfig.SDKConfig{}, nil)
	base.UploadedFileStore = store
	api := NewOpenAIFilesAPIHandler(base)

	router := gin.New()
	router.POST("/v1/files", api.Create)

	req := newMultipartUploadRequest(t, "/v1/files", "report.txt", "text/plain", "hello", "assistants")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", resp.Code, http.StatusOK, resp.Body.String())
	}

	var fileObj openAIFileObject
	if err := json.Unmarshal(resp.Body.Bytes(), &fileObj); err != nil {
		t.Fatalf("Unmarshal response: %v", err)
	}
	if !strings.HasPrefix(fileObj.ID, "file_cpa_") {
		t.Fatalf("id = %q, want file_cpa_*", fileObj.ID)
	}
	if fileObj.Object != "file" {
		t.Fatalf("object = %q, want %q", fileObj.Object, "file")
	}
	if fileObj.Filename != "report.txt" {
		t.Fatalf("filename = %q, want %q", fileObj.Filename, "report.txt")
	}
	if fileObj.Purpose != "assistants" {
		t.Fatalf("purpose = %q, want %q", fileObj.Purpose, "assistants")
	}
	if fileObj.Bytes != int64(len("hello")) {
		t.Fatalf("bytes = %d, want %d", fileObj.Bytes, len("hello"))
	}
	if _, err := store.GetMetadata(fileObj.ID); err != nil {
		t.Fatalf("store.GetMetadata: %v", err)
	}
}

func TestOpenAIFilesGetAndDelete(t *testing.T) {
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

	base := handlers.NewBaseAPIHandlers(&sdkconfig.SDKConfig{}, nil)
	base.UploadedFileStore = store
	api := NewOpenAIFilesAPIHandler(base)

	router := gin.New()
	router.GET("/v1/files/:id", api.Get)
	router.DELETE("/v1/files/:id", api.Delete)

	getReq := httptest.NewRequest(http.MethodGet, "/v1/files/"+record.ID, nil)
	getResp := httptest.NewRecorder()
	router.ServeHTTP(getResp, getReq)
	if getResp.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want %d, body=%s", getResp.Code, http.StatusOK, getResp.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/v1/files/"+record.ID, nil)
	deleteResp := httptest.NewRecorder()
	router.ServeHTTP(deleteResp, deleteReq)
	if deleteResp.Code != http.StatusOK {
		t.Fatalf("DELETE status = %d, want %d, body=%s", deleteResp.Code, http.StatusOK, deleteResp.Body.String())
	}
	if _, err = store.GetMetadata(record.ID); !errors.Is(err, internalfiles.ErrNotFound) {
		t.Fatalf("GetMetadata after delete error = %v, want ErrNotFound", err)
	}
}

func TestOpenAIFilesGetUnknownReturnsNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := internalfiles.NewStore(t.TempDir())
	base := handlers.NewBaseAPIHandlers(&sdkconfig.SDKConfig{}, nil)
	base.UploadedFileStore = store
	api := NewOpenAIFilesAPIHandler(base)

	router := gin.New()
	router.GET("/v1/files/:id", api.Get)

	req := httptest.NewRequest(http.MethodGet, "/v1/files/file_cpa_missing", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body=%s", resp.Code, http.StatusNotFound, resp.Body.String())
	}
}

func newMultipartUploadRequest(t *testing.T, path, filename, contentType, content, purpose string) *http.Request {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("purpose", purpose); err != nil {
		t.Fatalf("WriteField: %v", err)
	}
	partHeader := make(textproto.MIMEHeader)
	partHeader.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
	partHeader.Set("Content-Type", contentType)
	part, err := writer.CreatePart(partHeader)
	if err != nil {
		t.Fatalf("CreatePart: %v", err)
	}
	if _, err = part.Write([]byte(content)); err != nil {
		t.Fatalf("Write file content: %v", err)
	}
	if err = writer.Close(); err != nil {
		t.Fatalf("Close writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}
