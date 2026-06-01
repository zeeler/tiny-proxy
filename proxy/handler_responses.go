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

	"github.com/terry/tiny-proxy/convert"
	"github.com/terry/tiny-proxy/session"
	"github.com/terry/tiny-proxy/upstream"
)

// ResponsesHandler handles POST /v1/responses — the main proxy endpoint.
type ResponsesHandler struct {
	Upstream     *upstream.Client
	Store        *session.Store
	DefaultModel string
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
	if model == "" {
		model = h.DefaultModel
	}

	stream := gjson.Get(bodyStr, "stream").Bool()

	// Convert Responses → Chat Completions request first
	chatBody := convert.ConvertRequest(bodyStr)

	// Handle previous_response_id — merge stored history into chat body at Chat format level
	prevID := gjson.Get(bodyStr, "previous_response_id").String()
	if prevID != "" {
		if entry, ok := h.Store.Get(prevID); ok {
			chatBody = injectHistory(chatBody, entry)
		}
	}

	if !stream {
		h.handleNonStream(w, chatBody, model)
	} else {
		h.handleStream(w, r, chatBody, model)
	}
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

func (h *ResponsesHandler) handleNonStream(w http.ResponseWriter, chatBody, model string) {
	resp, err := h.Upstream.Send([]byte(chatBody))
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

	// Store request messages + reasoning for previous_response_id continuity
	respID := gjson.Get(responsesBody, "id").String()
	messages := gjson.Get(chatBody, "messages").Raw
	reasoning := convert.ExtractReasoning(respStr)
	h.Store.Put(respID, messages, reasoning)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(responsesBody))
}

func (h *ResponsesHandler) handleStream(w http.ResponseWriter, r *http.Request, chatBody, model string) {
	resp, err := h.Upstream.Send([]byte(chatBody))
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

	// Emit completion events
	finalEvents := state.Done()
	for _, e := range finalEvents {
		w.Write([]byte(e))
		flusher.Flush()
	}

	// Store request messages + reasoning for multi-turn continuity
	reasoning := state.GetReasoningText()
	messages := gjson.Get(chatBody, "messages").Raw
	h.Store.Put(respID, messages, reasoning)
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
