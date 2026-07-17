package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zeeler/tiny-proxy/session"
	"github.com/zeeler/tiny-proxy/upstream"
)

func TestHandleHealth(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/health", nil)
	handleHealth(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"ok"`) {
		t.Errorf("body = %s", w.Body.String())
	}
}

func TestHandleModels(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/v1/models", nil)
	models := []upstream.ModelInfo{
		{ID: "deepseek-v4-flash", Provider: "deepseek"},
		{ID: "glm-4-flash", Provider: "glm"},
	}
	handleModels(w, r, models)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d", w.Code)
	}
	var resp ModelsResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 models, got %d", len(resp.Data))
	}
	if resp.Data[0].ID != "deepseek-v4-flash" {
		t.Errorf("model[0].id = %q", resp.Data[0].ID)
	}
	if resp.Data[0].OwnedBy != "deepseek" {
		t.Errorf("model[0].owned_by = %q", resp.Data[0].OwnedBy)
	}
	if resp.Data[1].ID != "glm-4-flash" {
		t.Errorf("model[1].id = %q", resp.Data[1].ID)
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, http.StatusBadRequest, "bad stuff")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d", w.Code)
	}
	var body map[string]any
	json.NewDecoder(w.Body).Decode(&body)
	err, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatal("response should contain error object")
	}
	if err["message"] != "bad stuff" {
		t.Errorf("message = %q", err["message"])
	}
	if err["code"] != float64(http.StatusBadRequest) {
		t.Errorf("code = %v", err["code"])
	}
}

func TestInjectHistoryEmptyEntry(t *testing.T) {
	chatBody := `{"messages":[{"role":"user","content":"hi"}]}`
	entry := &session.Entry{Messages: "", Reasoning: ""}
	result := injectHistory(chatBody, entry)
	if result != chatBody {
		t.Error("empty entry should return chat body unchanged")
	}
}

func TestInjectHistoryMergeWithReasoning(t *testing.T) {
	chatBody := `{"messages":[{"role":"user","content":"tell me more"}]}`
	entry := &session.Entry{
		Messages:  `[{"role":"user","content":"hello"},{"role":"assistant","content":"hi there!","reasoning_content":"think..."}]`,
		Reasoning: "thinking step 1...",
	}
	result := injectHistory(chatBody, entry)

	// Should contain both user messages (stored + new)
	if !strings.Contains(result, `"hello"`) {
		t.Error("should contain stored user message")
	}
	if !strings.Contains(result, `"tell me more"`) {
		t.Error("should contain new user message")
	}
	// Should contain reasoning injected into the last assistant message
	if !strings.Contains(result, `"reasoning_content"`) {
		t.Errorf("should have reasoning_content, got:\n%s", result)
	}
}

func TestInjectHistoryInvalidMessages(t *testing.T) {
	chatBody := `{"messages":[]}`
	entry := &session.Entry{
		Messages:  `not-json`,
		Reasoning: "",
	}
	result := injectHistory(chatBody, entry)
	if result != chatBody {
		t.Error("invalid stored messages should return body unchanged")
	}
}
