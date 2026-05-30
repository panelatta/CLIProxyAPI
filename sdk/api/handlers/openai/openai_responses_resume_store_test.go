package openai

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/api/handlers"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	sdkconfig "github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
)

func TestResponsesRouteStoreRemembersAndGetsSameOwner(t *testing.T) {
	store := newResponsesRouteStore(time.Minute)
	now := time.Date(2026, 5, 31, 1, 2, 3, 0, time.UTC)
	store.Remember("resp_123", "auth-b", "owner-a", now)

	entry, ok := store.Get("resp_123", "owner-a", now.Add(time.Second))
	if !ok {
		t.Fatalf("expected route entry")
	}
	if entry.AuthID != "auth-b" || entry.ResponseID != "resp_123" || entry.Owner != "owner-a" {
		t.Fatalf("unexpected entry: %+v", entry)
	}
}

func TestResponsesRouteStoreRejectsDifferentOwner(t *testing.T) {
	store := newResponsesRouteStore(time.Minute)
	now := time.Date(2026, 5, 31, 1, 2, 3, 0, time.UTC)
	store.Remember("resp_123", "auth-b", "owner-a", now)

	if _, ok := store.Get("resp_123", "owner-b", now.Add(time.Second)); ok {
		t.Fatalf("expected different owner to be rejected")
	}
}

func TestResponsesRouteStoreExpires(t *testing.T) {
	store := newResponsesRouteStore(time.Minute)
	now := time.Date(2026, 5, 31, 1, 2, 3, 0, time.UTC)
	store.Remember("resp_123", "auth-b", "owner-a", now)

	if _, ok := store.Get("resp_123", "owner-a", now.Add(2*time.Minute)); ok {
		t.Fatalf("expected expired entry to be rejected")
	}
}

func TestSelectedAuthTrackerKeepsLatestNonEmptyID(t *testing.T) {
	tracker := &selectedAuthTracker{}
	tracker.Set("auth-a")
	tracker.Set("  ")
	tracker.Set("auth-b")

	if got := tracker.Get(); got != "auth-b" {
		t.Fatalf("selected auth = %q, want auth-b", got)
	}
}

func TestResponsesSSEFramerObserverRecordsCreatedResponseBeforeCompletion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager := coreauth.NewManager(nil, nil, nil)
	auth := &coreauth.Auth{ID: "auth-b", Provider: "codex", Status: coreauth.StatusActive}
	if _, err := manager.Register(context.Background(), auth); err != nil {
		t.Fatalf("Register auth: %v", err)
	}
	base := handlers.NewBaseAPIHandlers(&sdkconfig.SDKConfig{}, manager)
	h := NewOpenAIResponsesAPIHandler(base)
	h.responseRoutes = newResponsesRouteStore(time.Minute)

	c, _ := gin.CreateTestContext(nil)
	c.Request = &http.Request{Header: make(http.Header)}
	c.Set("userApiKey", "owner-a")
	tracker := &selectedAuthTracker{}
	tracker.Set("auth-b")
	framer := &responsesSSEFramer{observePayload: h.observeResponseRoutePayload(c, tracker, true)}
	framer.WriteChunk(nil, []byte("event: response.created\ndata: {\"type\":\"response.created\",\"sequence_number\":0,\"response\":{\"id\":\"resp_123\"}}\n\n"))

	entry, ok := h.responseRoutes.Get("resp_123", "owner-a", time.Now())
	if !ok {
		t.Fatalf("expected response route to be recorded")
	}
	if entry.AuthID != "auth-b" {
		t.Fatalf("auth id = %q, want auth-b", entry.AuthID)
	}
}
