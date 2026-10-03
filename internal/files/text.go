package files

import (
	"bytes"
	"fmt"
	"mime"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// UploadedText expands textual uploads without relying on provider document
// parsers. Binary documents, including PDFs, retain their native file transport.
func UploadedText(record *Record, data []byte) (string, bool, error) {
	mediaType, _, errParse := mime.ParseMediaType(record.MIMEType)
	if errParse != nil {
		mediaType = strings.ToLower(strings.TrimSpace(record.MIMEType))
	}
	textual := strings.HasPrefix(mediaType, "text/") ||
		strings.HasSuffix(mediaType, "+json") || strings.HasSuffix(mediaType, "+xml")
	switch mediaType {
	case "application/json", "application/xml", "application/javascript", "application/yaml",
		"application/x-yaml", "application/toml", "application/x-sh", "application/sql":
		textual = true
	case "", "application/octet-stream":
		switch strings.ToLower(filepath.Ext(record.Filename)) {
		case ".txt", ".md", ".markdown", ".csv", ".tsv", ".log", ".json", ".jsonl",
			".yaml", ".yml", ".xml", ".html", ".htm", ".toml", ".ini", ".cfg", ".conf",
			".properties", ".env", ".sql", ".py", ".go", ".js", ".ts", ".tsx", ".jsx",
			".sh", ".bash", ".rs", ".java", ".c", ".h", ".cpp", ".css":
			textual = true
		}
	}
	if !textual {
		return "", false, nil
	}
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return "", true, fmt.Errorf("uploaded text file %q must contain valid UTF-8 text without NUL bytes", record.Filename)
	}
	return fmt.Sprintf("Contents of uploaded file %q:\n%s", record.Filename, data), true, nil
}
