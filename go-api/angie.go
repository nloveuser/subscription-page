package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"
)

// ── Angie conf directory ──────────────────────────────────────────────────────

func angieConfdDir() string {
	return getEnv("ANGIE_CONFD_DIR", "/etc/angie/conf.d/domains")
}

// Per-domain server block.
// Uses Angie's native ACME client (acme_le defined in angie.conf).
var domainConfTmpl = template.Must(template.New("domain").Parse(`server {
    server_name {{.Domain}};

    listen 443 ssl;
    listen [::]:443 ssl;
    http2 on;

    acme acme_le;

    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384:ECDHE-ECDSA-CHACHA20-POLY1305:ECDHE-RSA-CHACHA20-POLY1305:DHE-RSA-AES128-GCM-SHA256:DHE-RSA-AES256-GCM-SHA384:DHE-RSA-CHACHA20-POLY1305;
    ssl_session_timeout 1d;
    ssl_session_cache shared:SSL:1m;
    ssl_session_tickets off;
    ssl_certificate $acme_cert_acme_le;
    ssl_certificate_key $acme_cert_key_acme_le;

    # Block internal management API from public access
    location /internal/ {
        deny all;
        return 404;
    }

    location / {
        proxy_http_version 1.1;
        proxy_pass http://rwsp_backend;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    gzip on;
    gzip_vary on;
    gzip_proxied any;
    gzip_comp_level 6;
    gzip_buffers 16 8k;
    gzip_http_version 1.1;
    gzip_min_length 256;
    gzip_types
        application/atom+xml
        application/geo+json
        application/javascript
        application/x-javascript
        application/json
        application/ld+json
        application/manifest+json
        application/rdf+xml
        application/rss+xml
        application/xhtml+xml
        application/xml
        font/eot
        font/otf
        font/ttf
        image/svg+xml
        text/css
        text/javascript
        text/plain
        text/xml;
}
`))

func writeAngieConf(d DomainConfig) error {
	confdDir := angieConfdDir()
	if err := os.MkdirAll(confdDir, 0755); err != nil {
		return fmt.Errorf("mkdir conf.d: %w", err)
	}

	safeName := sanitizeDomainForFilename(d.Domain)
	confPath := filepath.Join(confdDir, safeName+".conf")

	var buf bytes.Buffer
	if err := domainConfTmpl.Execute(&buf, d); err != nil {
		return fmt.Errorf("template: %w", err)
	}

	if err := os.WriteFile(confPath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("write conf: %w", err)
	}
	log.Printf("[angie] Wrote conf: %s", confPath)
	return nil
}

func removeAngieConf(domain string) {
	confPath := filepath.Join(angieConfdDir(), sanitizeDomainForFilename(domain)+".conf")
	if err := os.Remove(confPath); err != nil && !os.IsNotExist(err) {
		log.Printf("[angie] WARN: remove conf %s: %v", confPath, err)
	} else {
		log.Printf("[angie] Removed conf: %s", confPath)
	}
}

func sanitizeDomainForFilename(domain string) string {
	s := strings.ReplaceAll(domain, "..", "")
	s = strings.ReplaceAll(s, "/", "")
	s = strings.ReplaceAll(s, "\\", "")
	return s
}

// ── Docker API — reload Angie via SIGHUP ──────────────────────────────────────

func dockerUnixClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", "/var/run/docker.sock")
			},
		},
		Timeout: 15 * time.Second,
	}
}

// reloadAngie sends SIGHUP to the Angie container — graceful config reload.
func reloadAngie() error {
	containerName := getEnv("ANGIE_CONTAINER", "rwsp-angie")
	url := fmt.Sprintf("http://localhost/containers/%s/kill?signal=HUP", containerName)

	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Host", "localhost")

	resp, err := dockerUnixClient().Do(req)
	if err != nil {
		return fmt.Errorf("docker HUP: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("docker HUP status %d: %s", resp.StatusCode, string(body))
	}

	log.Printf("[angie] Reload signal sent (HTTP %d).", resp.StatusCode)
	return nil
}

// testAngieConfig runs `angie -t` inside the container to validate config before reload.
func testAngieConfig() error {
	containerName := getEnv("ANGIE_CONTAINER", "rwsp-angie")

	execCmd := map[string]any{
		"AttachStdout": true,
		"AttachStderr": true,
		"Cmd":          []string{"angie", "-t"},
	}

	execID, err := dockerExecCreate(containerName, execCmd)
	if err != nil {
		return fmt.Errorf("exec create: %w", err)
	}

	output, exitCode, err := dockerExecStart(execID)
	if err != nil {
		return fmt.Errorf("exec start: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("angie -t failed (exit %d): %s", exitCode, output)
	}

	log.Printf("[angie] Config test passed.")
	return nil
}

func dockerExecCreate(containerName string, cmd map[string]any) (string, error) {
	body, err := json.Marshal(cmd)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("http://localhost/containers/%s/exec", containerName)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Host", "localhost")

	resp, err := dockerUnixClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("exec create status %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		ID string `json:"Id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.ID, nil
}

func dockerExecStart(execID string) (string, int, error) {
	startCmd := map[string]any{"Detach": false, "Tty": false}
	body, _ := json.Marshal(startCmd)

	url := fmt.Sprintf("http://localhost/exec/%s/start", execID)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", -1, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Host", "localhost")

	resp, err := dockerUnixClient().Do(req)
	if err != nil {
		return "", -1, err
	}
	defer resp.Body.Close()
	outputBytes, _ := io.ReadAll(resp.Body)

	// Get exit code from exec inspect
	inspectURL := fmt.Sprintf("http://localhost/exec/%s/json", execID)
	inspectReq, _ := http.NewRequest(http.MethodGet, inspectURL, nil)
	inspectReq.Header.Set("Host", "localhost")

	inspectResp, err := dockerUnixClient().Do(inspectReq)
	if err != nil {
		return string(outputBytes), 0, nil
	}
	defer inspectResp.Body.Close()

	var inspect struct {
		ExitCode int `json:"ExitCode"`
	}
	json.NewDecoder(inspectResp.Body).Decode(&inspect)

	return string(outputBytes), inspect.ExitCode, nil
}
