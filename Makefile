DATA_DIR ?= ./data

.PHONY: seed seed-roles seed-skills seed-list seed-clean run

seed: seed-roles seed-skills

seed-roles:
	@mkdir -p "$(DATA_DIR)"
	@if command -v rsync >/dev/null 2>&1; then \
		rsync -a --delete "defaults/roles/" "$(DATA_DIR)/roles/"; \
	else \
		rm -rf "$(DATA_DIR)/roles"; \
		cp -R "defaults/roles" "$(DATA_DIR)/roles"; \
	fi

seed-skills:
	@mkdir -p "$(DATA_DIR)"
	@if command -v rsync >/dev/null 2>&1; then \
		rsync -a --delete "defaults/skills/" "$(DATA_DIR)/skills/"; \
	else \
		rm -rf "$(DATA_DIR)/skills"; \
		cp -R "defaults/skills" "$(DATA_DIR)/skills"; \
	fi

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

run: seed
	@go run ./workbench-core/cmd/workbench --data-dir "$(DATA_DIR)"

