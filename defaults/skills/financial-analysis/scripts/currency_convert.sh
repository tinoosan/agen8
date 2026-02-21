#!/usr/bin/env bash
set -euo pipefail
#
# Real-time currency conversion.
#
# Usage:
#   ./currency_convert.sh 100 USD GBP           # convert 100 USD to GBP
#   ./currency_convert.sh 500 EUR JPY           # convert 500 EUR to JPY
#   ./currency_convert.sh --rates USD            # show all rates for USD
#
# Environment:
#   EXCHANGE_RATE_API_KEY  – exchangerate-api.com key (optional, uses free tier without)
#   OPEN_EXCHANGE_APP_ID   – openexchangerates.org app ID (optional fallback)
#
# Fallback: Uses the free frankfurter.app API when no keys are set.

usage() {
    echo "Usage: $0 <amount> <from_currency> <to_currency>"
    echo "       $0 --rates <base_currency>"
    exit 1
}

# --- Providers ----------------------------------------------------------------

fetch_frankfurter() {
    local from="$1" to="$2" amount="$3"
    local url="https://api.frankfurter.app/latest?amount=${amount}&from=${from}&to=${to}"
    local resp
    resp=$(curl -sf --max-time 10 "$url") || { echo "ERROR: frankfurter.app request failed" >&2; return 1; }
    local rate converted
    converted=$(echo "$resp" | python3 -c "import sys,json; d=json.load(sys.stdin); print(list(d['rates'].values())[0])" 2>/dev/null)
    rate=$(python3 -c "print(round(${converted}/${amount}, 6))" 2>/dev/null)
    echo "{\"amount\": ${amount}, \"from\": \"${from}\", \"to\": \"${to}\", \"rate\": ${rate}, \"converted\": ${converted}, \"source\": \"frankfurter.app\"}"
}

fetch_exchangerate_api() {
    local key="$1" from="$2" to="$3" amount="$4"
    local url="https://v6.exchangerate-api.com/v6/${key}/pair/${from}/${to}/${amount}"
    local resp
    resp=$(curl -sf --max-time 10 "$url") || { echo "ERROR: exchangerate-api request failed" >&2; return 1; }
    local rate converted
    rate=$(echo "$resp" | python3 -c "import sys,json; print(json.load(sys.stdin)['conversion_rate'])" 2>/dev/null)
    converted=$(echo "$resp" | python3 -c "import sys,json; print(json.load(sys.stdin)['conversion_result'])" 2>/dev/null)
    echo "{\"amount\": ${amount}, \"from\": \"${from}\", \"to\": \"${to}\", \"rate\": ${rate}, \"converted\": ${converted}, \"source\": \"exchangerate-api.com\"}"
}

fetch_rates() {
    local base="$1"
    local url="https://api.frankfurter.app/latest?from=${base}"
    local resp
    resp=$(curl -sf --max-time 10 "$url") || { echo "ERROR: rates request failed" >&2; exit 1; }
    echo "$resp" | python3 -c "
import sys, json
d = json.load(sys.stdin)
print(json.dumps({'base': d['base'], 'date': d['date'], 'rates': d['rates']}, indent=2))
"
}

# --- Main ---------------------------------------------------------------------

if [ $# -lt 2 ]; then
    usage
fi

if [ "$1" = "--rates" ]; then
    BASE="${2^^}"
    fetch_rates "$BASE"
    exit 0
fi

if [ $# -lt 3 ]; then
    usage
fi

AMOUNT="$1"
FROM="${2^^}"
TO="${3^^}"

# Validate amount is numeric
if ! python3 -c "float('${AMOUNT}')" 2>/dev/null; then
    echo "ERROR: amount must be numeric" >&2
    exit 1
fi

EXCH_KEY="${EXCHANGE_RATE_API_KEY:-}"

if [ -n "$EXCH_KEY" ]; then
    fetch_exchangerate_api "$EXCH_KEY" "$FROM" "$TO" "$AMOUNT"
else
    fetch_frankfurter "$FROM" "$TO" "$AMOUNT"
fi
