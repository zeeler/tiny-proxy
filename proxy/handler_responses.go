package proxy

import (
	"bufio"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/zeeler/tiny-proxy/convert"
	"github.com/zeeler/tiny-proxy/session"
	"github.com/zeeler/tiny-proxy/upstream"
)

// ResponsesHandler handles POST /v1/responses — the main proxy endpoint.
type ResponsesHandler struct {
	Router *upstream.Router
	Store  *session.Store
}

// ServeHTTP implements the main protocol conversion logic.
func (h *ResponsesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 10*1024*1024)) // 10MB limit
	if err != nil {
		writeError(w, http.StatusBadRequest, "cannot read body")
		return
	}
	bodyStr := string(body)

	model := gjson.Get(bodyStr, "model").String()
	client := h.Router.ClientFor(model)

	stream := gjson.Get(bodyStr, "stream").Bool()

	// Convert Responses → Chat Completions request first
	chatBody := convert.ConvertRequest(bodyStr)

	// Handle previous_response_id — merge stored history into chat body at Chat format level.
	// Skip merge when the input already contains function_call items, which means the
	// full conversation (including tool round-trips) is already in the input array.
	// Injecting stored history on top of that would duplicate assistant/tool_calls messages.
	prevID := gjson.Get(bodyStr, "previous_response_id").String()
	if prevID != "" && !inputHasFuncCalls(bodyStr) {
		if entry, ok := h.Store.Get(prevID); ok {
			chatBody = injectHistory(chatBody, entry)
		}
	}

	// Safety net: if any assistant message has tool_calls without reasoning_content,
	// disable thinking to avoid DeepSeek rejecting the request.
	chatBody = convert.EnsureThinkingSafety(chatBody)

	// Normalize message ordering: ensure tool responses immediately follow
	// their corresponding assistant tool_calls; downgrade orphan tool messages.
	chatBody = convert.NormalizeMessages(chatBody)

	if !stream {
		h.handleNonStream(w, chatBody, model, client)
	} else {
		h.handleStream(w, chatBody, model, client)
	}
}

// appendAndStore appends assistant to requestMessages and stores the combined
// result. On any JSON error it falls back to storing requestMessages alone.
func (h *ResponsesHandler) appendAndStore(respID, requestMessages string, assistant map[string]any, reasoning string) {
	var msgs []any
	if err := json.Unmarshal([]byte(requestMessages), &msgs); err != nil {
		log.Printf("[WARN] appendAndStore: cannot unmarshal messages: %v", err)
		h.Store.Put(respID, requestMessages, reasoning)
		return
	}
	msgs = append(msgs, assistant)
	fullMsgsJSON, err := json.Marshal(msgs)
	if err != nil {
		log.Printf("[WARN] appendAndStore: cannot marshal full messages: %v", err)
		h.Store.Put(respID, requestMessages, reasoning)
		return
	}
	h.Store.Put(respID, string(fullMsgsJSON), reasoning)
}

// injectHistory merges stored chat messages + current messages and injects cached reasoning.
func injectHistory(chatBody string, entry *session.Entry) string {
	if entry.Messages == "" {
		return chatBody
	}

	// Parse stored messages from previous request
	var storedMsgs []any
	if err := json.Unmarshal([]byte(entry.Messages), &storedMsgs); err != nil {
		log.Printf("[WARN] injectHistory: cannot unmarshal stored messages: %v", err)
		return chatBody
	}

	// Parse current messages from the converted request
	currentRaw := gjson.Get(chatBody, "messages").Raw
	var currentMsgs []any
	if err := json.Unmarshal([]byte(currentRaw), &currentMsgs); err != nil {
		log.Printf("[WARN] injectHistory: cannot unmarshal current messages: %v", err)
		return chatBody
	}

	// Merge: stored conversation + new messages
	allMsgs := append(storedMsgs, currentMsgs...)
	allMsgsJSON, err := json.Marshal(allMsgs)
	if err != nil {
		log.Printf("[WARN] injectHistory: cannot marshal merged messages: %v", err)
		return chatBody
	}

	result, err := sjson.SetRaw(chatBody, "messages", string(allMsgsJSON))
	if err != nil {
		log.Printf("[WARN] injectHistory: sjson.SetRaw failed: %v", err)
		return chatBody
	}

	// Inject cached reasoning into the last assistant message
	if entry.Reasoning != "" {
		result = convert.InjectReasoning(result, entry.Reasoning)
	}

	return result
}

func (h *ResponsesHandler) handleNonStream(w http.ResponseWriter, chatBody, model string, client *upstream.Client) {
	resp, err := client.Send([]byte(chatBody))
	if err != nil {
		log.Printf("[ERROR] upstream: %v", err)
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		writeError(w, http.StatusBadGateway, "cannot read upstream response")
		return
	}

	// Convert Chat → Responses response
	respStr := string(respBody)
	responsesBody := convert.ConvertResponse(respStr, model)

	// Store request messages + assistant response for conversation continuity
	respID := gjson.Get(responsesBody, "id").String()
	reasoning := convert.ExtractReasoning(respStr)
	messages := gjson.Get(chatBody, "messages").Raw
	if assistantMsg := gjson.Get(respStr, "choices.0.message"); assistantMsg.Exists() {
		var assistant map[string]any
		if err := json.Unmarshal([]byte(assistantMsg.Raw), &assistant); err != nil {
			log.Printf("[WARN] nonstream store: cannot unmarshal assistant: %v", err)
			h.Store.Put(respID, messages, reasoning)
		} else {
			h.appendAndStore(respID, messages, assistant, reasoning)
		}
	} else {
		h.Store.Put(respID, messages, reasoning)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(responsesBody))
}

func (h *ResponsesHandler) handleStream(w http.ResponseWriter, chatBody, model string, client *upstream.Client) {
	resp, err := client.Send([]byte(chatBody))
	if err != nil {
		log.Printf("[ERROR] upstream stream: %v", err)
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close()

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	respID := convert.GenerateResponseID()
	state := convert.NewStreamState(respID, model)

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // 1MB max line

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		events := state.ProcessChunk(data)
		for _, e := range events {
			w.Write([]byte(e))
			flusher.Flush()
		}
	}

	// Check for stream read errors before emitting completion.
	// If the upstream stream was interrupted, skip Done() and storage — the
	// accumulated state is incomplete and would corrupt future multi-turn history.
	if err := scanner.Err(); err != nil {
		log.Printf("[ERROR] stream read error: %v", err)
		return
	}

	// Emit completion events
	finalEvents := state.Done()
	for _, e := range finalEvents {
		w.Write([]byte(e))
		flusher.Flush()
	}

	// Store request messages + reasoning + assistant response for multi-turn continuity
	reasoning := state.GetReasoningText()
	messages := gjson.Get(chatBody, "messages").Raw
	if assistant := state.GetAssistantMessage(); assistant != nil {
		h.appendAndStore(respID, messages, assistant, reasoning)
	} else {
		h.Store.Put(respID, messages, reasoning)
	}
}

// inputHasFuncCalls checks whether the Responses API body's input array
// contains function_call items, which indicates the full conversation history
// (including tool round-trips) is already present in the input.
func inputHasFuncCalls(bodyStr string) bool {
	input := gjson.Get(bodyStr, "input")
	if !input.IsArray() {
		return false
	}
	for _, item := range input.Array() {
		if item.Get("type").String() == "function_call" {
			return true
		}
	}
	return false
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"message": msg,
			"type":    "proxy_error",
			"code":    code,
		},
	})
}
