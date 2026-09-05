package executor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gorilla/websocket"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
)

const codexQuotaEvent = `{"type":"codex.rate_limits","rate_limits":{"primary":{"used_percent":11,"window_minutes":10080,"reset_at":1788847375}},"credits":{"has_credits":false,"unlimited":false,"balance":"0"}}`

func TestCodexQuotaBootstrapResponseHeaders(t *testing.T) {
	for _, transport := range []string{"http", "websocket"} {
		for _, buffering := range []bool{true, false} {
			t.Run(transport+"/buffering="+strconv.FormatBool(buffering), func(t *testing.T) {
				events := []string{codexCreatedEvent, codexQuotaEvent, codexOutputAddedEvent, codexCompletedEventBody}
				var server *httptest.Server
				if transport == "http" {
					server = codexSSEServer(events...)
				} else {
					server = codexWebsocketServer(t, events...)
				}
				defer server.Close()
				req, opts := codexTestRequest()
				var result *cliproxyexecutor.StreamResult
				var err error
				if transport == "http" {
					result, err = NewCodexExecutor(codexBufferingConfig(buffering)).ExecuteStream(context.Background(), codexTestAuth(server.URL), req, opts)
				} else {
					result, err = NewCodexWebsocketsExecutor(codexBufferingConfig(buffering)).ExecuteStream(context.Background(), codexTestAuth(server.URL), req, opts)
				}
				if err != nil {
					t.Fatal(err)
				}
				// Headers must be finalized before the caller starts reading chunks.
				want := ""
				if buffering {
					want = "11"
				}
				if got := result.Headers.Get("X-Codex-Primary-Used-Percent"); got != want {
					t.Fatalf("quota = %q, want %q", got, want)
				}
				if buffering && result.Headers.Get("X-Codex-Primary-Reset-At") != "1788847375" {
					t.Fatalf("missing reset: %v", result.Headers)
				}
				body, err := drainChunks(result)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(body, "response.completed") {
					t.Fatalf("response did not complete: %s", body)
				}
				if got := result.Headers.Get("X-Codex-Primary-Used-Percent"); got != want {
					t.Fatalf("headers mutated after publication: %v", result.Headers)
				}
			})
		}
	}
}

func TestCodexQuotaHeadersOnReusedWebsocket(t *testing.T) {
	var connections atomic.Int32
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		connections.Add(1)
		defer func() { _ = conn.Close() }()
		for i := 0; i < 2; i++ {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
			quota := strings.Replace(codexQuotaEvent, `"used_percent":11`, `"used_percent":`+strconv.Itoa(11+i), 1)
			for _, event := range []string{quota, codexCreatedEvent, codexOutputAddedEvent, codexCompletedEventBody} {
				if err := conn.WriteMessage(websocket.TextMessage, []byte(event)); err != nil {
					return
				}
			}
		}
	}))
	defer server.Close()
	executor := NewCodexWebsocketsExecutor(codexBufferingConfig(true))
	req, opts := codexTestRequest()
	opts.Metadata = map[string]any{cliproxyexecutor.ExecutionSessionMetadataKey: "quota-reuse-test"}
	defer executor.CloseExecutionSession("quota-reuse-test")
	auth := codexTestAuth(server.URL)
	auth.ID = "quota-test-auth"
	for i := 0; i < 2; i++ {
		result, err := executor.ExecuteStream(context.Background(), auth, req, opts)
		if err != nil {
			t.Fatal(err)
		}
		if got := result.Headers.Get("X-Codex-Primary-Used-Percent"); got != strconv.Itoa(11+i) {
			t.Fatalf("turn %d quota = %q", i, got)
		}
		if _, err := drainChunks(result); err != nil {
			t.Fatal(err)
		}
	}
	if got := connections.Load(); got != 1 {
		t.Fatalf("expected reused connection, got %d", got)
	}
}

func TestCodexQuotaWebsocketEventStillForwarded(t *testing.T) {
	server := codexWebsocketServer(t, codexQuotaEvent, codexCreatedEvent, codexOutputAddedEvent, codexCompletedEventBody)
	defer server.Close()
	req, opts := codexTestRequest()
	ctx := cliproxyexecutor.WithDownstreamWebsocket(context.Background())
	result, err := NewCodexWebsocketsExecutor(codexBufferingConfig(true)).ExecuteStream(ctx, codexTestAuth(server.URL), req, opts)
	if err != nil {
		t.Fatal(err)
	}
	body, err := drainChunks(result)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, codexQuotaEvent) {
		t.Fatalf("quota event lost: %s", body)
	}
}
