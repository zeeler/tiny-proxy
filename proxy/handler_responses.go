package proxy

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/tidwall/gjson"

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

	// Handle previous_response_id for session continuity
	prevID := gjson.Get(bodyStr, "previous_response_id").String()
	if prevID != "" {
		if entry, ok := h.Store.Get(prevID); ok {
			bodyStr = injectHistory(bodyStr, entry)
		}
	}

	// Convert Responses → Chat Completions request
	chatBody := convert.ConvertRequest(bodyStr)

	if !stream {
		h.handleNonStream(w, chatBody, model)
	} else {
		h.handleStream(w, r, chatBody, model, bodyStr)
	}
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
	responsesBody := convert.ConvertResponse(string(respBody), model)

	// Store for previous_response_id continuity
	respID := gjson.Get(responsesBody, "id").String()
	reasoning := convert.ExtractReasoning(string(respBody))
	h.Store.Put(respID, string(respBody), reasoning)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(responsesBody))
}

func (h *ResponsesHandler) handleStream(w http.ResponseWriter, r *http.Request, chatBody, model, originalReq string) {
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

	// Store reasoning for multi-turn continuity
	reasoning := state.GetReasoningText()
	messages := gjson.Get(chatBody, "messages").Raw
	h.Store.Put(respID, messages, reasoning)
}

// injectHistory injects cached reasoning into the conversation for multi-turn continuity.
func injectHistory(body string, entry *session.Entry) string {
	if entry.Reasoning != "" {
		body = convert.InjectReasoning(body, entry.Reasoning)
	}
	return body
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

// Ensure fmt is used.
var _ = fmt.Sprintf
