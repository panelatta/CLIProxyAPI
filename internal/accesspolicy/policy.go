// Package accesspolicy enforces client API key channel permissions.
package accesspolicy

import (
	"context"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
)

type principalKey struct{}
type principalValue struct {
	key        string
	restricted bool
}

// WithPrincipal records the authenticated client, never a client-supplied policy.
func WithPrincipal(ctx context.Context, key string, restricted bool) context.Context {
	return context.WithValue(ctx, principalKey{}, principalValue{key: key, restricted: restricted})
}

func Principal(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if principal, ok := ctx.Value(principalKey{}).(principalValue); ok {
		return principal.key
	}
	if c, ok := ctx.Value("gin").(interface{ Get(string) (any, bool) }); ok {
		key, ok := c.Get("accessClientApiKey")
		if !ok {
			key, _ = c.Get("userApiKey")
		}
		value, _ := key.(string)
		return value
	}
	return ""
}

// Policy retains each matching entry so duplicate key definitions cannot broaden access.
type Policy struct{ restrictions [][]string }

func ForKey(cfg *config.SDKConfig, key string) Policy {
	p := Policy{}
	if cfg == nil || key == "" {
		return p
	}
	for _, entry := range cfg.APIKeyEntries {
		if strings.TrimSpace(entry.APIKey) != key {
			continue
		}
		if entry.Disabled {
			p.restrictions = append(p.restrictions, []string{})
		}
		if len(entry.AllowedProviders) > 0 {
			p.restrictions = append(p.restrictions, entry.AllowedProviders)
		}
	}
	return p
}

// ForRequest also rejects a restricted key removed while a WebSocket stays open.
func ForRequest(cfg *config.SDKConfig, ctx context.Context) Policy {
	key := Principal(ctx)
	restricted := false
	if ctx != nil {
		if principal, ok := ctx.Value(principalKey{}).(principalValue); ok {
			restricted = principal.restricted
		}
		if c, ok := ctx.Value("gin").(interface{ Get(string) (any, bool) }); ok {
			value, _ := c.Get("accessProductsRestricted")
			wasRestricted, _ := value.(bool)
			restricted = restricted || wasRestricted
		}
	}
	if restricted {
		found := false
		if cfg != nil {
			for _, allowedKey := range cfg.APIKeys {
				if allowedKey == key {
					found = true
					break
				}
			}
		}
		if !found {
			return Policy{restrictions: [][]string{{}}}
		}
	}
	policy := ForKey(cfg, key)
	if policy.Restricted() && ctx != nil {
		if c, ok := ctx.Value("gin").(interface{ Set(string, any) }); ok {
			c.Set("accessProductsRestricted", true)
		}
	}
	return policy
}

func (p Policy) Restricted() bool { return len(p.restrictions) > 0 }

func (p Policy) Allows(provider string) bool {
	provider = strings.ToLower(strings.TrimSpace(provider))
	for _, allowed := range p.restrictions {
		found := false
		for _, item := range allowed {
			item = strings.ToLower(strings.TrimSpace(item))
			if item == provider || (item == "openai-compatibility" && strings.HasPrefix(provider, "openai-compatible-")) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func (p Policy) Filter(providers []string) ([]string, error) {
	if !p.Restricted() {
		return providers, nil
	}
	filtered := make([]string, 0, len(providers))
	for _, provider := range providers {
		if p.Allows(provider) {
			filtered = append(filtered, provider)
		}
	}
	if len(filtered) == 0 {
		return nil, DeniedError{}
	}
	return filtered, nil
}

type DeniedError struct{}

func (DeniedError) Error() string {
	return `{"error":{"message":"This API key is not allowed to access the requested product channel","type":"permission_error","code":"product_not_allowed"}}`
}
func (DeniedError) StatusCode() int { return 403 }

// AllowsEndpoint admits shared model APIs for execution-time checks and guards
// provider-specific APIs before they can allocate sessions or proxy requests.
// Unknown extension routes fail closed for restricted clients.
func (p Policy) AllowsEndpoint(path string) bool {
	if !p.Restricted() {
		return true
	}
	switch path {
	case "/v1/models", "/v1/chat/completions", "/v1/completions", "/v1/messages", "/v1/messages/count_tokens", "/v1/responses", "/v1/responses/compact", "/backend-api/codex/responses", "/backend-api/codex/responses/compact", "/v1beta/models", "/v1beta/interactions", "/v1/images/generations", "/v1/images/edits":
		return true
	}
	if path == "/v1/files" || strings.HasPrefix(path, "/v1/files/") || strings.HasPrefix(path, "/v1beta/models/") {
		return true
	}
	if strings.HasPrefix(path, "/v1/realtime") || strings.HasPrefix(path, "/v1/live") || strings.HasPrefix(path, "/openai/v1/videos") || path == "/v1/alpha/search" || path == "/backend-api/codex/alpha/search" || strings.HasPrefix(path, "/v1/responses/") || strings.HasPrefix(path, "/backend-api/codex/responses/") {
		return p.Allows("codex")
	}
	if path == "/v1/videos" || strings.HasPrefix(path, "/v1/videos/") {
		return p.Allows("xai")
	}
	return false
}
