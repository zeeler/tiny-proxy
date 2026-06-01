package convert

import (
	"strings"
	"testing"
)

func hasEvent(events []string, eventType string) bool {
	for _, e := range events {
		if strings.Contains(e, eventType) {
			return true
		}
	}
	return false
}

func TestStreamFirstChunkEmitsCreated(t *testing.T) {
	st := NewStreamState("resp_1", "deepseek-v4")
	events := st.ProcessChunk(`{"choices":[{"delta":{"content":"Hi"},"index":0}]}`)
	if !hasEvent(events, "response.created") {
		t.Error("first chunk should emit response.created")
	}
	if !hasEvent(events, "response.in_progress") {
		t.Error("first chunk should emit response.in_progress")
	}
}

func TestStreamContentDelta(t *testing.T) {
	st := NewStreamState("resp_1", "x")
	events := st.ProcessChunk(`{"choices":[{"delta":{"content":"Hello"},"index":0}]}`)
	if !hasEvent(events, "response.output_text.delta") {
		t.Errorf("should emit output_text.delta, got: %v", events)
	}
}

func TestStreamReasoningDelta(t *testing.T) {
	st := NewStreamState("resp_1", "x")
	events := st.ProcessChunk(`{"choices":[{"delta":{"reasoning_content":"hmm"},"index":0}]}`)
	if !hasEvent(events, "response.reasoning_summary_text.delta") {
		t.Errorf("should emit reasoning delta, got: %v", events)
	}
}

func TestStreamToolCallDelta(t *testing.T) {
	st := NewStreamState("resp_1", "x")
	events := st.ProcessChunk(`{
		"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"bash","arguments":"ls"}}]},"index":0}]
	}`)
	if !hasEvent(events, "response.output_item.added") {
		t.Errorf("tool call should emit output_item.added, got: %v", events)
	}
}

func TestStreamDoneEmitsCompleted(t *testing.T) {
	st := NewStreamState("resp_1", "x")
	st.ProcessChunk(`{"choices":[{"delta":{"content":"Hi"},"index":0}]}`)
	events := st.Done()
	if !hasEvent(events, "response.completed") {
		t.Errorf("Done() should emit response.completed, got: %v", events)
	}
}

func TestStreamEventFormat(t *testing.T) {
	st := NewStreamState("resp_1", "x")
	events := st.ProcessChunk(`{"choices":[{"delta":{"content":"Hi"},"index":0}]}`)
	for _, e := range events {
		if e == "" {
			continue
		}
		if !strings.HasPrefix(e, "event: ") {
			t.Errorf("SSE should start with 'event: ', got: %s", e)
		}
	}
}

func TestStreamEmptyChunk(t *testing.T) {
	st := NewStreamState("resp_1", "x")
	events := st.ProcessChunk("")
	if len(events) != 0 {
		t.Error("empty chunk should return no events")
	}
}

func TestStreamGetReasoning(t *testing.T) {
	st := NewStreamState("resp_1", "x")
	st.ProcessChunk(`{"choices":[{"delta":{"reasoning_content":"think"},"index":0}]}`)
	reasoning := st.GetReasoningText()
	if reasoning != "think" {
		t.Errorf("GetReasoningText = %q, want %q", reasoning, "think")
	}
}
