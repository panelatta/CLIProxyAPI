package openai

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	internalfiles "github.com/router-for-me/CLIProxyAPI/v6/internal/files"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/api/handlers"
	log "github.com/sirupsen/logrus"
)

type fileReferenceTracker struct {
	ids map[string]struct{}
}

func (t *fileReferenceTracker) Add(id string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	if t.ids == nil {
		t.ids = make(map[string]struct{})
	}
	t.ids[id] = struct{}{}
}

func (t *fileReferenceTracker) List() []string {
	if len(t.ids) == 0 {
		return nil
	}
	out := make([]string, 0, len(t.ids))
	for id := range t.ids {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func expandResponsesFileReferences(rawJSON []byte, store *internalfiles.Store) ([]byte, []string, error) {
	if !bytes.Contains(rawJSON, []byte(`"file_id"`)) {
		return rawJSON, nil, nil
	}

	var root map[string]any
	if err := json.Unmarshal(rawJSON, &root); err != nil {
		return nil, nil, fmt.Errorf("invalid request JSON: %w", err)
	}

	tracker := &fileReferenceTracker{}
	changed, err := walkResponsesValue(root, store, tracker)
	if err != nil {
		return nil, nil, err
	}
	if !changed {
		return rawJSON, nil, nil
	}
	expanded, err := json.Marshal(root)
	if err != nil {
		return nil, nil, err
	}
	return expanded, tracker.List(), nil
}

func expandChatCompletionsFileReferences(rawJSON []byte, store *internalfiles.Store) ([]byte, []string, error) {
	if !bytes.Contains(rawJSON, []byte(`"file_id"`)) {
		return rawJSON, nil, nil
	}

	var root map[string]any
	if err := json.Unmarshal(rawJSON, &root); err != nil {
		return nil, nil, fmt.Errorf("invalid request JSON: %w", err)
	}

	tracker := &fileReferenceTracker{}
	changed, err := walkChatValue(root, store, tracker)
	if err != nil {
		return nil, nil, err
	}
	if !changed {
		return rawJSON, nil, nil
	}
	expanded, err := json.Marshal(root)
	if err != nil {
		return nil, nil, err
	}
	return expanded, tracker.List(), nil
}

func walkResponsesValue(value any, store *internalfiles.Store, tracker *fileReferenceTracker) (bool, error) {
	switch node := value.(type) {
	case map[string]any:
		changed := false
		if strings.TrimSpace(stringValue(node["type"])) == "input_file" {
			didChange, err := expandResponsesFilePart(node, store, tracker)
			if err != nil {
				return false, err
			}
			changed = changed || didChange
		}
		for _, child := range node {
			didChange, err := walkResponsesValue(child, store, tracker)
			if err != nil {
				return false, err
			}
			changed = changed || didChange
		}
		return changed, nil
	case []any:
		changed := false
		for i := range node {
			didChange, err := walkResponsesValue(node[i], store, tracker)
			if err != nil {
				return false, err
			}
			changed = changed || didChange
		}
		return changed, nil
	default:
		return false, nil
	}
}

func walkChatValue(value any, store *internalfiles.Store, tracker *fileReferenceTracker) (bool, error) {
	switch node := value.(type) {
	case map[string]any:
		changed := false
		switch strings.TrimSpace(stringValue(node["type"])) {
		case "file", "input_file":
			didChange, err := expandChatFilePart(node, store, tracker)
			if err != nil {
				return false, err
			}
			changed = changed || didChange
		}
		for _, child := range node {
			didChange, err := walkChatValue(child, store, tracker)
			if err != nil {
				return false, err
			}
			changed = changed || didChange
		}
		return changed, nil
	case []any:
		changed := false
		for i := range node {
			didChange, err := walkChatValue(node[i], store, tracker)
			if err != nil {
				return false, err
			}
			changed = changed || didChange
		}
		return changed, nil
	default:
		return false, nil
	}
}

func expandResponsesFilePart(node map[string]any, store *internalfiles.Store, tracker *fileReferenceTracker) (bool, error) {
	if strings.TrimSpace(stringValue(node["file_data"])) != "" {
		return false, nil
	}

	fileID := referencedFileID(node)
	if fileID == "" {
		return false, nil
	}

	record, dataURL, err := resolveReferencedFile(store, fileID)
	if err != nil {
		return false, err
	}
	tracker.Add(fileID)

	node["file_data"] = dataURL
	if record.Filename != "" {
		node["filename"] = record.Filename
	}
	delete(node, "file_id")
	delete(node, "file")
	return true, nil
}

func expandChatFilePart(node map[string]any, store *internalfiles.Store, tracker *fileReferenceTracker) (bool, error) {
	fileMap, _ := node["file"].(map[string]any)
	if fileMap != nil && strings.TrimSpace(stringValue(fileMap["file_data"])) != "" {
		return false, nil
	}

	fileID := referencedFileID(node)
	if fileID == "" {
		return false, nil
	}

	record, dataURL, err := resolveReferencedFile(store, fileID)
	if err != nil {
		return false, err
	}
	tracker.Add(fileID)

	if fileMap == nil {
		fileMap = make(map[string]any)
	}
	fileMap["file_data"] = dataURL
	if record.Filename != "" {
		fileMap["filename"] = record.Filename
	}
	delete(fileMap, "file_id")

	node["type"] = "file"
	node["file"] = fileMap
	delete(node, "file_id")
	delete(node, "file_data")
	delete(node, "filename")
	return true, nil
}

func referencedFileID(node map[string]any) string {
	if fileID := strings.TrimSpace(stringValue(node["file_id"])); fileID != "" {
		return fileID
	}
	fileMap, _ := node["file"].(map[string]any)
	if fileMap == nil {
		return ""
	}
	return strings.TrimSpace(stringValue(fileMap["file_id"]))
}

func resolveReferencedFile(store *internalfiles.Store, fileID string) (*internalfiles.Record, string, error) {
	if store == nil {
		return nil, "", fmt.Errorf("uploaded file store is not configured")
	}
	record, data, err := store.Get(fileID)
	if err != nil {
		return nil, "", err
	}
	mimeType := strings.TrimSpace(record.MIMEType)
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	return record, "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

func cleanupUploadedFilesAfterSuccess(store *internalfiles.Store, fileIDs []string) {
	if store == nil || len(fileIDs) == 0 {
		return
	}
	for _, fileID := range fileIDs {
		if err := store.Delete(fileID); err != nil && !errors.Is(err, internalfiles.ErrNotFound) && !errors.Is(err, internalfiles.ErrExpired) {
			log.Warnf("Failed to delete uploaded file %s after successful upstream request: %v", fileID, err)
		}
	}
}

func stringValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	default:
		return ""
	}
}

func writeOpenAIRequestError(c *gin.Context, status int, message string) {
	errorType := "invalid_request_error"
	if status >= http.StatusInternalServerError {
		errorType = "server_error"
	}
	c.AbortWithStatusJSON(status, handlers.ErrorResponse{
		Error: handlers.ErrorDetail{
			Message: message,
			Type:    errorType,
		},
	})
}

func writeOpenAIFileReferenceError(c *gin.Context, err error) {
	if err == nil {
		writeOpenAIRequestError(c, http.StatusBadRequest, "invalid file reference")
		return
	}

	status := http.StatusBadRequest
	message := err.Error()
	switch {
	case errors.Is(err, internalfiles.ErrNotFound):
		message = "unknown file_id"
	case errors.Is(err, internalfiles.ErrExpired):
		message = "file_id has expired"
	case errors.Is(err, internalfiles.ErrInvalidID):
		message = "invalid file_id"
	default:
		if strings.TrimSpace(message) == "" {
			status = http.StatusInternalServerError
			message = "failed to resolve file reference"
		} else if !strings.Contains(strings.ToLower(message), "file") &&
			!strings.Contains(strings.ToLower(message), "json") {
			status = http.StatusInternalServerError
			message = "failed to resolve file reference"
		}
	}

	writeOpenAIRequestError(c, status, message)
}
