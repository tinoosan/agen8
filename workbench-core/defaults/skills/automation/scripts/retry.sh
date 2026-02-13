#!/usr/bin/env bash
set -euo pipefail
#
# Generic retry-with-backoff wrapper.
#
# Usage:
#   ./retry.sh 3 curl -sf https://api.example.com/data
#   ./retry.sh 5 --delay 2 python3 script.py
#   ./retry.sh 3 --backoff exponential make test
#   ./retry.sh 3 --on-fail "echo 'All retries failed'" ./deploy.sh
#
# Arguments:
#   <max_retries>   Maximum number of attempts (required, first arg)
#   --delay <secs>  Initial delay between retries (default: 1)
#   --backoff       Backoff strategy: fixed, linear, exponential (default: exponential)
#   --on-fail       Command to run if all retries fail
#   --quiet         Suppress retry progress messages
#   <command ...>   The command to retry

MAX_RETRIES=""
DELAY=1
BACKOFF="exponential"
ON_FAIL=""
QUIET=false
CMD=()

usage() {
    echo "Usage: $0 <max_retries> [--delay N] [--backoff fixed|linear|exponential] [--on-fail CMD] [--quiet] <command...>"
    exit 1
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        --delay)    DELAY="$2"; shift 2 ;;
        --backoff)  BACKOFF="$2"; shift 2 ;;
        --on-fail)  ON_FAIL="$2"; shift 2 ;;
        --quiet)    QUIET=true; shift ;;
        -h|--help)  usage ;;
        *)
            if [ -z "$MAX_RETRIES" ] && [[ "$1" =~ ^[0-9]+$ ]]; then
                MAX_RETRIES="$1"
            else
                CMD+=("$1")
            fi
            shift ;;
    esac
done

[ -z "$MAX_RETRIES" ] && { echo "ERROR: max_retries is required" >&2; usage; }
[ ${#CMD[@]} -eq 0 ] && { echo "ERROR: no command specified" >&2; usage; }

# Compute delay for attempt N
compute_delay() {
    local attempt=$1
    case "$BACKOFF" in
        fixed)       echo "$DELAY" ;;
        linear)      echo $((DELAY * attempt)) ;;
        exponential) python3 -c "print(int($DELAY * (2 ** ($attempt - 1))))" 2>/dev/null || echo $((DELAY * attempt)) ;;
        *)           echo "$DELAY" ;;
    esac
}

# Run with retries
ATTEMPT=0
START_TIME=$(date +%s)

while [ $ATTEMPT -lt "$MAX_RETRIES" ]; do
    ATTEMPT=$((ATTEMPT + 1))

    if [ "$QUIET" = false ]; then
        echo "▶ Attempt ${ATTEMPT}/${MAX_RETRIES}: ${CMD[*]}" >&2
    fi

    if "${CMD[@]}"; then
        ELAPSED=$(( $(date +%s) - START_TIME ))
        if [ "$QUIET" = false ]; then
            echo "✓ Succeeded on attempt ${ATTEMPT} (${ELAPSED}s elapsed)" >&2
        fi
        exit 0
    fi

    EXIT_CODE=$?

    if [ $ATTEMPT -lt "$MAX_RETRIES" ]; then
        WAIT=$(compute_delay $ATTEMPT)
        if [ "$QUIET" = false ]; then
            echo "✗ Attempt ${ATTEMPT} failed (exit ${EXIT_CODE}). Retrying in ${WAIT}s..." >&2
        fi
        sleep "$WAIT"
    fi
done

ELAPSED=$(( $(date +%s) - START_TIME ))

echo "{\"status\": \"failed\", \"attempts\": ${ATTEMPT}, \"max_retries\": ${MAX_RETRIES}, \"elapsed_seconds\": ${ELAPSED}, \"command\": \"${CMD[*]}\"}" >&2

if [ -n "$ON_FAIL" ]; then
    eval "$ON_FAIL"
fi

exit 1
