package executor

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/runtime/executor/helps"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

const (
	codexImageGenerationsURL   = "https://api.openai.com/v1/images/generations"
	codexBackendDefaultBaseURL = "https://chatgpt.com/backend-api/codex"
	codexModelCapacityCooldown = 30 * time.Minute
)

// CodexExecutor is a stateless executor for Codex (OpenAI Responses API entrypoint).
// If api_key is unavailable on auth, it falls back to legacy via ClientAdapter.
type CodexExecutor struct {
	cfg *config.Config
}

func NewCodexExecutor(cfg *config.Config) *CodexExecutor { return &CodexExecutor{cfg: cfg} }

func (e *CodexExecutor) Identifier() string { return "codex" }

// ResponsesHTTPRequest retrieves or resumes a Codex/OpenAI Responses object.
func (e *CodexExecutor) ResponsesHTTPRequest(ctx context.Context, auth *cliproxyauth.Auth, responseID string, query url.Values) (*http.Response, error) {
	responseID = strings.TrimSpace(responseID)
	if responseID == "" {
		return nil, fmt.Errorf("codex executor: response id is empty")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	apiKey, baseURL := codexCreds(auth)
	if baseURL == "" {
		baseURL = codexBackendDefaultBaseURL
	}

	targetURL := strings.TrimSuffix(baseURL, "/") + "/responses/" + url.PathEscape(responseID)
	if len(query) > 0 {
		parsed, err := url.Parse(targetURL)
		if err != nil {
			return nil, err
		}
		parsed.RawQuery = query.Encode()
		targetURL = parsed.String()
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, err
	}

	stream := strings.EqualFold(strings.TrimSpace(query.Get("stream")), "true")
	applyCodexHeaders(httpReq, auth, apiKey, stream, e.cfg)
	if stream {
		httpReq.Header.Set("Cache-Control", "no-cache")
	}

	var authID, authLabel, authType, authValue string
	if auth != nil {
		authID = auth.ID
		authLabel = auth.Label
		authType, authValue = auth.AccountInfo()
	}
	helps.RecordAPIRequest(ctx, e.cfg, helps.UpstreamRequestLog{
		URL:       targetURL,
		Method:    http.MethodGet,
		Headers:   httpReq.Header.Clone(),
		Provider:  e.Identifier(),
		AuthID:    authID,
		AuthLabel: authLabel,
		AuthType:  authType,
		AuthValue: authValue,
	})

	httpClient := helps.NewProxyAwareHTTPClient(ctx, e.cfg, auth, 0)
	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		helps.RecordAPIResponseError(ctx, e.cfg, err)
		return nil, err
	}
	helps.RecordAPIResponseMetadata(ctx, e.cfg, httpResp.StatusCode, httpResp.Header.Clone())
	return httpResp, nil
}

func codexImageGenerationsEndpoint(baseURL string) string {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return codexImageGenerationsURL
	}
	return strings.TrimSuffix(baseURL, "/") + "/images/generations"
}
