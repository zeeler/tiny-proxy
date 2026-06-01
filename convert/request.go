package convert

import (
	"fmt"
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
		out, _ = sjson.Set(out, "stream", v.Bool())
		if v.Bool() {
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

	// tools passthrough (only when present and non-empty)
	if tools := gjson.Get(body, "tools"); tools.Exists() && tools.IsArray() && len(tools.Array()) > 0 {
		out, _ = sjson.SetRaw(out, "tools", tools.Raw)
		if v := gjson.Get(body, "tool_choice"); v.Exists() {
			out, _ = sjson.Set(out, "tool_choice", v.Value())
		}
		if v := gjson.Get(body, "parallel_tool_calls"); v.Exists() {
			out, _ = sjson.Set(out, "parallel_tool_calls", v.Bool())
		}
	}

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

// Ensure fmt is used.
var _ = fmt.Sprintf
