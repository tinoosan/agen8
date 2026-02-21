#!/usr/bin/env bash
set -euo pipefail
#
# Health check script for services and endpoints.
#
# Usage:
#   ./health_check.sh https://api.example.com/health              # HTTP check
#   ./health_check.sh --tcp localhost:5432                         # TCP port check
#   ./health_check.sh --file /var/run/app.pid                     # PID file check
#   ./health_check.sh --command "docker ps | grep myapp"          # command check
#   ./health_check.sh --config checks.json                        # run multiple checks
#
# checks.json format:
#   {
#     "checks": [
#       {"name": "API", "type": "http", "target": "https://api.example.com/health"},
#       {"name": "Database", "type": "tcp", "target": "localhost:5432"},
#       {"name": "Worker", "type": "command", "target": "pgrep -f worker"}
#     ]
#   }

usage() {
    echo "Usage: $0 [URL] [--tcp host:port] [--file path] [--command CMD] [--config file.json]"
    echo ""
    echo "Options:"
    echo "  URL              HTTP(S) endpoint to check"
    echo "  --tcp host:port  Check if TCP port is open"
    echo "  --file path      Check if file exists (and PID is running if .pid)"
    echo "  --command CMD    Run a command and check exit code"
    echo "  --config FILE    JSON file with multiple checks"
    echo "  --timeout N      Timeout in seconds (default: 5)"
    exit 1
}

TIMEOUT=5
HTTP_URL=""
TCP_TARGET=""
FILE_TARGET=""
CMD_CHECK=""
CONFIG_FILE=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --tcp)     TCP_TARGET="$2"; shift 2 ;;
        --file)    FILE_TARGET="$2"; shift 2 ;;
        --command) CMD_CHECK="$2"; shift 2 ;;
        --config)  CONFIG_FILE="$2"; shift 2 ;;
        --timeout) TIMEOUT="$2"; shift 2 ;;
        -h|--help) usage ;;
        http://*|https://*) HTTP_URL="$1"; shift ;;
        *) echo "ERROR: Unknown argument: $1" >&2; usage ;;
    esac
done

# --- Check functions ----------------------------------------------------------

check_http() {
    local url="$1" name="${2:-HTTP}"
    local start elapsed status_code
    start=$(python3 -c "import time; print(int(time.time()*1000))")
    status_code=$(curl -sf -o /dev/null -w "%{http_code}" --max-time "$TIMEOUT" "$url" 2>/dev/null) || status_code="000"
    elapsed=$(python3 -c "import time; print(int(time.time()*1000) - $start)")

    local status="healthy"
    if [[ "$status_code" =~ ^[45] ]] || [ "$status_code" = "000" ]; then
        status="unhealthy"
    fi

    echo "{\"name\": \"${name}\", \"type\": \"http\", \"target\": \"${url}\", \"status\": \"${status}\", \"http_code\": ${status_code}, \"response_ms\": ${elapsed}}"
}

check_tcp() {
    local target="$1" name="${2:-TCP}"
    local host port start elapsed
    host=$(echo "$target" | cut -d: -f1)
    port=$(echo "$target" | cut -d: -f2)
    start=$(python3 -c "import time; print(int(time.time()*1000))")

    local status="healthy"
    if ! python3 -c "
import socket, sys
s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
s.settimeout(${TIMEOUT})
try:
    s.connect(('${host}', ${port}))
    s.close()
except:
    sys.exit(1)
" 2>/dev/null; then
        status="unhealthy"
    fi

    elapsed=$(python3 -c "import time; print(int(time.time()*1000) - $start)")
    echo "{\"name\": \"${name}\", \"type\": \"tcp\", \"target\": \"${target}\", \"status\": \"${status}\", \"response_ms\": ${elapsed}}"
}

check_file() {
    local path="$1" name="${2:-File}"
    local status="healthy"
    if [ ! -f "$path" ]; then
        status="unhealthy"
        echo "{\"name\": \"${name}\", \"type\": \"file\", \"target\": \"${path}\", \"status\": \"${status}\", \"reason\": \"file not found\"}"
        return
    fi

    # If it's a PID file, check if process is running
    if [[ "$path" == *.pid ]]; then
        local pid
        pid=$(cat "$path" 2>/dev/null | tr -d '[:space:]')
        if [ -n "$pid" ] && ! kill -0 "$pid" 2>/dev/null; then
            status="unhealthy"
            echo "{\"name\": \"${name}\", \"type\": \"pid_file\", \"target\": \"${path}\", \"status\": \"${status}\", \"pid\": ${pid}, \"reason\": \"process not running\"}"
            return
        fi
        echo "{\"name\": \"${name}\", \"type\": \"pid_file\", \"target\": \"${path}\", \"status\": \"${status}\", \"pid\": ${pid}}"
        return
    fi

    echo "{\"name\": \"${name}\", \"type\": \"file\", \"target\": \"${path}\", \"status\": \"${status}\"}"
}

check_command() {
    local cmd="$1" name="${2:-Command}"
    local start elapsed
    start=$(python3 -c "import time; print(int(time.time()*1000))")

    local status="healthy"
    if ! eval "$cmd" >/dev/null 2>&1; then
        status="unhealthy"
    fi

    elapsed=$(python3 -c "import time; print(int(time.time()*1000) - $start)")
    echo "{\"name\": \"${name}\", \"type\": \"command\", \"target\": \"${cmd}\", \"status\": \"${status}\", \"elapsed_ms\": ${elapsed}}"
}

# --- Config-based checks -----------------------------------------------------

run_config_checks() {
    local config_file="$1"
    echo "{"
    echo "  \"timestamp\": \"$(date -u +%Y-%m-%dT%H:%M:%SZ)\","
    echo "  \"checks\": ["

    local checks
    checks=$(python3 -c "
import json, sys
with open('${config_file}') as f:
    d = json.load(f)
checks = d.get('checks', [])
for i, c in enumerate(checks):
    print(f\"{c['type']}|{c['target']}|{c.get('name', c['type'])}\")
")

    local first=true
    while IFS='|' read -r type target name; do
        [ -z "$type" ] && continue
        if [ "$first" = true ]; then first=false; else echo ","; fi
        case "$type" in
            http)    printf "    %s" "$(check_http "$target" "$name")" ;;
            tcp)     printf "    %s" "$(check_tcp "$target" "$name")" ;;
            file)    printf "    %s" "$(check_file "$target" "$name")" ;;
            command) printf "    %s" "$(check_command "$target" "$name")" ;;
            *)       printf "    {\"name\": \"%s\", \"error\": \"unknown type: %s\"}" "$name" "$type" ;;
        esac
    done <<< "$checks"

    echo ""
    echo "  ]"
    echo "}"
}

# --- Main ---------------------------------------------------------------------

if [ -n "$CONFIG_FILE" ]; then
    run_config_checks "$CONFIG_FILE"
elif [ -n "$HTTP_URL" ]; then
    check_http "$HTTP_URL"
elif [ -n "$TCP_TARGET" ]; then
    check_tcp "$TCP_TARGET"
elif [ -n "$FILE_TARGET" ]; then
    check_file "$FILE_TARGET"
elif [ -n "$CMD_CHECK" ]; then
    check_command "$CMD_CHECK"
else
    usage
fi
