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
