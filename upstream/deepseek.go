package upstream

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client sends Chat Completions requests to DeepSeek.
type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
}

// NewClient creates a DeepSeek API client with 120s timeout.
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		BaseURL: baseURL,
		APIKey:  apiKey,
		HTTP: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// Send posts a request body to DeepSeek's /chat/completions endpoint.
// Caller must close the response body.
func (c *Client) Send(body []byte) (*http.Response, error) {
	url := c.BaseURL + "/chat/completions"

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Accept", "text/event-stream, application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send: %w", err)
	}

	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("upstream error %d: %s", resp.StatusCode, string(errBody))
	}

	return resp, nil
}
