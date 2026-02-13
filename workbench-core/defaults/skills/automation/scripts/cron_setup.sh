#!/usr/bin/env bash
set -euo pipefail
#
# Cross-platform cron job setup helper.
#
# Usage:
#   ./cron_setup.sh --add "*/5 * * * *" "/path/to/script.sh"     # add a cron job
#   ./cron_setup.sh --add "0 9 * * 1-5" "python3 daily.py" --name "daily-report"
#   ./cron_setup.sh --list                                         # list cron jobs
#   ./cron_setup.sh --remove "daily-report"                       # remove by name
#   ./cron_setup.sh --validate "*/5 * * * *"                      # validate expression
#
# On macOS, also supports launchd via --launchd flag:
#   ./cron_setup.sh --add "0 9 * * *" "./run.sh" --launchd --name "my-job"
#
# Jobs added with --name get a comment marker for easy identification.

usage() {
    cat << 'EOF'
Usage:
  cron_setup.sh --add "<schedule>" "<command>" [--name <label>] [--launchd]
  cron_setup.sh --list
  cron_setup.sh --remove <name>
  cron_setup.sh --validate "<schedule>"

Schedule format: minute hour day-of-month month day-of-week
Examples:
  "*/5 * * * *"       Every 5 minutes
  "0 9 * * 1-5"       9 AM weekdays
  "0 0 1 * *"         Midnight on the 1st of each month
  "0 */2 * * *"       Every 2 hours
  "@daily"            Once a day at midnight
  "@hourly"           Once an hour
EOF
    exit 1
}

ACTION=""
SCHEDULE=""
COMMAND=""
NAME=""
USE_LAUNCHD=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --add)      ACTION="add"; SCHEDULE="$2"; COMMAND="$3"; shift 3 ;;
        --list)     ACTION="list"; shift ;;
        --remove)   ACTION="remove"; NAME="$2"; shift 2 ;;
        --validate) ACTION="validate"; SCHEDULE="$2"; shift 2 ;;
        --name)     NAME="$2"; shift 2 ;;
        --launchd)  USE_LAUNCHD=true; shift ;;
        -h|--help)  usage ;;
        *)          echo "ERROR: Unknown option: $1" >&2; usage ;;
    esac
done

[ -z "$ACTION" ] && usage

# --- Validate cron expression -------------------------------------------------
validate_cron() {
    local expr="$1"
    # Accept shorthand
    if [[ "$expr" =~ ^@(yearly|annually|monthly|weekly|daily|midnight|hourly|reboot)$ ]]; then
        echo '{"valid": true, "expression": "'"$expr"'", "type": "shorthand"}'
        return 0
    fi
    local fields
    fields=$(echo "$expr" | wc -w | tr -d ' ')
    if [ "$fields" -ne 5 ]; then
        echo '{"valid": false, "expression": "'"$expr"'", "error": "Expected 5 fields, got '"$fields"'"}'
        return 1
    fi
    echo '{"valid": true, "expression": "'"$expr"'", "fields": '"$fields"'}'
    return 0
}

# --- List cron jobs -----------------------------------------------------------
list_cron() {
    local current
    current=$(crontab -l 2>/dev/null) || current=""
    if [ -z "$current" ]; then
        echo '{"jobs": [], "count": 0}'
        return
    fi
    python3 -c "
import json
lines = '''${current}'''.strip().split('\n')
jobs = []
for line in lines:
    line = line.strip()
    if not line or line.startswith('#'):
        # Check for name comment
        continue
    parts = line.split(None, 5)
    if len(parts) >= 6:
        jobs.append({
            'schedule': ' '.join(parts[:5]),
            'command': parts[5],
            'raw': line,
        })
    elif line.startswith('@'):
        parts = line.split(None, 1)
        jobs.append({
            'schedule': parts[0],
            'command': parts[1] if len(parts) > 1 else '',
            'raw': line,
        })
print(json.dumps({'jobs': jobs, 'count': len(jobs)}, indent=2))
"
}

# --- Add cron job -------------------------------------------------------------
add_cron() {
    local schedule="$1" command="$2" name="${3:-}"
    local current
    current=$(crontab -l 2>/dev/null) || current=""

    local new_entry="$schedule $command"
    local new_crontab="$current"

    if [ -n "$name" ]; then
        # Add name comment above the entry
        new_crontab=$(printf "%s\n# workbench:%s\n%s" "$current" "$name" "$new_entry")
    else
        new_crontab=$(printf "%s\n%s" "$current" "$new_entry")
    fi

    echo "$new_crontab" | crontab -
    echo "{\"status\": \"added\", \"schedule\": \"$schedule\", \"command\": \"$command\", \"name\": \"$name\"}"
}

# --- Add launchd plist (macOS) ------------------------------------------------
add_launchd() {
    local schedule="$1" command="$2" name="${3:-workbench.job}"
    local label="com.workbench.${name}"
    local plist_dir="$HOME/Library/LaunchAgents"
    local plist_path="${plist_dir}/${label}.plist"

    mkdir -p "$plist_dir"

    # Parse cron schedule → launchd calendar interval
    local minute hour day month weekday
    read -r minute hour day month weekday <<< "$schedule"

    # Build calendar interval dict
    local cal_entries=""
    [ "$minute" != "*" ] && cal_entries="${cal_entries}<key>Minute</key><integer>${minute}</integer>"
    [ "$hour" != "*" ] && cal_entries="${cal_entries}<key>Hour</key><integer>${hour}</integer>"
    [ "$day" != "*" ] && cal_entries="${cal_entries}<key>Day</key><integer>${day}</integer>"
    [ "$month" != "*" ] && cal_entries="${cal_entries}<key>Month</key><integer>${month}</integer>"
    [ "$weekday" != "*" ] && cal_entries="${cal_entries}<key>Weekday</key><integer>${weekday}</integer>"

    cat > "$plist_path" << PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>${label}</string>
    <key>ProgramArguments</key>
    <array>
        <string>/bin/bash</string>
        <string>-c</string>
        <string>${command}</string>
    </array>
    <key>StartCalendarInterval</key>
    <dict>
        ${cal_entries}
    </dict>
    <key>StandardOutPath</key>
    <string>/tmp/${label}.out.log</string>
    <key>StandardErrorPath</key>
    <string>/tmp/${label}.err.log</string>
</dict>
</plist>
PLIST

    launchctl load "$plist_path" 2>/dev/null || true
    echo "{\"status\": \"added\", \"type\": \"launchd\", \"label\": \"$label\", \"plist\": \"$plist_path\"}"
}

# --- Remove cron job ----------------------------------------------------------
remove_cron() {
    local name="$1"
    local current
    current=$(crontab -l 2>/dev/null) || { echo '{"status": "error", "error": "no crontab"}'; return 1; }

    local new_crontab
    new_crontab=$(echo "$current" | python3 -c "
import sys
lines = sys.stdin.read().strip().split('\n')
result = []
skip_next = False
for line in lines:
    if skip_next:
        skip_next = False
        continue
    if '# workbench:${name}' in line:
        skip_next = True
        continue
    result.append(line)
print('\n'.join(result))
")

    echo "$new_crontab" | crontab -
    echo "{\"status\": \"removed\", \"name\": \"$name\"}"

    # Also try to unload launchd if exists
    local label="com.workbench.${name}"
    local plist="$HOME/Library/LaunchAgents/${label}.plist"
    if [ -f "$plist" ]; then
        launchctl unload "$plist" 2>/dev/null || true
        rm -f "$plist"
        echo "{\"status\": \"removed_launchd\", \"label\": \"$label\"}"
    fi
}

# --- Dispatch -----------------------------------------------------------------
case "$ACTION" in
    validate)
        validate_cron "$SCHEDULE"
        ;;
    list)
        list_cron
        ;;
    add)
        if [ "$USE_LAUNCHD" = true ] && [[ "$(uname)" == "Darwin" ]]; then
            add_launchd "$SCHEDULE" "$COMMAND" "$NAME"
        else
            add_cron "$SCHEDULE" "$COMMAND" "$NAME"
        fi
        ;;
    remove)
        remove_cron "$NAME"
        ;;
    *)
        usage
        ;;
esac
