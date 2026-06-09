package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

var db *sql.DB

// ── Domain types (nested JSON for API) ───────────────────────────────────────

type DomainBrand struct {
	Name       string `json:"name"`
	SupportURL string `json:"supportUrl"`
	LogoURL    string `json:"logoUrl"`
}

type DomainMeta struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

type DomainTheme struct {
	PrimaryColor     string `json:"primaryColor"`
	BgColor          string `json:"bgColor"`
	AccentLeftColor  string `json:"accentLeftColor"`
	AccentRightColor string `json:"accentRightColor"`
}

type DomainUI struct {
	SubscriptionInfoBlockType   string `json:"subscriptionInfoBlockType"`
	InstallationGuidesBlockType string `json:"installationGuidesBlockType"`
	HideGetLinkButton           bool   `json:"hideGetLinkButton"`
	ShowConnectionKeys          bool   `json:"showConnectionKeys"`
}

type DomainLocale struct {
	Primary    string   `json:"primary"`
	Additional []string `json:"additional"`
}

type DomainConfig struct {
	ID              int          `json:"id,omitempty"`
	Domain          string       `json:"domain"`
	PanelConfigName string       `json:"panelConfigName"`
	Brand           DomainBrand  `json:"brand"`
	Meta            DomainMeta   `json:"meta"`
	Theme           DomainTheme  `json:"theme"`
	UI              DomainUI     `json:"ui"`
	Locale          DomainLocale `json:"locale"`
	SSLEnabled      bool         `json:"sslEnabled"`
	CreatedAt       string       `json:"createdAt,omitempty"`
	UpdatedAt       string       `json:"updatedAt,omitempty"`
}

// ── MySQL connection ──────────────────────────────────────────────────────────

func initDB() error {
	host := getEnv("MYSQL_HOST", "rwsp-mysql")
	port := getEnv("MYSQL_PORT", "3306")
	user := getEnv("MYSQL_USER", "rwsp")
	pass := os.Getenv("MYSQL_PASSWORD")
	dbname := getEnv("MYSQL_DATABASE", "rwsp")

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci",
		user, pass, host, port, dbname)

	var err error
	db, err = sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	for i := 1; i <= 15; i++ {
		if err = db.Ping(); err == nil {
			log.Printf("[db] MySQL connected (attempt %d).", i)
			break
		}
		log.Printf("[db] Waiting for MySQL... (%d/15): %v", i, err)
		time.Sleep(3 * time.Second)
	}
	if err != nil {
		return fmt.Errorf("ping: %w", err)
	}

	return initSchema()
}

func initSchema() error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS domains (
			id                           INT AUTO_INCREMENT PRIMARY KEY,
			domain                       VARCHAR(255) UNIQUE NOT NULL,
			panel_config_name            VARCHAR(255)        NOT NULL DEFAULT '',
			brand_name                   VARCHAR(255)        NOT NULL DEFAULT '',
			brand_support_url            VARCHAR(500)        NOT NULL DEFAULT '',
			brand_logo_url               VARCHAR(500)        NOT NULL DEFAULT '',
			meta_title                   VARCHAR(255)        NOT NULL DEFAULT '',
			meta_description             VARCHAR(500)        NOT NULL DEFAULT '',
			theme_primary_color          VARCHAR(50)         NOT NULL DEFAULT 'cyan',
			theme_bg_color               VARCHAR(20)         NOT NULL DEFAULT '#161b23',
			theme_accent_left            VARCHAR(50)         NOT NULL DEFAULT 'violet',
			theme_accent_right           VARCHAR(50)         NOT NULL DEFAULT 'cyan',
			ui_subscription_info_block   VARCHAR(50)         NOT NULL DEFAULT 'cards',
			ui_installation_guides_block VARCHAR(50)         NOT NULL DEFAULT 'cards',
			ui_hide_get_link             BOOLEAN             NOT NULL DEFAULT FALSE,
			ui_show_connection_keys      BOOLEAN             NOT NULL DEFAULT TRUE,
			locale_primary               VARCHAR(10)         NOT NULL DEFAULT 'en',
			locale_additional            VARCHAR(255)        NOT NULL DEFAULT '',
			ssl_enabled                  BOOLEAN             NOT NULL DEFAULT TRUE,
			created_at                   TIMESTAMP           DEFAULT CURRENT_TIMESTAMP,
			updated_at                   TIMESTAMP           DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
	`)
	if err != nil {
		return fmt.Errorf("initSchema: %w", err)
	}
	log.Println("[db] Schema ready.")
	return nil
}

// ── CRUD ──────────────────────────────────────────────────────────────────────

func listDomains() ([]DomainConfig, error) {
	rows, err := db.Query(`
		SELECT id, domain, panel_config_name,
		       brand_name, brand_support_url, brand_logo_url,
		       meta_title, meta_description,
		       theme_primary_color, theme_bg_color, theme_accent_left, theme_accent_right,
		       ui_subscription_info_block, ui_installation_guides_block,
		       ui_hide_get_link, ui_show_connection_keys,
		       locale_primary, locale_additional,
		       ssl_enabled, created_at, updated_at
		FROM domains ORDER BY id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []DomainConfig
	for rows.Next() {
		var d DomainConfig
		var localeAdditional string
		var createdAt, updatedAt time.Time

		if err := rows.Scan(
			&d.ID, &d.Domain, &d.PanelConfigName,
			&d.Brand.Name, &d.Brand.SupportURL, &d.Brand.LogoURL,
			&d.Meta.Title, &d.Meta.Description,
			&d.Theme.PrimaryColor, &d.Theme.BgColor, &d.Theme.AccentLeftColor, &d.Theme.AccentRightColor,
			&d.UI.SubscriptionInfoBlockType, &d.UI.InstallationGuidesBlockType,
			&d.UI.HideGetLinkButton, &d.UI.ShowConnectionKeys,
			&d.Locale.Primary, &localeAdditional,
			&d.SSLEnabled, &createdAt, &updatedAt,
		); err != nil {
			return nil, err
		}

		d.Locale.Additional = parseCSV(localeAdditional)
		d.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		d.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
		result = append(result, d)
	}
	return result, rows.Err()
}

func getDomain(domain string) (*DomainConfig, error) {
	row := db.QueryRow(`
		SELECT id, domain, panel_config_name,
		       brand_name, brand_support_url, brand_logo_url,
		       meta_title, meta_description,
		       theme_primary_color, theme_bg_color, theme_accent_left, theme_accent_right,
		       ui_subscription_info_block, ui_installation_guides_block,
		       ui_hide_get_link, ui_show_connection_keys,
		       locale_primary, locale_additional,
		       ssl_enabled, created_at, updated_at
		FROM domains WHERE domain = ?
	`, domain)

	var d DomainConfig
	var localeAdditional string
	var createdAt, updatedAt time.Time

	err := row.Scan(
		&d.ID, &d.Domain, &d.PanelConfigName,
		&d.Brand.Name, &d.Brand.SupportURL, &d.Brand.LogoURL,
		&d.Meta.Title, &d.Meta.Description,
		&d.Theme.PrimaryColor, &d.Theme.BgColor, &d.Theme.AccentLeftColor, &d.Theme.AccentRightColor,
		&d.UI.SubscriptionInfoBlockType, &d.UI.InstallationGuidesBlockType,
		&d.UI.HideGetLinkButton, &d.UI.ShowConnectionKeys,
		&d.Locale.Primary, &localeAdditional,
		&d.SSLEnabled, &createdAt, &updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	d.Locale.Additional = parseCSV(localeAdditional)
	d.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	d.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
	return &d, nil
}

func upsertDomain(d DomainConfig) error {
	localeAdditional := strings.Join(d.Locale.Additional, ",")

	_, err := db.Exec(`
		INSERT INTO domains (
			domain, panel_config_name,
			brand_name, brand_support_url, brand_logo_url,
			meta_title, meta_description,
			theme_primary_color, theme_bg_color, theme_accent_left, theme_accent_right,
			ui_subscription_info_block, ui_installation_guides_block,
			ui_hide_get_link, ui_show_connection_keys,
			locale_primary, locale_additional, ssl_enabled
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			panel_config_name            = VALUES(panel_config_name),
			brand_name                   = VALUES(brand_name),
			brand_support_url            = VALUES(brand_support_url),
			brand_logo_url               = VALUES(brand_logo_url),
			meta_title                   = VALUES(meta_title),
			meta_description             = VALUES(meta_description),
			theme_primary_color          = VALUES(theme_primary_color),
			theme_bg_color               = VALUES(theme_bg_color),
			theme_accent_left            = VALUES(theme_accent_left),
			theme_accent_right           = VALUES(theme_accent_right),
			ui_subscription_info_block   = VALUES(ui_subscription_info_block),
			ui_installation_guides_block = VALUES(ui_installation_guides_block),
			ui_hide_get_link             = VALUES(ui_hide_get_link),
			ui_show_connection_keys      = VALUES(ui_show_connection_keys),
			locale_primary               = VALUES(locale_primary),
			locale_additional            = VALUES(locale_additional),
			ssl_enabled                  = VALUES(ssl_enabled)
	`,
		d.Domain, d.PanelConfigName,
		d.Brand.Name, d.Brand.SupportURL, d.Brand.LogoURL,
		d.Meta.Title, d.Meta.Description,
		d.Theme.PrimaryColor, d.Theme.BgColor, d.Theme.AccentLeftColor, d.Theme.AccentRightColor,
		d.UI.SubscriptionInfoBlockType, d.UI.InstallationGuidesBlockType,
		d.UI.HideGetLinkButton, d.UI.ShowConnectionKeys,
		d.Locale.Primary, localeAdditional, d.SSLEnabled,
	)
	return err
}

func deleteDomain(domain string) (bool, error) {
	res, err := db.Exec(`DELETE FROM domains WHERE domain = ?`, domain)
	if err != nil {
		return false, err
	}
	rows, _ := res.RowsAffected()
	return rows > 0, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func parseCSV(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return []string{}
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			result = append(result, t)
		}
	}
	return result
}
