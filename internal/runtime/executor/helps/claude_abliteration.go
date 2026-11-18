package helps

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/MoeForge/moego/ojson"
)

// StripAbliterationSearchCacheControl removes a field rejected by this model's
// server-side search tool. Run after cache injection and before final payload rules.
// Both generated and caller-supplied markers are covered. Other tools and the
// system/message cache breakpoints remain unchanged.
func StripAbliterationSearchCacheControl(payload []byte, upstreamURL string) []byte {
	doc := ojson.NewDoc(payload)
	StripAbliterationSearchCacheControlDoc(doc, upstreamURL)
	return doc.Bytes()
}

func StripAbliterationSearchCacheControlDoc(doc *ojson.Doc, upstreamURL string) {
	if doc.Get("model").String() != "abliterated-model-large-v2" {
		return
	}
	endpoint, err := url.Parse(upstreamURL)
	if err != nil || !strings.EqualFold(endpoint.Hostname(), "api.abliteration.ai") {
		return
	}
	tools := doc.Get("tools")
	if !tools.IsArray() {
		return
	}
	tools.ForEach(func(index, tool ojson.Result) bool {
		toolType := tool.Get("type").String()
		if (toolType == "web_search" || strings.HasPrefix(toolType, "web_search_")) && tool.Get("cache_control").Exists() {
			_ = doc.Delete(fmt.Sprintf("tools.%d.cache_control", index.Int()))
		}
		return true
	})
	return
}
