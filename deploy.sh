#!/usr/bin/env bash
set -euo pipefail

# ─────────────────────────────────────────────────────────────────
# Remnawave Subscription Page — deploy script
#
# Usage:
#   ./deploy.sh init    — create .env and auto-generate all secrets
#   ./deploy.sh         — pull latest image and start/restart (prod)
#   ./deploy.sh full    — build & start full stack
#                         (MySQL + Go API + Angie + NestJS)
#   ./deploy.sh down    — stop all containers
#   ./deploy.sh logs    — follow logs
# ─────────────────────────────────────────────────────────────────

MODE="${1:-}"

COMPOSE_PROD="docker compose -f docker-compose-prod.yml"
COMPOSE_FULL="docker compose"
ENV_FILE=".env"

# ── Secret helpers ────────────────────────────────────────────────

_gen_secret() {
    # Generate N bytes as hex (default 32 bytes = 64 hex chars)
    openssl rand -hex "${1:-32}"
}

_get_value() {
    grep -m1 "^${1}=" "$ENV_FILE" 2>/dev/null | cut -d= -f2- || true
}

_set_value() {
    local key="$1" val="$2"
    if grep -q "^${key}=" "$ENV_FILE" 2>/dev/null; then
        sed -i "s|^${key}=.*|${key}=${val}|" "$ENV_FILE"
    else
        printf '\n%s=%s\n' "$key" "$val" >> "$ENV_FILE"
    fi
}

# Fill key if its current value is empty or matches a known placeholder
_fill_if_blank() {
    local key="$1" placeholder="${2:-}" bytes="${3:-32}"
    local current
    current=$(_get_value "$key")
    if [ -z "$current" ] || [ "$current" = "$placeholder" ]; then
        _set_value "$key" "$(_gen_secret "$bytes")"
        echo "  + $key"
        return 0
    fi
    return 1
}

# ── Auto-generate all secrets in .env ────────────────────────────

generate_secrets() {
    local any=0

    # Required: JWT signing key (64 bytes = 128 hex chars for extra strength)
    _fill_if_blank "INTERNAL_JWT_SECRET"   ""               64 && any=1 || true

    # Internal API auth token
    _fill_if_blank "INTERNAL_API_TOKEN"    ""               32 && any=1 || true

    # MySQL passwords (replace sample placeholders)
    _fill_if_blank "MYSQL_ROOT_PASSWORD"   "change_me_root" 24 && any=1 || true
    _fill_if_blank "MYSQL_PASSWORD"        "change_me"      24 && any=1 || true

    if [ "$any" -eq 1 ]; then
        echo "Secrets saved to $ENV_FILE"
    else
        echo "All secrets already set — nothing changed."
    fi
}

# ── Prerequisites ─────────────────────────────────────────────────

check_env() {
    if [ ! -f "$ENV_FILE" ]; then
        echo "ERROR: $ENV_FILE not found. Run first:"
        echo "  ./deploy.sh init"
        exit 1
    fi
    # Always fill in any missing/placeholder secrets before deploying
    generate_secrets
}

ensure_network() {
    if ! docker network ls --format '{{.Name}}' | grep -q '^remnawave-network$'; then
        echo "Creating Docker network: remnawave-network"
        docker network create remnawave-network
    fi
}

# ── Commands ──────────────────────────────────────────────────────

cmd_init() {
    if [ ! -f "$ENV_FILE" ]; then
        if [ ! -f .env.sample ]; then
            echo "ERROR: .env.sample not found"
            exit 1
        fi
        cp .env.sample "$ENV_FILE"
        echo "Created $ENV_FILE from .env.sample"
    else
        echo "$ENV_FILE already exists — skipping copy"
    fi

    echo "Generating secrets..."
    generate_secrets

    echo ""
    echo "Now fill in the two required values and you are ready to deploy:"
    echo ""
    echo "  REMNAWAVE_PANEL_URL   — URL of your Remnawave panel"
    echo "                          e.g. https://panel.example.com"
    echo "  REMNAWAVE_API_TOKEN   — from Remnawave Dashboard → Settings → API Tokens"
    echo ""
    echo "  \$ nano $ENV_FILE"
    echo ""
    echo "Then run:  ./deploy.sh"
}

cmd_deploy() {
    check_env
    ensure_network
    echo "Pulling latest image..."
    $COMPOSE_PROD pull
    echo "Starting container..."
    $COMPOSE_PROD up -d
    echo ""
    echo "Container status:"
    $COMPOSE_PROD ps
    echo ""
    echo "Logs: ./deploy.sh logs"
}

cmd_full() {
    check_env
    echo "Building and starting full stack..."
    $COMPOSE_FULL pull --ignore-buildable 2>/dev/null || true
    $COMPOSE_FULL up -d --build
    echo ""
    echo "Full stack is up. Logs: ./deploy.sh logs"
}

cmd_down() {
    echo "Stopping containers..."
    $COMPOSE_PROD down 2>/dev/null || true
    $COMPOSE_FULL down 2>/dev/null || true
    echo "Done."
}

cmd_logs() {
    $COMPOSE_PROD logs -f 2>/dev/null || $COMPOSE_FULL logs -f
}

# ── Dispatch ──────────────────────────────────────────────────────

case "$MODE" in
    init)   cmd_init   ;;
    full)   cmd_full   ;;
    down)   cmd_down   ;;
    logs)   cmd_logs   ;;
    "")     cmd_deploy ;;
    *)
        echo "Unknown command: $MODE"
        echo "Usage: ./deploy.sh [init|full|down|logs]"
        exit 1
        ;;
esac
