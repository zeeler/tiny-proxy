package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/terry/tiny-proxy/session"
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
	handleModels(w, r, "deepseek-v4")

	if w.Code != http.StatusOK {
		t.Errorf("status = %d", w.Code)
	}
	var resp ModelsResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Data) == 0 {
		t.Fatal("no models returned")
	}
	if resp.Data[0].ID != "deepseek-v4" {
		t.Errorf("model id = %q", resp.Data[0].ID)
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

func TestCheckAuth(t *testing.T) {
	tests := []struct {
		name   string
		header string
		key    string
		allow  bool
	}{
		{"empty key skips auth", "", "", true},
		{"missing header", "", "sk-key", false},
		{"wrong bearer key", "Bearer wrong", "sk-key", false},
		{"correct bearer key", "Bearer sk-key", "sk-key", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/", nil)
			if tt.header != "" {
				r.Header.Set("Authorization", tt.header)
			}
			got := checkAuth(w, r, tt.key)
			if got != tt.allow {
				t.Errorf("checkAuth = %v, want %v", got, tt.allow)
			}
		})
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
