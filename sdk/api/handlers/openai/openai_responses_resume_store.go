package openai

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const defaultResponsesRouteCacheTTL = 15 * time.Minute

type selectedAuthTracker struct {
	mu sync.RWMutex
	id string
}

func (t *selectedAuthTracker) Set(authID string) {
	authID = strings.TrimSpace(authID)
	if authID == "" {
		return
	}
	t.mu.Lock()
	t.id = authID
	t.mu.Unlock()
}

func (t *selectedAuthTracker) Get() string {
	if t == nil {
		return ""
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.id
}

type responsesRouteEntry struct {
	ResponseID string
	AuthID     string
	Owner      string
	CreatedAt  time.Time
	ExpiresAt  time.Time
}

type responsesRouteStore struct {
	mu      sync.RWMutex
	ttl     time.Duration
	entries map[string]responsesRouteEntry
}

func newResponsesRouteStore(ttl time.Duration) *responsesRouteStore {
	if ttl <= 0 {
		ttl = defaultResponsesRouteCacheTTL
	}
	return &responsesRouteStore{
		ttl:     ttl,
		entries: make(map[string]responsesRouteEntry),
	}
}

func (s *responsesRouteStore) Remember(responseID, authID, owner string, now time.Time) {
	if s == nil {
		return
	}
	responseID = strings.TrimSpace(responseID)
	authID = strings.TrimSpace(authID)
	if responseID == "" || authID == "" {
		return
	}
	if now.IsZero() {
		now = time.Now()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(now)
	s.entries[responseID] = responsesRouteEntry{
		ResponseID: responseID,
		AuthID:     authID,
		Owner:      owner,
		CreatedAt:  now,
		ExpiresAt:  now.Add(s.ttl),
	}
}

func (s *responsesRouteStore) Get(responseID, owner string, now time.Time) (responsesRouteEntry, bool) {
	if s == nil {
		return responsesRouteEntry{}, false
	}
	responseID = strings.TrimSpace(responseID)
	if responseID == "" {
		return responsesRouteEntry{}, false
	}
	if now.IsZero() {
		now = time.Now()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[responseID]
	if !ok {
		return responsesRouteEntry{}, false
	}
	if !entry.ExpiresAt.IsZero() && now.After(entry.ExpiresAt) {
		delete(s.entries, responseID)
		return responsesRouteEntry{}, false
	}
	if entry.Owner != owner {
		return responsesRouteEntry{}, false
	}
	return entry, true
}

func (s *responsesRouteStore) pruneLocked(now time.Time) {
	for responseID, entry := range s.entries {
		if !entry.ExpiresAt.IsZero() && now.After(entry.ExpiresAt) {
			delete(s.entries, responseID)
		}
	}
}

func responsesRouteOwner(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if value, ok := c.Get("userApiKey"); ok {
		return strings.TrimSpace(fmt.Sprint(value))
	}
	return ""
}
