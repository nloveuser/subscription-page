package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)


// ── Health ────────────────────────────────────────────────────────────────────

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"service": "rwsp-internal-api",
	})
}

// ── NestJS reload proxy ───────────────────────────────────────────────────────

func handleReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !checkToken(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	backendURL := getEnv("BACKEND_URL", "http://remnawave-subscription-page:3010")
	token := os.Getenv("INTERNAL_API_TOKEN")

	req, err := http.NewRequest(http.MethodPost, backendURL+"/internal/reload", nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if token != "" {
		req.Header.Set("X-Internal-Token", token)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)
}

// ── Status ────────────────────────────────────────────────────────────────────

func handleStatus(w http.ResponseWriter, r *http.Request) {
	if !checkToken(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	domains, err := listDomains()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"service":     "rwsp-internal-api",
		"domainCount": len(domains),
		"flags": map[string]string{
			"RWSP_AUTO_SETUP":    getEnv("RWSP_AUTO_SETUP", "false"),
			"RWSP_AUTO_UPDATE":   getEnv("RWSP_AUTO_UPDATE", "false"),
			"INTERNAL_API_TOKEN": maskSecret(os.Getenv("INTERNAL_API_TOKEN")),
		},
	})
}

// ── Domain CRUD ───────────────────────────────────────────────────────────────

// GET /api/domains  — list all domains (used by NestJS on reload)
func handleListDomains(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !checkToken(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	domains, err := listDomains()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if domains == nil {
		domains = []DomainConfig{}
	}
	writeJSON(w, http.StatusOK, domains)
}

// POST /api/domains — upsert a domain
func handleUpsertDomain(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !checkToken(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var d DomainConfig
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}

	d.Domain = strings.ToLower(strings.TrimSpace(d.Domain))
	if d.Domain == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "domain is required"})
		return
	}

	// Apply defaults for empty fields
	if d.Theme.PrimaryColor == "" {
		d.Theme.PrimaryColor = "cyan"
	}
	if d.Theme.BgColor == "" {
		d.Theme.BgColor = "#161b23"
	}
	if d.Theme.AccentLeftColor == "" {
		d.Theme.AccentLeftColor = "violet"
	}
	if d.Theme.AccentRightColor == "" {
		d.Theme.AccentRightColor = "cyan"
	}
	if d.UI.SubscriptionInfoBlockType == "" {
		d.UI.SubscriptionInfoBlockType = "cards"
	}
	if d.UI.InstallationGuidesBlockType == "" {
		d.UI.InstallationGuidesBlockType = "cards"
	}
	if d.Locale.Primary == "" {
		d.Locale.Primary = "en"
	}
	if d.Locale.Additional == nil {
		d.Locale.Additional = []string{}
	}

	// Save to MySQL
	if err := upsertDomain(d); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	log.Printf("[domains] Upserted domain: %s", d.Domain)

	// Async: write Angie conf + reload + sync to panel + reload NestJS
	go func(cfg DomainConfig) {
		// 1. Write per-domain Angie conf file
		if err := writeAngieConf(cfg); err != nil {
			log.Printf("[domains] WARN: write Angie conf for %s: %v", cfg.Domain, err)
			return
		}

		// 2. Validate Angie config before reloading
		if err := testAngieConfig(); err != nil {
			log.Printf("[domains] WARN: Angie config test failed: %v", err)
			// Still attempt reload — Angie will skip bad configs gracefully
		}

		// 3. Reload Angie (triggers ACME cert issuance for new server_name)
		if err := reloadAngie(); err != nil {
			log.Printf("[domains] WARN: Angie reload: %v", err)
		}

		// 4. Sync domain to panel
		panel := newPanelClient()
		syncDomainToPanel(panel, cfg)

		// 5. Trigger NestJS reload
		triggerNestReload()
	}(d)

	// Return saved record
	saved, _ := getDomain(d.Domain)
	if saved == nil {
		saved = &d
	}
	writeJSON(w, http.StatusOK, saved)
}

// DELETE /api/domains — remove a domain (pass {"domain": "..."} in body)
func handleDeleteDomain(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !checkToken(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		Domain string `json:"domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	req.Domain = strings.ToLower(strings.TrimSpace(req.Domain))
	if req.Domain == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "domain is required"})
		return
	}

	deleted, err := deleteDomain(req.Domain)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if !deleted {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "domain not found"})
		return
	}

	log.Printf("[domains] Deleted domain: %s", req.Domain)

	go func(domain string) {
		removeAngieConf(domain)
		if err := reloadAngie(); err != nil {
			log.Printf("[domains] WARN: Angie reload after delete: %v", err)
		}
		triggerNestReload()
	}(req.Domain)

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "domain": req.Domain})
}

// POST /api/domains/sync — re-sync all domains to panel + NestJS reload
func handleSyncDomains(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !checkToken(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	domains, err := listDomains()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	go func(ds []DomainConfig) {
		panel := newPanelClient()
		for _, d := range ds {
			syncDomainToPanel(panel, d)
		}
		triggerNestReload()
	}(domains)

	writeJSON(w, http.StatusAccepted, map[string]any{
		"status":  "accepted",
		"domains": len(domains),
		"message": "sync triggered in background",
	})
}

// POST /api/angie/reload — reload Angie manually
func handleAngieReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !checkToken(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if err := reloadAngie(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "message": "Angie reloaded"})
}
