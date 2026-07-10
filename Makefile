DATA_DIR ?=
RESET_DATA_DIR ?= $(if $(strip $(DATA_DIR)),$(DATA_DIR),$(HOME)/.agen8)
HTTP_ADDR ?= 127.0.0.1:7777
VITE_ADDR ?= 127.0.0.1:5173
DEV_WEB_URL ?= http://$(VITE_ADDR)
DATA_DIR_FLAG := $(if $(strip $(DATA_DIR)),--data-dir "$(DATA_DIR)",)
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GO_LDFLAGS := -X github.com/tinoosan/agen8/pkg/buildinfo.Version=$(VERSION) -X github.com/tinoosan/agen8/pkg/buildinfo.Commit=$(COMMIT) -X github.com/tinoosan/agen8/pkg/buildinfo.BuildDate=$(BUILD_DATE)
WEB_NPM := npm
AIR ?= air
GO_PACKAGES ?= ./cmd/... ./internal/... ./pkg/...
GO_FILES := $(shell git ls-files '*.go')
VITE_HOST := $(word 1,$(subst :, ,$(VITE_ADDR)))
VITE_PORT := $(word 2,$(subst :, ,$(VITE_ADDR)))
LAN_IP ?= $(shell ipconfig getifaddr en0 2>/dev/null || ipconfig getifaddr en1 2>/dev/null || hostname -I 2>/dev/null | awk '{print $$1}' || echo 127.0.0.1)
REMOTE_HTTP_ADDR ?= 0.0.0.0:$(word 2,$(subst :, ,$(HTTP_ADDR)))
REMOTE_VITE_ADDR ?= 0.0.0.0:$(VITE_PORT)
REMOTE_HTTP_HOST := $(word 1,$(subst :, ,$(REMOTE_HTTP_ADDR)))
REMOTE_HTTP_PORT := $(word 2,$(subst :, ,$(REMOTE_HTTP_ADDR)))
REMOTE_VITE_HOST := $(word 1,$(subst :, ,$(REMOTE_VITE_ADDR)))
REMOTE_VITE_PORT := $(word 2,$(subst :, ,$(REMOTE_VITE_ADDR)))
DOCS_DIR ?= docs
DOCS_HOST ?= 0.0.0.0
DOCS_PORT ?= 8088

.PHONY: run clean reset-local-data ensure-air web-install web-build build-go build dev remote dev-remote daemon-remote test test-go test-web lint lint-go lint-web fmt-check guardrails workflow-check manifest-check audit ci race docs

# Seeding is automatic at startup (from ./defaults) for now.
run: daemon-remote

clean:
	@rm -rf ./tmp ./bin ./agen8 ./agen8-mcp ./agen8-mcp-server ./agen8-claude-hook ~/.agen8/daemon.log ~/.agen8/debug.log

reset-local-data:
	@if [ "$(CONFIRM)" != "DELETE" ]; then \
		echo 'Refusing to remove local Agen8 data. Re-run with CONFIRM=DELETE only when an explicit reset is intended.'; \
		exit 1; \
	fi
	@rm -f "$(RESET_DATA_DIR)/agen8.db" "$(RESET_DATA_DIR)/agen8.db-shm" "$(RESET_DATA_DIR)/agen8.db-wal"

ensure-air:
	@command -v $(AIR) >/dev/null

# Web UI targets
web-install:
	@cd web && $(WEB_NPM) ci

web-build: web-install
	@cd web && $(WEB_NPM) run build
	@maps="$$(find internal/web/dist -type f -name '*.map' -print)"; \
	if [ -n "$$maps" ]; then printf 'production source maps are forbidden:\n%s\n' "$$maps" >&2; exit 1; fi

build-go:
	@mkdir -p ./bin
	@go build -ldflags "$(GO_LDFLAGS)" -o bin/agen8 ./cmd/agen8

# Build the full binary (requires web assets to be built first)
build: web-build build-go

test: test-go test-web

test-go:
	@go test $(GO_PACKAGES) -count=1

test-web: web-install
	@cd web && $(WEB_NPM) test -- --run

lint: lint-go lint-web

lint-go:
	@go vet $(GO_PACKAGES)
	@staticcheck $(GO_PACKAGES)
	@revive -config revive.toml -set_exit_status $(GO_PACKAGES)

lint-web: web-install
	@cd web && $(WEB_NPM) run lint -- --max-warnings 0

fmt-check:
	@drift="$$(gofmt -l $(GO_FILES))"; \
	if [ -n "$$drift" ]; then printf 'gofmt drift:\n%s\n' "$$drift" >&2; exit 1; fi

guardrails:
	@go test ./internal/architecture -count=1

workflow-check:
	@actionlint

manifest-check:
	@kubeconform -strict -summary deploy/kubernetes/agen8.yaml

audit: web-install
	@govulncheck $(GO_PACKAGES)
	@gosec -quiet -exclude-generated $(GO_PACKAGES)
	@$(WEB_NPM) audit --package-lock-only --audit-level=low
	@cd web && $(WEB_NPM) audit --audit-level=low

ci: fmt-check guardrails workflow-check manifest-check lint test race audit build

race:
	@go test -race $(GO_PACKAGES) -count=1

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
	go run ./cmd/agen8 daemon start $(DATA_DIR_FLAG) --listener http --http-addr "$(HTTP_ADDR)"

dev-remote: ensure-air web-install
	@set -e; \
	AIR_BIN="$(AIR)"; \
	cd web; AGEN8_WEB_PROXY_TARGET="http://127.0.0.1:$(REMOTE_HTTP_PORT)" $(WEB_NPM) run dev -- --host "$(REMOTE_VITE_HOST)" --port "$(REMOTE_VITE_PORT)" & web_pid=$$!; cd ..; \
	trap 'kill $$web_pid 2>/dev/null || true; wait $$web_pid 2>/dev/null || true' EXIT INT TERM; \
	printf 'daemon: http://%s:%s\n' "$(LAN_IP)" "$(REMOTE_HTTP_PORT)"; \
	printf 'vite:   http://%s:%s\n' "$(LAN_IP)" "$(REMOTE_VITE_PORT)"; \
	AGEN8_DAEMON_LISTENER=http \
	AGEN8_HTTP_ADDR="$(REMOTE_HTTP_ADDR)" \
	AGEN8_LOG_FILE="tmp/daemon.log" \
	AGEN8_DEV_WEB_URL="http://127.0.0.1:$(REMOTE_VITE_PORT)" \
	"$$AIR_BIN" \
		-build.full_bin "./tmp/agen8-dev daemon start $(DATA_DIR_FLAG) --listener http --http-addr \"$(REMOTE_HTTP_ADDR)\""

# Serve the static architecture docs over HTTP for easy reading.
#
#   make docs
#
# Binds to 0.0.0.0 so another device (phone/tablet) on the same Wi-Fi can reach
# it at the printed LAN URL. -c-1 disables caching so doc edits show on reload.
# Override DOCS_PORT / DOCS_HOST / DOCS_DIR as needed.
docs:
	@printf 'docs (LAN):   http://%s:%s/\n' "$(LAN_IP)" "$(DOCS_PORT)"
	@printf 'docs (local): http://127.0.0.1:%s/\n' "$(DOCS_PORT)"
	@npx --yes http-server "$(DOCS_DIR)" -a "$(DOCS_HOST)" -p "$(DOCS_PORT)" -c-1
