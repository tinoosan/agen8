#!/usr/bin/env python3
"""Audit an Obsidian-style markdown vault and emit graph-quality metrics as JSON."""

import argparse
import json
import os
import re
from collections import Counter, defaultdict
from pathlib import Path

WIKILINK_RE = re.compile(r"\[\[([^\]]+)\]\]")


def resolve_vault_path(explicit: str | None) -> Path:
    if explicit:
        return Path(explicit).expanduser().resolve()

    env_path = os.environ.get("OBSIDIAN_VAULT_PATH", "").strip()
    if env_path:
        return Path(env_path).expanduser().resolve()

    conf = Path.home() / ".agents" / "vault.conf"
    if conf.exists():
        for line in conf.read_text(encoding="utf-8").splitlines():
            clean = line.strip()
            if not clean or clean.startswith("#"):
                continue
            return Path(clean).expanduser().resolve()

    return (Path.home() / ".agents" / "vault").resolve()


def parse_frontmatter(text: str) -> tuple[str, str, bool]:
    if not text.startswith("---\n"):
        return "", text, False
    end = text.find("\n---\n", 4)
    if end == -1:
        return "", text, False
    front = text[4:end]
    body = text[end + 5 :]
    return front, body, True


def parse_type(front: str) -> str:
    m = re.search(r"(?m)^type:\s*['\"]?([^'\"\n]+)", front)
    return m.group(1).strip().upper() if m else "UNKNOWN"


def normalize_link(raw: str) -> str:
    token = raw.strip()
    if "|" in token:
        token = token.split("|", 1)[0]
    if "#" in token:
        token = token.split("#", 1)[0]
    token = token.strip().rstrip("/")
    if token.lower().endswith(".md"):
        token = token[:-3]
    return os.path.basename(token).lower()


def main() -> None:
    parser = argparse.ArgumentParser(description="Audit markdown knowledge graph for an Obsidian vault.")
    parser.add_argument("--path", help="Vault path override")
    parser.add_argument("--top", type=int, default=10, help="Top N hub notes to return (default: 10)")
    parser.add_argument("--json-pretty", action="store_true", help="Pretty-print JSON output")
    args = parser.parse_args()

    vault = resolve_vault_path(args.path)
    if not vault.exists() or not vault.is_dir():
        out = {
            "vault_path": str(vault),
            "stats": {
                "total_notes": 0,
                "total_links": 0,
                "notes_with_frontmatter": 0,
                "notes_without_frontmatter": 0,
            },
            "orphans": [],
            "broken_links": [],
            "top_hubs": [],
            "type_breakdown": {},
            "status": "ok",
        }
        print(json.dumps(out, indent=2 if args.json_pretty else None))
        return

    notes: list[str] = []
    note_types: dict[str, str] = {}
    has_frontmatter: dict[str, bool] = {}
    links_raw: dict[str, list[str]] = {}
    basename_index: dict[str, list[str]] = defaultdict(list)

    for path in sorted(vault.rglob("*.md")):
        rel = path.relative_to(vault).as_posix()
        notes.append(rel)
        basename_index[path.stem.lower()].append(rel)

        try:
            text = path.read_text(encoding="utf-8")
        except UnicodeDecodeError:
            text = path.read_text(encoding="utf-8", errors="replace")

        front, body, present = parse_frontmatter(text)
        has_frontmatter[rel] = present
        note_types[rel] = parse_type(front) if present else "UNKNOWN"

        raw_links = [m.group(1).strip() for m in WIKILINK_RE.finditer(body if present else text)]
        links_raw[rel] = raw_links

    inbound = Counter()
    outbound = Counter()
    broken_links: list[dict[str, str]] = []

    for src in notes:
        for raw in links_raw.get(src, []):
            outbound[src] += 1
            target_basename = normalize_link(raw)
            candidates = sorted(basename_index.get(target_basename, []))
            if not candidates:
                broken_links.append(
                    {
                        "from": src,
                        "raw_target": raw,
                        "normalized_target": target_basename,
                    }
                )
                continue
            inbound[candidates[0]] += 1

    orphans = sorted([n for n in notes if inbound[n] == 0 and outbound[n] == 0])

    hubs = []
    for note in notes:
        total = inbound[note] + outbound[note]
        hubs.append(
            {
                "note": note,
                "inbound": inbound[note],
                "outbound": outbound[note],
                "total": total,
            }
        )
    hubs.sort(key=lambda x: (-x["total"], -x["inbound"], x["note"]))
    top_hubs = hubs[: max(0, args.top)]

    type_breakdown = dict(sorted(Counter(note_types.values()).items(), key=lambda x: x[0]))

    out = {
        "vault_path": str(vault),
        "stats": {
            "total_notes": len(notes),
            "total_links": sum(outbound.values()),
            "notes_with_frontmatter": sum(1 for v in has_frontmatter.values() if v),
            "notes_without_frontmatter": sum(1 for v in has_frontmatter.values() if not v),
        },
        "orphans": orphans,
        "broken_links": broken_links,
        "top_hubs": top_hubs,
        "type_breakdown": type_breakdown,
        "status": "ok",
    }

    print(json.dumps(out, indent=2 if args.json_pretty else None))


if __name__ == "__main__":
    main()
