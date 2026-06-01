package convert

import (
	"strings"
	"testing"
)

func TestInjectReasoning(t *testing.T) {
	body := `{"messages":[{"role":"user","content":"hi"},{"role":"assistant","content":"hello"}]}`
	result := InjectReasoning(body, "thinking step 1...")

	if !strings.Contains(result, `"reasoning_content"`) {
		t.Errorf("should contain reasoning_content, got:\n%s", result)
	}
	if !strings.Contains(result, "thinking step 1...") {
		t.Errorf("should contain reasoning text, got:\n%s", result)
	}
	if !strings.Contains(result, `"hello"`) {
		t.Error("original assistant content should be preserved")
	}
}

func TestInjectReasoningEmpty(t *testing.T) {
	body := `{"messages":[{"role":"assistant","content":"hello"}]}`
	result := InjectReasoning(body, "")
	if result != body {
		t.Error("empty reasoning should return body unchanged")
	}
}

func TestInjectReasoningNoMessages(t *testing.T) {
	body := `{"model":"x"}`
	result := InjectReasoning(body, "thinking...")
	if result != body {
		t.Error("no messages should return body unchanged")
	}
}

func TestInjectReasoningLastAssistant(t *testing.T) {
	body := `{"messages":[{"role":"user","content":"a"},{"role":"assistant","content":"b"},{"role":"user","content":"c"},{"role":"assistant","content":"d"}]}`
	result := InjectReasoning(body, "test-reasoning")
	// Reasoning should be injected into the LAST assistant (index 3)
	if !strings.Contains(result, `"content":"d"`) {
		t.Error("should preserve last assistant content")
	}
	if !strings.Contains(result, "test-reasoning") {
		t.Error("reasoning should be present")
	}
}

func TestBuildMessagesJSON(t *testing.T) {
	history := []HistoryEntry{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi", Reasoning: "thinking..."},
	}
	result := BuildMessagesJSON(history)
	if !strings.Contains(result, `"role":"user"`) {
		t.Error("should contain user role")
	}
	if !strings.Contains(result, `"reasoning_content"`) {
		t.Error("should contain reasoning_content when reasoning is provided")
	}
	if !strings.Contains(result, "thinking...") {
		t.Error("should contain reasoning text")
	}
}

func TestEscapeJSON(t *testing.T) {
	tests := []struct{ input, expected string }{
		{`hello`, `hello`},
		{`he"llo`, `he\"llo`},
		{"line1\nline2", `line1\nline2`},
		{"tab\there", `tab\there`},
		{`a\b`, `a\\b`},
	}
	for _, tt := range tests {
		got := escapeJSON(tt.input)
		if got != tt.expected {
			t.Errorf("escapeJSON(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
