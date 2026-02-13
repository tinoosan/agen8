#!/usr/bin/env python3
"""Detect changes on a webpage by comparing snapshots.

Usage:
    python diff_webpage.py https://example.com/pricing              # first run: saves snapshot
    python diff_webpage.py https://example.com/pricing              # second run: shows diff
    python diff_webpage.py https://example.com/pricing --selector ".pricing-table"
    python diff_webpage.py https://example.com/pricing --state ./snapshots
    python diff_webpage.py --list --state ./snapshots                # list tracked pages

How it works:
    1. Fetches the page via HTTP
    2. Extracts text content (strips HTML tags)
    3. Compares to the previous snapshot (stored in --state directory)
    4. Reports additions, removals, and overall change percentage

State directory structure:
    ./snapshots/<url_hash>.json  → {url, last_checked, content_hash, content}

Requires only Python stdlib.
"""

import argparse
import difflib
import hashlib
import html
import json
import os
import re
import sys
import urllib.request
from datetime import datetime
from pathlib import Path


def fetch_page(url: str) -> str:
    """Fetch page content via HTTP."""
    req = urllib.request.Request(url, headers={
        "User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36",
        "Accept": "text/html,application/xhtml+xml",
    })
    with urllib.request.urlopen(req, timeout=20) as resp:
        return resp.read().decode("utf-8", errors="replace")


def extract_text(html_content: str, selector: str = "") -> str:
    """Extract meaningful text from HTML. Basic extraction without BeautifulSoup."""
    content = html_content

    # If selector provided, try to find the matching section
    if selector:
        # Basic class/id selector matching
        sel = selector.lstrip(".")
        sel = sel.lstrip("#")
        # Try to find content inside matching element
        patterns = [
            rf'class="[^"]*{re.escape(sel)}[^"]*"[^>]*>(.*?)</(?:div|section|table|main)',
            rf'id="{re.escape(sel)}"[^>]*>(.*?)</(?:div|section|table|main)',
        ]
        for pattern in patterns:
            match = re.search(pattern, content, re.DOTALL | re.IGNORECASE)
            if match:
                content = match.group(1)
                break

    # Remove script and style content
    content = re.sub(r"<script[^>]*>.*?</script>", "", content, flags=re.DOTALL | re.IGNORECASE)
    content = re.sub(r"<style[^>]*>.*?</style>", "", content, flags=re.DOTALL | re.IGNORECASE)
    content = re.sub(r"<!--.*?-->", "", content, flags=re.DOTALL)

    # Strip HTML tags
    content = re.sub(r"<[^>]+>", "\n", content)

    # Decode HTML entities
    content = html.unescape(content)

    # Normalize whitespace
    lines = [line.strip() for line in content.split("\n")]
    lines = [line for line in lines if line]  # remove empty lines

    return "\n".join(lines)


def url_hash(url: str) -> str:
    """Generate a safe filename hash for a URL."""
    return hashlib.md5(url.encode()).hexdigest()[:16]


def load_snapshot(state_dir: str, url: str) -> dict | None:
    """Load previous snapshot for a URL."""
    path = Path(state_dir) / f"{url_hash(url)}.json"
    if not path.exists():
        return None
    with open(path) as f:
        return json.load(f)


def save_snapshot(state_dir: str, url: str, content: str):
    """Save current snapshot."""
    path = Path(state_dir) / f"{url_hash(url)}.json"
    Path(state_dir).mkdir(parents=True, exist_ok=True)
    data = {
        "url": url,
        "content_hash": hashlib.md5(content.encode()).hexdigest(),
        "last_checked": datetime.utcnow().isoformat() + "Z",
        "content": content,
    }
    with open(path, "w") as f:
        json.dump(data, f, indent=2)


def compute_diff(old_content: str, new_content: str) -> dict:
    """Compute a structured diff between old and new content."""
    old_lines = old_content.split("\n")
    new_lines = new_content.split("\n")

    diff = list(difflib.unified_diff(old_lines, new_lines, lineterm=""))

    additions = [line[1:] for line in diff if line.startswith("+") and not line.startswith("+++")]
    removals = [line[1:] for line in diff if line.startswith("-") and not line.startswith("---")]

    total_lines = max(len(old_lines), len(new_lines), 1)
    change_pct = round((len(additions) + len(removals)) / total_lines * 100, 1)

    return {
        "changed": len(additions) + len(removals) > 0,
        "additions": len(additions),
        "removals": len(removals),
        "change_pct": change_pct,
        "added_lines": additions[:20],  # cap for readability
        "removed_lines": removals[:20],
        "diff_preview": "\n".join(diff[:50]),
    }


def list_tracked(state_dir: str) -> list[dict]:
    """List all tracked pages."""
    state_path = Path(state_dir)
    if not state_path.exists():
        return []
    tracked = []
    for f in sorted(state_path.glob("*.json")):
        with open(f) as fh:
            data = json.load(fh)
        tracked.append({
            "url": data.get("url", ""),
            "last_checked": data.get("last_checked", ""),
            "content_hash": data.get("content_hash", ""),
            "content_length": len(data.get("content", "")),
        })
    return tracked


def main():
    parser = argparse.ArgumentParser(description="Detect webpage changes")
    parser.add_argument("url", nargs="?", help="URL to monitor")
    parser.add_argument("--selector", help="CSS-like selector to focus on (class or id)")
    parser.add_argument("--state", default=".webpage_snapshots", help="State directory (default: .webpage_snapshots)")
    parser.add_argument("--list", action="store_true", help="List tracked pages")
    parser.add_argument("--output", choices=["json", "diff"], default="json")
    args = parser.parse_args()

    if args.list:
        tracked = list_tracked(args.state)
        print(json.dumps({"tracked_pages": tracked, "count": len(tracked)}, indent=2))
        return

    if not args.url:
        parser.error("URL is required (or use --list)")

    # Fetch current page
    try:
        raw_html = fetch_page(args.url)
    except Exception as e:
        print(json.dumps({"status": "error", "url": args.url, "error": str(e)}))
        sys.exit(1)

    current_text = extract_text(raw_html, args.selector or "")

    # Load previous snapshot
    previous = load_snapshot(args.state, args.url)

    if previous is None:
        # First time — save baseline
        save_snapshot(args.state, args.url, current_text)
        print(json.dumps({
            "status": "baseline_saved",
            "url": args.url,
            "content_length": len(current_text),
            "content_hash": hashlib.md5(current_text.encode()).hexdigest(),
            "message": "First snapshot saved. Run again to detect changes.",
        }, indent=2))
        return

    # Compare
    diff_result = compute_diff(previous["content"], current_text)

    # Save new snapshot
    save_snapshot(args.state, args.url, current_text)

    result = {
        "status": "changed" if diff_result["changed"] else "unchanged",
        "url": args.url,
        "previous_check": previous.get("last_checked", ""),
        "current_check": datetime.utcnow().isoformat() + "Z",
        **diff_result,
    }

    if args.output == "diff" and diff_result["diff_preview"]:
        print(diff_result["diff_preview"])
    else:
        print(json.dumps(result, indent=2))


if __name__ == "__main__":
    main()
