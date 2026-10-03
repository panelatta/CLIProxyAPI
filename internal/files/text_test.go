package files

import (
	"strings"
	"testing"
)

func TestUploadedText(t *testing.T) {
	for _, tc := range []struct {
		name, filename, mime, data string
		textual, invalid           bool
	}{
		{"UTF8 text", "notes.txt", "text/plain", "校验码 Ω\nline 2\r\n", true, false},
		{"BOM", "notes.txt", "text/plain; charset=utf-8", "\ufeffhello", true, false},
		{"JSON", "data.json", "application/json", "{\"value\":42}", true, false},
		{"structured JSON", "data.bin", "application/ld+json", "{}", true, false},
		{"generic log", "server.LOG", "application/octet-stream", "log entry", true, false},
		{"generic source", "main.go", "", "package main", true, false},
		{"PDF", "notes.pdf", "application/pdf", "%PDF-1.4", false, false},
		{"binary with misleading extension", "notes.txt", "application/pdf", "%PDF-1.4", false, false},
		{"unknown binary", "data.bin", "application/octet-stream", "\x00\xff", false, false},
		{"invalid UTF8", "notes.txt", "text/plain", "\xff", true, true},
		{"binary labeled text", "notes.txt", "text/plain", "before\x00after", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text, textual, err := UploadedText(&Record{Filename: tc.filename, MIMEType: tc.mime}, []byte(tc.data))
			if textual != tc.textual || (err != nil) != tc.invalid {
				t.Fatalf("textual=%v err=%v", textual, err)
			}
			if tc.textual && !tc.invalid {
				want := strings.TrimPrefix(tc.data, "\ufeff")
				if !strings.HasSuffix(text, "\n"+want) || !strings.Contains(text, tc.filename) {
					t.Fatalf("file content or name lost: %q", text)
				}
			}
		})
	}
}
