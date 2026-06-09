package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"
)

// syncDomainToPanel creates or updates the panel config for a given domain.
// panelConfigName must be set on the domain; if empty, sync is skipped.
func syncDomainToPanel(panel *panelClient, d DomainConfig) {
	if d.PanelConfigName == "" {
		log.Printf("[panel-sync] Domain %s has no panelConfigName, skipping.", d.Domain)
		return
	}

	log.Printf("[panel-sync] Syncing domain %s → panel config '%s'...", d.Domain, d.PanelConfigName)

	// 1. List existing panel configs
	data, status, err := panel.do("GET", "/api/subscription-page-configs/", nil)
	if err != nil || status != 200 {
		log.Printf("[panel-sync] ERROR: list configs (status=%d): %v", status, err)
		return
	}

	var list listResponse
	if err := json.Unmarshal(data, &list); err != nil {
		log.Printf("[panel-sync] ERROR: parse list: %v", err)
		return
	}

	// 2. Find existing config by name
	var existingUUID string
	for _, cfg := range list.Response.Configs {
		if cfg.Name == d.PanelConfigName {
			existingUUID = cfg.UUID
			break
		}
	}

	// 3. Create if not found
	if existingUUID == "" {
		log.Printf("[panel-sync] Creating panel config '%s'...", d.PanelConfigName)
		body, status, err := panel.do("POST", "/api/subscription-page-configs/", createRequest{Name: d.PanelConfigName})
		if err != nil || (status != 200 && status != 201) {
			log.Printf("[panel-sync] ERROR: create (status=%d): %v — %s", status, err, string(body))
			return
		}
		var created createResponse
		if err := json.Unmarshal(body, &created); err != nil {
			log.Printf("[panel-sync] ERROR: parse create: %v", err)
			return
		}
		existingUUID = created.Response.UUID
		log.Printf("[panel-sync] Created '%s' (uuid=%s).", d.PanelConfigName, existingUUID)
	} else {
		log.Printf("[panel-sync] Found existing '%s' (uuid=%s).", d.PanelConfigName, existingUUID)
	}

	// 4. Build config patch and update
	patch := buildDomainConfigPatch(d)
	patchReq := updateRequest{
		UUID:   existingUUID,
		Name:   d.PanelConfigName,
		Config: patch,
	}

	body, status, err := panel.do("PATCH", "/api/subscription-page-configs/", patchReq)
	if err != nil || status >= 400 {
		log.Printf("[panel-sync] WARN: update (status=%d): %v — %s", status, err, string(body))
		return
	}
	log.Printf("[panel-sync] Updated '%s' (status=%d).", d.PanelConfigName, status)
}

func buildDomainConfigPatch(d DomainConfig) map[string]any {
	branding := map[string]any{
		"name": d.Brand.Name,
	}
	if d.Brand.SupportURL != "" {
		branding["supportUrl"] = d.Brand.SupportURL
	}
	if d.Brand.LogoURL != "" {
		branding["logoUrl"] = d.Brand.LogoURL
	}

	uiConfig := map[string]any{
		"subscriptionInfoBlockType":   d.UI.SubscriptionInfoBlockType,
		"installationGuidesBlockType": d.UI.InstallationGuidesBlockType,
	}

	baseSettings := map[string]any{
		"metaTitle":          d.Meta.Title,
		"metaDescription":    d.Meta.Description,
		"hideGetLinkButton":  d.UI.HideGetLinkButton,
		"showConnectionKeys": d.UI.ShowConnectionKeys,
	}

	inner := map[string]any{
		"branding":     branding,
		"uiConfig":     uiConfig,
		"baseSettings": baseSettings,
	}

	if len(d.Locale.Additional) > 0 {
		inner["additionalLocales"] = d.Locale.Additional
	}

	return map[string]any{
		"config": inner,
	}
}

// autoSetup reads all domains from MySQL and syncs them to the panel.
func autoSetup(panel *panelClient) {
	log.Println("[auto-setup] Starting auto-setup from MySQL...")

	domains, err := listDomains()
	if err != nil {
		log.Printf("[auto-setup] ERROR: list domains: %v", err)
		return
	}

	if len(domains) == 0 {
		log.Println("[auto-setup] No domains in MySQL, nothing to sync.")
		return
	}

	log.Printf("[auto-setup] Syncing %d domain(s) to panel...", len(domains))
	for _, d := range domains {
		syncDomainToPanel(panel, d)
	}

	triggerNestReload()
}

// triggerNestReload tells NestJS to reload subpage configs + domain configs from APIs.
func triggerNestReload() {
	backendURL := getEnv("BACKEND_URL", "http://remnawave-subscription-page:3010")
	token := getEnv("INTERNAL_API_TOKEN", "")

	time.Sleep(3 * time.Second)

	req, err := http.NewRequest(http.MethodPost, backendURL+"/internal/reload", nil)
	if err != nil {
		log.Printf("[reload] ERROR: build request: %v", err)
		return
	}
	if token != "" {
		req.Header.Set("X-Internal-Token", token)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[reload] ERROR: backend unreachable: %v", err)
		return
	}
	defer resp.Body.Close()
	log.Printf("[reload] NestJS reload triggered → HTTP %d.", resp.StatusCode)
}
