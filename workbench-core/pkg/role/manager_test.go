package role

import (
	"os"
	"testing"
)

func TestLoadRoleFile_ParsesValidityAndTaskPolicy(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/ROLE.md"
	data := []byte(`---
id: general
description: General
obligations:
  - id: keep_inbox_clear
    validity: "10m"
    evidence: ok
task_policy:
  create_tasks_only_if:
    - obligation_unsatisfied
  max_tasks_per_cycle: 2
---

# Guidance
Hi
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write ROLE.md: %v", err)
	}

	r, err := loadRoleFile(path)
	if err != nil {
		t.Fatalf("loadRoleFile: %v", err)
	}
	if got := r.Obligations[0].ValidityRaw; got != "10m" {
		t.Fatalf("ValidityRaw = %q, want %q", got, "10m")
	}
	if got := r.TaskPolicy.MaxTasksPerCycle; got != 2 {
		t.Fatalf("MaxTasksPerCycle = %d, want %d", got, 2)
	}
	if len(r.TaskPolicy.CreateTasksOnlyIf) != 1 || r.TaskPolicy.CreateTasksOnlyIf[0] != "obligation_unsatisfied" {
		t.Fatalf("CreateTasksOnlyIf = %#v", r.TaskPolicy.CreateTasksOnlyIf)
	}
}
