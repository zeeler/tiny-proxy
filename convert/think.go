package convert

import (
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// InjectReasoning adds cached reasoning_content to the last assistant message
// in a chat request body. This ensures DeepSeek sees its previous thinking
// during multi-turn tool call conversations.
func InjectReasoning(chatBody, reasoning string) string {
	if reasoning == "" {
		return chatBody
	}

	messages := gjson.Get(chatBody, "messages")
	if !messages.Exists() || !messages.IsArray() {
		return chatBody
	}

	msgs := messages.Array()
	// Find the last assistant message and inject reasoning_content
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Get("role").String() == "assistant" {
			out, err := sjson.Set(chatBody,
				"messages."+itoa(i)+".reasoning_content", reasoning)
			if err == nil {
				return out
			}
			break
		}
	}

	return chatBody
}

// EnsureThinkingSafety disables thinking if the conversation history contains
// any assistant message with tool_calls but without reasoning_content.
// DeepSeek requires reasoning_content on every assistant that has tool_calls
// when thinking is enabled; without this safety net, the request would fail
// with "The reasoning_content in the thinking mode must be passed back to the API."
func EnsureThinkingSafety(chatBody string) string {
	// Already disabled — nothing to do
	if gjson.Get(chatBody, "thinking.type").String() == "disabled" {
		return chatBody
	}

	msgs := gjson.Get(chatBody, "messages").Array()
	for _, msg := range msgs {
		if msg.Get("role").String() != "assistant" {
			continue
		}
		if msg.Get("tool_calls").Exists() && len(msg.Get("tool_calls").Array()) > 0 &&
			!msg.Get("reasoning_content").Exists() {
			chatBody, _ = sjson.Set(chatBody, "thinking", map[string]any{"type": "disabled"})
			chatBody, _ = sjson.Delete(chatBody, "reasoning_effort")
			return chatBody
		}
	}
	return chatBody
}
