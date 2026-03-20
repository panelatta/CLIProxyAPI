package files

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreCreateGetDelete(t *testing.T) {
	store := NewStore(t.TempDir())
	record, err := store.Create(CreateParams{
		Filename: "notes.txt",
		Purpose:  "assistants",
		MIMEType: "text/plain",
		Data:     []byte("hello"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if record.ID == "" {
		t.Fatal("expected generated file id")
	}
	if record.Filename != "notes.txt" {
		t.Fatalf("filename = %q, want %q", record.Filename, "notes.txt")
	}

	loaded, data, err := store.Get(record.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if loaded.ID != record.ID {
		t.Fatalf("loaded id = %q, want %q", loaded.ID, record.ID)
	}
	if string(data) != "hello" {
		t.Fatalf("payload = %q, want %q", string(data), "hello")
	}

	if err = store.Delete(record.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, _, err = store.Get(record.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after delete error = %v, want ErrNotFound", err)
	}
}

func TestStoreExpiresAndCleansUp(t *testing.T) {
	current := time.Unix(1710000000, 0)
	store := NewStore(t.TempDir(), WithTTL(time.Hour), WithNow(func() time.Time { return current }))
	record, err := store.Create(CreateParams{
		Filename: "expired.txt",
		Purpose:  "assistants",
		MIMEType: "text/plain",
		Data:     []byte("stale"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	current = current.Add(2 * time.Hour)
	if _, err = store.GetMetadata(record.ID); !errors.Is(err, ErrExpired) {
		t.Fatalf("GetMetadata error = %v, want ErrExpired", err)
	}
	if _, statErr := os.Stat(filepath.Join(store.BaseDir(), record.ID+".json")); !os.IsNotExist(statErr) {
		t.Fatalf("metadata file still exists, stat err = %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(store.BaseDir(), record.ID+".bin")); !os.IsNotExist(statErr) {
		t.Fatalf("payload file still exists, stat err = %v", statErr)
	}
}
