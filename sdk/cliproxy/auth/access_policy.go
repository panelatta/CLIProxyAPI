package auth

import (
	"context"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/accesspolicy"
)

// AccessPolicy reads the latest runtime config for each execution, including WebSocket turns.
func (m *Manager) AccessPolicy(ctx context.Context) accesspolicy.Policy {
	cfg := m.runtimeConfigSnapshot()
	if cfg == nil {
		return accesspolicy.Policy{}
	}
	return accesspolicy.ForRequest(&cfg.SDKConfig, ctx)
}
