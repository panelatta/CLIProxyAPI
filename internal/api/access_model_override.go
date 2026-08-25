package api

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const forcedModelContextKey = "forcedModel"

// accessModelOverrideMiddleware rewrites the top-level model field for clients
// with a force-model policy after access authentication has identified the key.
func (s *Server) accessModelOverrideMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		forcedModel := s.forcedModelForRequest(c)
		if forcedModel == "" || c.Request == nil || c.Request.Body == nil {
			c.Next()
			return
		}

		contentType := strings.ToLower(strings.TrimSpace(c.GetHeader("Content-Type")))
		if !strings.Contains(contentType, "application/json") {
			c.Next()
			return
		}

		body, errRead := io.ReadAll(c.Request.Body)
		_ = c.Request.Body.Close()
		if errRead != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
			return
		}

		if !gjson.ValidBytes(body) || !gjson.GetBytes(body, "model").Exists() {
			resetRequestBody(c.Request, body)
			c.Next()
			return
		}

		updated, errSet := sjson.SetBytes(body, "model", forcedModel)
		if errSet != nil {
			resetRequestBody(c.Request, body)
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "failed to apply forced model"})
			return
		}

		resetRequestBody(c.Request, updated)
		c.Set(forcedModelContextKey, forcedModel)
		c.Next()
	}
}

func (s *Server) forcedModelForRequest(c *gin.Context) string {
	if s == nil || s.cfg == nil || c == nil {
		return ""
	}
	value, ok := c.Get("userApiKey")
	if !ok {
		return ""
	}
	apiKey, ok := value.(string)
	if !ok {
		return ""
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return ""
	}
	for _, entry := range s.cfg.APIKeyEntries {
		if strings.TrimSpace(entry.APIKey) == apiKey {
			return strings.TrimSpace(entry.ForceModel)
		}
	}
	return ""
}

func resetRequestBody(request *http.Request, body []byte) {
	request.Body = io.NopCloser(bytes.NewReader(body))
	request.ContentLength = int64(len(body))
	request.Header.Set("Content-Length", strconv.Itoa(len(body)))
	request.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
}
