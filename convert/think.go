package convert

import (
	"strings"

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

// BuildMessagesJSON constructs a JSON messages array from stored entries
// for previous_response_id continuity.
func BuildMessagesJSON(history []HistoryEntry) string {
	var msgs []string
	for _, h := range history {
		msg := `{"role":"` + h.Role + `","content":"` + escapeJSON(h.Content) + `"}`
		if h.Reasoning != "" {
			msg = strings.TrimSuffix(msg, "}") +
				`,"reasoning_content":"` + escapeJSON(h.Reasoning) + `"}`
		}
		msgs = append(msgs, msg)
	}
	return "[" + strings.Join(msgs, ",") + "]"
}

// HistoryEntry represents a single message in conversation history.
type HistoryEntry struct {
	Role      string
	Content   string
	Reasoning string
}

func escapeJSON(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	return s
}
