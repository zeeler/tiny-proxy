package convert

import (
	"strings"
	"testing"
)

func TestConvertResponseSimpleText(t *testing.T) {
	chatResp := `{
		"id":"chatcmpl-abc","model":"deepseek-v4-flash",
		"choices":[{"message":{"role":"assistant","content":"Hello!"},"finish_reason":"stop"}],
		"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}
	}`
	got := ConvertResponse(chatResp, "deepseek-v4-flash")

	checks := []string{
		`"status":"completed"`,
		`"output_text"`,
		`"Hello!"`,
		`"input_tokens":10`,
		`"output_tokens":20`,
	}
	for _, c := range checks {
		if !strings.Contains(got, c) {
			t.Errorf("expected %q in output", c)
		}
	}
}

func TestConvertResponseToolCalls(t *testing.T) {
	chatResp := `{
		"id":"chatcmpl-tc","model":"x",
		"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"c1","type":"function","function":{"name":"bash","arguments":"{\"cmd\":\"ls\"}"}}]},"finish_reason":"tool_calls"}],
		"usage":{"prompt_tokens":5,"completion_tokens":15,"total_tokens":20}
	}`
	got := ConvertResponse(chatResp, "x")
	if !strings.Contains(got, `"function_call"`) {
		t.Error("should have function_call")
	}
	if !strings.Contains(got, `"call_id":"c1"`) {
		t.Error("should have call_id")
	}
}

func TestConvertResponseReasoning(t *testing.T) {
	chatResp := `{
		"id":"chatcmpl-r","model":"x",
		"choices":[{"message":{"role":"assistant","content":"42","reasoning_content":"thinking..."},"finish_reason":"stop"}],
		"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}
	}`
	got := ConvertResponse(chatResp, "x")
	if !strings.Contains(got, `"reasoning"`) {
		t.Error("should have reasoning item in output")
	}
	if !strings.Contains(got, `"thinking..."`) {
		t.Error("should contain reasoning text")
	}
}

func TestConvertResponseLengthFinish(t *testing.T) {
	chatResp := `{
		"id":"c","model":"x",
		"choices":[{"message":{"role":"assistant","content":"partial"},"finish_reason":"length"}],
		"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}
	}`
	got := ConvertResponse(chatResp, "x")
	if !strings.Contains(got, `"status":"incomplete"`) {
		t.Error("finish_reason=length should give status=incomplete")
	}
}

func TestConvertResponseUsageFallback(t *testing.T) {
	chatResp := `{
		"id":"c","model":"x",
		"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],
		"usage":{"prompt_tokens":100,"completion_tokens":200}
	}`
	got := ConvertResponse(chatResp, "x")
	if !strings.Contains(got, `"total_tokens":300`) {
		t.Error("total_tokens should default to prompt+completion sum")
	}
}

func TestExtractReasoning(t *testing.T) {
	chatResp := `{"choices":[{"message":{"reasoning_content":"deep thought"}}]}`
	if r := ExtractReasoning(chatResp); r != "deep thought" {
		t.Errorf("ExtractReasoning = %q, want %q", r, "deep thought")
	}
}
