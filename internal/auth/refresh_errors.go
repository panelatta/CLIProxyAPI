package auth

import "strings"

// IsInvalidRefreshCredential reports OAuth errors that require new credentials.
func IsInvalidRefreshCredential(message string) bool {
	lower := strings.ToLower(message)
	for _, code := range []string{"invalid_grant", "refresh_token_reused", "refresh_token_invalidated", "refresh_token_expired"} {
		if strings.Contains(lower, code) {
			return true
		}
	}
	return false
}
