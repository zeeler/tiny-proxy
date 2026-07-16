package convert

import (
	"encoding/json"
	"fmt"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// NormalizeMessages ensures chat messages are in valid Chat Completions order:
// 1. Tool messages must immediately follow their corresponding assistant tool_calls.
// 2. Orphan tool messages (without a preceding assistant tool_calls) are downgraded
//    to user messages to avoid DeepSeek rejecting the request.
//
// This mirrors ccx's normalizeOpenAIToolCallMessageOrder + downgradeOrphanOpenAIToolMessages.
func NormalizeMessages(chatBody string) string {
	messages := gjson.Get(chatBody, "messages")
	if !messages.IsArray() || len(messages.Array()) == 0 {
		return chatBody
	}

	var msgs []map[string]any
	if err := json.Unmarshal([]byte(messages.Raw), &msgs); err != nil {
		return chatBody
	}

	normalized := normalizeToolCallOrder(msgs)

	if msgSlicesEqual(normalized, msgs) {
		return chatBody
	}

	b, err := json.Marshal(normalized)
	if err != nil {
		return chatBody
	}
	result, err := sjson.SetRaw(chatBody, "messages", string(b))
	if err != nil {
		return chatBody
	}
	return result
}

func msgSlicesEqual(a, b []map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		ja, _ := json.Marshal(a[i])
		jb, _ := json.Marshal(b[i])
		if string(ja) != string(jb) {
			return false
		}
	}
	return true
}

// normalizeToolCallOrder uses a three-pass approach:
// 1. Collect all tool_call_ids referenced by assistant messages.
// 2. Build output without tool messages (stash them), tracking each assistant's position.
// 3. For each assistant, insert matching stashed tools immediately after it in the output.
// Leftover stashed tools (orphans) are downgraded and appended at the end.
func normalizeToolCallOrder(msgs []map[string]any) []map[string]any {
	// Pass 1: collect known tool_call_ids from assistant messages
	knownCallIDs := make(map[string]bool)
	for _, msg := range msgs {
		if role, _ := msg["role"].(string); role == "assistant" {
			forEachToolCallID(msg, func(id string) { knownCallIDs[id] = true })
		}
	}

	// Pass 2: stash tool messages; build ordered output with assistant positions tracked
	stashed := make(map[string][]map[string]any)
	type assistPos struct {
		idx int
		msg map[string]any
	}
	var assistants []assistPos
	var out []map[string]any
	var orphanTools []map[string]any

	for _, msg := range msgs {
		role, _ := msg["role"].(string)

		if role == "tool" {
			callID, _ := msg["tool_call_id"].(string)
			if knownCallIDs[callID] {
				stashed[callID] = append(stashed[callID], msg)
			} else {
				orphanTools = append(orphanTools, msg)
			}
			continue
		}

		if role == "assistant" && hasToolCalls(msg) {
			assistants = append(assistants, assistPos{idx: len(out), msg: msg})
		}
		out = append(out, msg)
	}

	// Pass 3: insert stashed tools after their matching assistant, in correct tool_calls order.
	// Work backwards so earlier insertions don't shift later indices.
	for i := len(assistants) - 1; i >= 0; i-- {
		a := assistants[i]
		// Collect tools in the order they appear in tool_calls
		var myTools []map[string]any
		forEachToolCallID(a.msg, func(tcID string) {
			if tools, ok := stashed[tcID]; ok {
				myTools = append(myTools, tools...)
				delete(stashed, tcID)
			}
		})
		if len(myTools) > 0 {
			out = insertAfter(out, a.idx, myTools)
		}
	}

	// Append orphan tools (unknown call IDs + leftover stashed) as downgraded
	for _, msg := range orphanTools {
		out = append(out, downgradeToolToUser(msg))
	}
	for _, id := range sortedStringKeys(stashed) {
		for _, toolMsg := range stashed[id] {
			out = append(out, downgradeToolToUser(toolMsg))
		}
	}

	return out
}

// insertAfter inserts items after position idx in the slice.
func insertAfter(slice []map[string]any, idx int, items []map[string]any) []map[string]any {
	result := make([]map[string]any, 0, len(slice)+len(items))
	result = append(result, slice[:idx+1]...)
	result = append(result, items...)
	result = append(result, slice[idx+1:]...)
	return result
}

func forEachToolCallID(msg map[string]any, fn func(string)) {
	tcList, ok := msg["tool_calls"].([]any)
	if !ok {
		return
	}
	for _, tc := range tcList {
		if tcMap, ok := tc.(map[string]any); ok {
			if id, _ := tcMap["id"].(string); id != "" {
				fn(id)
			}
		}
	}
}

func hasToolCalls(msg map[string]any) bool {
	tc, ok := msg["tool_calls"]
	if !ok {
		return false
	}
	arr, ok := tc.([]any)
	return ok && len(arr) > 0
}

func downgradeToolToUser(msg map[string]any) map[string]any {
	content, _ := msg["content"].(string)
	callID, _ := msg["tool_call_id"].(string)
	prefix := "Function call output"
	if callID != "" {
		prefix += " (" + callID + ")"
	}
	return map[string]any{
		"role":    "user",
		"content": fmt.Sprintf("%s: %s", prefix, content),
	}
}

func sortedStringKeys(m map[string][]map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[i] > keys[j] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}
