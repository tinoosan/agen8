DATA_DIR ?=
HTTP_ADDR ?= 127.0.0.1:7777
VITE_ADDR ?= 127.0.0.1:5173
DEV_WEB_URL ?= http://$(VITE_ADDR)
DATA_DIR_FLAG := $(if $(strip $(DATA_DIR)),--data-dir "$(DATA_DIR)",)
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GO_LDFLAGS :=
WEB_NPM := npm
AIR ?= air
GO_PACKAGES ?= ./cmd/... ./internal/... ./pkg/...
VITE_HOST := $(word 1,$(subst :, ,$(VITE_ADDR)))
VITE_PORT := $(word 2,$(subst :, ,$(VITE_ADDR)))

.PHONY: run clean seed-clean seed-list ensure-air web-install web-build build-go build dev remote dev-remote daemon-remote test test-go test-web lint lint-go lint-web fmt-check guardrails ci race install-hooks worktree-create worktree-clean

# Seeding is automatic at startup (from ./defaults) for now.
run: daemon-remote

clean:
	@rm -rf ./tmp ./bin ~/.agen8/agen8.db ~/.agen8/agen8.db-shm ~/.agen8/agen8.db-wal ~/.agen8/daemon.log ~/.agen8/debug.log

ensure-air:
	@command -v $(AIR) >/dev/null

# Web UI targets
web-install:
	@cd web && $(WEB_NPM) ci

web-build: web-install
	@cd web && $(WEB_NPM) run build

build-go:
	@mkdir -p ./bin
	@go build -ldflags "$(GO_LDFLAGS)" -o bin/agen8-mcp ./cmd/agen8-mcp

# Build the full binary (requires web assets to be built first)
build: web-build build-go

test: test-go test-web

test-go:
	@go test $(GO_PACKAGES)

test-web: web-install
	@cd web && $(WEB_NPM) test

lint: lint-go lint-web

lint-go:
	@go vet $(GO_PACKAGES)

lint-go-strict:
	@./scripts/ci/check_go_lint.sh

lint-web: web-install
	@cd web && $(WEB_NPM) run lint

fmt-check:
	@./scripts/ci/check_gofmt.sh

guardrails:
	@./scripts/ci/agent_guardrails.sh

ci: fmt-check guardrails lint test race build

race:
	@./scripts/ci/check_go_race.sh

install-hooks:
	@ln -sf ../../scripts/ci/pre-commit.sh .git/hooks/pre-commit
	@echo "Pre-commit hook installed"

# Development runtime.
#
#   make dev
#   make dev remote
#
# Both commands start one HTTP daemon behind Air and one Vite dev server. The
# browser always opens the daemon URL; Vite is only the hot-reload asset server.
#
# DATA_DIR overrides runtime state. When empty, agen8 uses the platform default.
# macOS: ~/.agen8; Linux: XDG state; Windows: AppData.
# HTTP_ADDR overrides the daemon endpoint. VITE_ADDR overrides the asset server.
dev:
ifeq ($(filter remote,$(MAKECMDGOALS)),)
	@$(MAKE) dev-remote
else
	@:
endif

remote: dev-remote

daemon-remote:
	@AGEN8_DAEMON_LISTENER=http \
	AGEN8_HTTP_ADDR="$(HTTP_ADDR)" \
	go run ./cmd/agen8-mcp daemon start $(DATA_DIR_FLAG) --listener http --http-addr "$(HTTP_ADDR)"

dev-remote: ensure-air web-install
	@set -e; \
	AIR_BIN="$(AIR)"; \
	cd web; $(WEB_NPM) run dev -- --host "$(VITE_HOST)" --port "$(VITE_PORT)" & web_pid=$$!; cd ..; \
	trap 'kill $$web_pid 2>/dev/null || true; wait $$web_pid 2>/dev/null || true' EXIT INT TERM; \
	printf 'daemon: http://%s\n' "$(HTTP_ADDR)"; \
	printf 'vite:   %s\n' "$(DEV_WEB_URL)"; \
	AGEN8_DAEMON_LISTENER=http \
	AGEN8_HTTP_ADDR="$(HTTP_ADDR)" \
	AGEN8_LOG_FILE="tmp/daemon.log" \
	AGEN8_DEV_WEB_URL="$(DEV_WEB_URL)" \
	"$$AIR_BIN" \
		-build.full_bin "./tmp/agen8-mcp daemon start $(DATA_DIR_FLAG) --listener http --http-addr \"$(HTTP_ADDR)\""

worktree-create:
	@./scripts/worktree/create.sh "$(KIND)" "$(TASK)" "$(SLUG)" "$(or $(BASE),dev)"

worktree-clean:
	@./scripts/worktree/cleanup.sh "$(BRANCH)"
