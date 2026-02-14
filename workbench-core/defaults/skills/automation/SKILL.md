---
name: automation
description: Design, build, and maintain automated workflows that save time, reduce errors, and run reliably without manual intervention.
compatibility: "Requires bash, curl. Supports macOS (launchd) and Linux (cron) for scheduling."
---

# Instructions

Use this skill to eliminate repetitive manual work by building automated workflows. This covers everything from simple task scripts to scheduled jobs, monitoring pipelines, notification systems, and multi-step orchestration. The goal is reliable, auditable automation that runs consistently.

## When to use

- A task is performed more than twice and follows a predictable pattern.
- Manual execution is error-prone, slow, or tedious.
- You need scheduled or triggered execution (daily reports, event-driven alerts, periodic checks).
- Multiple steps need to be chained together reliably.
- You want consistent, auditable execution with logs and outputs.

## Workflow

1. **Identify the automation candidate.** Clarify: What triggers it? What are the inputs? What's the expected output? What does success look like? How often does it run?

2. **Assess automation value.** Quick test: (time saved per run × frequency) vs. (time to build + maintain). Don't automate a 2-minute monthly task that takes a day to script.

3. **Choose the right approach.** Match complexity to the solution (see decision matrix below).

4. **Prototype minimally.** Build the smallest version that demonstrates end-to-end behavior. Test with real inputs.

5. **Add reliability.** Error handling, retries for flaky operations, input validation, logging, and idempotence. An automation that fails silently is worse than doing it manually.

6. **Schedule or trigger.** Set up the execution method — cron, webhook, file watcher, event listener, or manual trigger with parameters.

7. **Monitor and maintain.** Track success/failure rates, output quality, and execution time. Set up alerts for failures. Review periodically for relevance.

## Decision Matrix

| Complexity | Solution | When to use |
|-----------|----------|-------------|
| Simple | Single bash/python script | One-step task, no dependencies |
| Moderate | Script with config file | Parameterized task, multiple environments |
| Complex | Multi-step workflow | Dependencies between steps, conditional logic |
| Recurring | Scheduled job (cron/scheduler) | Time-based execution |
| Event-driven | Webhook/watcher/trigger | React to external events |
| Orchestrated | Workflow engine | Many steps, parallelism, error recovery |

## Decision rules

- **Automate the 80%.** Don't try to handle every edge case — automate the common path and document manual fallbacks for exceptions.
- **Idempotent by default.** Running the automation twice should be safe — no duplicate data, no double-sends, no corrupted state.
- **Fail loudly.** Log errors, send alerts, and exit with non-zero codes. Silent failures are the worst kind.
- **Keep it simple.** A 20-line bash script beats a 500-line framework for most automation tasks. Match complexity to need.
- **Version control everything.** Scripts, configs, schedules — all in version control. "It was on that one server" is not a strategy.
- **Separate config from code.** Don't hardcode paths, URLs, credentials, or environment-specific values.

## Quality checks

- Automation runs successfully end-to-end with representative inputs.
- Error cases are handled (bad input, network failure, missing files).
- Logs capture: start time, key steps, outcome, duration, and any errors.
- Running twice produces the same result (idempotence verified).
- Documentation covers: purpose, inputs, outputs, schedule, and how to troubleshoot.

## Automation Categories

### Task Automation
Single operations made repeatable:
- File processing (rename, convert, organize, compress)
- Data extraction and formatting
- Report generation from templates
- Environment setup and teardown

### Workflow Automation
Multi-step processes with dependencies:
- Build → test → deploy pipelines
- Data collect → clean → analyze → report
- Research → draft → review → publish
- Ingest → validate → transform → distribute

### Monitoring Automation
Continuous observation with alerting:
- Health checks (is the service up?)
- Metric tracking (has the KPI crossed a threshold?)
- Change detection (did the competitor's pricing page change?)
- Freshness checks (is the data older than expected?)

### Notification Automation
Event-triggered communication:
- Alert on failure/success of other automations
- Digest emails or summaries on schedule
- Threshold-based notifications (budget exceeded, metric anomaly)
- Status updates to stakeholders

## Templates

### Automation Script
```bash
#!/usr/bin/env bash
set -euo pipefail

# --- Config ---
LOG_FILE="automation_$(date +%Y%m%d_%H%M%S).log"
INPUT="${1:?Usage: $0 <input_path>}"
OUTPUT="${2:?Usage: $0 <input_path> <output_path>}"

# --- Functions ---
log() { echo "[$(date -Iseconds)] $*" | tee -a "$LOG_FILE"; }
die() { log "ERROR: $*"; exit 1; }

# --- Main ---
log "Starting automation"
log "Input: $INPUT, Output: $OUTPUT"

[ -f "$INPUT" ] || die "Input file not found: $INPUT"

# <core logic here>

log "Automation completed successfully"
```

### Automation Specification
```markdown
# Automation: <Name>

**Purpose**: <What it does and why>
**Trigger**: <Manual / Cron / Event / Webhook>
**Schedule**: <If recurring: frequency and time>
**Owner**: <Who maintains it>

## Inputs
| Input | Source | Format | Required |
|-------|--------|--------|----------|
| <input> | <where from> | <format> | Yes/No |

## Steps
1. <Step with expected outcome>
2. <Step with expected outcome>
3. <Step with expected outcome>

## Outputs
| Output | Destination | Format |
|--------|------------|--------|
| <output> | <where it goes> | <format> |

## Error Handling
| Error | Detection | Response |
|-------|-----------|----------|
| <error type> | <how detected> | <what happens> |

## Monitoring
- **Success indicator**: <how you know it worked>
- **Alert on failure**: <who gets notified and how>
- **Review frequency**: <how often to check it's still relevant>
```

### Scheduled Job
```markdown
# Scheduled Job: <Name>

**Schedule**: <cron expression or plain English>
**Runtime**: ~<expected duration>

## Pre-conditions
- <What must be true before this runs>

## Execution
1. <Step>
2. <Step>

## Post-conditions
- <What should be true after this runs>

## Failure recovery
- <How to re-run if it fails>
```

## Example

**Task**: "Automate weekly competitor pricing checks"

**Automation**:
```markdown
## Weekly Competitor Pricing Monitor

**Trigger**: Cron — every Monday at 8:00 AM
**Runtime**: ~5 minutes

### Steps
1. Fetch pricing pages for 5 competitors via http_fetch
2. Parse price tables, extract product/price pairs
3. Compare to last week's prices (stored in pricing_history.csv)
4. Generate diff report highlighting changes >5%
5. Append current prices to history file
6. If any significant changes detected, create summary alert

### Error Handling
- If a competitor site is unreachable: log warning, skip, continue with others
- If parsing fails (layout changed): alert for manual review
- If no changes detected: log "no changes" — don't alert

### Output
- Updated pricing_history.csv
- Weekly pricing diff report (only if changes found)
```

## Advanced Techniques

### Retry with backoff
For flaky operations (network calls, API requests):
```
Attempt 1: immediate
Attempt 2: wait 5s
Attempt 3: wait 30s
Attempt 4: wait 2m
Give up: alert and log
```

### Pipeline composition
Chain small automations into larger workflows:
```
collect_data.sh → clean_data.py → generate_report.sh → send_email.sh
```
Each step reads from the previous step's output. If any step fails, the pipeline stops and alerts.

### Dry run mode
Always build a `--dry-run` flag that shows what would happen without doing it:
```bash
if [ "$DRY_RUN" = "true" ]; then
    log "DRY RUN: Would process $COUNT files"
else
    process_files
fi
```

### Change detection
Only act when something actually changed:
```
1. Fetch current state
2. Compare to stored previous state (hash, diff, timestamp)
3. If different: process and update stored state
4. If same: log "no changes" and exit
```

## Anti-patterns

- **Automating too early**: Don't automate until you've done it manually at least twice and understand the full process
- **No error handling**: "It works on my machine" doesn't count — handle network failures, missing files, bad data
- **No logging**: If you can't tell whether it ran or what it did, it's not production-ready
- **Hardcoded everything**: Paths, URLs, and credentials in the script guarantee it breaks when anything changes
- **Over-engineering**: Don't build a Kubernetes operator for a task that needs a cron job
- **Forgotten automations**: Automations that nobody monitors become ticking time bombs

## When NOT to use

- One-off tasks (just do them manually)
- Tasks requiring human judgment at every step (automate the routine parts, not the decisions)
- Exploratory work where the process isn't yet defined (figure out the process first)
- Tasks that change shape frequently (the automation will constantly break)

## Scripts

This skill includes utility scripts in the `scripts/` directory and templates in `assets/`.

### Utilities

| Script | Purpose | Example |
|--------|---------|---------|
| `retry.sh` | Retry any command with backoff | `./scripts/retry.sh 3 --backoff exponential curl -sf https://api.example.com` |
| `health_check.sh` | Check HTTP, TCP, file, or command health | `./scripts/health_check.sh https://api.example.com/health` |
| `cron_setup.sh` | Manage cron jobs (add/list/remove) | `./scripts/cron_setup.sh --add "0 9 * * 1-5" "./run.sh" --name daily-job` |

### Examples

| Example | Purpose |
|---------|---------|
| `assets/daily_report.sh` | Template for a daily automated report workflow |

**Health check modes**: HTTP endpoint, TCP port, PID file, arbitrary command, or JSON config for batch checks.

**Cron setup**: On macOS, add `--launchd` to create a launchd plist instead of a cron entry.
