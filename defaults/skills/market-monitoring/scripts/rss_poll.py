#!/usr/bin/env python3
"""Monitor RSS/Atom feeds for new entries.

Usage:
    python rss_poll.py https://feeds.bbci.co.uk/news/rss.xml       # poll a single feed
    python rss_poll.py --feeds feeds.txt                             # poll multiple feeds
    python rss_poll.py --feeds feeds.txt --since 2026-02-01          # only entries after date
    python rss_poll.py https://example.com/rss --state state.json    # track seen entries
    python rss_poll.py https://example.com/rss --limit 5             # max entries per feed

feeds.txt format (one URL per line):
    https://feeds.bbci.co.uk/news/rss.xml
    https://techcrunch.com/feed/
    # lines starting with # are ignored

When --state is used, re-running only returns NEW entries since last poll.
Requires only Python stdlib.
"""

import argparse
import hashlib
import json
import os
import sys
import urllib.request
import xml.etree.ElementTree as ET
from datetime import datetime
from email.utils import parsedate_to_datetime
from pathlib import Path


def parse_feed(url: str, since: str = "", limit: int = 20) -> list[dict]:
    """Parse an RSS or Atom feed and return entries."""
    req = urllib.request.Request(url, headers={
        "User-Agent": "Mozilla/5.0 (compatible; Agen8Bot/1.0)",
        "Accept": "application/rss+xml, application/xml, text/xml",
    })
    with urllib.request.urlopen(req, timeout=20) as resp:
        content = resp.read()

    root = ET.fromstring(content)
    entries = []

    # RSS 2.0
    for item in root.findall(".//item"):
        entry = _parse_rss_item(item, url)
        if entry:
            entries.append(entry)

    # Atom (if no RSS items found)
    if not entries:
        ns = {"atom": "http://www.w3.org/2005/Atom"}
        for item in root.findall(".//atom:entry", ns):
            entry = _parse_atom_entry(item, ns, url)
            if entry:
                entries.append(entry)
        # Try without namespace
        if not entries:
            for item in root.findall(".//entry"):
                entry = _parse_atom_entry_no_ns(item, url)
                if entry:
                    entries.append(entry)

    # Filter by date
    if since:
        try:
            since_dt = datetime.fromisoformat(since.replace("Z", "+00:00"))
            filtered = []
            for e in entries:
                pub = e.get("published_at", "")
                if pub:
                    try:
                        pub_dt = parsedate_to_datetime(pub)
                        if pub_dt >= since_dt:
                            filtered.append(e)
                    except (ValueError, TypeError):
                        filtered.append(e)  # include if can't parse date
                else:
                    filtered.append(e)
            entries = filtered
        except ValueError:
            pass  # ignore invalid since date

    return entries[:limit]


def _parse_rss_item(item, feed_url: str) -> dict:
    title = (item.findtext("title") or "").strip()
    link = (item.findtext("link") or "").strip()
    desc = (item.findtext("description") or "").strip()
    pub_date = (item.findtext("pubDate") or "").strip()
    guid = (item.findtext("guid") or link or title).strip()

    return {
        "id": hashlib.md5(guid.encode()).hexdigest()[:12],
        "title": title,
        "url": link,
        "description": desc[:300],
        "published_at": pub_date,
        "feed_url": feed_url,
    }


def _parse_atom_entry(item, ns: dict, feed_url: str) -> dict:
    title = (item.findtext("atom:title", namespaces=ns) or "").strip()
    link_el = item.find("atom:link", ns)
    link = link_el.get("href", "") if link_el is not None else ""
    summary = (item.findtext("atom:summary", namespaces=ns) or "").strip()
    updated = (item.findtext("atom:updated", namespaces=ns) or "").strip()
    entry_id = (item.findtext("atom:id", namespaces=ns) or link or title).strip()

    return {
        "id": hashlib.md5(entry_id.encode()).hexdigest()[:12],
        "title": title,
        "url": link,
        "description": summary[:300],
        "published_at": updated,
        "feed_url": feed_url,
    }


def _parse_atom_entry_no_ns(item, feed_url: str) -> dict:
    title = (item.findtext("title") or "").strip()
    link_el = item.find("link")
    link = link_el.get("href", "") if link_el is not None else ""
    summary = (item.findtext("summary") or item.findtext("content") or "").strip()
    updated = (item.findtext("updated") or item.findtext("published") or "").strip()
    entry_id = (item.findtext("id") or link or title).strip()

    return {
        "id": hashlib.md5(entry_id.encode()).hexdigest()[:12],
        "title": title,
        "url": link,
        "description": summary[:300],
        "published_at": updated,
        "feed_url": feed_url,
    }


def load_state(state_file: str) -> set:
    """Load previously seen entry IDs."""
    if not os.path.exists(state_file):
        return set()
    with open(state_file) as f:
        data = json.load(f)
    return set(data.get("seen_ids", []))


def save_state(state_file: str, seen_ids: set):
    """Save seen entry IDs. Keep only last 5000 to prevent unbounded growth."""
    ids_list = sorted(seen_ids)[-5000:]
    with open(state_file, "w") as f:
        json.dump({"seen_ids": ids_list, "last_poll": datetime.utcnow().isoformat() + "Z"}, f)


def main():
    parser = argparse.ArgumentParser(description="Poll RSS/Atom feeds")
    parser.add_argument("url", nargs="?", help="Single feed URL to poll")
    parser.add_argument("--feeds", metavar="FILE", help="File with one feed URL per line")
    parser.add_argument("--since", help="Only entries after this date (YYYY-MM-DD or ISO)")
    parser.add_argument("--state", metavar="FILE", help="State file to track seen entries")
    parser.add_argument("--limit", type=int, default=20, help="Max entries per feed (default: 20)")
    parser.add_argument("--output", choices=["json", "table"], default="json")
    args = parser.parse_args()

    urls = []
    if args.url:
        urls.append(args.url)
    if args.feeds:
        with open(args.feeds) as f:
            for line in f:
                line = line.strip()
                if line and not line.startswith("#"):
                    urls.append(line)

    if not urls:
        parser.error("Provide a feed URL or --feeds file")

    seen_ids = load_state(args.state) if args.state else set()
    all_entries = []
    errors = []

    for url in urls:
        try:
            entries = parse_feed(url, since=args.since or "", limit=args.limit)
            if args.state:
                entries = [e for e in entries if e["id"] not in seen_ids]
                for e in entries:
                    seen_ids.add(e["id"])
            all_entries.extend(entries)
        except Exception as e:
            errors.append({"feed": url, "error": str(e)})

    if args.state:
        save_state(args.state, seen_ids)

    result = {
        "entries": all_entries,
        "total": len(all_entries),
        "feeds_polled": len(urls),
        "errors": errors,
        "polled_at": datetime.utcnow().isoformat() + "Z",
    }

    if args.output == "json":
        print(json.dumps(result, indent=2))
    else:
        if errors:
            for e in errors:
                print(f"⚠  {e['feed']}: {e['error']}")
        for i, entry in enumerate(all_entries, 1):
            print(f"\n{i}. {entry['title']}")
            print(f"   {entry['published_at']}  |  {entry['feed_url']}")
            print(f"   {entry['url']}")


if __name__ == "__main__":
    main()
