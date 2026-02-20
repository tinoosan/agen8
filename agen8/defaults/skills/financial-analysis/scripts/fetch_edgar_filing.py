#!/usr/bin/env python3
"""Fetch SEC EDGAR filings for a company.

Usage:
    python fetch_edgar_filing.py AAPL                          # latest 10-K
    python fetch_edgar_filing.py AAPL --type 10-Q              # latest 10-Q
    python fetch_edgar_filing.py AAPL --type 10-K --count 3    # last 3 10-Ks
    python fetch_edgar_filing.py AAPL --download ./filings     # download to dir

Environment:
    SEC_USER_AGENT – Required by SEC. Format: "Name email@example.com"
                     Falls back to a generic agent if not set, but SEC may
                     rate-limit or block requests without a proper identity.

Notes:
    The EDGAR full-text search API is free and requires no API key.
    Rate limit: 10 requests/second per SEC policy.
"""

import argparse
import json
import os
import sys
import time
import urllib.request
import urllib.error
from pathlib import Path

EDGAR_COMPANY_SEARCH = "https://efts.sec.gov/LATEST/search-index?q=%22{cik}%22&dateRange=custom&startdt={start}&enddt={end}&forms={form_type}"
EDGAR_SUBMISSIONS = "https://data.sec.gov/submissions/CIK{cik}.json"
EDGAR_FILING_BASE = "https://www.sec.gov/Archives/edgar/data"

DEFAULT_USER_AGENT = "Agen8Agent contact@example.com"


def _headers() -> dict:
    ua = os.environ.get("SEC_USER_AGENT", "").strip() or DEFAULT_USER_AGENT
    return {"User-Agent": ua, "Accept": "application/json"}


def _get_json(url: str) -> dict:
    req = urllib.request.Request(url, headers=_headers())
    with urllib.request.urlopen(req, timeout=20) as resp:
        return json.loads(resp.read())


def _resolve_cik(ticker: str) -> str:
    """Resolve a ticker symbol to a zero-padded CIK number."""
    url = "https://www.sec.gov/files/company_tickers.json"
    data = _get_json(url)
    ticker_upper = ticker.upper()
    for entry in data.values():
        if entry.get("ticker", "").upper() == ticker_upper:
            return str(entry["cik_str"]).zfill(10)
    raise ValueError(f"Could not resolve ticker '{ticker}' to a CIK number")


def fetch_filings(ticker: str, form_type: str = "10-K", count: int = 5) -> list[dict]:
    """Fetch recent filings metadata from EDGAR."""
    cik = _resolve_cik(ticker)
    url = EDGAR_SUBMISSIONS.format(cik=cik)
    data = _get_json(url)

    recent = data.get("filings", {}).get("recent", {})
    forms = recent.get("form", [])
    dates = recent.get("filingDate", [])
    accessions = recent.get("accessionNumber", [])
    primary_docs = recent.get("primaryDocument", [])
    descriptions = recent.get("primaryDocDescription", [])

    results = []
    for i, form in enumerate(forms):
        if form.upper() != form_type.upper():
            continue
        accession_clean = accessions[i].replace("-", "")
        filing_url = f"{EDGAR_FILING_BASE}/{cik.lstrip('0')}/{accession_clean}/{primary_docs[i]}"
        results.append({
            "ticker": ticker.upper(),
            "cik": cik,
            "form_type": form,
            "filing_date": dates[i],
            "accession_number": accessions[i],
            "description": descriptions[i] if i < len(descriptions) else "",
            "url": filing_url,
        })
        if len(results) >= count:
            break

    return results


def download_filing(filing: dict, output_dir: str) -> str:
    """Download a filing document to disk."""
    out_path = Path(output_dir)
    out_path.mkdir(parents=True, exist_ok=True)

    filename = f"{filing['ticker']}_{filing['form_type']}_{filing['filing_date']}.htm"
    dest = out_path / filename

    req = urllib.request.Request(filing["url"], headers=_headers())
    with urllib.request.urlopen(req, timeout=30) as resp:
        dest.write_bytes(resp.read())

    return str(dest)


def main():
    parser = argparse.ArgumentParser(description="Fetch SEC EDGAR filings")
    parser.add_argument("ticker", help="Company ticker symbol (e.g. AAPL)")
    parser.add_argument("--type", default="10-K", dest="form_type",
                        help="Filing type (default: 10-K). Common: 10-K, 10-Q, 8-K, DEF 14A")
    parser.add_argument("--count", type=int, default=3, help="Number of filings to fetch (default: 3)")
    parser.add_argument("--download", metavar="DIR", help="Download filings to directory")
    parser.add_argument("--output", choices=["json", "table"], default="json")
    args = parser.parse_args()

    filings = fetch_filings(args.ticker, args.form_type, args.count)

    if not filings:
        print(json.dumps({"error": f"No {args.form_type} filings found for {args.ticker}"}))
        sys.exit(1)

    if args.download:
        for f in filings:
            path = download_filing(f, args.download)
            f["local_path"] = path
            time.sleep(0.2)  # respect SEC rate limits

    if args.output == "json":
        print(json.dumps(filings, indent=2))
    else:
        for f in filings:
            print(f"{f['filing_date']}  {f['form_type']}  {f['url']}")


if __name__ == "__main__":
    main()
