#!/usr/bin/env python3
"""Convert between JSON and CSV formats.

Usage:
    python convert_format.py data.json --to csv --output data.csv    # JSON → CSV
    python convert_format.py data.csv  --to json --output data.json  # CSV → JSON
    python convert_format.py data.json --to csv                      # stdout
    python convert_format.py data.json --to csv --flatten             # flatten nested JSON
    python convert_format.py data.csv  --to jsonl                     # JSON Lines output

Auto-detects input format from file extension.
Handles nested JSON by flattening with dot notation when --flatten is used.
Requires only Python stdlib.
"""

import argparse
import csv
import io
import json
import sys
from pathlib import Path


def flatten_dict(d: dict, parent_key: str = "", sep: str = ".") -> dict:
    """Flatten a nested dictionary using dot notation."""
    items = []
    for k, v in d.items():
        new_key = f"{parent_key}{sep}{k}" if parent_key else k
        if isinstance(v, dict):
            items.extend(flatten_dict(v, new_key, sep).items())
        elif isinstance(v, list):
            # Convert lists to JSON strings
            items.append((new_key, json.dumps(v)))
        else:
            items.append((new_key, v))
    return dict(items)


def load_json(path: str, do_flatten: bool = False) -> list[dict]:
    """Load JSON file and return list of flat dicts."""
    with open(path) as f:
        content = f.read().strip()

    # Try JSON Lines first
    if content.startswith("{") and "\n" in content:
        lines = content.split("\n")
        try:
            data = [json.loads(line) for line in lines if line.strip()]
            if do_flatten:
                data = [flatten_dict(d) for d in data]
            return data
        except json.JSONDecodeError:
            pass

    data = json.loads(content)

    # Unwrap common wrapper patterns
    if isinstance(data, dict):
        for key in ("data", "results", "rows", "records", "items"):
            if key in data and isinstance(data[key], list):
                data = data[key]
                break
        else:
            data = [data]  # single object → list

    if do_flatten:
        data = [flatten_dict(d) if isinstance(d, dict) else d for d in data]

    return data


def load_csv_data(path: str) -> list[dict]:
    """Load CSV and return list of dicts."""
    with open(path, newline="", encoding="utf-8-sig") as f:
        reader = csv.DictReader(f)
        return list(reader)


def to_csv_string(data: list[dict]) -> str:
    """Convert list of dicts to CSV string."""
    if not data:
        return ""
    # Collect all keys preserving order
    keys = []
    seen = set()
    for row in data:
        for k in row.keys():
            if k not in seen:
                keys.append(k)
                seen.add(k)
    buf = io.StringIO()
    writer = csv.DictWriter(buf, fieldnames=keys, extrasaction="ignore")
    writer.writeheader()
    for row in data:
        # Convert non-string values
        clean = {}
        for k in keys:
            v = row.get(k, "")
            if isinstance(v, (dict, list)):
                clean[k] = json.dumps(v)
            elif v is None:
                clean[k] = ""
            else:
                clean[k] = v
        writer.writerow(clean)
    return buf.getvalue()


def to_json_string(data: list[dict], fmt: str = "json") -> str:
    """Convert list of dicts to JSON or JSON Lines string."""
    if fmt == "jsonl":
        return "\n".join(json.dumps(row) for row in data)
    return json.dumps(data, indent=2)


def main():
    parser = argparse.ArgumentParser(description="Convert between JSON and CSV")
    parser.add_argument("input", help="Input file (CSV or JSON)")
    parser.add_argument("--to", required=True, choices=["csv", "json", "jsonl"],
                        help="Output format")
    parser.add_argument("--output", "-o", help="Output file (default: stdout)")
    parser.add_argument("--flatten", action="store_true",
                        help="Flatten nested JSON objects with dot notation")
    args = parser.parse_args()

    input_path = Path(args.input)
    ext = input_path.suffix.lower()

    # Load data
    if ext == ".json" or ext == ".jsonl":
        data = load_json(args.input, args.flatten)
    elif ext == ".csv":
        data = load_csv_data(args.input)
    else:
        # Try JSON first, then CSV
        try:
            data = load_json(args.input, args.flatten)
        except (json.JSONDecodeError, UnicodeDecodeError):
            data = load_csv_data(args.input)

    # Convert
    if args.to == "csv":
        output = to_csv_string(data)
    else:
        output = to_json_string(data, args.to)

    # Write
    if args.output:
        with open(args.output, "w") as f:
            f.write(output)
        print(json.dumps({
            "status": "ok",
            "input": args.input,
            "output": args.output,
            "format": args.to,
            "rows": len(data),
        }))
    else:
        print(output)


if __name__ == "__main__":
    main()
