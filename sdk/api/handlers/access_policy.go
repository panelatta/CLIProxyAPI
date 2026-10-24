package handlers

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/accesspolicy"
	claudemodels "github.com/router-for-me/CLIProxyAPI/v8/internal/client/claude/models"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/interfaces"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/util"
)

func (h *BaseAPIHandler) requestAccessPolicy(ctx context.Context) accesspolicy.Policy {
	if h.AuthManager != nil {
		return h.AuthManager.AccessPolicy(ctx)
	}
	return accesspolicy.ForRequest(h.Cfg, ctx)
}

func (h *BaseAPIHandler) pluginAccessPolicyError(ctx context.Context) *interfaces.ErrorMessage {
	if h.requestAccessPolicy(ctx).Restricted() {
		return executionErrorMessage(accesspolicy.DeniedError{})
	}
	return nil
}

func (h *BaseAPIHandler) filterAccessModelList(c *gin.Context, body []byte) ([]byte, error) {
	ctx := context.WithValue(context.Background(), "gin", c)
	policy := h.requestAccessPolicy(ctx)
	if !policy.Restricted() {
		return body, nil
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	for _, field := range []string{"data", "models"} {
		raw, ok := payload[field]
		if !ok {
			continue
		}
		var models []map[string]json.RawMessage
		if err := json.Unmarshal(raw, &models); err != nil {
			return nil, err
		}
		filtered := make([]map[string]json.RawMessage, 0, len(models))
		for _, model := range models {
			id := ""
			for _, key := range []string{"id", "name", "slug"} {
				if value, ok := model[key]; ok {
					_ = json.Unmarshal(value, &id)
					if id != "" {
						break
					}
				}
			}
			id = claudemodels.ResolveClaudeModelIDPrefix(strings.TrimPrefix(id, "models/"))
			if _, err := policy.Filter(util.GetProviderName(id)); err == nil {
				filtered = append(filtered, model)
			}
		}
		encoded, err := json.Marshal(filtered)
		if err != nil {
			return nil, err
		}
		payload[field] = encoded
		// Anthropic pagination must describe the filtered catalog.
		if _, ok := payload["first_id"]; ok {
			payload["first_id"], payload["last_id"] = json.RawMessage("null"), json.RawMessage("null")
			if len(filtered) > 0 {
				payload["first_id"] = filtered[0]["id"]
				payload["last_id"] = filtered[len(filtered)-1]["id"]
			}
		}
	}
	return json.Marshal(payload)
}
