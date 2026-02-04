---
name: Automation
description: Create and run scripts and workflows to automate repetitive tasks in the workspace (data extraction, formatting, test orchestration, and environment setup).
---

# Instructions

Use this skill to define, implement, and maintain small, reliable automation that saves time and reduces human error. Focus on repeatable, auditable processes that can be triggered on demand or scheduled.

## When to use

- Repetitive tasks across the workspace (data gathering, formatting, report generation, test orchestration).
- Tasks that have a predictable sequence but are tedious to perform manually.
- You need consistent, auditable execution with logs and outputs.

## Workflow

1. **Identify candidate task(s).** Clarify input, expected outputs, and success criteria.
2. **Choose automation approach.** Decide between quick scripts (bash/python), small workflows, or simple orchestration.
3. **Prototype minimally.** Implement a focused script that demonstrates end-to-end behavior.
4. **Refine and add guards.** Add input validation, error handling, logging, and idempotence considerations.
5. **Test autonomously.** Run the automation against representative data; verify idempotence and reliability.
6. **Package and document.** Provide usage instructions, dependencies, and how to re-run or schedule.
7. **Review and iterate.** If useful, extend automation to related tasks.

## Decision rules

- Prefer small, verifiable automations that can be audited.
- Avoid over-automation; document manual fallback if automation cannot cover a corner case.
- Ensure reproducibility (versioned scripts, simple dependencies).

## Quality checks

- Script passes basic tests and logs outputs clearly.
- Dependencies are minimal and documented.
- Idempotent behavior is demonstrated.
- Documentation includes usage and troubleshooting.

## Templates

### Simple Data Dump Script (example)
```bash
#!/usr/bin/env bash
set -euo pipefail

DATA_SRC="$1"
OUT="$2"

echo "Dumping from $DATA_SRC to $OUT"
# placeholder command
cp "$DATA_SRC" "$OUT"
```

### Simple Python Automation (example)
```python
#!/usr/bin/env python3
import sys
from pathlib import Path

src = Path(sys.argv[1])
dst = Path(sys.argv[2])

print(f"Copying {src} to {dst}")
dst.write_text(src.read_text())
```
