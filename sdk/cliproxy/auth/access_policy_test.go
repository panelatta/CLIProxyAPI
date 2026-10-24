package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/accesspolicy"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/registry"
	executor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
)

func TestAccessProductsExecutionAndReload(t *testing.T) {
	manager := NewManager(nil, nil, nil)
	cfg := &config.Config{SDKConfig: config.SDKConfig{APIKeys: []string{"client"}, APIKeyEntries: []config.AccessAPIKeyEntry{{APIKey: "client", AllowedProviders: []string{"claude"}}}}}
	manager.SetConfig(cfg)
	ctx := accesspolicy.WithPrincipal(context.Background(), "client", true)
	const model = "shared-product-alias"
	claudeExecutor := &forceMappingExecutor{id: "claude"}
	codexExecutor := &forceMappingExecutor{id: "codex"}
	for _, exec := range []*forceMappingExecutor{claudeExecutor, codexExecutor} {
		manager.RegisterExecutor(exec)
		a := &Auth{ID: "product-test-" + exec.id, Provider: exec.id, Status: StatusActive}
		if _, err := manager.Register(context.Background(), a); err != nil {
			t.Fatal(err)
		}
		registry.GetGlobalRegistry().RegisterClient(a.ID, a.Provider, []*registry.ModelInfo{{ID: model}})
		t.Cleanup(func() { registry.GetGlobalRegistry().UnregisterClient(a.ID) })
		manager.RefreshSchedulerEntry(a.ID)
	}
	request := executor.Request{Model: model}
	assertDenied := func(err error) {
		t.Helper()
		var denied accesspolicy.DeniedError
		if !errors.As(err, &denied) || denied.StatusCode() != 403 {
			t.Fatalf("expected product denial, got %v", err)
		}
	}
	_, err := manager.Execute(ctx, []string{"codex"}, request, executor.Options{})
	assertDenied(err)
	_, err = manager.ExecuteCount(ctx, []string{"codex"}, request, executor.Options{})
	assertDenied(err)
	_, err = manager.ExecuteStream(ctx, []string{"codex"}, request, executor.Options{})
	assertDenied(err)
	_, err = manager.SelectAuth(ctx, "codex", model, executor.Options{})
	assertDenied(err)
	_, err = manager.HttpRequest(ctx, &Auth{Provider: "codex"}, httptest.NewRequest(http.MethodGet, "https://example.test", nil))
	assertDenied(err)
	_, err = manager.ResponsesHTTPRequest(ctx, &Auth{Provider: "codex"}, "resp_test", nil)
	assertDenied(err)
	for i := 0; i < 3; i++ {
		if _, err = manager.Execute(ctx, []string{"codex", "claude"}, request, executor.Options{}); err != nil {
			t.Fatal(err)
		}
		stream, errStream := manager.ExecuteStream(ctx, []string{"codex", "claude"}, request, executor.Options{})
		if errStream != nil {
			t.Fatal(errStream)
		}
		for range stream.Chunks {
		}
	}
	if len(codexExecutor.ExecuteModels()) != 0 || len(codexExecutor.StreamModels()) != 0 {
		t.Fatal("forbidden provider invoked")
	}
	if len(claudeExecutor.ExecuteModels()) != 3 || len(claudeExecutor.StreamModels()) != 3 {
		t.Fatal("allowed provider not invoked")
	}
	cfg.APIKeyEntries[0].AllowedProviders = []string{"codex"}
	manager.SetConfig(cfg)
	_, err = manager.Execute(ctx, []string{"claude"}, request, executor.Options{})
	assertDenied(err)
	if _, err = manager.Execute(ctx, []string{"codex"}, request, executor.Options{}); err != nil {
		t.Fatal(err)
	}
	cfg.APIKeys = nil
	cfg.APIKeyEntries = nil
	manager.SetConfig(cfg)
	_, err = manager.Execute(ctx, []string{"codex"}, request, executor.Options{})
	assertDenied(err)

}
