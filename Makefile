# Makefile for version bumping and dependency installation

.PHONY: bump-patch bump-minor bump-major install help tag-release deploy up down logs

# Default target
help:
	@echo "Available targets:"
	@echo ""
	@echo "  deploy      - Pull latest image and start/restart production service"
	@echo "  up          - Build and start the full dev stack"
	@echo "  down        - Stop all containers"
	@echo "  logs        - Follow container logs"
	@echo ""
	@echo "  bump-patch  - Bump patch version (x.x.X) for both backend and frontend"
	@echo "  bump-minor  - Bump minor version (x.X.x) for both backend and frontend"
	@echo "  bump-major  - Bump major version (X.x.x) for both backend and frontend"
	@echo "  install     - Run npm install in both backend and frontend directories"
	@echo "  tag-release - Create and push git tag for current version"

# ── Deploy ──────────────────────────────────────────────────────────────────

# Pull latest image and start/restart production container
deploy:
	@./deploy.sh

# Build and start the full dev stack (MySQL + Go API + Angie + NestJS)
up:
	@./deploy.sh full

# Stop all running containers
down:
	@./deploy.sh down

# Follow logs
logs:
	@./deploy.sh logs

# Bump patch version (x.x.X)
bump-patch:
	@echo "Bumping patch version..."
	@cd backend && npm version patch --no-git-tag-version
	@cd frontend && npm version patch --no-git-tag-version
	@echo "✅ Patch version bumped successfully!"

# Bump minor version (x.X.x)
bump-minor:
	@echo "Bumping minor version..."
	@cd backend && npm version minor --no-git-tag-version
	@cd frontend && npm version minor --no-git-tag-version
	@echo "✅ Minor version bumped successfully!"

# Bump major version (X.x.x)
bump-major:
	@echo "Bumping major version..."
	@cd backend && npm version major --no-git-tag-version
	@cd frontend && npm version major --no-git-tag-version
	@echo "✅ Major version bumped successfully!"

# Install dependencies
install:
	@echo "Installing dependencies..."
	@echo "📦 Installing backend dependencies..."
	@cd backend && npm install
	@echo "📦 Installing frontend dependencies..."
	@cd frontend && npm install
	@echo "✅ Dependencies installed successfully!"

# Combined targets
bump-and-install-patch: bump-patch install
	@echo "🎉 Patch version bumped and dependencies installed!"

bump-and-install-minor: bump-minor install
	@echo "🎉 Minor version bumped and dependencies installed!"

bump-and-install-major: bump-major install
	@echo "🎉 Major version bumped and dependencies installed!"

# Show current versions
show-versions:
	@echo "Current versions:"
	@echo "Backend: $(shell cd backend && node -p "require('./package.json').version")"
	@echo "Frontend: $(shell cd frontend && node -p "require('./package.json').version")"


tag-release:
	@VERSION=$$(cd backend && node -p "require('./package.json').version") && \
	echo "Creating signed tag for version $$VERSION..." && \
	git tag -s "$$VERSION" -m "Release $$VERSION" && \
	git push origin --follow-tags && \
	echo "Signed tag $$VERSION created and pushed"