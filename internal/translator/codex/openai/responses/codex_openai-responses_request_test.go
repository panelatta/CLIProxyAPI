package responses

import (
	"testing"

	"github.com/tidwall/gjson"
)

// TestConvertSystemRoleToDeveloper_BasicConversion tests system instructions are lifted out of input.
func TestConvertSystemRoleToDeveloper_BasicConversion(t *testing.T) {
	inputJSON := []byte(`{
		"model": "gpt-5.2",
		"input": [
			{
				"type": "message",
				"role": "system",
				"content": [{"type": "input_text", "text": "You are a pirate."}]
			},
			{
				"type": "message",
				"role": "user",
				"content": [{"type": "input_text", "text": "Say hello."}]
			}
		]
	}`)

	output := ConvertOpenAIResponsesRequestToCodex("gpt-5.2", inputJSON, false)
	outputStr := string(output)

	if instructions := gjson.Get(outputStr, "instructions").String(); instructions != "You are a pirate." {
		t.Errorf("Expected instructions %q, got %q", "You are a pirate.", instructions)
	}

	input := gjson.Get(outputStr, "input").Array()
	if len(input) != 1 {
		t.Fatalf("Expected one input message after lifting instructions, got %d: %s", len(input), gjson.Get(outputStr, "input").Raw)
	}

	// Check that user role remains unchanged
	firstItemRole := gjson.Get(outputStr, "input.0.role")
	if firstItemRole.String() != "user" {
		t.Errorf("Expected role 'user', got '%s'", firstItemRole.String())
	}

	// Check content is preserved
	firstItemContent := gjson.Get(outputStr, "input.0.content.0.text")
	if firstItemContent.String() != "Say hello." {
		t.Errorf("Expected content 'Say hello.', got '%s'", firstItemContent.String())
	}
}

func TestConvertOpenAIResponsesRequestToCodex_LiftsDeveloperMessageToInstructions(t *testing.T) {
	inputJSON := []byte(`{
		"model": "gpt-5.5",
		"input": [
			{
				"type": "message",
				"role": "developer",
				"content": [{"type": "input_text", "text": "Use web search when useful."}]
			},
			{
				"type": "message",
				"role": "user",
				"content": [{"type": "input_text", "text": "Search for Codex docs."}]
			}
		],
		"tools": [{"type": "web_search"}]
	}`)

	output := ConvertOpenAIResponsesRequestToCodex("gpt-5.5", inputJSON, false)
	outputStr := string(output)

	if instructions := gjson.Get(outputStr, "instructions").String(); instructions != "Use web search when useful." {
		t.Fatalf("Expected developer instructions to be lifted, got %q: %s", instructions, outputStr)
	}
	if role := gjson.Get(outputStr, "input.0.role").String(); role != "user" {
		t.Fatalf("Expected only user input to remain, got role %q: %s", role, outputStr)
	}
	if got := gjson.Get(outputStr, "tools.0.type").String(); got != "web_search" {
		t.Fatalf("Expected hosted web_search tool to be preserved, got %q: %s", got, outputStr)
	}
}

// TestConvertSystemRoleToDeveloper_MultipleSystemMessages tests lifting multiple system messages.
func TestConvertSystemRoleToDeveloper_MultipleSystemMessages(t *testing.T) {
	inputJSON := []byte(`{
		"model": "gpt-5.2",
		"input": [
			{
				"type": "message",
				"role": "system",
				"content": [{"type": "input_text", "text": "You are helpful."}]
			},
			{
				"type": "message",
				"role": "system",
				"content": [{"type": "input_text", "text": "Be concise."}]
			},
			{
				"type": "message",
				"role": "user",
				"content": [{"type": "input_text", "text": "Hello"}]
			}
		]
	}`)

	output := ConvertOpenAIResponsesRequestToCodex("gpt-5.2", inputJSON, false)
	outputStr := string(output)

	if instructions := gjson.Get(outputStr, "instructions").String(); instructions != "You are helpful.\n\nBe concise." {
		t.Errorf("Expected combined instructions, got %q", instructions)
	}
	input := gjson.Get(outputStr, "input").Array()
	if len(input) != 1 {
		t.Fatalf("Expected one input message after lifting instructions, got %d: %s", len(input), gjson.Get(outputStr, "input").Raw)
	}
	// Check that user role is unchanged
	firstRole := gjson.Get(outputStr, "input.0.role")
	if firstRole.String() != "user" {
		t.Errorf("Expected first role 'user', got '%s'", firstRole.String())
	}
}

// TestConvertSystemRoleToDeveloper_NoSystemMessages tests that requests without system messages are unchanged
func TestConvertSystemRoleToDeveloper_NoSystemMessages(t *testing.T) {
	inputJSON := []byte(`{
		"model": "gpt-5.2",
		"input": [
			{
				"type": "message",
				"role": "user",
				"content": [{"type": "input_text", "text": "Hello"}]
			},
			{
				"type": "message",
				"role": "assistant",
				"content": [{"type": "output_text", "text": "Hi there!"}]
			}
		]
	}`)

	output := ConvertOpenAIResponsesRequestToCodex("gpt-5.2", inputJSON, false)
	outputStr := string(output)

	// Check that user and assistant roles are unchanged
	firstRole := gjson.Get(outputStr, "input.0.role")
	if firstRole.String() != "user" {
		t.Errorf("Expected role 'user', got '%s'", firstRole.String())
	}

	secondRole := gjson.Get(outputStr, "input.1.role")
	if secondRole.String() != "assistant" {
		t.Errorf("Expected role 'assistant', got '%s'", secondRole.String())
	}
}

// TestConvertSystemRoleToDeveloper_EmptyInput tests that empty input arrays are handled correctly
func TestConvertSystemRoleToDeveloper_EmptyInput(t *testing.T) {
	inputJSON := []byte(`{
		"model": "gpt-5.2",
		"input": []
	}`)

	output := ConvertOpenAIResponsesRequestToCodex("gpt-5.2", inputJSON, false)
	outputStr := string(output)

	// Check that input is still an empty array
	inputArray := gjson.Get(outputStr, "input")
	if !inputArray.IsArray() {
		t.Error("Input should still be an array")
	}
	if len(inputArray.Array()) != 0 {
		t.Errorf("Expected empty array, got %d items", len(inputArray.Array()))
	}
}

// TestConvertSystemRoleToDeveloper_NoInputField tests that requests without input field are unchanged
func TestConvertSystemRoleToDeveloper_NoInputField(t *testing.T) {
	inputJSON := []byte(`{
		"model": "gpt-5.2",
		"stream": false
	}`)

	output := ConvertOpenAIResponsesRequestToCodex("gpt-5.2", inputJSON, false)
	outputStr := string(output)

	// Check that other fields are still set correctly
	stream := gjson.Get(outputStr, "stream")
	if !stream.Bool() {
		t.Error("Stream should be set to true by conversion")
	}

	store := gjson.Get(outputStr, "store")
	if store.Bool() {
		t.Error("Store should be set to false by conversion")
	}
}

// TestConvertOpenAIResponsesRequestToCodex_OriginalIssue tests the exact issue reported by the user
func TestConvertOpenAIResponsesRequestToCodex_OriginalIssue(t *testing.T) {
	// This is the exact input that was failing with "System messages are not allowed"
	inputJSON := []byte(`{
		"model": "gpt-5.2",
		"input": [
			{
				"type": "message",
				"role": "system",
				"content": "You are a pirate. Always respond in pirate speak."
			},
			{
				"type": "message",
				"role": "user",
				"content": "Say hello."
			}
		],
		"stream": false
	}`)

	output := ConvertOpenAIResponsesRequestToCodex("gpt-5.2", inputJSON, false)
	outputStr := string(output)

	if instructions := gjson.Get(outputStr, "instructions").String(); instructions != "You are a pirate. Always respond in pirate speak." {
		t.Errorf("Expected instructions to contain system prompt, got %q", instructions)
	}

	input := gjson.Get(outputStr, "input").Array()
	if len(input) != 1 {
		t.Fatalf("Expected one input message after lifting instructions, got %d: %s", len(input), gjson.Get(outputStr, "input").Raw)
	}
	if role := gjson.Get(outputStr, "input.0.role").String(); role != "user" {
		t.Errorf("Expected role 'user', got '%s'", role)
	}

	// Verify stream was set to true (as required by Codex)
	stream := gjson.Get(outputStr, "stream")
	if !stream.Bool() {
		t.Error("Stream should be set to true")
	}

	// Verify other required fields for Codex
	store := gjson.Get(outputStr, "store")
	if store.Bool() {
		t.Error("Store should be false")
	}

	parallelCalls := gjson.Get(outputStr, "parallel_tool_calls")
	if !parallelCalls.Bool() {
		t.Error("parallel_tool_calls should be true")
	}

	if got := gjson.Get(outputStr, `include.#(=="reasoning.encrypted_content")`).String(); got != "reasoning.encrypted_content" {
		t.Errorf("Expected include to contain 'reasoning.encrypted_content', got %s", gjson.Get(outputStr, "include").Raw)
	}
	if got := gjson.Get(outputStr, `include.#(=="web_search_call.action.sources")`).String(); got != "web_search_call.action.sources" {
		t.Errorf("Expected include to contain 'web_search_call.action.sources', got %s", gjson.Get(outputStr, "include").Raw)
	}
}

// TestConvertSystemRoleToDeveloper_AssistantRole tests that assistant role is preserved
func TestConvertSystemRoleToDeveloper_AssistantRole(t *testing.T) {
	inputJSON := []byte(`{
		"model": "gpt-5.2",
		"input": [
			{
				"type": "message",
				"role": "system",
				"content": [{"type": "input_text", "text": "You are helpful."}]
			},
			{
				"type": "message",
				"role": "user",
				"content": [{"type": "input_text", "text": "Hello"}]
			},
			{
				"type": "message",
				"role": "assistant",
				"content": [{"type": "output_text", "text": "Hi!"}]
			}
		]
	}`)

	output := ConvertOpenAIResponsesRequestToCodex("gpt-5.2", inputJSON, false)
	outputStr := string(output)

	if instructions := gjson.Get(outputStr, "instructions").String(); instructions != "You are helpful." {
		t.Errorf("Expected instructions %q, got %q", "You are helpful.", instructions)
	}
	input := gjson.Get(outputStr, "input").Array()
	if len(input) != 2 {
		t.Fatalf("Expected two input messages after lifting instructions, got %d: %s", len(input), gjson.Get(outputStr, "input").Raw)
	}

	// Check user unchanged
	firstRole := gjson.Get(outputStr, "input.0.role")
	if firstRole.String() != "user" {
		t.Errorf("Expected first role 'user', got '%s'", firstRole.String())
	}

	// Check assistant unchanged
	secondRole := gjson.Get(outputStr, "input.1.role")
	if secondRole.String() != "assistant" {
		t.Errorf("Expected second role 'assistant', got '%s'", secondRole.String())
	}
}

func TestConvertOpenAIResponsesRequestToCodex_NormalizesWebSearchPreview(t *testing.T) {
	inputJSON := []byte(`{
		"model": "gpt-5.4-mini",
		"input": "find latest OpenAI model news",
		"tools": [
			{"type": "web_search_preview_2025_03_11"}
		],
		"tool_choice": {
			"type": "allowed_tools",
			"tools": [
				{"type": "web_search_preview"},
				{"type": "web_search_preview_2025_03_11"}
			]
		}
	}`)

	output := ConvertOpenAIResponsesRequestToCodex("gpt-5.4-mini", inputJSON, false)

	if got := gjson.GetBytes(output, "tools.0.type").String(); got != "web_search" {
		t.Fatalf("tools.0.type = %q, want %q: %s", got, "web_search", string(output))
	}
	if got := gjson.GetBytes(output, "tool_choice.type").String(); got != "allowed_tools" {
		t.Fatalf("tool_choice.type = %q, want %q: %s", got, "allowed_tools", string(output))
	}
	if got := gjson.GetBytes(output, "tool_choice.tools.0.type").String(); got != "web_search" {
		t.Fatalf("tool_choice.tools.0.type = %q, want %q: %s", got, "web_search", string(output))
	}
	if got := gjson.GetBytes(output, "tool_choice.tools.1.type").String(); got != "web_search" {
		t.Fatalf("tool_choice.tools.1.type = %q, want %q: %s", got, "web_search", string(output))
	}
}

func TestConvertOpenAIResponsesRequestToCodex_NormalizesTopLevelToolChoicePreviewAlias(t *testing.T) {
	inputJSON := []byte(`{
		"model": "gpt-5.4-mini",
		"input": "find latest OpenAI model news",
		"tool_choice": {"type": "web_search_preview_2025_03_11"}
	}`)

	output := ConvertOpenAIResponsesRequestToCodex("gpt-5.4-mini", inputJSON, false)

	if got := gjson.GetBytes(output, "tool_choice.type").String(); got != "web_search" {
		t.Fatalf("tool_choice.type = %q, want %q: %s", got, "web_search", string(output))
	}
}

func TestUserFieldDeletion(t *testing.T) {
	inputJSON := []byte(`{  
		"model": "gpt-5.2",  
		"user": "test-user",  
		"input": [{"role": "user", "content": "Hello"}]  
	}`)

	output := ConvertOpenAIResponsesRequestToCodex("gpt-5.2", inputJSON, false)
	outputStr := string(output)

	// Verify user field is deleted
	userField := gjson.Get(outputStr, "user")
	if userField.Exists() {
		t.Errorf("user field should be deleted, but it was found with value: %s", userField.Raw)
	}
}

func TestContextManagementCompactionCompatibility(t *testing.T) {
	inputJSON := []byte(`{
		"model": "gpt-5.2",
		"context_management": [
			{
				"type": "compaction",
				"compact_threshold": 12000
			}
		],
		"input": [{"role":"user","content":"hello"}]
	}`)

	output := ConvertOpenAIResponsesRequestToCodex("gpt-5.2", inputJSON, false)
	outputStr := string(output)

	if gjson.Get(outputStr, "context_management").Exists() {
		t.Fatalf("context_management should be removed for Codex compatibility")
	}
	if gjson.Get(outputStr, "truncation").Exists() {
		t.Fatalf("truncation should be removed for Codex compatibility")
	}
}

func TestTruncationRemovedForCodexCompatibility(t *testing.T) {
	inputJSON := []byte(`{
		"model": "gpt-5.2",
		"truncation": "disabled",
		"input": [{"role":"user","content":"hello"}]
	}`)

	output := ConvertOpenAIResponsesRequestToCodex("gpt-5.2", inputJSON, false)
	outputStr := string(output)

	if gjson.Get(outputStr, "truncation").Exists() {
		t.Fatalf("truncation should be removed for Codex compatibility")
	}
}

func TestConvertOpenAIResponsesRequestToCodex_PreservesBackgroundStore(t *testing.T) {
	inputJSON := []byte(`{
		"model": "gpt-5.5",
		"input": "long task",
		"background": true,
		"stream": true,
		"store": true
	}`)

	output := ConvertOpenAIResponsesRequestToCodex("gpt-5.5", inputJSON, true)

	if !gjson.GetBytes(output, "background").Bool() {
		t.Fatalf("background was not preserved: %s", output)
	}
	if !gjson.GetBytes(output, "store").Bool() {
		t.Fatalf("store was not preserved for background response: %s", output)
	}
	if !gjson.GetBytes(output, "stream").Bool() {
		t.Fatalf("stream should remain true: %s", output)
	}
}

func TestConvertOpenAIResponsesRequestToCodex_PreservesExplicitStoreWithoutBackground(t *testing.T) {
	inputJSON := []byte(`{"model":"gpt-5.5","input":"long task","background":false,"store":true}`)

	output := ConvertOpenAIResponsesRequestToCodex("gpt-5.5", inputJSON, true)

	if !gjson.GetBytes(output, "store").Bool() {
		t.Fatalf("explicit store=true should be preserved: %s", output)
	}
	if !gjson.GetBytes(output, "stream").Bool() {
		t.Fatalf("stream should remain true: %s", output)
	}
}

func TestConvertOpenAIResponsesRequestToCodex_DefaultsStoreFalse(t *testing.T) {
	inputJSON := []byte(`{"model":"gpt-5.5","input":"ordinary task"}`)

	output := ConvertOpenAIResponsesRequestToCodex("gpt-5.5", inputJSON, true)

	if gjson.GetBytes(output, "store").Bool() {
		t.Fatalf("ordinary request should keep store=false: %s", output)
	}
}
