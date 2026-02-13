#!/usr/bin/env python3
"""Profile a dataset: summary statistics, distributions, and quality metrics.

Usage:
    python data_profile.py data.csv                       # full profile (stdout)
    python data_profile.py data.csv --output profile.json # save to file
    python data_profile.py data.json                      # also works with JSON
    python data_profile.py data.csv --columns name,age    # profile specific columns
    python data_profile.py data.csv --sample 1000         # sample first N rows

Requires only Python stdlib.
"""

import argparse
import csv
import json
import math
import sys
from collections import Counter
from pathlib import Path


def load_data(path: str) -> tuple[list[str], list[dict]]:
    """Load CSV or JSON data."""
    p = Path(path)
    if p.suffix.lower() in (".json", ".jsonl"):
        with open(p) as f:
            content = f.read().strip()
        # Try JSON Lines
        if content.startswith("{") and "\n" in content:
            try:
                rows = [json.loads(line) for line in content.split("\n") if line.strip()]
                headers = list(rows[0].keys()) if rows else []
                return headers, rows
            except json.JSONDecodeError:
                pass
        data = json.loads(content)
        if isinstance(data, dict):
            for key in ("data", "results", "rows", "records"):
                if key in data and isinstance(data[key], list):
                    data = data[key]
                    break
            else:
                data = [data]
        headers = list(data[0].keys()) if data else []
        return headers, data
    else:
        with open(p, newline="", encoding="utf-8-sig") as f:
            reader = csv.DictReader(f)
            headers = reader.fieldnames or []
            rows = list(reader)
        return headers, rows


def infer_type(values: list[str]) -> str:
    """Infer the dominant type of a column."""
    int_count = 0
    float_count = 0
    bool_count = 0
    date_count = 0
    total = 0

    for v in values:
        if v == "" or v is None:
            continue
        total += 1
        try:
            int(v)
            int_count += 1
            continue
        except (ValueError, TypeError):
            pass
        try:
            float(v)
            float_count += 1
            continue
        except (ValueError, TypeError):
            pass
        if str(v).lower() in ("true", "false", "yes", "no", "1", "0"):
            bool_count += 1
            continue
        import re
        if re.match(r"^\d{4}-\d{2}-\d{2}", str(v)):
            date_count += 1
            continue

    if total == 0:
        return "empty"
    if int_count / total > 0.8:
        return "integer"
    if (int_count + float_count) / total > 0.8:
        return "float"
    if bool_count / total > 0.8:
        return "boolean"
    if date_count / total > 0.8:
        return "date"
    return "string"


def numeric_stats(values: list) -> dict:
    """Compute stats for numeric columns."""
    nums = []
    for v in values:
        if v == "" or v is None:
            continue
        try:
            nums.append(float(v))
        except (ValueError, TypeError):
            pass

    if not nums:
        return {}

    nums_sorted = sorted(nums)
    n = len(nums)

    def percentile(p):
        k = (n - 1) * p / 100
        f = math.floor(k)
        c = math.ceil(k)
        if f == c:
            return nums_sorted[int(k)]
        return nums_sorted[f] * (c - k) + nums_sorted[c] * (k - f)

    mean = sum(nums) / n
    variance = sum((x - mean) ** 2 for x in nums) / n if n > 1 else 0
    std_dev = math.sqrt(variance)

    return {
        "count": n,
        "mean": round(mean, 4),
        "std_dev": round(std_dev, 4),
        "min": round(min(nums), 4),
        "p25": round(percentile(25), 4),
        "median": round(percentile(50), 4),
        "p75": round(percentile(75), 4),
        "max": round(max(nums), 4),
        "zeros": sum(1 for x in nums if x == 0),
        "negatives": sum(1 for x in nums if x < 0),
    }


def string_stats(values: list) -> dict:
    """Compute stats for string columns."""
    non_null = [str(v) for v in values if v and v != ""]
    if not non_null:
        return {}

    lengths = [len(s) for s in non_null]
    freq = Counter(non_null)
    top = freq.most_common(5)

    return {
        "count": len(non_null),
        "unique": len(freq),
        "min_length": min(lengths),
        "max_length": max(lengths),
        "avg_length": round(sum(lengths) / len(lengths), 1),
        "top_values": [{"value": v[:50], "count": c} for v, c in top],
    }


def profile_column(name: str, values: list) -> dict:
    """Generate a full profile for a single column."""
    total = len(values)
    null_count = sum(1 for v in values if v == "" or v is None)
    null_pct = round((null_count / total) * 100, 1) if total > 0 else 0
    unique_count = len(set(str(v) for v in values if v and v != ""))

    inferred = infer_type(values)

    profile = {
        "name": name,
        "inferred_type": inferred,
        "total": total,
        "non_null": total - null_count,
        "null_count": null_count,
        "null_pct": null_pct,
        "unique_count": unique_count,
        "uniqueness_pct": round((unique_count / (total - null_count)) * 100, 1) if total - null_count > 0 else 0,
    }

    if inferred in ("integer", "float"):
        profile["stats"] = numeric_stats(values)
    else:
        profile["stats"] = string_stats(values)

    # Quality flags
    flags = []
    if null_pct > 50:
        flags.append("high_nulls")
    if unique_count == 1:
        flags.append("constant_value")
    if unique_count == total - null_count and total > 10:
        flags.append("all_unique")
    if null_count == total:
        flags.append("entirely_null")
    profile["quality_flags"] = flags

    return profile


def main():
    parser = argparse.ArgumentParser(description="Data profiling tool")
    parser.add_argument("input", help="Input file (CSV or JSON)")
    parser.add_argument("--output", "-o", help="Output file (default: stdout)")
    parser.add_argument("--columns", help="Comma-separated columns to profile")
    parser.add_argument("--sample", type=int, help="Sample first N rows only")
    args = parser.parse_args()

    headers, rows = load_data(args.input)

    if args.sample and args.sample < len(rows):
        rows = rows[:args.sample]

    target_cols = headers
    if args.columns:
        target_cols = [c.strip() for c in args.columns.split(",")]
        missing = [c for c in target_cols if c not in headers]
        if missing:
            print(json.dumps({"error": f"Columns not found: {missing}", "available": headers}), file=sys.stderr)
            sys.exit(1)

    # Check for duplicate rows
    seen = set()
    dupe_count = 0
    for row in rows:
        key = tuple(sorted(row.items()))
        if key in seen:
            dupe_count += 1
        seen.add(key)

    column_profiles = []
    for col in target_cols:
        values = [row.get(col, "") for row in rows]
        column_profiles.append(profile_column(col, values))

    # Overall quality score (simple heuristic)
    total_issues = sum(len(cp["quality_flags"]) for cp in column_profiles)
    completeness = round(
        (1 - sum(cp["null_count"] for cp in column_profiles) / (len(rows) * len(target_cols))) * 100, 1
    ) if rows and target_cols else 0

    report = {
        "file": args.input,
        "rows": len(rows),
        "columns": len(target_cols),
        "duplicate_rows": dupe_count,
        "completeness_pct": completeness,
        "quality_issue_count": total_issues,
        "column_profiles": column_profiles,
    }

    output = json.dumps(report, indent=2)
    if args.output:
        with open(args.output, "w") as f:
            f.write(output)
        print(json.dumps({"status": "ok", "output": args.output, "rows": len(rows), "columns": len(target_cols)}))
    else:
        print(output)


if __name__ == "__main__":
    main()
