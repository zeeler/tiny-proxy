package proxy

import (
	"encoding/json"
	"net/http"

	"github.com/zeeler/codex-miniproxy/upstream"
)

type ModelsResponse struct {
	Object string      `json:"object"`
	Data   []ModelItem `json:"data"`
}

type ModelItem struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

func handleModels(w http.ResponseWriter, r *http.Request, models []upstream.ModelInfo) {
	var items []ModelItem
	for _, m := range models {
		items = append(items, ModelItem{
			ID:      m.ID,
			Object:  "model",
			Created: 1717000000,
			OwnedBy: m.Provider,
		})
	}
	resp := ModelsResponse{Object: "list", Data: items}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
