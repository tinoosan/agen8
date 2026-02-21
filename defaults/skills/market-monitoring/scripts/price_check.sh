#!/usr/bin/env bash
set -euo pipefail
#
# Quick stock/crypto price check.
#
# Usage:
#   ./price_check.sh AAPL                    # single stock
#   ./price_check.sh AAPL MSFT GOOG          # multiple stocks
#   ./price_check.sh BTC-USD ETH-USD         # crypto (Yahoo format)
#   ./price_check.sh --watchlist watch.txt    # file with one symbol per line
#
# Environment:
#   FINNHUB_API_KEY     – Finnhub key (optional, for real-time quotes)
#   ALPHA_VANTAGE_API_KEY – Alpha Vantage key (optional)
#
# Falls back to Yahoo Finance (no key required).

usage() {
    echo "Usage: $0 <symbol> [symbol2 ...] [--watchlist file.txt]"
    exit 1
}

SYMBOLS=()
WATCHLIST=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --watchlist) WATCHLIST="$2"; shift 2 ;;
        -h|--help) usage ;;
        *) SYMBOLS+=("${1^^}"); shift ;;
    esac
done

if [ -n "$WATCHLIST" ] && [ -f "$WATCHLIST" ]; then
    while IFS= read -r line; do
        line=$(echo "$line" | tr -d '[:space:]')
        [ -z "$line" ] && continue
        [[ "$line" == \#* ]] && continue
        SYMBOLS+=("${line^^}")
    done < "$WATCHLIST"
fi

if [ ${#SYMBOLS[@]} -eq 0 ]; then
    usage
fi

# --- Fetch via Finnhub --------------------------------------------------------
fetch_finnhub() {
    local sym="$1" key="$2"
    local resp
    resp=$(curl -sf --max-time 8 "https://finnhub.io/api/v1/quote?symbol=${sym}&token=${key}" 2>/dev/null) || return 1
    python3 -c "
import json, sys
d = json.loads('${resp}')
c = d.get('c', 0)
pc = d.get('pc', 0)
change = c - pc if pc else 0
pct = (change / pc * 100) if pc else 0
arrow = '▲' if change >= 0 else '▼'
print(json.dumps({
    'symbol': '${sym}',
    'price': c,
    'change': round(change, 2),
    'change_pct': round(pct, 2),
    'direction': arrow,
    'high': d.get('h'),
    'low': d.get('l'),
    'source': 'finnhub'
}))
" 2>/dev/null
}

# --- Fetch via Yahoo Finance --------------------------------------------------
fetch_yahoo() {
    local sym="$1"
    local resp
    resp=$(curl -sf --max-time 10 \
        -H "User-Agent: Mozilla/5.0" \
        "https://query1.finance.yahoo.com/v8/finance/chart/${sym}?interval=1d&range=2d" 2>/dev/null) || return 1
    python3 -c "
import json, sys
d = json.loads('''${resp}''')
meta = d['chart']['result'][0]['meta']
c = meta.get('regularMarketPrice', 0)
pc = meta.get('previousClose', 0)
change = c - pc if pc else 0
pct = (change / pc * 100) if pc else 0
arrow = '▲' if change >= 0 else '▼'
print(json.dumps({
    'symbol': meta.get('symbol', '${sym}'),
    'price': c,
    'change': round(change, 2),
    'change_pct': round(pct, 2),
    'direction': arrow,
    'currency': meta.get('currency', 'USD'),
    'exchange': meta.get('exchangeName', ''),
    'source': 'yahoo'
}))
" 2>/dev/null
}

# --- Main loop ----------------------------------------------------------------
FINNHUB_KEY="${FINNHUB_API_KEY:-}"

echo "["
FIRST=true
for SYM in "${SYMBOLS[@]}"; do
    RESULT=""
    if [ -n "$FINNHUB_KEY" ]; then
        RESULT=$(fetch_finnhub "$SYM" "$FINNHUB_KEY" 2>/dev/null) || true
    fi
    if [ -z "$RESULT" ]; then
        RESULT=$(fetch_yahoo "$SYM" 2>/dev/null) || RESULT="{\"symbol\": \"$SYM\", \"error\": \"fetch failed\"}"
    fi
    if [ "$FIRST" = true ]; then
        FIRST=false
    else
        echo ","
    fi
    echo "  $RESULT"
done
echo ""
echo "]"
