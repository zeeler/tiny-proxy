package proxy

import (
	"io"
	"log"
	"net/http"
	"time"

	"github.com/tidwall/gjson"

	"github.com/zeeler/tiny-proxy/config"
	"github.com/zeeler/tiny-proxy/session"
	"github.com/zeeler/tiny-proxy/upstream"
)

// extractModel reads the model field from a JSON request body.
func extractModel(body []byte) string {
	return gjson.GetBytes(body, "model").String()
}

// Server is the HTTP proxy server.
type Server struct {
	cfg     *config.Config
	handler http.Handler
}

// NewServer creates a new proxy server with all handlers wired up.
func NewServer(cfg *config.Config) *Server {
	store := session.New(cfg.StoreMax, time.Duration(cfg.StoreTTL)*time.Second)
	router, err := upstream.NewRouter(cfg.Providers)
	if err != nil {
		log.Fatalf("[FATAL] %v", err)
	}

	respHandler := &ResponsesHandler{
		Router: router,
		Store:  store,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		handleModels(w, r, router.Models())
	})
	mux.HandleFunc("/v1/responses", func(w http.ResponseWriter, r *http.Request) {
		respHandler.ServeHTTP(w, r)
	})
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		handleChatPassthrough(w, r, router)
	})

	return &Server{cfg: cfg, handler: mux}
}

func handleChatPassthrough(w http.ResponseWriter, r *http.Request, router *upstream.Router) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 10*1024*1024))
	if err != nil {
		writeError(w, http.StatusBadRequest, "cannot read body")
		return
	}

	// Route by model in request body
	model := "any"
	if m := extractModel(body); m != "" {
		model = m
	}
	client := router.ClientFor(model)

	resp, err := client.Send(body)
	if err != nil {
		log.Printf("[ERROR] chat passthrough: %v", err)
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		writeError(w, http.StatusBadGateway, "cannot read upstream response")
		return
	}

	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)
}

// Start begins listening on the configured port.
func (s *Server) Start() error {
	addr := "127.0.0.1:" + s.cfg.ProxyPort
	log.Printf("[INFO] tiny-proxy starting on %s", addr)
	for _, p := range s.cfg.Providers {
		log.Printf("[INFO] provider %s: %s → %s", p.Name, p.Model, p.BaseURL)
	}
	return http.ListenAndServe(addr, s.handler)
}
