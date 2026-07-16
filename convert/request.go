package convert

import (
	"encoding/json"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// reasoningEffortOverrides maps Codex effort values to DeepSeek equivalents.
var reasoningEffortOverrides = map[string]string{
	"minimal": "low",
}

// ConvertRequest transforms a Responses API request into a Chat Completions request.
func ConvertRequest(body string) string {
	out := `{"messages":[]}`

	// Passthrough fields
	for _, f := range []string{"model", "temperature", "top_p", "user"} {
		if v := gjson.Get(body, f); v.Exists() {
			out, _ = sjson.Set(out, f, v.Value())
		}
	}

	// max_output_tokens -> max_tokens
	if v := gjson.Get(body, "max_output_tokens"); v.Exists() {
		out, _ = sjson.Set(out, "max_tokens", v.Int())
	}

	// stream + stream_options
	if v := gjson.Get(body, "stream"); v.Exists() {
		streamVal := v.Bool()
		out, _ = sjson.Set(out, "stream", streamVal)
		if streamVal {
			out, _ = sjson.Set(out, "stream_options.include_usage", true)
		}
	}

	// instructions -> system message
	if v := gjson.Get(body, "instructions"); v.Exists() && v.String() != "" {
		out, _ = sjson.Set(out, "messages.-1", map[string]any{
			"role": "system", "content": v.String(),
		})
	}

	// input -> messages
	input := gjson.Get(body, "input")
	switch {
	case input.Type == gjson.String:
		out, _ = sjson.Set(out, "messages.-1", map[string]any{
			"role": "user", "content": input.String(),
		})
	case input.IsArray():
		out = convertInputArray(input, out)
	}

	// reasoning.effort -> reasoning_effort or thinking
	if effort := gjson.Get(body, "reasoning.effort"); effort.Exists() {
		val := effort.String()
		switch val {
		case "none":
			out, _ = sjson.Set(out, "thinking", map[string]any{"type": "disabled"})
		default:
			if mapped, ok := reasoningEffortOverrides[val]; ok {
				val = mapped
			}
			out, _ = sjson.Set(out, "reasoning_effort", val)
		}
	}

	// tools: Responses API format → Chat Completions format
	// Responses: {type, name, description, parameters} (flat)
	// Chat:      {type, function: {name, description, parameters}} (wrapped)
	if tools := gjson.Get(body, "tools"); tools.Exists() && tools.IsArray() && len(tools.Array()) > 0 {
		out = convertTools(tools, out)
		if v := gjson.Get(body, "tool_choice"); v.Exists() {
			out, _ = sjson.Set(out, "tool_choice", v.Value())
		}
		if v := gjson.Get(body, "parallel_tool_calls"); v.Exists() {
			out, _ = sjson.Set(out, "parallel_tool_calls", v.Bool())
		}
	}

	return out
}

// convertTools converts tools from Responses API format (flat fields) to
// Chat Completions format (with function wrapper). Non-function tools
// (e.g. type "custom" for web_search) are dropped — Chat Completions
// only supports type "function".
func convertTools(tools gjson.Result, out string) string {
	var converted []map[string]any
	for _, t := range tools.Array() {
		// Already in Chat format — passthrough
		if t.Get("function").Exists() {
			out, _ = sjson.SetRaw(out, "tools", tools.Raw)
			return out
		}
		// Skip non-function tools (e.g. type "custom")
		if t.Get("type").String() != "function" {
			continue
		}
		// Responses format: wrap name/description/parameters into function
		conv := map[string]any{"type": "function"}
		fn := map[string]any{}
		if v := t.Get("name"); v.Exists() {
			fn["name"] = v.String()
		}
		if v := t.Get("description"); v.Exists() {
			fn["description"] = v.String()
		}
		if v := t.Get("parameters"); v.Exists() {
			fn["parameters"] = v.Value()
		}
		if len(fn) > 0 {
			conv["function"] = fn
		}
		converted = append(converted, conv)
	}
	b, _ := json.Marshal(converted)
	out, _ = sjson.SetRaw(out, "tools", string(b))
	return out
}

// convertInputArray converts Responses input array items to Chat messages.
func convertInputArray(input gjson.Result, out string) string {
	for _, item := range input.Array() {
		itemType := item.Get("type").String()
		if itemType == "" && item.Get("role").Exists() {
			itemType = "message"
		}
		switch itemType {
		case "message":
			out = convertMsgItem(item, out)
		case "function_call":
			out = convertFuncCallItem(item, out)
		case "function_call_output":
			out = convertFuncCallOutputItem(item, out)
		}
	}
	return out
}

func convertMsgItem(item gjson.Result, out string) string {
	role := item.Get("role").String()
	if role == "" {
		role = "user"
	}
	// OpenAI Responses API uses "developer" as a system-message variant
	// (alongside "system"), but DeepSeek only recognizes "system".
	if role == "developer" {
		role = "system"
	}
	content := item.Get("content")
	if content.Type == gjson.String {
		out, _ = sjson.Set(out, "messages.-1", map[string]any{
			"role": role, "content": content.String(),
		})
	} else if content.IsArray() {
		var parts []string
		for _, block := range content.Array() {
			switch block.Get("type").String() {
			case "input_text", "output_text", "text":
				parts = append(parts, block.Get("text").String())
			}
		}
		out, _ = sjson.Set(out, "messages.-1", map[string]any{
			"role": role, "content": strings.Join(parts, "\n"),
		})
	} else {
		out, _ = sjson.Set(out, "messages.-1", map[string]any{"role": role})
	}
	return out
}

func convertFuncCallItem(item gjson.Result, out string) string {
	callID := item.Get("call_id").String()
	name := item.Get("name").String()
	args := item.Get("arguments").String()
	if args == "" {
		args = "{}"
	}
	out, _ = sjson.Set(out, "messages.-1", map[string]any{
		"role": "assistant",
		"tool_calls": []map[string]any{{
			"id": callID, "type": "function",
			"function": map[string]any{"name": name, "arguments": args},
		}},
	})
	return out
}

func convertFuncCallOutputItem(item gjson.Result, out string) string {
	out, _ = sjson.Set(out, "messages.-1", map[string]any{
		"role":         "tool",
		"tool_call_id": item.Get("call_id").String(),
		"content":      item.Get("output").String(),
	})
	return out
}
