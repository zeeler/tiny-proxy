package convert

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizeMessagesNoChange(t *testing.T) {
	// Already well-ordered messages
	body := `{"messages":[{"role":"user","content":"hi"},{"role":"assistant","content":"hello"}]}`
	result := NormalizeMessages(body)

	var original, normalized any
	json.Unmarshal([]byte(body), &original)
	json.Unmarshal([]byte(result), &normalized)
	if !deepEqual(original, normalized) {
		t.Errorf("well-ordered messages should not change\noriginal:  %s\nresult:   %s", body, result)
	}
}

func TestNormalizeMessagesReorderToolResponses(t *testing.T) {
	// Tool response (role=tool) should follow immediately after its assistant tool_calls
	body := `{
		"messages": [
			{"role":"user","content":"search for weather"},
			{"role":"tool","tool_call_id":"t1","content":"sunny"},
			{"role":"assistant","content":"ok","tool_calls":[{"id":"t1","type":"function","function":{"name":"get_weather","arguments":"{}"}}]}
		]
	}`
	result := NormalizeMessages(body)

	var msgs []map[string]any
	var resultMap map[string]any
	json.Unmarshal([]byte(result), &resultMap)
	b, _ := json.Marshal(resultMap["messages"])
	json.Unmarshal(b, &msgs)

	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	// After normalization: user, assistant(tool_calls), tool
	if msgs[0]["role"] != "user" {
		t.Error("first should be user")
	}
	if msgs[1]["role"] != "assistant" {
		t.Errorf("second should be assistant, got %v", msgs[1]["role"])
	}
	if msgs[2]["role"] != "tool" {
		t.Errorf("third should be tool, got %v", msgs[2]["role"])
	}
}

func TestNormalizeMessagesDowngradeOrphanTool(t *testing.T) {
	// Orphan tool message (no matching assistant tool_calls) should become user message
	body := `{
		"messages": [
			{"role":"user","content":"hi"},
			{"role":"tool","tool_call_id":"orphan_1","content":"some result"}
		]
	}`
	result := NormalizeMessages(body)

	if !strings.Contains(result, `"role":"user"`) {
		t.Error("orphan tool should be downgraded to user")
	}
	if strings.Contains(result, `"role":"tool"`) {
		t.Error("orphan tool role should not remain")
	}
	if !strings.Contains(result, "Function call output") {
		t.Error("orphan tool content should have prefix")
	}
	if !strings.Contains(result, "some result") {
		t.Error("orphan tool original content should be preserved")
	}
}

func TestNormalizeMessagesMixedToolCalls(t *testing.T) {
	// Multiple tool calls correctly paired
	body := `{
		"messages": [
			{"role":"user","content":"do two things"},
			{"role":"assistant","content":null,"tool_calls":[
				{"id":"t1","type":"function","function":{"name":"f1","arguments":"{}"}},
				{"id":"t2","type":"function","function":{"name":"f2","arguments":"{}"}}
			]},
			{"role":"tool","tool_call_id":"t2","content":"result2"},
			{"role":"user","content":"another user message"},
			{"role":"tool","tool_call_id":"t1","content":"result1"}
		]
	}`
	result := NormalizeMessages(body)

	var msgs []map[string]any
	var resultMap map[string]any
	json.Unmarshal([]byte(result), &resultMap)
	b, _ := json.Marshal(resultMap["messages"])
	json.Unmarshal(b, &msgs)

	// Expected order: user, assistant(tool_calls), tool(t1), tool(t2), user
	if msgs[0]["role"] != "user" {
		t.Error("[0] should be user")
	}
	if msgs[1]["role"] != "assistant" {
		t.Error("[1] should be assistant")
	}
	if msgs[2]["role"] != "tool" || msgs[2]["tool_call_id"] != "t1" {
		t.Errorf("[2] should be tool with t1, got %v", msgs[2])
	}
	if msgs[3]["role"] != "tool" || msgs[3]["tool_call_id"] != "t2" {
		t.Errorf("[3] should be tool with t2, got %v", msgs[3])
	}
	if msgs[4]["role"] != "user" {
		t.Error("[4] should be user (another user message)")
	}
}

func TestNormalizeMessagesEmpty(t *testing.T) {
	body := `{"model":"x"}`
	result := NormalizeMessages(body)
	if result != body {
		t.Error("body without messages should be unchanged")
	}
}

func deepEqual(a, b any) bool {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}
