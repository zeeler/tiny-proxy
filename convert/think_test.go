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

func TestEnsureThinkingSafetyNoProblem(t *testing.T) {
	// All assistant messages have reasoning_content — no change needed
	body := `{"messages":[{"role":"user","content":"hi"},{"role":"assistant","content":"hello","reasoning_content":"think..."}],"reasoning_effort":"high"}`
	result := EnsureThinkingSafety(body)
	if !strings.Contains(result, `"reasoning_effort"`) {
		t.Error("reasoning_effort should be kept when all assistants have reasoning_content")
	}
}

func TestEnsureThinkingSafetyToolCallsMissingReasoning(t *testing.T) {
	// Assistant has tool_calls but no reasoning_content — must disable thinking
	body := `{"messages":[{"role":"user","content":"hi"},{"role":"assistant","content":"ok","tool_calls":[{"id":"t1","type":"function","function":{"name":"get_weather","arguments":"{}"}}]}],"reasoning_effort":"high"}`
	result := EnsureThinkingSafety(body)
	if strings.Contains(result, `"reasoning_effort"`) {
		t.Error("reasoning_effort should be removed when assistant has tool_calls without reasoning_content")
	}
	if !strings.Contains(result, `"disabled"`) {
		t.Error("thinking should be disabled, got:", result)
	}
}

func TestEnsureThinkingSafetyAlreadyDisabled(t *testing.T) {
	// Thinking already disabled — should be unchanged
	body := `{"messages":[{"role":"assistant","tool_calls":[{"id":"t1","type":"function","function":{"name":"f","arguments":"{}"}}]}],"thinking":{"type":"disabled"}}`
	result := EnsureThinkingSafety(body)
	if result != body {
		t.Error("already-disabled thinking should be unchanged")
	}
}

func TestEnsureThinkingSafetyNoToolCalls(t *testing.T) {
	// Assistant with content but no tool_calls — no issue, thinking stays
	body := `{"messages":[{"role":"user","content":"hi"},{"role":"assistant","content":"hello"}],"reasoning_effort":"high"}`
	result := EnsureThinkingSafety(body)
	if !strings.Contains(result, `"reasoning_effort"`) {
		t.Error("reasoning_effort should stay when no assistant has tool_calls")
	}
}
