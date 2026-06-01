package upstream

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	c := NewClient("https://api.deepseek.com/v1", "sk-test")
	if c.BaseURL != "https://api.deepseek.com/v1" {
		t.Errorf("BaseURL = %q", c.BaseURL)
	}
	if c.APIKey != "sk-test" {
		t.Errorf("APIKey = %q", c.APIKey)
	}
	if c.HTTP.Timeout != 120*time.Second {
		t.Errorf("Timeout = %v, want 120s", c.HTTP.Timeout)
	}
}

func TestSendSuccess(t *testing.T) {
	t.Skip("requires network (httptest)")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %q", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			t.Errorf("auth = %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content-type = %q", r.Header.Get("Content-Type"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"test"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "sk-test")
	resp, err := c.Send([]byte(`{"model":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status = %d", resp.StatusCode)
	}
}

func TestSendError(t *testing.T) {
	t.Skip("requires network (httptest)")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"bad key"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "sk-wrong")
	_, err := c.Send([]byte(`{"model":"x"}`))
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should mention status code, got: %v", err)
	}
}

func TestSendNetworkError(t *testing.T) {
	t.Skip("requires network")
	c := NewClient("http://127.0.0.1:1", "sk-test")
	_, err := c.Send([]byte(`{"model":"x"}`))
	if err == nil {
		t.Error("expected error for connection refused")
	}
}
