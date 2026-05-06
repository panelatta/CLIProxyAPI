package registry

import (
	"strings"
	"testing"
	"time"
)

func TestGetModelInfoReturnsClone(t *testing.T) {
	r := newTestModelRegistry()
	r.RegisterClient("client-1", "gemini", []*ModelInfo{{
		ID:          "m1",
		DisplayName: "Model One",
		Thinking:    &ThinkingSupport{Min: 1, Max: 2, Levels: []string{"low", "high"}},
	}})

	first := r.GetModelInfo("m1", "gemini")
	if first == nil {
		t.Fatal("expected model info")
	}
	first.DisplayName = "mutated"
	first.Thinking.Levels[0] = "mutated"

	second := r.GetModelInfo("m1", "gemini")
	if second.DisplayName != "Model One" {
		t.Fatalf("expected cloned display name, got %q", second.DisplayName)
	}
	if second.Thinking == nil || len(second.Thinking.Levels) == 0 || second.Thinking.Levels[0] != "low" {
		t.Fatalf("expected cloned thinking levels, got %+v", second.Thinking)
	}
}

func TestGetModelsForClientReturnsClones(t *testing.T) {
	r := newTestModelRegistry()
	r.RegisterClient("client-1", "gemini", []*ModelInfo{{
		ID:          "m1",
		DisplayName: "Model One",
		Thinking:    &ThinkingSupport{Levels: []string{"low", "high"}},
	}})

	first := r.GetModelsForClient("client-1")
	if len(first) != 1 || first[0] == nil {
		t.Fatalf("expected one model, got %+v", first)
	}
	first[0].DisplayName = "mutated"
	first[0].Thinking.Levels[0] = "mutated"

	second := r.GetModelsForClient("client-1")
	if len(second) != 1 || second[0] == nil {
		t.Fatalf("expected one model on second fetch, got %+v", second)
	}
	if second[0].DisplayName != "Model One" {
		t.Fatalf("expected cloned display name, got %q", second[0].DisplayName)
	}
	if second[0].Thinking == nil || len(second[0].Thinking.Levels) == 0 || second[0].Thinking.Levels[0] != "low" {
		t.Fatalf("expected cloned thinking levels, got %+v", second[0].Thinking)
	}
}

func TestGetAvailableModelsByProviderReturnsClones(t *testing.T) {
	r := newTestModelRegistry()
	r.RegisterClient("client-1", "gemini", []*ModelInfo{{
		ID:          "m1",
		DisplayName: "Model One",
		Thinking:    &ThinkingSupport{Levels: []string{"low", "high"}},
	}})

	first := r.GetAvailableModelsByProvider("gemini")
	if len(first) != 1 || first[0] == nil {
		t.Fatalf("expected one model, got %+v", first)
	}
	first[0].DisplayName = "mutated"
	first[0].Thinking.Levels[0] = "mutated"

	second := r.GetAvailableModelsByProvider("gemini")
	if len(second) != 1 || second[0] == nil {
		t.Fatalf("expected one model on second fetch, got %+v", second)
	}
	if second[0].DisplayName != "Model One" {
		t.Fatalf("expected cloned display name, got %q", second[0].DisplayName)
	}
	if second[0].Thinking == nil || len(second[0].Thinking.Levels) == 0 || second[0].Thinking.Levels[0] != "low" {
		t.Fatalf("expected cloned thinking levels, got %+v", second[0].Thinking)
	}
}

func TestCleanupExpiredQuotasInvalidatesAvailableModelsCache(t *testing.T) {
	r := newTestModelRegistry()
	r.RegisterClient("client-1", "openai", []*ModelInfo{{ID: "m1", Created: 1}})
	r.SetModelQuotaExceeded("client-1", "m1")
	if models := r.GetAvailableModels("openai"); len(models) != 1 {
		t.Fatalf("expected cooldown model to remain listed before cleanup, got %d", len(models))
	}

	r.mutex.Lock()
	quotaTime := time.Now().Add(-6 * time.Minute)
	r.models["m1"].QuotaExceededClients["client-1"] = &quotaTime
	r.mutex.Unlock()

	r.CleanupExpiredQuotas()

	if count := r.GetModelCount("m1"); count != 1 {
		t.Fatalf("expected model count 1 after cleanup, got %d", count)
	}
	models := r.GetAvailableModels("openai")
	if len(models) != 1 {
		t.Fatalf("expected model to stay available after cleanup, got %d", len(models))
	}
	if got := models[0]["id"]; got != "m1" {
		t.Fatalf("expected model id m1, got %v", got)
	}
}

func TestGetAvailableModelsReturnsClonedSupportedParameters(t *testing.T) {
	r := newTestModelRegistry()
	r.RegisterClient("client-1", "openai", []*ModelInfo{{
		ID:                  "m1",
		DisplayName:         "Model One",
		SupportedParameters: []string{"temperature", "top_p"},
	}})

	first := r.GetAvailableModels("openai")
	if len(first) != 1 {
		t.Fatalf("expected one model, got %d", len(first))
	}
	params, ok := first[0]["supported_parameters"].([]string)
	if !ok || len(params) != 2 {
		t.Fatalf("expected supported_parameters slice, got %#v", first[0]["supported_parameters"])
	}
	params[0] = "mutated"

	second := r.GetAvailableModels("openai")
	params, ok = second[0]["supported_parameters"].([]string)
	if !ok || len(params) != 2 || params[0] != "temperature" {
		t.Fatalf("expected cloned supported_parameters, got %#v", second[0]["supported_parameters"])
	}
}

func TestLookupModelInfoReturnsCloneForStaticDefinitions(t *testing.T) {
	first := LookupModelInfo("claude-sonnet-4-6")
	if first == nil || first.Thinking == nil || len(first.Thinking.Levels) == 0 {
		t.Fatalf("expected static model with thinking levels, got %+v", first)
	}
	first.Thinking.Levels[0] = "mutated"

	second := LookupModelInfo("claude-sonnet-4-6")
	if second == nil || second.Thinking == nil || len(second.Thinking.Levels) == 0 || second.Thinking.Levels[0] == "mutated" {
		t.Fatalf("expected static lookup clone, got %+v", second)
	}
}

func TestCodexStaticImageModelsAvailableForAllPlans(t *testing.T) {
	plans := map[string][]*ModelInfo{
		"free": GetCodexFreeModels(),
		"team": GetCodexTeamModels(),
		"plus": GetCodexPlusModels(),
		"pro":  GetCodexProModels(),
	}

	for plan, models := range plans {
		for _, modelID := range []string{"gpt-image-1", "gpt-image-2"} {
			info := findTestModelInfo(models, modelID)
			if info == nil {
				t.Fatalf("expected %s in codex %s models", modelID, plan)
			}
			if !testStringSliceContainsFold(info.SupportedOutputModalities, "image") {
				t.Fatalf("expected %s in codex %s models to support image output, got %+v", modelID, plan, info.SupportedOutputModalities)
			}
		}
	}

	if info := LookupStaticModelInfo("gpt-image-2"); info == nil {
		t.Fatal("expected gpt-image-2 to be discoverable from static model lookup")
	}
}

func TestOpenAIAvailableModelsExposeImageGenerationCapability(t *testing.T) {
	info := LookupStaticModelInfo("gpt-image-2")
	if info == nil {
		t.Fatal("expected static gpt-image-2 model info")
	}

	r := newTestModelRegistry()
	r.RegisterClient("codex-1", "codex", []*ModelInfo{info})

	providers := r.GetModelProviders("gpt-image-2")
	if len(providers) != 1 || providers[0] != "codex" {
		t.Fatalf("expected gpt-image-2 to route to codex, got %+v", providers)
	}

	models := r.GetAvailableModels("openai")
	if len(models) != 1 {
		t.Fatalf("expected one openai model, got %d", len(models))
	}

	infoMap, ok := models[0]["info"].(map[string]any)
	if !ok {
		t.Fatalf("expected info map, got %#v", models[0]["info"])
	}
	meta, ok := infoMap["meta"].(map[string]any)
	if !ok {
		t.Fatalf("expected meta map, got %#v", infoMap["meta"])
	}
	capabilities, ok := meta["capabilities"].(map[string]any)
	if !ok {
		t.Fatalf("expected capabilities map, got %#v", meta["capabilities"])
	}
	if capabilities["image_generation"] != true {
		t.Fatalf("expected image_generation capability, got %#v", capabilities)
	}
}

func TestLocalModelOverridesAddCodexImageModelsOnce(t *testing.T) {
	data := &staticModelsJSON{
		CodexFree: []*ModelInfo{{ID: "gpt-5.2"}},
		CodexTeam: []*ModelInfo{{ID: "gpt-5.2"}},
		CodexPlus: []*ModelInfo{
			{ID: "gpt-5.2"},
			{ID: "gpt-image-2"},
		},
		CodexPro: []*ModelInfo{{ID: "gpt-5.2"}},
	}

	applyLocalModelOverrides(data)
	applyLocalModelOverrides(data)

	plans := map[string][]*ModelInfo{
		"free": data.CodexFree,
		"team": data.CodexTeam,
		"plus": data.CodexPlus,
		"pro":  data.CodexPro,
	}
	for plan, models := range plans {
		if got := countTestModelInfos(models, "gpt-image-1"); got != 1 {
			t.Fatalf("expected one gpt-image-1 in codex %s models, got %d", plan, got)
		}
		if got := countTestModelInfos(models, "gpt-image-2"); got != 1 {
			t.Fatalf("expected one gpt-image-2 in codex %s models, got %d", plan, got)
		}
	}
}

func findTestModelInfo(models []*ModelInfo, modelID string) *ModelInfo {
	for _, model := range models {
		if model != nil && model.ID == modelID {
			return model
		}
	}
	return nil
}

func countTestModelInfos(models []*ModelInfo, modelID string) int {
	count := 0
	for _, model := range models {
		if model != nil && model.ID == modelID {
			count++
		}
	}
	return count
}

func testStringSliceContainsFold(values []string, needle string) bool {
	for _, value := range values {
		if strings.EqualFold(value, needle) {
			return true
		}
	}
	return false
}
