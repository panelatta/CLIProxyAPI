package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestV1FilesRouteRegistered(t *testing.T) {
	server := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/v1/files", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer test-key")
	req.Header.Set("Content-Type", "application/json")

	resp := httptest.NewRecorder()
	server.engine.ServeHTTP(resp, req)

	if resp.Code == http.StatusNotFound {
		t.Fatalf("expected /v1/files route to be registered, got 404 body=%s", resp.Body.String())
	}
}
