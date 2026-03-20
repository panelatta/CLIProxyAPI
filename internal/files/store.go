package files

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	DefaultTTL            = 72 * time.Hour
	DefaultMaxUploadBytes = 20 << 20
)

var (
	ErrNotFound  = errors.New("uploaded file not found")
	ErrExpired   = errors.New("uploaded file expired")
	ErrInvalidID = errors.New("invalid uploaded file id")
	ErrTooLarge  = errors.New("uploaded file exceeds the maximum allowed size")
	ErrEmptyFile = errors.New("uploaded file is empty")
)

type Record struct {
	ID        string `json:"id"`
	Filename  string `json:"filename"`
	Purpose   string `json:"purpose"`
	MIMEType  string `json:"mime_type"`
	SizeBytes int64  `json:"size_bytes"`
	CreatedAt int64  `json:"created_at"`
	Path      string `json:"path"`
}

type CreateParams struct {
	Filename string
	Purpose  string
	MIMEType string
	Data     []byte
}

type Option func(*Store)

type Store struct {
	mu             sync.Mutex
	baseDir        string
	ttl            time.Duration
	maxUploadBytes int64
	now            func() time.Time
}

func NewStore(baseDir string, opts ...Option) *Store {
	s := &Store{
		baseDir:        filepath.Clean(strings.TrimSpace(baseDir)),
		ttl:            DefaultTTL,
		maxUploadBytes: DefaultMaxUploadBytes,
		now:            time.Now,
	}
	for i := range opts {
		if opts[i] != nil {
			opts[i](s)
		}
	}
	return s
}

func WithTTL(ttl time.Duration) Option {
	return func(s *Store) {
		if ttl > 0 {
			s.ttl = ttl
		}
	}
}

func WithMaxUploadBytes(limit int64) Option {
	return func(s *Store) {
		if limit > 0 {
			s.maxUploadBytes = limit
		}
	}
}

func WithNow(now func() time.Time) Option {
	return func(s *Store) {
		if now != nil {
			s.now = now
		}
	}
}

func (s *Store) BaseDir() string {
	if s == nil {
		return ""
	}
	return s.baseDir
}

func (s *Store) MaxUploadBytes() int64 {
	if s == nil || s.maxUploadBytes <= 0 {
		return DefaultMaxUploadBytes
	}
	return s.maxUploadBytes
}

func (s *Store) Create(params CreateParams) (*Record, error) {
	if s == nil {
		return nil, fmt.Errorf("uploaded file store is not configured")
	}

	data := append([]byte(nil), params.Data...)
	if len(data) == 0 {
		return nil, ErrEmptyFile
	}
	if int64(len(data)) > s.MaxUploadBytes() {
		return nil, ErrTooLarge
	}

	filename := sanitizeFilename(params.Filename)
	purpose := strings.TrimSpace(params.Purpose)
	if purpose == "" {
		purpose = "assistants"
	}
	mimeType := strings.ToLower(strings.TrimSpace(params.MIMEType))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureBaseDirLocked(); err != nil {
		return nil, err
	}
	if err := s.cleanupExpiredLocked(); err != nil {
		return nil, err
	}

	var (
		id         string
		metadata   string
		payload    string
		selectErr  error
		recordTime = s.now().Unix()
	)
	for i := 0; i < 8; i++ {
		id, selectErr = generateID()
		if selectErr != nil {
			return nil, selectErr
		}
		metadata = s.metadataPath(id)
		payload = s.payloadPath(id)
		if _, err := os.Stat(metadata); os.IsNotExist(err) {
			selectErr = nil
			break
		} else if err != nil {
			return nil, fmt.Errorf("uploaded file store: stat metadata failed: %w", err)
		}
		selectErr = fmt.Errorf("uploaded file store: id collision for %s", id)
	}
	if selectErr != nil {
		return nil, selectErr
	}

	record := &Record{
		ID:        id,
		Filename:  filename,
		Purpose:   purpose,
		MIMEType:  mimeType,
		SizeBytes: int64(len(data)),
		CreatedAt: recordTime,
		Path:      payload,
	}

	if err := os.WriteFile(payload, data, 0o600); err != nil {
		return nil, fmt.Errorf("uploaded file store: write payload failed: %w", err)
	}

	raw, err := json.Marshal(record)
	if err != nil {
		_ = os.Remove(payload)
		return nil, fmt.Errorf("uploaded file store: marshal metadata failed: %w", err)
	}
	if err = os.WriteFile(metadata, raw, 0o600); err != nil {
		_ = os.Remove(payload)
		return nil, fmt.Errorf("uploaded file store: write metadata failed: %w", err)
	}

	return record, nil
}

func (s *Store) Get(id string) (*Record, []byte, error) {
	record, err := s.GetMetadata(id)
	if err != nil {
		return nil, nil, err
	}
	data, err := os.ReadFile(record.Path)
	if err != nil {
		if os.IsNotExist(err) {
			_ = s.Delete(id)
			return nil, nil, ErrNotFound
		}
		return nil, nil, fmt.Errorf("uploaded file store: read payload failed: %w", err)
	}
	return record, data, nil
}

func (s *Store) GetMetadata(id string) (*Record, error) {
	if s == nil {
		return nil, fmt.Errorf("uploaded file store is not configured")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureBaseDirLocked(); err != nil {
		return nil, err
	}

	record, err := s.readRecordLocked(id)
	if err != nil {
		return nil, err
	}
	if s.isExpired(record) {
		_ = s.deleteRecordFilesLocked(record)
		return nil, ErrExpired
	}
	return record, nil
}

func (s *Store) Delete(id string) error {
	if s == nil {
		return fmt.Errorf("uploaded file store is not configured")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	record, err := s.readRecordLocked(id)
	if err != nil {
		if errors.Is(err, ErrNotFound) || errors.Is(err, ErrInvalidID) {
			return err
		}
		if errors.Is(err, ErrExpired) {
			return nil
		}
		return err
	}
	return s.deleteRecordFilesLocked(record)
}

func (s *Store) CleanupExpired() error {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cleanupExpiredLocked()
}

func (s *Store) cleanupExpiredLocked() error {
	if err := s.ensureBaseDirLocked(); err != nil {
		return err
	}

	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("uploaded file store: read dir failed: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		record, err := s.readRecordLocked(id)
		if err != nil {
			if errors.Is(err, ErrNotFound) || errors.Is(err, ErrInvalidID) {
				continue
			}
			return err
		}
		if s.isExpired(record) {
			if err = s.deleteRecordFilesLocked(record); err != nil && !errors.Is(err, ErrNotFound) {
				return err
			}
		}
	}

	return nil
}

func (s *Store) ensureBaseDirLocked() error {
	if strings.TrimSpace(s.baseDir) == "" {
		return fmt.Errorf("uploaded file store: base directory is empty")
	}
	if err := os.MkdirAll(s.baseDir, 0o700); err != nil {
		return fmt.Errorf("uploaded file store: create base directory failed: %w", err)
	}
	return nil
}

func (s *Store) readRecordLocked(id string) (*Record, error) {
	normalizedID, err := normalizeID(id)
	if err != nil {
		return nil, err
	}

	raw, err := os.ReadFile(s.metadataPath(normalizedID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("uploaded file store: read metadata failed: %w", err)
	}

	var record Record
	if err = json.Unmarshal(raw, &record); err != nil {
		return nil, fmt.Errorf("uploaded file store: unmarshal metadata failed: %w", err)
	}
	if record.ID == "" {
		record.ID = normalizedID
	}
	if record.Path == "" {
		record.Path = s.payloadPath(record.ID)
	}
	return &record, nil
}

func (s *Store) deleteRecordFilesLocked(record *Record) error {
	if record == nil {
		return ErrNotFound
	}
	payloadPath := strings.TrimSpace(record.Path)
	if payloadPath == "" {
		payloadPath = s.payloadPath(record.ID)
	}
	metadataPath := s.metadataPath(record.ID)

	var firstErr error
	if err := os.Remove(payloadPath); err != nil && !os.IsNotExist(err) {
		firstErr = fmt.Errorf("uploaded file store: delete payload failed: %w", err)
	}
	if err := os.Remove(metadataPath); err != nil && !os.IsNotExist(err) && firstErr == nil {
		firstErr = fmt.Errorf("uploaded file store: delete metadata failed: %w", err)
	}
	return firstErr
}

func (s *Store) isExpired(record *Record) bool {
	if record == nil || s == nil || s.ttl <= 0 {
		return false
	}
	if record.CreatedAt <= 0 {
		return false
	}
	createdAt := time.Unix(record.CreatedAt, 0)
	return s.now().After(createdAt.Add(s.ttl))
}

func (s *Store) metadataPath(id string) string {
	return filepath.Join(s.baseDir, id+".json")
}

func (s *Store) payloadPath(id string) string {
	return filepath.Join(s.baseDir, id+".bin")
}

func normalizeID(id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", ErrInvalidID
	}
	if strings.ContainsAny(id, `/\\`) || filepath.Base(id) != id {
		return "", ErrInvalidID
	}
	if !strings.HasPrefix(id, "file_cpa_") {
		return "", ErrInvalidID
	}
	return id, nil
}

func sanitizeFilename(name string) string {
	name = strings.TrimSpace(filepath.Base(name))
	if name == "." || name == string(filepath.Separator) || name == "" {
		return "upload.bin"
	}
	return name
}

func generateID() (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("uploaded file store: generate id failed: %w", err)
	}
	return "file_cpa_" + hex.EncodeToString(buf), nil
}
