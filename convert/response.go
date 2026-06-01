package convert

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ConvertResponse transforms a Chat Completions response into Responses API format.
func ConvertResponse(chatBody, model string) string {
	out := `{}`

	respID := gjson.Get(chatBody, "id").String()
	out, _ = sjson.Set(out, "id", respID)
	out, _ = sjson.Set(out, "object", "response")

	if model != "" {
		out, _ = sjson.Set(out, "model", model)
	} else {
		out, _ = sjson.Set(out, "model", gjson.Get(chatBody, "model").String())
	}

	if v := gjson.Get(chatBody, "created"); v.Exists() {
		out, _ = sjson.Set(out, "created_at", v.Int())
	}

	message := gjson.Get(chatBody, "choices.0.message")
	finishReason := gjson.Get(chatBody, "choices.0.finish_reason").String()

	idx := 0

	// Reasoning output
	if rc := message.Get("reasoning_content"); rc.Exists() && rc.String() != "" {
		out, _ = sjson.Set(out, "output."+itoa(idx), map[string]any{
			"type": "reasoning",
			"summary": []map[string]any{
				{"type": "summary_text", "text": rc.String()},
			},
			"status": "completed",
		})
		idx++
	}

	// Message output
	content := message.Get("content")
	toolCalls := message.Get("tool_calls")
	hasTextContent := content.Exists() && content.String() != ""
	hasToolCalls := toolCalls.Exists() && len(toolCalls.Array()) > 0

	if content.Exists() && (content.String() != "" || !hasToolCalls) {
		msg := map[string]any{
			"type": "message",
			"role": "assistant",
		}
		if hasTextContent {
			msg["content"] = []map[string]any{
				{"type": "output_text", "text": content.String()},
			}
		} else {
			msg["content"] = []map[string]any{}
		}
		msg["status"] = "completed"
		out, _ = sjson.Set(out, "output."+itoa(idx), msg)
		idx++
	}

	// Function call outputs
	if hasToolCalls {
		for _, tc := range toolCalls.Array() {
			args := tc.Get("function.arguments").String()
			if args == "" {
				args = "{}"
			}
			out, _ = sjson.Set(out, "output."+itoa(idx), map[string]any{
				"type":      "function_call",
				"call_id":   tc.Get("id").String(),
				"name":      tc.Get("function.name").String(),
				"arguments": args,
				"status":    "completed",
			})
			idx++
		}
	}

	// Status
	status := finishReasonToStatus(finishReason)
	out, _ = sjson.Set(out, "status", status)

	// Usage
	usage := gjson.Get(chatBody, "usage")
	if usage.Exists() {
		in := usage.Get("prompt_tokens").Int()
		outT := usage.Get("completion_tokens").Int()
		total := usage.Get("total_tokens").Int()
		if total == 0 {
			total = in + outT
		}
		out, _ = sjson.Set(out, "usage.input_tokens", in)
		out, _ = sjson.Set(out, "usage.output_tokens", outT)
		out, _ = sjson.Set(out, "usage.total_tokens", total)
	}

	return out
}

// ExtractReasoning extracts reasoning_content from a Chat Completions response.
func ExtractReasoning(chatBody string) string {
	return gjson.Get(chatBody, "choices.0.message.reasoning_content").String()
}

// GenerateResponseID generates a unique response ID.
func GenerateResponseID() string {
	b := make([]byte, 12)
	rand.Read(b)
	return "resp_" + hex.EncodeToString(b)
}

// finishReasonToStatus maps a Chat Completions finish_reason to a Responses API status.
func finishReasonToStatus(reason string) string {
	if reason == "length" {
		return "incomplete"
	}
	return "completed"
}

// itoa converts int to string using the standard library.
func itoa(n int) string {
	return strconv.Itoa(n)
}
