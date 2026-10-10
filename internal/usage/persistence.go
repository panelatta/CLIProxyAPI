package usage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

type historyPayload struct {
	Version int                `json:"version"`
	Usage   StatisticsSnapshot `json:"usage"`
}

// StartPersistence restores history before serving requests. The returned stop
// function must run after the usage queue drains. A corrupt file is never overwritten.
func (s *RequestStatistics) StartPersistence(path string) (func(), error) {
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read usage history: %w", err)
	}
	if err == nil {
		var payload historyPayload
		if errDecode := json.Unmarshal(data, &payload); errDecode != nil {
			return nil, fmt.Errorf("decode usage history: %w", errDecode)
		}
		if payload.Version != 1 {
			return nil, fmt.Errorf("unsupported usage history version %d", payload.Version)
		}
		s.MergeSnapshot(payload.Usage)
	}
	if errMkdir := os.MkdirAll(filepath.Dir(path), 0o700); errMkdir != nil {
		return nil, errMkdir
	}
	lastTotal := int64(-1)
	flush := func() error {
		s.mu.RLock()
		total := s.totalRequests
		s.mu.RUnlock()
		if total == lastTotal {
			return nil
		}
		snapshot := s.Snapshot()
		if errSave := saveHistory(path, snapshot); errSave != nil {
			return errSave
		}
		lastTotal = snapshot.TotalRequests
		return nil
	}
	if errFlush := flush(); errFlush != nil {
		return nil, errFlush
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if errFlush := flush(); errFlush != nil {
					log.Errorf("save usage history: %v", errFlush)
				}
			case <-stop:
				if errFlush := flush(); errFlush != nil {
					log.Errorf("save usage history on shutdown: %v", errFlush)
				}
				return
			}
		}
	}()
	var once sync.Once
	return func() { once.Do(func() { close(stop) }); <-done }, nil
}

func saveHistory(path string, snapshot StatisticsSnapshot) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".usage-history-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(file.Name()) }()
	errEncode := json.NewEncoder(file).Encode(historyPayload{Version: 1, Usage: snapshot})
	if errEncode == nil {
		errEncode = file.Sync()
	}
	errClose := file.Close()
	if errEncode != nil {
		return errEncode
	}
	if errClose != nil {
		return errClose
	}
	return os.Rename(file.Name(), path)
}
