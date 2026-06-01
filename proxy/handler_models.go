package proxy

import (
	"encoding/json"
	"net/http"
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

func handleModels(w http.ResponseWriter, r *http.Request, model string) {
	resp := ModelsResponse{
		Object: "list",
		Data: []ModelItem{
			{
				ID:      model,
				Object:  "model",
				Created: 1717000000,
				OwnedBy: "tiny-proxy",
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
