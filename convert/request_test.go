package convert

import (
	"encoding/json"
	"strings"
	"testing"
)

func compactJSON(t *testing.T, raw string) string {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, raw)
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func TestConvertRequestStringInput(t *testing.T) {
	req := `{"model":"deepseek-v4-flash","input":"hello world"}`
	got := ConvertRequest(req)
	want := `{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hello world"}]}`
	if compactJSON(t, got) != compactJSON(t, want) {
		t.Errorf("\n got:  %s\n want: %s", got, want)
	}
}

func TestConvertRequestWithInstructions(t *testing.T) {
	req := `{"model":"deepseek-v4-flash","input":"hello","instructions":"You are helpful."}`
	got := ConvertRequest(req)
	if !strings.Contains(got, `"role":"system"`) {
		t.Error("instructions should become system message")
	}
	if !strings.Contains(got, `"You are helpful."`) {
		t.Error("instructions content should be in system message")
	}
}

func TestConvertRequestWithArrayInput(t *testing.T) {
	req := `{"model":"deepseek-v4-pro","input":[{"type":"message","role":"user","content":"hello"},{"type":"message","role":"assistant","content":"hi"}]}`
	got := ConvertRequest(req)
	if !strings.Contains(got, `"role":"user"`) {
		t.Error("should have user message")
	}
	if !strings.Contains(got, `"role":"assistant"`) {
		t.Error("should have assistant message")
	}
}

func TestConvertRequestReasoningEffort(t *testing.T) {
	tests := []struct{ codex, contains string }{
		{"none", `"thinking"`},
		{"minimal", `"low"`},
		{"high", `"high"`},
		{"xhigh", `"xhigh"`},
	}
	for _, tt := range tests {
		req := `{"model":"x","input":"hi","reasoning":{"effort":"` + tt.codex + `"}}`
		got := ConvertRequest(req)
		if !strings.Contains(got, tt.contains) {
			t.Errorf("effort=%q: want %q in output, got:\n%s", tt.codex, tt.contains, got)
		}
	}
}

func TestConvertRequestMaxOutputTokens(t *testing.T) {
	req := `{"model":"x","input":"hi","max_output_tokens":4096}`
	got := ConvertRequest(req)
	if !strings.Contains(got, `"max_tokens":4096`) {
		t.Errorf("max_output_tokens should map to max_tokens, got:\n%s", got)
	}
}

func TestConvertRequestToolChoiceOnlyWithTools(t *testing.T) {
	withTools := `{"model":"x","input":"hi","tools":[{"type":"function","name":"t"}],"tool_choice":"auto"}`
	got := ConvertRequest(withTools)
	if !strings.Contains(got, `"tool_choice"`) {
		t.Error("tool_choice should appear when tools present")
	}

	withoutTools := `{"model":"x","input":"hi","tool_choice":"auto"}`
	got2 := ConvertRequest(withoutTools)
	if strings.Contains(got2, `"tool_choice"`) {
		t.Error("tool_choice should NOT appear when tools absent")
	}
}

func TestConvertRequestFunctionCallInput(t *testing.T) {
	req := `{"model":"x","input":[{"type":"message","role":"user","content":"run ls"},{"type":"function_call","call_id":"call_1","name":"bash","arguments":"{\"cmd\":\"ls\"}"},{"type":"function_call_output","call_id":"call_1","output":"file list here"}]}`
	got := ConvertRequest(req)
	if !strings.Contains(got, `"tool_calls"`) {
		t.Error("function_call should produce tool_calls")
	}
	if !strings.Contains(got, `"role":"tool"`) {
		t.Error("function_call_output should produce tool role message")
	}
}
