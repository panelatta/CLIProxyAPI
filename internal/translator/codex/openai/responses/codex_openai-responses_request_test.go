package responses

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

var benchmarkConvertSystemRoleOutput []byte
var benchmarkConvertNormalizedOutput []byte

// TestConvertSystemRoleToDeveloper_BasicConversion tests the basic system -> developer role conversion
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

func TestConvertOpenAIResponsesRequestToCodexReusesNormalizedPayload(t *testing.T) {
	inputJSON := []byte(`{"model":"gpt-5.6","stream":true,"store":false,"parallel_tool_calls":true,"include":["reasoning.encrypted_content","web_search_call.action.sources"],"service_tier":"priority","input":[{"type":"message","role":"user","content":"hello"}]}`)

	output := ConvertOpenAIResponsesRequestToCodex("gpt-5.6", inputJSON, true)

	if &output[0] != &inputJSON[0] {
		t.Fatal("normalized request payload was copied")
	}
	if string(output) != string(inputJSON) {
		t.Fatalf("normalized request changed:\n got: %s\nwant: %s", output, inputJSON)
	}
}

func TestConvertOpenAIResponsesRequestToCodexNormalizesRequiredFields(t *testing.T) {
	inputJSON := []byte(`{
		"model":"gpt-5.6",
		"stream":"true",
		"store":true,
		"parallel_tool_calls":false,
		"include":["file_search_call.results","reasoning.encrypted_content"],
		"max_output_tokens":4096,
		"max_completion_tokens":4096,
		"temperature":0.2,
		"top_p":0.9,
		"service_tier":"standard",
		"truncation":"auto",
		"prompt_cache_options":{"mode":"implicit"},
		"prompt_cache_retention":"24h",
		"user":"request-owner",
		"input":[{"type":"message","role":"system","content":"hello"}]
	}`)

	output := ConvertOpenAIResponsesRequestToCodex("gpt-5.6", inputJSON, true)

	if stream := gjson.GetBytes(output, "stream"); stream.Type != gjson.True {
		t.Fatalf("stream = %s, want true", stream.Raw)
	}
	if store := gjson.GetBytes(output, "store"); store.Type != gjson.True {
		t.Fatalf("store = %s, want preserved true", store.Raw)
	}
	if parallel := gjson.GetBytes(output, "parallel_tool_calls"); parallel.Type != gjson.True {
		t.Fatalf("parallel_tool_calls = %s, want true", parallel.Raw)
	}
	include := gjson.GetBytes(output, "include").Array()
	if len(include) != 2 || include[0].String() != "reasoning.encrypted_content" || include[1].String() != "web_search_call.action.sources" {
		t.Fatalf("include = %s, want the Codex-supported encrypted content and web search sources", gjson.GetBytes(output, "include").Raw)
	}
	if instructions := gjson.GetBytes(output, "instructions").String(); instructions != "hello" {
		t.Fatalf("instructions = %q, want lifted system content", instructions)
	}
	if count := gjson.GetBytes(output, "input.#").Int(); count != 0 {
		t.Fatalf("input count = %d, want 0 after lifting instructions", count)
	}
	for _, path := range []string{
		"max_output_tokens",
		"max_completion_tokens",
		"temperature",
		"top_p",
		"service_tier",
		"truncation",
		"prompt_cache_options",
		"prompt_cache_retention",
		"user",
	} {
		if gjson.GetBytes(output, path).Exists() {
			t.Fatalf("%s should be removed: %s", path, output)
		}
	}
}

func TestConvertOpenAIResponsesRequestToCodex_FiltersPromptCacheRetention(t *testing.T) {
	inputJSON := []byte(`{
		"model": "gpt-5.6-terra",
		"prompt_cache_retention": "24h",
		"input": [
			{
				"type": "message",
				"role": "user",
				"content": [
					{
						"type": "input_text",
						"text": "hello"
					}
				]
			}
		]
	}`)

	output := ConvertOpenAIResponsesRequestToCodex("gpt-5.6-terra", inputJSON, true)
	if gjson.GetBytes(output, "prompt_cache_retention").Exists() {
		t.Fatalf("prompt_cache_retention should be removed: %s", string(output))
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

func TestStripCodexResponsesCacheBreakpoints(t *testing.T) {
	inputJSON := []byte(`{
		"model": "gpt-5.2",
		"input": [
			{
				"type": "message",
				"role": "user",
				"content": [
					{
						"type": "input_text",
						"text": "Hello world",
						"prompt_cache_breakpoint": {"mode": "explicit"}
					},
					{
						"type": "input_text",
						"text": "Second part"
					}
				]
			}
		]
	}`)

	output := ConvertOpenAIResponsesRequestToCodex("gpt-5.2", inputJSON, false)
	outputStr := string(output)

	if strings.Contains(outputStr, "prompt_cache_breakpoint") {
		t.Fatalf("prompt_cache_breakpoint should not exist in the output JSON")
	}
	if gjson.Get(outputStr, "input.0.content.0.text").String() != "Hello world" {
		t.Fatalf("text content should be preserved")
	}
	if gjson.Get(outputStr, "input.0.content.1.text").String() != "Second part" {
		t.Fatalf("second content part should be preserved")
	}
}

func TestStripCodexResponsesCacheBreakpoints_WithSystemRole(t *testing.T) {
	inputJSON := []byte(`{
		"model": "gpt-5.2",
		"input": [
			{
				"type": "message",
				"role": "system",
				"content": [
					{
						"type": "input_text",
						"text": "System prompt",
						"prompt_cache_breakpoint": {"mode": "explicit"}
					}
				]
			},
			{
				"type": "message",
				"role": "user",
				"content": [
					{
						"type": "input_text",
						"text": "User query",
						"prompt_cache_breakpoint": {"mode": "explicit"}
					}
				]
			}
		]
	}`)

	output := ConvertOpenAIResponsesRequestToCodex("gpt-5.2", inputJSON, false)
	outputStr := string(output)

	// The system message is lifted to instructions before the remaining input is normalized.
	if got := gjson.Get(outputStr, "instructions").String(); got != "System prompt" {
		t.Fatalf("expected lifted system instructions, got %q", got)
	}
	if got := gjson.Get(outputStr, "input.#").Int(); got != 1 {
		t.Fatalf("expected one user input after lifting instructions, got %d", got)
	}
	// Check prompt_cache_breakpoint is completely removed from payload
	if strings.Contains(outputStr, "prompt_cache_breakpoint") {
		t.Fatalf("prompt_cache_breakpoint should not exist in the output JSON")
	}
	if gjson.Get(outputStr, "input.0.content.0.text").String() != "User query" {
		t.Fatalf("expected user query text preserved, got %q", gjson.Get(outputStr, "input.0.content.0.text").String())
	}
}

func BenchmarkConvertSystemRoleToDeveloperLargeInput(b *testing.B) {
	cases := []struct {
		name      string
		inputJSON []byte
	}{
		{
			name:      "200_input_1_system",
			inputJSON: makeLargeResponsesInputForBenchmark(200, 200),
		},
		{
			name:      "200_input_2_system",
			inputJSON: makeLargeResponsesInputForBenchmark(200, 100),
		},
		{
			name:      "2000_input_20_system",
			inputJSON: makeLargeResponsesInputForBenchmark(2000, 100),
		},
	}
	benchmarks := []struct {
		name string
		fn   func([]byte) []byte
	}{
		{
			name: "previous_root_path_rewrite",
			fn:   convertSystemRoleToDeveloperPreviousRootPathRewriteForBenchmark,
		},
		{
			name: "current_rebuilt_input_json_marshal",
			fn:   convertSystemRoleToDeveloper,
		},
	}

	for _, testCase := range cases {
		for _, benchmark := range benchmarks {
			b.Run(testCase.name+"/"+benchmark.name, func(b *testing.B) {
				output := benchmark.fn(testCase.inputJSON)
				if got := gjson.GetBytes(output, "input.0.role").String(); got != "developer" {
					b.Fatalf("input.0.role = %q, want %q", got, "developer")
				}
				if got := gjson.GetBytes(output, "input.1.role").String(); got != "user" {
					b.Fatalf("input.1.role = %q, want %q", got, "user")
				}

				b.ReportAllocs()
				b.SetBytes(int64(len(testCase.inputJSON)))
				b.ResetTimer()

				var benchmarkOutput []byte
				for i := 0; i < b.N; i++ {
					benchmarkOutput = benchmark.fn(testCase.inputJSON)
				}
				benchmarkConvertSystemRoleOutput = benchmarkOutput
			})
		}
	}
}

func BenchmarkConvertOpenAIResponsesRequestToCodexNormalizedPayload(b *testing.B) {
	cases := []struct {
		name      string
		inputJSON []byte
	}{
		{name: "1KiB", inputJSON: makeNormalizedResponsesRequestForBenchmark(1 << 10)},
		{name: "1MiB", inputJSON: makeNormalizedResponsesRequestForBenchmark(1 << 20)},
		{name: "8MiB", inputJSON: makeNormalizedResponsesRequestForBenchmark(8 << 20)},
	}

	for _, testCase := range cases {
		b.Run(testCase.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(testCase.inputJSON)))
			b.ResetTimer()

			var output []byte
			for b.Loop() {
				output = ConvertOpenAIResponsesRequestToCodex("gpt-5.6", testCase.inputJSON, true)
			}
			benchmarkConvertNormalizedOutput = output
		})
	}
}

func makeNormalizedResponsesRequestForBenchmark(contentBytes int) []byte {
	var builder strings.Builder
	builder.Grow(contentBytes + 256)
	builder.WriteString(`{"model":"gpt-5.6","stream":true,"store":false,"parallel_tool_calls":true,"include":["reasoning.encrypted_content"],"input":[{"type":"message","role":"user","content":"`)
	builder.WriteString(strings.Repeat("x", contentBytes))
	builder.WriteString(`"}]}`)
	return []byte(builder.String())
}

func makeLargeResponsesInputForBenchmark(inputCount int, systemEvery int) []byte {
	var builder strings.Builder
	builder.Grow(inputCount * 96)
	builder.WriteString(`{"model":"gpt-5.2","input":[`)
	for i := 0; i < inputCount; i++ {
		if i > 0 {
			builder.WriteByte(',')
		}
		role := "user"
		if i%systemEvery == 0 {
			role = "system"
		}
		builder.WriteString(`{"type":"message","role":"`)
		builder.WriteString(role)
		builder.WriteString(`","content":[{"type":"input_text","text":"message `)
		builder.WriteString(strconv.Itoa(i))
		builder.WriteString(`"}]}`)
	}
	builder.WriteString(`]}`)
	return []byte(builder.String())
}

func convertSystemRoleToDeveloperPreviousRootPathRewriteForBenchmark(rawJSON []byte) []byte {
	inputResult := gjson.GetBytes(rawJSON, "input")
	if !inputResult.IsArray() {
		return rawJSON
	}

	inputArray := inputResult.Array()
	result := rawJSON

	for i := 0; i < len(inputArray); i++ {
		rolePath := fmt.Sprintf("input.%d.role", i)
		if gjson.GetBytes(result, rolePath).String() == "system" {
			result, _ = sjson.SetBytes(result, rolePath, "developer")
		}
	}

	return result
}
