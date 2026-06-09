#!/usr/bin/env bash
set -euo pipefail

# ─────────────────────────────────────────────────────────────────
# One-command deploy for Remnawave Subscription Page
#
# Usage:
#   ./deploy.sh           — pull latest image, start/restart service
#   ./deploy.sh full      — build & start the full dev stack
#                           (MySQL + Go API + Angie + NestJS)
#   ./deploy.sh down      — stop all containers
#   ./deploy.sh logs      — follow logs
# ─────────────────────────────────────────────────────────────────

MODE="${1:-}"

COMPOSE_PROD="docker compose -f docker-compose-prod.yml"
COMPOSE_FULL="docker compose"

check_env() {
    if [ ! -f .env ]; then
        echo "ERROR: .env not found. Copy .env.sample and fill in your values:"
        echo "  cp .env.sample .env && nano .env"
        exit 1
    fi
}

ensure_network() {
    if ! docker network ls --format '{{.Name}}' | grep -q '^remnawave-network$'; then
        echo "Creating Docker network: remnawave-network"
        docker network create remnawave-network
    fi
}

case "$MODE" in
    full)
        check_env
        echo "Building and starting full stack..."
        $COMPOSE_FULL pull --ignore-buildable 2>/dev/null || true
        $COMPOSE_FULL up -d --build
        echo ""
        echo "Full stack is up. Follow logs with: docker compose logs -f"
        ;;

    down)
        echo "Stopping all containers..."
        $COMPOSE_PROD down 2>/dev/null || true
        $COMPOSE_FULL down 2>/dev/null || true
        echo "Done."
        ;;

    logs)
        $COMPOSE_PROD logs -f 2>/dev/null || $COMPOSE_FULL logs -f
        ;;

    "")
        check_env
        ensure_network
        echo "Deploying Remnawave Subscription Page..."
        $COMPOSE_PROD pull
        $COMPOSE_PROD up -d
        echo ""
        echo "Deployed! Container status:"
        $COMPOSE_PROD ps
        echo ""
        echo "Follow logs: docker compose -f docker-compose-prod.yml logs -f"
        ;;

    *)
        echo "Unknown command: $MODE"
        echo "Usage: ./deploy.sh [full|down|logs]"
        exit 1
        ;;
esac
