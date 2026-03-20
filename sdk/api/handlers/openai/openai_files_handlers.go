package openai

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	internalfiles "github.com/router-for-me/CLIProxyAPI/v6/internal/files"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/api/handlers"
)

type OpenAIFilesAPIHandler struct {
	*handlers.BaseAPIHandler
}

type openAIFileObject struct {
	ID        string `json:"id"`
	Object    string `json:"object"`
	Bytes     int64  `json:"bytes"`
	CreatedAt int64  `json:"created_at"`
	Filename  string `json:"filename"`
	Purpose   string `json:"purpose"`
}

type openAIFileDeleteObject struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Deleted bool   `json:"deleted"`
}

func NewOpenAIFilesAPIHandler(apiHandlers *handlers.BaseAPIHandler) *OpenAIFilesAPIHandler {
	return &OpenAIFilesAPIHandler{BaseAPIHandler: apiHandlers}
}

func (h *OpenAIFilesAPIHandler) Create(c *gin.Context) {
	if h == nil || h.UploadedFileStore == nil {
		writeOpenAIRequestError(c, http.StatusInternalServerError, "uploaded file store is not configured")
		return
	}

	limit := h.UploadedFileStore.MaxUploadBytes()
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
	if err := c.Request.ParseMultipartForm(limit); err != nil {
		writeOpenAIRequestError(c, statusForMultipartError(err), messageForMultipartError(err))
		return
	}
	defer func() {
		if c.Request.MultipartForm != nil {
			_ = c.Request.MultipartForm.RemoveAll()
		}
	}()

	fileHeader, err := c.FormFile("file")
	if err != nil {
		writeOpenAIRequestError(c, http.StatusBadRequest, "missing multipart file field \"file\"")
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		writeOpenAIRequestError(c, http.StatusBadRequest, "failed to open uploaded file")
		return
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		writeOpenAIRequestError(c, http.StatusBadRequest, "failed to read uploaded file")
		return
	}
	if int64(len(data)) > limit {
		writeOpenAIRequestError(c, http.StatusRequestEntityTooLarge, fmt.Sprintf("uploaded file exceeds the %d byte limit", limit))
		return
	}
	if len(data) == 0 {
		writeOpenAIRequestError(c, http.StatusBadRequest, internalfiles.ErrEmptyFile.Error())
		return
	}

	mimeType, err := detectSupportedUploadMIMEType(fileHeader, data)
	if err != nil {
		writeOpenAIRequestError(c, http.StatusBadRequest, err.Error())
		return
	}

	record, err := h.UploadedFileStore.Create(internalfiles.CreateParams{
		Filename: fileHeader.Filename,
		Purpose:  c.PostForm("purpose"),
		MIMEType: mimeType,
		Data:     data,
	})
	if err != nil {
		writeOpenAIRequestError(c, statusForStoreCreateError(err), err.Error())
		return
	}

	c.JSON(http.StatusOK, openAIFileObjectFromRecord(record))
}

func (h *OpenAIFilesAPIHandler) Get(c *gin.Context) {
	if h == nil || h.UploadedFileStore == nil {
		writeOpenAIRequestError(c, http.StatusInternalServerError, "uploaded file store is not configured")
		return
	}

	record, err := h.UploadedFileStore.GetMetadata(c.Param("id"))
	if err != nil {
		writeOpenAIRequestError(c, statusForFileLookupError(err), messageForFileLookupError(err))
		return
	}

	c.JSON(http.StatusOK, openAIFileObjectFromRecord(record))
}

func (h *OpenAIFilesAPIHandler) Delete(c *gin.Context) {
	if h == nil || h.UploadedFileStore == nil {
		writeOpenAIRequestError(c, http.StatusInternalServerError, "uploaded file store is not configured")
		return
	}

	id := strings.TrimSpace(c.Param("id"))
	if _, err := h.UploadedFileStore.GetMetadata(id); err != nil {
		writeOpenAIRequestError(c, statusForFileLookupError(err), messageForFileLookupError(err))
		return
	}
	if err := h.UploadedFileStore.Delete(id); err != nil {
		writeOpenAIRequestError(c, statusForFileLookupError(err), messageForFileLookupError(err))
		return
	}

	c.JSON(http.StatusOK, openAIFileDeleteObject{
		ID:      id,
		Object:  "file",
		Deleted: true,
	})
}

func openAIFileObjectFromRecord(record *internalfiles.Record) openAIFileObject {
	return openAIFileObject{
		ID:        record.ID,
		Object:    "file",
		Bytes:     record.SizeBytes,
		CreatedAt: record.CreatedAt,
		Filename:  record.Filename,
		Purpose:   record.Purpose,
	}
}

func detectSupportedUploadMIMEType(fileHeader *multipart.FileHeader, data []byte) (string, error) {
	candidates := make([]string, 0, 3)
	if fileHeader != nil {
		if rawType := strings.TrimSpace(fileHeader.Header.Get("Content-Type")); rawType != "" {
			parsed, _, err := mime.ParseMediaType(rawType)
			if err == nil {
				candidates = append(candidates, strings.ToLower(parsed))
			} else {
				candidates = append(candidates, strings.ToLower(rawType))
			}
		}
		if ext := strings.ToLower(filepath.Ext(fileHeader.Filename)); ext != "" {
			if byExt := mime.TypeByExtension(ext); byExt != "" {
				parsed, _, err := mime.ParseMediaType(byExt)
				if err == nil {
					candidates = append(candidates, strings.ToLower(parsed))
				} else {
					candidates = append(candidates, strings.ToLower(byExt))
				}
			}
		}
	}
	if len(data) > 0 {
		candidates = append(candidates, strings.ToLower(http.DetectContentType(data)))
	}

	for _, candidate := range candidates {
		if isAllowedUploadMIMEType(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("unsupported file type")
}

func isAllowedUploadMIMEType(mimeType string) bool {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	switch mimeType {
	case "application/pdf",
		"application/json",
		"application/xml",
		"application/yaml",
		"application/x-yaml",
		"text/x-python",
		"text/x-go",
		"text/x-java-source",
		"text/x-c",
		"text/x-c++",
		"text/x-shellscript":
		return true
	}
	return strings.HasPrefix(mimeType, "text/")
}

func statusForMultipartError(err error) int {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
}

func messageForMultipartError(err error) string {
	if err == nil {
		return "invalid multipart request"
	}
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		return fmt.Sprintf("uploaded file exceeds the %d byte limit", maxBytesErr.Limit)
	}
	return "invalid multipart request"
}

func statusForStoreCreateError(err error) int {
	switch {
	case errors.Is(err, internalfiles.ErrTooLarge):
		return http.StatusRequestEntityTooLarge
	case errors.Is(err, internalfiles.ErrEmptyFile):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

func statusForFileLookupError(err error) int {
	switch {
	case errors.Is(err, internalfiles.ErrNotFound), errors.Is(err, internalfiles.ErrExpired):
		return http.StatusNotFound
	case errors.Is(err, internalfiles.ErrInvalidID):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

func messageForFileLookupError(err error) string {
	switch {
	case err == nil:
		return "file not found"
	case errors.Is(err, internalfiles.ErrNotFound), errors.Is(err, internalfiles.ErrExpired):
		return "file not found"
	case errors.Is(err, internalfiles.ErrInvalidID):
		return "invalid file id"
	default:
		return "failed to access file"
	}
}
