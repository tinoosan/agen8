DATA_DIR ?= ./data

.PHONY: run seed-clean seed-list web-install web-build build dev dev-go dev-web

# Seeding is automatic at startup (from ./defaults) for now.
run:
	@go run ./cmd/agen8 --data-dir "$(DATA_DIR)"

# Web UI targets
web-install:
	@cd web && npm install

web-build: web-install
	@cd web && npm run build

# Build the full binary (requires web assets to be built first)
build: web-build
	@go build -o bin/agen8 ./cmd/agen8

# Dev mode: Go daemon with Air hot-reload + Vite dev server (HMR)
# Access the UI at http://localhost:5173
dev:
	@$(MAKE) -j2 dev-go dev-web

dev-go:
	@air

dev-web: web-install
	@cd web && npm run dev

seed-list:
	@echo "DATA_DIR=$(DATA_DIR)"
	@echo ""
	@echo "Roles:"
	@if [ -d "$(DATA_DIR)/roles" ]; then \
		(cd "$(DATA_DIR)/roles" && find . -mindepth 1 -maxdepth 1 -type d -print | sed 's|^\\./||' | sort); \
	else \
		echo "(none)"; \
	fi
	@echo ""
	@echo "Skills:"
	@if [ -d "$(DATA_DIR)/skills" ]; then \
		(cd "$(DATA_DIR)/skills" && find . -mindepth 1 -maxdepth 1 -type f -name '*.md' -print | sed 's|^\\./||' | sort); \
	else \
		echo "(none)"; \
	fi

seed-clean:
	@rm -rf "$(DATA_DIR)/roles" "$(DATA_DIR)/skills"

