package convert

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/tidwall/gjson"
)

// StreamState manages streaming SSE conversion from Chat Completions -> Responses API.
type StreamState struct {
	ResponseID string
	Model      string
	CreatedAt  int64
	seq        int64

	reasoningID  string
	messageID    string
	reasoningIdx int
	messageIdx   int

	funcCallIDs   map[int]string
	funcNames     map[int]string
	funcArgs      map[int]*strings.Builder
	funcItemAdded map[int]bool

	reasoningBuf strings.Builder
	contentBuf   strings.Builder

	inputTokens  int64
	outputTokens int64

	finishReason string // captured from last chunk's finish_reason
	funcCount    int    // number of function calls for unique output_index
}

// NewStreamState creates a new streaming state machine.
func NewStreamState(responseID, model string) *StreamState {
	return &StreamState{
		ResponseID:    responseID,
		Model:         model,
		CreatedAt:     time.Now().Unix(),
		funcCallIDs:   make(map[int]string),
		funcNames:     make(map[int]string),
		funcArgs:      make(map[int]*strings.Builder),
		funcItemAdded: make(map[int]bool),
	}
}

// ProcessChunk processes a Chat Completions SSE data chunk (without "data: " prefix).
// Returns Responses API SSE events to emit downstream.
func (s *StreamState) ProcessChunk(chunk string) []string {
	if chunk == "" || chunk == "[DONE]" {
		return nil
	}

	if !gjson.Valid(chunk) {
		return nil
	}

	var events []string
	isFirst := atomic.LoadInt64(&s.seq) == 0

	if isFirst {
		events = append(events, s.event("response.created", map[string]any{
			"type": "response.created",
			"response": map[string]any{
				"id": s.ResponseID, "object": "response",
				"created_at": s.CreatedAt, "model": s.Model,
				"status": "in_progress",
			},
		}))
		events = append(events, s.event("response.in_progress", map[string]any{
			"type": "response.in_progress", "response_id": s.ResponseID,
		}))
	}

	choices := gjson.Get(chunk, "choices")
	if !choices.Exists() || len(choices.Array()) == 0 {
		return events
	}

	for _, choice := range choices.Array() {
		delta := choice.Get("delta")

		// Capture finish_reason for the completed event
		if fr := choice.Get("finish_reason"); fr.Exists() && fr.String() != "" {
			s.finishReason = fr.String()
		}

		// reasoning_content
		if rc := delta.Get("reasoning_content"); rc.Exists() && rc.String() != "" {
			events = append(events, s.handleReasoning(rc.String())...)
		}

		// content
		if c := delta.Get("content"); c.Exists() && c.String() != "" {
			events = append(events, s.handleContent(c.String())...)
		}

		// tool_calls
		for _, tc := range delta.Get("tool_calls").Array() {
			idx := int(tc.Get("index").Int())
			events = append(events, s.handleToolCall(idx, tc)...)
		}
	}

	// Accumulate usage
	if u := gjson.Get(chunk, "usage"); u.Exists() {
		s.inputTokens = u.Get("prompt_tokens").Int()
		s.outputTokens = u.Get("completion_tokens").Int()
	}

	return events
}

// Done signals stream end and returns final events including response.completed.
func (s *StreamState) Done() []string {
	var events []string

	if s.reasoningID != "" {
		events = append(events, s.closeReasoning()...)
	}
	if s.messageID != "" {
		events = append(events, s.closeText()...)
	}
	events = append(events, s.closeFuncBlocks()...)

	// Build output array
	var output []map[string]any

	if s.reasoningID != "" {
		reasoningText := s.reasoningBuf.String()
		output = append(output, map[string]any{
			"type": "reasoning", "id": s.reasoningID, "status": "completed",
			"summary": []map[string]any{
				{"type": "summary_text", "text": reasoningText},
			},
		})
	}
	if s.messageID != "" {
		contentText := s.contentBuf.String()
		output = append(output, map[string]any{
			"type": "message", "id": s.messageID, "role": "assistant", "status": "completed",
			"content": []map[string]any{
				{"type": "output_text", "text": contentText},
			},
		})
	}
	for i := 0; ; i++ {
		name, ok := s.funcNames[i]
		if !ok {
			break
		}
		args := "{}"
		if buf, ok := s.funcArgs[i]; ok && buf.String() != "" {
			args = buf.String()
		}
		output = append(output, map[string]any{
			"type": "function_call", "id": s.funcCallIDs[i],
			"call_id": s.funcCallIDs[i], "name": name,
			"arguments": args, "status": "completed",
		})
	}

	status := finishReasonToStatus(s.finishReason)

	totalTokens := s.inputTokens + s.outputTokens
	events = append(events, s.event("response.completed", map[string]any{
		"type": "response.completed", "response_id": s.ResponseID,
		"response": map[string]any{
			"id": s.ResponseID, "object": "response",
			"created_at": s.CreatedAt, "model": s.Model,
			"status": status, "output": output,
			"usage": map[string]any{
				"input_tokens": s.inputTokens, "output_tokens": s.outputTokens,
				"total_tokens": totalTokens,
			},
		},
	}))

	return events
}

// GetReasoningText returns accumulated reasoning text for caching.
func (s *StreamState) GetReasoningText() string {
	return s.reasoningBuf.String()
}

// --- internals ---

func (s *StreamState) handleReasoning(text string) []string {
	var events []string
	s.reasoningBuf.WriteString(text)

	if s.reasoningID == "" {
		if s.messageID != "" {
			events = append(events, s.closeText()...)
		}
		s.reasoningID = "rs_" + s.ResponseID
		events = append(events, s.event("response.output_item.added", map[string]any{
			"type": "response.output_item.added", "response_id": s.ResponseID,
			"output_index": s.reasoningIdx,
			"item": map[string]any{
				"type": "reasoning", "id": s.reasoningID, "status": "in_progress",
			},
		}))
		events = append(events, s.event("response.reasoning_summary_part.added", map[string]any{
			"type": "response.reasoning_summary_part.added", "response_id": s.ResponseID,
			"output_index": s.reasoningIdx, "content_index": 0,
			"part": map[string]any{"type": "summary_text", "text": ""},
		}))
	}

	events = append(events, s.event("response.reasoning_summary_text.delta", map[string]any{
		"type": "response.reasoning_summary_text.delta", "response_id": s.ResponseID,
		"output_index": s.reasoningIdx, "content_index": 0, "delta": text,
	}))
	return events
}

func (s *StreamState) handleContent(text string) []string {
	var events []string
	s.contentBuf.WriteString(text)

	if s.messageID == "" {
		if s.reasoningID != "" {
			events = append(events, s.closeReasoning()...)
			s.messageIdx = 1
		}
		s.messageID = "msg_" + s.ResponseID
		events = append(events, s.event("response.output_item.added", map[string]any{
			"type": "response.output_item.added", "response_id": s.ResponseID,
			"output_index": s.messageIdx,
			"item": map[string]any{
				"type": "message", "id": s.messageID, "role": "assistant",
				"status": "in_progress", "content": []map[string]any{},
			},
		}))
		events = append(events, s.event("response.content_part.added", map[string]any{
			"type": "response.content_part.added", "response_id": s.ResponseID,
			"output_index": s.messageIdx, "content_index": 0,
			"part": map[string]any{"type": "output_text", "text": ""},
		}))
	}

	events = append(events, s.event("response.output_text.delta", map[string]any{
		"type": "response.output_text.delta", "response_id": s.ResponseID,
		"output_index": s.messageIdx, "content_index": 0, "delta": text,
	}))
	return events
}

func (s *StreamState) handleToolCall(idx int, tc gjson.Result) []string {
	var events []string

	if name := tc.Get("function.name"); name.Exists() && name.String() != "" {
		s.funcNames[idx] = name.String()
	}
	if id := tc.Get("id"); id.Exists() && id.String() != "" {
		s.funcCallIDs[idx] = id.String()
	}
	if s.funcArgs[idx] == nil {
		s.funcArgs[idx] = &strings.Builder{}
	}

	funcArgs := tc.Get("function.arguments")
	if funcArgs.Exists() {
		argsStr := funcArgs.String()
		s.funcArgs[idx].WriteString(argsStr)
	}

	if !s.funcItemAdded[idx] && s.funcCallIDs[idx] != "" && s.funcNames[idx] != "" {
		oi := s.nextFuncOutputIdx()
		events = append(events, s.event("response.output_item.added", map[string]any{
			"type": "response.output_item.added", "response_id": s.ResponseID,
			"output_index": oi,
			"item": map[string]any{
				"type": "function_call", "id": s.funcCallIDs[idx],
				"call_id": s.funcCallIDs[idx], "name": s.funcNames[idx],
				"arguments": "", "status": "in_progress",
			},
		}))
		s.funcItemAdded[idx] = true
	}

	if funcArgs.Exists() && funcArgs.String() != "" {
		oi := s.nextFuncOutputIdx()
		events = append(events, s.event("response.function_call_arguments.delta", map[string]any{
			"type": "response.function_call_arguments.delta", "response_id": s.ResponseID,
			"output_index": oi, "delta": funcArgs.String(),
		}))
	}
	return events
}

func (s *StreamState) closeReasoning() []string {
	reasoningText := s.reasoningBuf.String()
	return []string{
		s.event("response.reasoning_summary_text.done", map[string]any{
			"type": "response.reasoning_summary_text.done", "response_id": s.ResponseID,
			"output_index": s.reasoningIdx, "content_index": 0,
			"text": reasoningText,
		}),
		s.event("response.reasoning_summary_part.done", map[string]any{
			"type": "response.reasoning_summary_part.done", "response_id": s.ResponseID,
			"output_index": s.reasoningIdx, "content_index": 0,
			"part": map[string]any{"type": "summary_text", "text": reasoningText},
		}),
		s.event("response.output_item.done", map[string]any{
			"type": "response.output_item.done", "response_id": s.ResponseID,
			"output_index": s.reasoningIdx,
			"item":         map[string]any{"type": "reasoning", "id": s.reasoningID, "status": "completed"},
		}),
	}
}

func (s *StreamState) closeText() []string {
	contentText := s.contentBuf.String()
	return []string{
		s.event("response.output_text.done", map[string]any{
			"type": "response.output_text.done", "response_id": s.ResponseID,
			"output_index": s.messageIdx, "content_index": 0,
			"text": contentText,
		}),
		s.event("response.content_part.done", map[string]any{
			"type": "response.content_part.done", "response_id": s.ResponseID,
			"output_index": s.messageIdx, "content_index": 0,
			"part": map[string]any{"type": "output_text", "text": contentText},
		}),
		s.event("response.output_item.done", map[string]any{
			"type": "response.output_item.done", "response_id": s.ResponseID,
			"output_index": s.messageIdx,
			"item":         map[string]any{"type": "message", "id": s.messageID, "role": "assistant", "status": "completed"},
		}),
	}
}

func (s *StreamState) closeFuncBlocks() []string {
	var events []string
	for i := 0; ; i++ {
		name, ok := s.funcNames[i]
		if !ok {
			break
		}
		args := "{}"
		if buf, ok := s.funcArgs[i]; ok && buf.String() != "" {
			args = buf.String()
		}
		oi := s.nextFuncOutputIdx()
		events = append(events,
			s.event("response.function_call_arguments.done", map[string]any{
				"type": "response.function_call_arguments.done", "response_id": s.ResponseID,
				"output_index": oi, "arguments": args,
			}),
			s.event("response.output_item.done", map[string]any{
				"type": "response.output_item.done", "response_id": s.ResponseID,
				"output_index": oi,
				"item": map[string]any{
					"type": "function_call", "id": s.funcCallIDs[i],
					"call_id": s.funcCallIDs[i], "name": name,
					"arguments": args, "status": "completed",
				},
			}),
		)
	}
	return events
}

func (s *StreamState) nextFuncOutputIdx() int {
	n := 0
	if s.reasoningID != "" {
		n = 1
	}
	if s.messageID != "" {
		n++
	}
	n += s.funcCount
	s.funcCount++
	return n
}

func (s *StreamState) nextSeq() int64 {
	return atomic.AddInt64(&s.seq, 1)
}

func (s *StreamState) event(name string, payload map[string]any) string {
	payload["sequence_number"] = s.nextSeq()
	data, _ := json.Marshal(payload)
	return fmt.Sprintf("event: %s\ndata: %s\n\n", name, string(data))
}
