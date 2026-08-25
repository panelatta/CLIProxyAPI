package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	internalfiles "github.com/router-for-me/CLIProxyAPI/v7/internal/files"
)

func TestUpdateClientsContextReconfiguresUploadedFileStore(t *testing.T) {
	server := newTestServer(t)
	originalStore := server.handlers.UploadedFileStore
	if originalStore == nil {
		t.Fatal("uploaded file store is not configured")
	}

	nextAuthDir := filepath.Join(t.TempDir(), "next-auth")
	nextCfg := *server.cfg
	nextCfg.AuthDir = nextAuthDir
	if updated := server.UpdateClientsContext(context.Background(), &nextCfg); !updated {
		t.Fatal("UpdateClientsContext returned false")
	}

	if server.handlers.UploadedFileStore != originalStore {
		t.Fatal("uploaded file store pointer changed during hot reload")
	}
	wantBaseDir := filepath.Join(nextAuthDir, "uploaded-files")
	if gotBaseDir := originalStore.BaseDir(); gotBaseDir != wantBaseDir {
		t.Fatalf("uploaded file store base dir = %q, want %q", gotBaseDir, wantBaseDir)
	}

	record, errCreate := originalStore.Create(internalfiles.CreateParams{
		Filename: "reload.txt",
		Purpose:  "assistants",
		MIMEType: "text/plain",
		Data:     []byte("after reload"),
	})
	if errCreate != nil {
		t.Fatalf("Create() after reload error = %v", errCreate)
	}
	if gotRecordDir := filepath.Dir(record.Path); gotRecordDir != wantBaseDir {
		t.Fatalf("uploaded record directory = %q, want %q", gotRecordDir, wantBaseDir)
	}
	if _, errStat := os.Stat(record.Path); errStat != nil {
		t.Fatalf("uploaded record was not written to reconfigured store: %v", errStat)
	}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	server.engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("health request after reload status = %d, want %d", rec.Code, http.StatusOK)
	}
}
