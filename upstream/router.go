package upstream

import (
	"fmt"

	"github.com/zeeler/codex-miniproxy/config"
)

// Router maps model names to upstream clients.
type Router struct {
	clients map[string]*Client // model → client
	models  []ModelInfo
}

// ModelInfo describes an available model.
type ModelInfo struct {
	ID       string
	Provider string // provider name for logging
}

// NewRouter creates a router from provider configs. Only enabled providers
// (those with non-empty API keys) are included.
func NewRouter(providers []*config.ProviderConfig) (*Router, error) {
	if len(providers) == 0 {
		return nil, fmt.Errorf("no providers configured — set at least one *_API_KEY env var")
	}

	r := &Router{
		clients: make(map[string]*Client),
	}

	for _, p := range providers {
		client := NewClient(p.BaseURL, p.APIKey)
		r.clients[p.Model] = client
		r.models = append(r.models, ModelInfo{
			ID:       p.Model,
			Provider: p.Name,
		})
	}

	return r, nil
}

// ClientFor returns the upstream client for a given model name.
// Falls back to the first available client if the model is unknown.
func (r *Router) ClientFor(model string) *Client {
	if c, ok := r.clients[model]; ok {
		return c
	}
	// Fallback: return first available client
	for _, c := range r.clients {
		return c
	}
	return nil
}

// Models returns all available model info for the /v1/models endpoint.
func (r *Router) Models() []ModelInfo {
	return r.models
}
