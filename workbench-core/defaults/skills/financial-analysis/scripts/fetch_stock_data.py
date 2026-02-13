#!/usr/bin/env python3
"""Fetch stock/market data from free APIs.

Usage:
    python fetch_stock_data.py AAPL                        # quote
    python fetch_stock_data.py AAPL --history 30           # 30-day history
    python fetch_stock_data.py AAPL MSFT GOOG              # multiple quotes
    python fetch_stock_data.py AAPL --output json           # JSON output
    python fetch_stock_data.py AAPL --output csv            # CSV output

Environment:
    ALPHA_VANTAGE_API_KEY   – Alpha Vantage key (optional, enables richer data)
    POLYGON_API_KEY         – Polygon.io key (optional, preferred for history)
    FINNHUB_API_KEY         – Finnhub key (optional, used for quotes)

Falls back to Yahoo Finance (no key required) when API keys are not set.
"""

import argparse
import csv
import io
import json
import os
import sys
import urllib.request
import urllib.error
from datetime import datetime, timedelta

# ---------------------------------------------------------------------------
# Provider: Yahoo Finance (no key required)
# ---------------------------------------------------------------------------

def _yahoo_quote(symbol: str) -> dict:
    """Fetch a real-time quote from Yahoo Finance v8 API."""
    url = (
        f"https://query1.finance.yahoo.com/v8/finance/chart/{symbol}"
        f"?interval=1d&range=5d"
    )
    req = urllib.request.Request(url, headers={"User-Agent": "Mozilla/5.0"})
    with urllib.request.urlopen(req, timeout=15) as resp:
        data = json.loads(resp.read())
    result = data["chart"]["result"][0]
    meta = result["meta"]
    return {
        "symbol": meta.get("symbol", symbol).upper(),
        "price": meta.get("regularMarketPrice"),
        "previous_close": meta.get("previousClose"),
        "currency": meta.get("currency", "USD"),
        "exchange": meta.get("exchangeName", ""),
        "timestamp": datetime.utcnow().isoformat() + "Z",
        "source": "yahoo_finance",
    }


def _yahoo_history(symbol: str, days: int) -> list[dict]:
    """Fetch daily OHLCV history from Yahoo Finance."""
    period1 = int((datetime.utcnow() - timedelta(days=days)).timestamp())
    period2 = int(datetime.utcnow().timestamp())
    url = (
        f"https://query1.finance.yahoo.com/v8/finance/chart/{symbol}"
        f"?period1={period1}&period2={period2}&interval=1d"
    )
    req = urllib.request.Request(url, headers={"User-Agent": "Mozilla/5.0"})
    with urllib.request.urlopen(req, timeout=15) as resp:
        data = json.loads(resp.read())
    result = data["chart"]["result"][0]
    timestamps = result.get("timestamp", [])
    ohlcv = result.get("indicators", {}).get("quote", [{}])[0]
    rows = []
    for i, ts in enumerate(timestamps):
        rows.append({
            "date": datetime.utcfromtimestamp(ts).strftime("%Y-%m-%d"),
            "open": ohlcv.get("open", [None])[i],
            "high": ohlcv.get("high", [None])[i],
            "low": ohlcv.get("low", [None])[i],
            "close": ohlcv.get("close", [None])[i],
            "volume": ohlcv.get("volume", [None])[i],
        })
    return rows

# ---------------------------------------------------------------------------
# Provider: Alpha Vantage
# ---------------------------------------------------------------------------

def _av_quote(symbol: str, key: str) -> dict:
    url = (
        f"https://www.alphavantage.co/query?"
        f"function=GLOBAL_QUOTE&symbol={symbol}&apikey={key}"
    )
    with urllib.request.urlopen(url, timeout=15) as resp:
        data = json.loads(resp.read())
    gq = data.get("Global Quote", {})
    return {
        "symbol": gq.get("01. symbol", symbol).upper(),
        "price": float(gq["05. price"]) if "05. price" in gq else None,
        "previous_close": float(gq["08. previous close"]) if "08. previous close" in gq else None,
        "change_pct": gq.get("10. change percent", ""),
        "currency": "USD",
        "timestamp": datetime.utcnow().isoformat() + "Z",
        "source": "alpha_vantage",
    }

# ---------------------------------------------------------------------------
# Provider: Finnhub
# ---------------------------------------------------------------------------

def _finnhub_quote(symbol: str, key: str) -> dict:
    url = f"https://finnhub.io/api/v1/quote?symbol={symbol}&token={key}"
    with urllib.request.urlopen(url, timeout=15) as resp:
        data = json.loads(resp.read())
    return {
        "symbol": symbol.upper(),
        "price": data.get("c"),
        "previous_close": data.get("pc"),
        "high": data.get("h"),
        "low": data.get("l"),
        "open": data.get("o"),
        "currency": "USD",
        "timestamp": datetime.utcnow().isoformat() + "Z",
        "source": "finnhub",
    }

# ---------------------------------------------------------------------------
# Dispatcher
# ---------------------------------------------------------------------------

def fetch_quote(symbol: str) -> dict:
    """Fetch a quote using the best available provider."""
    av_key = os.environ.get("ALPHA_VANTAGE_API_KEY", "").strip()
    fh_key = os.environ.get("FINNHUB_API_KEY", "").strip()
    errors = []
    if av_key:
        try:
            return _av_quote(symbol, av_key)
        except Exception as e:
            errors.append(f"alpha_vantage: {e}")
    if fh_key:
        try:
            return _finnhub_quote(symbol, fh_key)
        except Exception as e:
            errors.append(f"finnhub: {e}")
    try:
        return _yahoo_quote(symbol)
    except Exception as e:
        errors.append(f"yahoo: {e}")
    return {"symbol": symbol, "error": "; ".join(errors)}


def fetch_history(symbol: str, days: int) -> list[dict]:
    """Fetch daily history using the best available provider."""
    try:
        return _yahoo_history(symbol, days)
    except Exception as e:
        return [{"error": str(e)}]


# ---------------------------------------------------------------------------
# Output formatting
# ---------------------------------------------------------------------------

def format_output(data, fmt: str) -> str:
    if fmt == "json":
        return json.dumps(data, indent=2)
    if fmt == "csv":
        if isinstance(data, list) and data:
            buf = io.StringIO()
            w = csv.DictWriter(buf, fieldnames=data[0].keys())
            w.writeheader()
            w.writerows(data)
            return buf.getvalue()
        elif isinstance(data, dict):
            buf = io.StringIO()
            w = csv.DictWriter(buf, fieldnames=data.keys())
            w.writeheader()
            w.writerow(data)
            return buf.getvalue()
    # default: human-readable table
    if isinstance(data, list):
        lines = []
        for row in data:
            lines.append("  ".join(f"{k}={v}" for k, v in row.items()))
        return "\n".join(lines)
    return "  ".join(f"{k}={v}" for k, v in data.items())


def main():
    parser = argparse.ArgumentParser(description="Fetch stock/market data")
    parser.add_argument("symbols", nargs="+", help="Ticker symbol(s)")
    parser.add_argument("--history", type=int, default=0, help="Fetch N days of history")
    parser.add_argument("--output", choices=["json", "csv", "table"], default="json", help="Output format")
    args = parser.parse_args()

    results = []
    for sym in args.symbols:
        sym = sym.upper().strip()
        if args.history > 0:
            rows = fetch_history(sym, args.history)
            for r in rows:
                r["symbol"] = sym
            results.extend(rows)
        else:
            results.append(fetch_quote(sym))

    if len(results) == 1 and args.output != "csv":
        print(format_output(results[0], args.output))
    else:
        print(format_output(results, args.output))


if __name__ == "__main__":
    main()
