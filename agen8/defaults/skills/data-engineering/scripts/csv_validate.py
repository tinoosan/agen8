#!/usr/bin/env python3
"""Validate a CSV file against a schema definition.

Usage:
    python csv_validate.py data.csv                            # auto-detect validation
    python csv_validate.py data.csv --schema schema.json       # validate against schema
    python csv_validate.py data.csv --require name,email,age   # require specific columns
    python csv_validate.py data.csv --unique id                # check uniqueness of column
    python csv_validate.py data.csv --no-nulls name,email      # disallow nulls in columns
    python csv_validate.py data.csv --max-rows 1000            # check row count limit

Schema JSON format:
    {
        "columns": {
            "id":    {"type": "int", "required": true, "unique": true},
            "name":  {"type": "str", "required": true},
            "email": {"type": "str", "required": true, "pattern": ".*@.*"},
            "age":   {"type": "int", "min": 0, "max": 150},
            "score": {"type": "float", "min": 0.0, "max": 100.0}
        },
        "max_rows": 100000,
        "no_duplicate_rows": false
    }

Requires only Python stdlib.
"""

import argparse
import csv
import json
import re
import sys
from collections import Counter
from pathlib import Path


def load_csv(path: str) -> tuple[list[str], list[dict]]:
    """Load CSV and return (headers, rows)."""
    with open(path, newline="", encoding="utf-8-sig") as f:
        reader = csv.DictReader(f)
        headers = reader.fieldnames or []
        rows = list(reader)
    return headers, rows


def validate_type(value: str, expected_type: str) -> bool:
    """Check if a string value can be parsed as the expected type."""
    if value == "" or value is None:
        return True  # nulls are checked separately
    try:
        if expected_type == "int":
            int(value)
        elif expected_type == "float":
            float(value)
        elif expected_type == "bool":
            if value.lower() not in ("true", "false", "1", "0", "yes", "no"):
                return False
        elif expected_type == "date":
            # Basic ISO date check
            if not re.match(r"^\d{4}-\d{2}-\d{2}", value):
                return False
    except (ValueError, TypeError):
        return False
    return True


def validate_against_schema(headers: list[str], rows: list[dict], schema: dict) -> list[dict]:
    """Validate data against a schema definition."""
    issues = []
    col_specs = schema.get("columns", {})
    max_rows = schema.get("max_rows")

    # Check row count
    if max_rows and len(rows) > max_rows:
        issues.append({
            "severity": "error",
            "type": "row_count",
            "message": f"Row count {len(rows)} exceeds max {max_rows}",
        })

    # Check required columns exist
    for col_name, spec in col_specs.items():
        if spec.get("required") and col_name not in headers:
            issues.append({
                "severity": "error",
                "type": "missing_column",
                "column": col_name,
                "message": f"Required column '{col_name}' not found",
            })

    # Check unexpected columns
    expected_cols = set(col_specs.keys())
    if expected_cols:
        unexpected = set(headers) - expected_cols
        for col in unexpected:
            issues.append({
                "severity": "warning",
                "type": "unexpected_column",
                "column": col,
                "message": f"Unexpected column '{col}' not in schema",
            })

    # Per-column validation
    for col_name, spec in col_specs.items():
        if col_name not in headers:
            continue

        values = [row.get(col_name, "") for row in rows]
        null_count = sum(1 for v in values if v == "" or v is None)

        # Null check
        if spec.get("required") and null_count > 0:
            issues.append({
                "severity": "error",
                "type": "null_values",
                "column": col_name,
                "count": null_count,
                "message": f"Column '{col_name}' has {null_count} null values but is required",
            })

        # Type check
        expected_type = spec.get("type", "str")
        type_errors = 0
        for i, v in enumerate(values):
            if v and not validate_type(v, expected_type):
                type_errors += 1
                if type_errors <= 3:
                    issues.append({
                        "severity": "error",
                        "type": "type_mismatch",
                        "column": col_name,
                        "row": i + 2,  # 1-indexed + header
                        "value": v[:50],
                        "expected": expected_type,
                        "message": f"Row {i+2}: '{v[:50]}' is not a valid {expected_type}",
                    })
        if type_errors > 3:
            issues.append({
                "severity": "error",
                "type": "type_mismatch_summary",
                "column": col_name,
                "count": type_errors,
                "message": f"Column '{col_name}' has {type_errors} total type errors",
            })

        # Uniqueness check
        if spec.get("unique"):
            non_nulls = [v for v in values if v]
            dupes = [v for v, c in Counter(non_nulls).items() if c > 1]
            if dupes:
                issues.append({
                    "severity": "error",
                    "type": "duplicate_values",
                    "column": col_name,
                    "count": len(dupes),
                    "examples": dupes[:5],
                    "message": f"Column '{col_name}' has {len(dupes)} duplicate values",
                })

        # Range check
        min_val = spec.get("min")
        max_val = spec.get("max")
        if min_val is not None or max_val is not None:
            range_errors = 0
            for v in values:
                if not v:
                    continue
                try:
                    num = float(v)
                    if min_val is not None and num < min_val:
                        range_errors += 1
                    if max_val is not None and num > max_val:
                        range_errors += 1
                except ValueError:
                    pass
            if range_errors:
                issues.append({
                    "severity": "warning",
                    "type": "range_violation",
                    "column": col_name,
                    "count": range_errors,
                    "min": min_val,
                    "max": max_val,
                    "message": f"Column '{col_name}' has {range_errors} values outside [{min_val}, {max_val}]",
                })

        # Pattern check
        pattern = spec.get("pattern")
        if pattern:
            regex = re.compile(pattern)
            pattern_errors = sum(1 for v in values if v and not regex.match(v))
            if pattern_errors:
                issues.append({
                    "severity": "warning",
                    "type": "pattern_violation",
                    "column": col_name,
                    "count": pattern_errors,
                    "pattern": pattern,
                    "message": f"Column '{col_name}' has {pattern_errors} values not matching /{pattern}/",
                })

    return issues


def auto_validate(headers: list[str], rows: list[dict]) -> list[dict]:
    """Run automatic validation without a schema."""
    issues = []

    if not rows:
        issues.append({"severity": "warning", "type": "empty", "message": "File contains no data rows"})
        return issues

    # Check for empty headers
    empty_headers = [i for i, h in enumerate(headers) if not h.strip()]
    if empty_headers:
        issues.append({
            "severity": "warning",
            "type": "empty_header",
            "positions": empty_headers,
            "message": f"Found {len(empty_headers)} empty column header(s)",
        })

    # Per-column stats
    for col in headers:
        if not col.strip():
            continue
        values = [row.get(col, "") for row in rows]
        null_count = sum(1 for v in values if v == "" or v is None)
        null_pct = (null_count / len(values)) * 100 if values else 0

        if null_pct > 50:
            issues.append({
                "severity": "warning",
                "type": "high_null_rate",
                "column": col,
                "null_pct": round(null_pct, 1),
                "message": f"Column '{col}' is {null_pct:.1f}% null",
            })

        # Check for duplicates in ID-like columns
        if col.lower() in ("id", "uuid", "key", "pk", "primary_key"):
            non_nulls = [v for v in values if v]
            dupes = sum(1 for _, c in Counter(non_nulls).items() if c > 1)
            if dupes:
                issues.append({
                    "severity": "error",
                    "type": "duplicate_ids",
                    "column": col,
                    "count": dupes,
                    "message": f"ID column '{col}' has {dupes} duplicate values",
                })

    # Check for fully duplicate rows
    seen = set()
    dupe_rows = 0
    for row in rows:
        key = tuple(sorted(row.items()))
        if key in seen:
            dupe_rows += 1
        seen.add(key)
    if dupe_rows:
        issues.append({
            "severity": "warning",
            "type": "duplicate_rows",
            "count": dupe_rows,
            "message": f"Found {dupe_rows} fully duplicate row(s)",
        })

    return issues


def main():
    parser = argparse.ArgumentParser(description="Validate CSV data quality")
    parser.add_argument("input", help="CSV file to validate")
    parser.add_argument("--schema", metavar="FILE", help="Schema JSON file")
    parser.add_argument("--require", help="Comma-separated required columns")
    parser.add_argument("--unique", help="Comma-separated columns that must be unique")
    parser.add_argument("--no-nulls", help="Comma-separated columns that cannot have nulls")
    parser.add_argument("--max-rows", type=int, help="Maximum allowed row count")
    args = parser.parse_args()

    headers, rows = load_csv(args.input)

    report = {
        "file": args.input,
        "rows": len(rows),
        "columns": len(headers),
        "column_names": headers,
    }

    if args.schema:
        with open(args.schema) as f:
            schema = json.load(f)
        issues = validate_against_schema(headers, rows, schema)
    else:
        # Build ad-hoc schema from CLI args
        schema = {"columns": {}}
        if args.require:
            for col in args.require.split(","):
                schema["columns"].setdefault(col.strip(), {})["required"] = True
        if args.unique:
            for col in args.unique.split(","):
                schema["columns"].setdefault(col.strip(), {})["unique"] = True
        if args.no_nulls:
            for col in args.no_nulls.split(","):
                schema["columns"].setdefault(col.strip(), {})["required"] = True
        if args.max_rows:
            schema["max_rows"] = args.max_rows

        if schema["columns"] or schema.get("max_rows"):
            issues = validate_against_schema(headers, rows, schema)
        else:
            issues = auto_validate(headers, rows)

    errors = [i for i in issues if i["severity"] == "error"]
    warnings = [i for i in issues if i["severity"] == "warning"]

    report["issues"] = issues
    report["error_count"] = len(errors)
    report["warning_count"] = len(warnings)
    report["valid"] = len(errors) == 0

    print(json.dumps(report, indent=2))
    sys.exit(0 if report["valid"] else 1)


if __name__ == "__main__":
    main()
