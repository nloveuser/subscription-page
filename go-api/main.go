package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

// ── Remnawave panel client ────────────────────────────────────────────────────

type panelClient struct {
	baseURL    string
	apiToken   string
	httpClient *http.Client
}

func newPanelClient() *panelClient {
	return &panelClient{
		baseURL:  mustEnv("REMNAWAVE_PANEL_URL"),
		apiToken: mustEnv("REMNAWAVE_API_TOKEN"),
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (c *panelClient) do(method, path string, body any) ([]byte, int, error) {
	var buf io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshal: %w", err)
		}
		buf = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, c.baseURL+path, buf)
	if err != nil {
		return nil, 0, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "rwsp-internal-api/2.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	return data, resp.StatusCode, nil
}

// ── Panel API types ───────────────────────────────────────────────────────────

type subpageConfigEntry struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

type listResponse struct {
	Response struct {
		Configs []subpageConfigEntry `json:"configs"`
	} `json:"response"`
}

type createRequest struct {
	Name string `json:"name"`
}

type createResponse struct {
	Response struct {
		UUID   string         `json:"uuid"`
		Name   string         `json:"name"`
		Config map[string]any `json:"config"`
	} `json:"response"`
}

type updateRequest struct {
	UUID   string         `json:"uuid"`
	Name   string         `json:"name"`
	Config map[string]any `json:"config,omitempty"`
}

// ── Entry point ───────────────────────────────────────────────────────────────

func main() {
	port := getEnv("INTERNAL_API_PORT", "3011")

	// Connect to MySQL (retries internally)
	if err := initDB(); err != nil {
		log.Fatalf("[rwsp-api] MySQL init failed: %v", err)
	}

	// Auto-setup: sync all MySQL domains to panel on startup
	if isTrue(os.Getenv("RWSP_AUTO_SETUP")) {
		panel := newPanelClient()
		go autoSetup(panel)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/api/reload", handleReload)
	mux.HandleFunc("/api/status", handleStatus)
	mux.HandleFunc("/api/domains/sync", handleSyncDomains)
	mux.HandleFunc("/api/domains", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleListDomains(w, r)
		case http.MethodPost:
			handleUpsertDomain(w, r)
		case http.MethodDelete:
			handleDeleteDomain(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/angie/reload", handleAngieReload)

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("[rwsp-api] Internal API listening on :%s  (auto_setup=%s)",
		port, getEnv("RWSP_AUTO_SETUP", "false"))
	log.Fatal(srv.ListenAndServe())
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func checkToken(r *http.Request) bool {
	expected := os.Getenv("INTERNAL_API_TOKEN")
	if expected == "" {
		return true
	}
	return r.Header.Get("X-Internal-Token") == expected
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("[rwsp-api] required env var %s is not set", key)
	}
	return v
}

func isTrue(s string) bool {
	return s == "true" || s == "1" || s == "yes"
}

func maskSecret(s string) string {
	if s == "" {
		return "(not set)"
	}
	return "(set)"
}
