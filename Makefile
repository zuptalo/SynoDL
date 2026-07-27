# SynoDL - root dev orchestration.
# `make start` brings up the whole stack for local development:
#   1. Mock DSM (synomock) on :8291 - a fake Synology NAS, so dev never needs real hardware
#   2. Go backend (synodl) in hot-reload mode via `air`, proxying to the mock
#   3. Frontend (Vite) in hot-reload mode on :5273
#
# All three run concurrently in the foreground; Ctrl+C stops them together.
# Point the backend at a real NAS instead with: SYNO_URL=https://nas:5001 make backend

SHELL := /bin/bash
SERVER_DIR := server
GOBIN := $(shell go env GOPATH)/bin
AIR := $(GOBIN)/air

.PHONY: start stop mock tools backend frontend hooks roadmap spec

## start: mock DSM + backend hot reload + frontend hot reload
start: tools
	@echo "▶ Starting mock DSM (:8291) + backend (air) + frontend (vite) - Ctrl+C to stop all"
	@trap 'kill 0' INT TERM EXIT; \
		( cd $(SERVER_DIR) && go run ./cmd/synomock ) & \
		( cd $(SERVER_DIR) && set -a && { [ -f .env ] && . ./.env; }; set +a; $(AIR) ) & \
		( npm run dev ) & \
		wait

## mock: run only the mock DSM (useful when testing the backend against it by hand)
mock:
	@cd $(SERVER_DIR) && go run ./cmd/synomock

## stop: kill any stray dev processes
stop:
	-@pkill -f "$(AIR)" 2>/dev/null || true
	-@pkill -f "synodl" 2>/dev/null || true
	-@pkill -f "synomock" 2>/dev/null || true
	-@pkill -f "vite" 2>/dev/null || true

## backend: run only the backend in hot-reload mode (SYNO_URL defaults to the mock)
backend: tools
	@cd $(SERVER_DIR) && set -a && { [ -f .env ] && . ./.env; }; set +a; $(AIR)

## frontend: run only the frontend in hot-reload mode
frontend:
	@npm run dev

## tools: install air (Go live-reload) if missing
tools: $(AIR)
$(AIR):
	@echo "▶ Installing air (Go live reload)…"
	@go install github.com/air-verse/air@latest

## hooks: opt in to the repo's git hooks (advisory release-bump pre-push warning)
hooks:
	@git config core.hooksPath scripts/hooks
	@echo "▶ Git hooks enabled (core.hooksPath = scripts/hooks)."
	@echo "  Disable with: git config --unset core.hooksPath"

## roadmap: regenerate ROADMAP.md from specs/ (CI fails if it's stale)
roadmap:
	@python3 scripts/roadmap-gen.py

## spec: start a new numbered spec — make spec CATEGORY=planned DESC="Add search"
##       CATEGORY is planned|adhoc|hotfix (default planned).
spec:
	@scripts/spec-new.sh "$(or $(CATEGORY),planned)" "$(DESC)"
