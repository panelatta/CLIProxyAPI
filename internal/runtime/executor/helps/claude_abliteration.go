package helps

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// StripAbliterationSearchCacheControl removes a field rejected by this model's
// server-side search tool. Run after cache injection and payload overrides so
// both generated and caller-supplied markers are covered. Other tools and the
// system/message cache breakpoints remain unchanged.
func StripAbliterationSearchCacheControl(payload []byte, upstreamURL string) []byte {
	if gjson.GetBytes(payload, "model").String() != "abliterated-model-large-v2" {
		return payload
	}
	endpoint, err := url.Parse(upstreamURL)
	if err != nil || !strings.EqualFold(endpoint.Hostname(), "api.abliteration.ai") {
		return payload
	}
	tools := gjson.GetBytes(payload, "tools")
	if !tools.IsArray() {
		return payload
	}
	tools.ForEach(func(index, tool gjson.Result) bool {
		toolType := tool.Get("type").String()
		if (toolType == "web_search" || strings.HasPrefix(toolType, "web_search_")) && tool.Get("cache_control").Exists() {
			if updated, errDelete := sjson.DeleteBytes(payload, fmt.Sprintf("tools.%d.cache_control", index.Int())); errDelete == nil {
				payload = updated
			}
		}
		return true
	})
	return payload
}
