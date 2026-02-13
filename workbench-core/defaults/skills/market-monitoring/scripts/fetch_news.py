#!/usr/bin/env python3
"""Fetch news headlines from news APIs.

Usage:
    python fetch_news.py "artificial intelligence"              # search headlines
    python fetch_news.py "Apple" --sources bbc-news,cnn        # specific sources
    python fetch_news.py --top-headlines --country us           # top US headlines
    python fetch_news.py "crypto" --from 2026-02-01 --limit 20 # date range
    python fetch_news.py "Tesla" --output json                 # JSON output

Environment:
    NEWS_API_KEY    – NewsAPI.org key (https://newsapi.org, free tier: 100 req/day)
    GNEWS_API_KEY   – GNews.io key (optional fallback)

Without API keys, falls back to a basic RSS-based news search.
"""

import argparse
import json
import os
import sys
import urllib.request
import urllib.error
import xml.etree.ElementTree as ET
from datetime import datetime


# ---------------------------------------------------------------------------
# Provider: NewsAPI
# ---------------------------------------------------------------------------

def _newsapi_search(query: str, key: str, **kwargs) -> list[dict]:
    """Search news via NewsAPI.org."""
    params = [f"q={urllib.request.quote(query)}", f"apiKey={key}", "sortBy=publishedAt"]
    if kwargs.get("from_date"):
        params.append(f"from={kwargs['from_date']}")
    if kwargs.get("to_date"):
        params.append(f"to={kwargs['to_date']}")
    if kwargs.get("sources"):
        params.append(f"sources={kwargs['sources']}")
    if kwargs.get("language"):
        params.append(f"language={kwargs['language']}")
    limit = kwargs.get("limit", 10)
    params.append(f"pageSize={limit}")

    url = f"https://newsapi.org/v2/everything?{'&'.join(params)}"
    req = urllib.request.Request(url, headers={"User-Agent": "WorkbenchAgent/1.0"})
    with urllib.request.urlopen(req, timeout=15) as resp:
        data = json.loads(resp.read())

    articles = data.get("articles", [])
    return [
        {
            "title": a.get("title", ""),
            "description": a.get("description", ""),
            "source": a.get("source", {}).get("name", ""),
            "url": a.get("url", ""),
            "published_at": a.get("publishedAt", ""),
            "author": a.get("author", ""),
        }
        for a in articles
    ]


def _newsapi_headlines(key: str, **kwargs) -> list[dict]:
    """Fetch top headlines via NewsAPI.org."""
    params = [f"apiKey={key}"]
    if kwargs.get("country"):
        params.append(f"country={kwargs['country']}")
    if kwargs.get("category"):
        params.append(f"category={kwargs['category']}")
    limit = kwargs.get("limit", 10)
    params.append(f"pageSize={limit}")

    url = f"https://newsapi.org/v2/top-headlines?{'&'.join(params)}"
    req = urllib.request.Request(url, headers={"User-Agent": "WorkbenchAgent/1.0"})
    with urllib.request.urlopen(req, timeout=15) as resp:
        data = json.loads(resp.read())

    return [
        {
            "title": a.get("title", ""),
            "description": a.get("description", ""),
            "source": a.get("source", {}).get("name", ""),
            "url": a.get("url", ""),
            "published_at": a.get("publishedAt", ""),
        }
        for a in data.get("articles", [])
    ]


# ---------------------------------------------------------------------------
# Provider: GNews
# ---------------------------------------------------------------------------

def _gnews_search(query: str, key: str, **kwargs) -> list[dict]:
    """Search news via GNews.io."""
    limit = kwargs.get("limit", 10)
    params = [
        f"q={urllib.request.quote(query)}",
        f"token={key}",
        f"max={limit}",
        "lang=en",
    ]
    if kwargs.get("from_date"):
        params.append(f"from={kwargs['from_date']}T00:00:00Z")

    url = f"https://gnews.io/api/v4/search?{'&'.join(params)}"
    req = urllib.request.Request(url, headers={"User-Agent": "WorkbenchAgent/1.0"})
    with urllib.request.urlopen(req, timeout=15) as resp:
        data = json.loads(resp.read())

    return [
        {
            "title": a.get("title", ""),
            "description": a.get("description", ""),
            "source": a.get("source", {}).get("name", ""),
            "url": a.get("url", ""),
            "published_at": a.get("publishedAt", ""),
        }
        for a in data.get("articles", [])
    ]


# ---------------------------------------------------------------------------
# Fallback: Google News RSS
# ---------------------------------------------------------------------------

def _rss_google_news(query: str, **kwargs) -> list[dict]:
    """Fetch news via Google News RSS (no API key required)."""
    limit = kwargs.get("limit", 10)
    encoded_q = urllib.request.quote(query)
    url = f"https://news.google.com/rss/search?q={encoded_q}&hl=en-US&gl=US&ceid=US:en"

    req = urllib.request.Request(url, headers={"User-Agent": "Mozilla/5.0"})
    with urllib.request.urlopen(req, timeout=15) as resp:
        content = resp.read()

    root = ET.fromstring(content)
    articles = []
    for item in root.findall(".//item"):
        if len(articles) >= limit:
            break
        articles.append({
            "title": (item.findtext("title") or "").strip(),
            "description": (item.findtext("description") or "").strip(),
            "source": (item.findtext("source") or "").strip(),
            "url": (item.findtext("link") or "").strip(),
            "published_at": (item.findtext("pubDate") or "").strip(),
        })

    return articles


# ---------------------------------------------------------------------------
# Dispatcher
# ---------------------------------------------------------------------------

def fetch_news(query: str = "", top_headlines: bool = False, **kwargs) -> list[dict]:
    """Fetch news using the best available provider."""
    newsapi_key = os.environ.get("NEWS_API_KEY", "").strip()
    gnews_key = os.environ.get("GNEWS_API_KEY", "").strip()
    errors = []

    if top_headlines and newsapi_key:
        try:
            return _newsapi_headlines(newsapi_key, **kwargs)
        except Exception as e:
            errors.append(f"newsapi_headlines: {e}")

    if query:
        if newsapi_key:
            try:
                return _newsapi_search(query, newsapi_key, **kwargs)
            except Exception as e:
                errors.append(f"newsapi: {e}")

        if gnews_key:
            try:
                return _gnews_search(query, gnews_key, **kwargs)
            except Exception as e:
                errors.append(f"gnews: {e}")

        try:
            return _rss_google_news(query, **kwargs)
        except Exception as e:
            errors.append(f"google_rss: {e}")

    return [{"error": "; ".join(errors) or "No query provided"}]


def main():
    parser = argparse.ArgumentParser(description="Fetch news headlines")
    parser.add_argument("query", nargs="?", default="", help="Search query")
    parser.add_argument("--top-headlines", action="store_true", help="Fetch top headlines")
    parser.add_argument("--country", default="us", help="Country code for headlines (default: us)")
    parser.add_argument("--category", help="Category: business, technology, health, science, sports, entertainment")
    parser.add_argument("--sources", help="Comma-separated source IDs")
    parser.add_argument("--from", dest="from_date", help="From date (YYYY-MM-DD)")
    parser.add_argument("--limit", type=int, default=10, help="Max results (default: 10)")
    parser.add_argument("--output", choices=["json", "table"], default="json")
    args = parser.parse_args()

    results = fetch_news(
        query=args.query,
        top_headlines=args.top_headlines,
        country=args.country,
        category=args.category,
        sources=args.sources,
        from_date=args.from_date,
        limit=args.limit,
    )

    if args.output == "json":
        print(json.dumps(results, indent=2))
    else:
        for i, article in enumerate(results, 1):
            if "error" in article:
                print(f"Error: {article['error']}")
                continue
            print(f"\n{i}. {article.get('title', 'N/A')}")
            print(f"   Source: {article.get('source', 'N/A')}")
            print(f"   Date: {article.get('published_at', 'N/A')}")
            print(f"   URL: {article.get('url', 'N/A')}")


if __name__ == "__main__":
    main()
