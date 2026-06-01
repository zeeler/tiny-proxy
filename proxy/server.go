package proxy

import (
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/terry/tiny-proxy/config"
	"github.com/terry/tiny-proxy/session"
	"github.com/terry/tiny-proxy/upstream"
)

// Server is the HTTP proxy server.
type Server struct {
	cfg     *config.Config
	handler http.Handler
}

// NewServer creates a new proxy server with all handlers wired up.
func NewServer(cfg *config.Config) *Server {
	store := session.New(cfg.StoreMax, time.Duration(cfg.StoreTTL)*time.Second)
	client := upstream.NewClient(cfg.DeepSeekBaseURL, cfg.DeepSeekAPIKey)

	respHandler := &ResponsesHandler{
		Upstream:     client,
		Store:        store,
		DefaultModel: cfg.DeepSeekModel,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		if !checkAuth(w, r, cfg.ProxyAuthKey) {
			return
		}
		handleModels(w, r, cfg.DeepSeekModel)
	})
	mux.HandleFunc("/v1/responses", func(w http.ResponseWriter, r *http.Request) {
		if !checkAuth(w, r, cfg.ProxyAuthKey) {
			return
		}
		respHandler.ServeHTTP(w, r)
	})
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		if !checkAuth(w, r, cfg.ProxyAuthKey) {
			return
		}
		handleChatPassthrough(w, r, client)
	})

	return &Server{cfg: cfg, handler: mux}
}

func handleChatPassthrough(w http.ResponseWriter, r *http.Request, client *upstream.Client) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 10*1024*1024))
	if err != nil {
		writeError(w, http.StatusBadRequest, "cannot read body")
		return
	}

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
	log.Printf("[INFO] upstream: %s", s.cfg.DeepSeekBaseURL)
	log.Printf("[INFO] model: %s", s.cfg.DeepSeekModel)
	log.Printf("[INFO] auth key: %s...", s.cfg.ProxyAuthKey[:min(8, len(s.cfg.ProxyAuthKey))])

	return http.ListenAndServe(addr, s.handler)
}

func checkAuth(w http.ResponseWriter, r *http.Request, expectedKey string) bool {
	if expectedKey == "" {
		return true
	}
	auth := r.Header.Get("Authorization")
	if auth == "" {
		writeError(w, http.StatusUnauthorized, "missing Authorization header")
		return false
	}
	key := strings.TrimPrefix(auth, "Bearer ")
	if key != expectedKey {
		writeError(w, http.StatusUnauthorized, "invalid API key")
		return false
	}
	return true
}
