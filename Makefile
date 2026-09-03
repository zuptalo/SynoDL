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

.PHONY: start stop mock tools backend frontend roadmap spec

## start: mock DSM + backend hot reload + frontend hot reload
##   Download sources use the in-repo FAKE sites unless REAL_SOURCES=1.
# The dev backend's environment, shared by `start` and `backend` so the two can
# never drift. It runs STATEFUL: SECRETS_KEY and DATA_DIR get throwaway dev
# values, because accounts and the download sources only exist in stateful mode
# and are otherwise unreachable locally. Override either in server/.env;
# production supplies its own.
#
# By default both download-source drivers are pointed at the in-repo FAKE sites,
# so the catalog can be browsed with no credentials and no traffic to a real
# site. That default is why Discover shows "Zar Title 1…" rather than a real
# catalog: the redirect wins regardless of what an admin has configured.
#
# Set REAL_SOURCES=1 to browse the REAL sites with your own configured
# credentials — `make start REAL_SOURCES=1`, or REAL_SOURCES=1 in server/.env.
# It is opt-in because the fake sites are what make the catalog testable offline,
# and because the real ones should not be reached by accident. The redirect is
# still a BUILD-TIME capability (the `sourcemock` tag): a release build has no
# such variable at all.
DEV_BACKEND_ENV = cd $(SERVER_DIR) && set -a && { [ -f .env ] && . ./.env; }; \
	: $${SECRETS_KEY:=dev-only-not-a-real-secret}; \
	: $${DATA_DIR:=$(CURDIR)/.devdata}; \
	: $${SYNO_TLS_INSECURE:=true}; \
	if [ -n "$(REAL_SOURCES)$$REAL_SOURCES" ]; then \
		unset SOURCE_MOCK_ZARFILM SOURCE_MOCK_THIRTYNAMA; \
		echo "▶ Download sources: REAL sites (using your configured credentials)"; \
	else \
		: $${SOURCE_MOCK_ZARFILM:=https://localhost:8291/mocksrc/zar}; \
		: $${SOURCE_MOCK_THIRTYNAMA:=https://localhost:8291/mocksrc/tn}; \
		echo "▶ Download sources: in-repo FAKE sites (make start REAL_SOURCES=1 for the real ones)"; \
	fi; \
	mkdir -p "$$DATA_DIR"; set +a;

start: tools
	@echo "▶ Starting mock DSM (:8291) + backend (air) + frontend (vite) - Ctrl+C to stop all"
	@trap 'kill 0' INT TERM EXIT; \
		( cd $(SERVER_DIR) && MOCK_TLS=1 go run ./cmd/synomock ) & \
		( $(DEV_BACKEND_ENV) $(AIR) ) & \
		( npm run dev ) & \
		wait

## mock: run only the mock DSM (useful when testing the backend against it by hand)
mock:
	@cd $(SERVER_DIR) && MOCK_TLS=1 go run ./cmd/synomock

## stop: kill any stray dev processes
stop:
	-@pkill -f "$(AIR)" 2>/dev/null || true
	-@pkill -f "synodl" 2>/dev/null || true
	-@pkill -f "synomock" 2>/dev/null || true
	-@pkill -f "vite" 2>/dev/null || true

## backend: run only the backend in hot-reload mode (SYNO_URL defaults to the mock)
##   Runs STATEFUL by default: SECRETS_KEY and DATA_DIR are given dev values so the
##   whole app — accounts, and the download sources, which exist only in stateful
##   mode — is reachable locally. Both are throwaway dev values kept out of git;
##   override either in server/.env. Production supplies its own.
backend: tools
	@$(DEV_BACKEND_ENV) $(AIR)

## frontend: run only the frontend in hot-reload mode
frontend:
	@npm run dev

## tools: install air (Go live-reload) if missing
tools: $(AIR)
$(AIR):
	@echo "▶ Installing air (Go live reload)…"
	@go install github.com/air-verse/air@latest

## roadmap: regenerate ROADMAP.md from specs/ (CI fails if it's stale)
roadmap:
	@python3 scripts/roadmap-gen.py

## spec: start a new numbered spec — make spec CATEGORY=planned DESC="Add search"
##       CATEGORY is planned|adhoc|hotfix (default planned).
spec:
	@scripts/spec-new.sh "$(or $(CATEGORY),planned)" "$(DESC)"
