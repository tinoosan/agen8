DATA_DIR ?= ./data

.PHONY: run seed-clean seed-list

# Seeding is automatic at startup (from ./defaults) for now.
run:
	@go run ./cmd/workbench --data-dir "$(DATA_DIR)"

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

