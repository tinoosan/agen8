#!/usr/bin/env python3
"""Generate charts from CSV/JSON data.

Usage:
    python chart_gen.py data.csv --type bar --x month --y revenue --output chart.png
    python chart_gen.py data.json --type line --x date --y price --title "Stock Price"
    python chart_gen.py data.csv --type pie --labels category --values count
    python chart_gen.py data.csv --type scatter --x weight --y height
    python chart_gen.py data.csv --type hbar --x category --y value --sort

Supported chart types: bar, hbar, line, pie, scatter, area, multi-line
Output formats: .png, .svg, .pdf (auto-detected from --output extension)

Requirements:
    matplotlib (pip install matplotlib)
"""

import argparse
import csv
import json
import sys
from pathlib import Path

try:
    import matplotlib
    matplotlib.use("Agg")  # Non-interactive backend
    import matplotlib.pyplot as plt
    import matplotlib.ticker as ticker
except ImportError:
    print(json.dumps({
        "status": "error",
        "error": "matplotlib not installed. Run: pip install matplotlib"
    }))
    sys.exit(1)


# ---------------------------------------------------------------------------
# Data loading
# ---------------------------------------------------------------------------

def load_data(path: str) -> list[dict]:
    """Load data from CSV or JSON file."""
    p = Path(path)
    if not p.exists():
        raise FileNotFoundError(f"File not found: {path}")

    if p.suffix.lower() == ".json":
        with open(p) as f:
            data = json.load(f)
        if isinstance(data, dict):
            # Try common wrapper keys
            for key in ("data", "results", "rows", "records"):
                if key in data and isinstance(data[key], list):
                    return data[key]
            # Flat dict → single-row list
            return [data]
        return data

    # CSV
    with open(p, newline="") as f:
        reader = csv.DictReader(f)
        return list(reader)


def extract_column(data: list[dict], col: str) -> list:
    """Extract a column, attempting numeric conversion."""
    values = [row.get(col, "") for row in data]
    try:
        return [float(v) if v != "" else 0.0 for v in values]
    except (ValueError, TypeError):
        return values


# ---------------------------------------------------------------------------
# Chart builders
# ---------------------------------------------------------------------------

COLORS = ["#2563eb", "#16a34a", "#dc2626", "#9333ea", "#ea580c", "#0891b2", "#4f46e5", "#be123c"]

def setup_style(fig, ax, title: str, dark: bool = False):
    """Apply consistent styling."""
    if dark:
        fig.patch.set_facecolor("#0d1117")
        ax.set_facecolor("#161b22")
        ax.spines["bottom"].set_color("#30363d")
        ax.spines["left"].set_color("#30363d")
        ax.spines["top"].set_visible(False)
        ax.spines["right"].set_visible(False)
        ax.tick_params(colors="#c9d1d9")
        ax.xaxis.label.set_color("#c9d1d9")
        ax.yaxis.label.set_color("#c9d1d9")
        if title:
            ax.set_title(title, color="#f0f6fc", fontsize=14, fontweight="bold", pad=16)
    else:
        ax.spines["top"].set_visible(False)
        ax.spines["right"].set_visible(False)
        ax.spines["bottom"].set_color("#d1d5db")
        ax.spines["left"].set_color("#d1d5db")
        if title:
            ax.set_title(title, fontsize=14, fontweight="bold", pad=16, color="#111827")
    ax.grid(True, alpha=0.15, linestyle="--")


def chart_bar(data, x, y, title, output, sort, dark):
    labels = extract_column(data, x)
    values = extract_column(data, y)
    if sort:
        pairs = sorted(zip(labels, values), key=lambda p: p[1], reverse=True)
        labels, values = zip(*pairs)
    fig, ax = plt.subplots(figsize=(10, 6))
    setup_style(fig, ax, title, dark)
    bars = ax.bar(range(len(labels)), values, color=COLORS[0], width=0.6, edgecolor="white", linewidth=0.5)
    ax.set_xticks(range(len(labels)))
    ax.set_xticklabels(labels, rotation=45, ha="right", fontsize=9)
    ax.set_ylabel(y)
    fig.tight_layout()
    fig.savefig(output, dpi=150, bbox_inches="tight")
    plt.close(fig)


def chart_hbar(data, x, y, title, output, sort, dark):
    labels = extract_column(data, x)
    values = extract_column(data, y)
    if sort:
        pairs = sorted(zip(labels, values), key=lambda p: p[1])
        labels, values = zip(*pairs)
    fig, ax = plt.subplots(figsize=(10, 6))
    setup_style(fig, ax, title, dark)
    ax.barh(range(len(labels)), values, color=COLORS[0], height=0.6, edgecolor="white", linewidth=0.5)
    ax.set_yticks(range(len(labels)))
    ax.set_yticklabels(labels, fontsize=9)
    ax.set_xlabel(y)
    fig.tight_layout()
    fig.savefig(output, dpi=150, bbox_inches="tight")
    plt.close(fig)


def chart_line(data, x, y, title, output, dark):
    labels = extract_column(data, x)
    values = extract_column(data, y)
    fig, ax = plt.subplots(figsize=(10, 5))
    setup_style(fig, ax, title, dark)
    ax.plot(range(len(labels)), values, color=COLORS[0], linewidth=2, marker="o", markersize=4)
    ax.fill_between(range(len(labels)), values, alpha=0.08, color=COLORS[0])
    ax.set_xticks(range(len(labels)))
    ax.set_xticklabels(labels, rotation=45, ha="right", fontsize=9)
    ax.set_ylabel(y)
    fig.tight_layout()
    fig.savefig(output, dpi=150, bbox_inches="tight")
    plt.close(fig)


def chart_area(data, x, y, title, output, dark):
    labels = extract_column(data, x)
    values = extract_column(data, y)
    fig, ax = plt.subplots(figsize=(10, 5))
    setup_style(fig, ax, title, dark)
    ax.fill_between(range(len(labels)), values, alpha=0.3, color=COLORS[0])
    ax.plot(range(len(labels)), values, color=COLORS[0], linewidth=2)
    ax.set_xticks(range(len(labels)))
    ax.set_xticklabels(labels, rotation=45, ha="right", fontsize=9)
    ax.set_ylabel(y)
    fig.tight_layout()
    fig.savefig(output, dpi=150, bbox_inches="tight")
    plt.close(fig)


def chart_scatter(data, x, y, title, output, dark):
    x_vals = extract_column(data, x)
    y_vals = extract_column(data, y)
    fig, ax = plt.subplots(figsize=(10, 6))
    setup_style(fig, ax, title, dark)
    ax.scatter(x_vals, y_vals, color=COLORS[0], alpha=0.7, edgecolors="white", linewidths=0.5, s=60)
    ax.set_xlabel(x)
    ax.set_ylabel(y)
    fig.tight_layout()
    fig.savefig(output, dpi=150, bbox_inches="tight")
    plt.close(fig)


def chart_pie(data, labels_col, values_col, title, output, dark):
    labels = extract_column(data, labels_col)
    values = extract_column(data, values_col)
    fig, ax = plt.subplots(figsize=(8, 8))
    if dark:
        fig.patch.set_facecolor("#0d1117")
    colors = COLORS[:len(labels)]
    wedges, texts, autotexts = ax.pie(
        values, labels=labels, colors=colors, autopct="%1.1f%%",
        startangle=90, pctdistance=0.85,
        wedgeprops=dict(width=0.5, edgecolor="white", linewidth=2),
    )
    for t in autotexts:
        t.set_fontsize(9)
        if dark:
            t.set_color("#f0f6fc")
    for t in texts:
        if dark:
            t.set_color("#c9d1d9")
    if title:
        ax.set_title(title, fontsize=14, fontweight="bold", pad=20,
                      color="#f0f6fc" if dark else "#111827")
    fig.tight_layout()
    fig.savefig(output, dpi=150, bbox_inches="tight")
    plt.close(fig)


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main():
    parser = argparse.ArgumentParser(description="Generate charts from data files")
    parser.add_argument("input", help="Input data file (CSV or JSON)")
    parser.add_argument("--type", required=True, choices=["bar", "hbar", "line", "area", "scatter", "pie"],
                        help="Chart type")
    parser.add_argument("--x", help="X-axis column name")
    parser.add_argument("--y", help="Y-axis column name")
    parser.add_argument("--labels", help="Labels column (for pie chart)")
    parser.add_argument("--values", help="Values column (for pie chart)")
    parser.add_argument("--title", default="", help="Chart title")
    parser.add_argument("--output", default="chart.png", help="Output file (png/svg/pdf)")
    parser.add_argument("--sort", action="store_true", help="Sort by value (bar/hbar)")
    parser.add_argument("--dark", action="store_true", help="Dark mode theme")
    args = parser.parse_args()

    data = load_data(args.input)
    if not data:
        print(json.dumps({"status": "error", "error": "No data found in input file"}))
        sys.exit(1)

    chart_type = args.type

    if chart_type == "pie":
        labels_col = args.labels or args.x
        values_col = args.values or args.y
        if not labels_col or not values_col:
            parser.error("Pie chart requires --labels/--x and --values/--y")
        chart_pie(data, labels_col, values_col, args.title, args.output, args.dark)
    elif chart_type == "bar":
        chart_bar(data, args.x, args.y, args.title, args.output, args.sort, args.dark)
    elif chart_type == "hbar":
        chart_hbar(data, args.x, args.y, args.title, args.output, args.sort, args.dark)
    elif chart_type == "line":
        chart_line(data, args.x, args.y, args.title, args.output, args.dark)
    elif chart_type == "area":
        chart_area(data, args.x, args.y, args.title, args.output, args.dark)
    elif chart_type == "scatter":
        chart_scatter(data, args.x, args.y, args.title, args.output, args.dark)

    print(json.dumps({"status": "ok", "output": args.output, "chart_type": chart_type, "rows": len(data)}))


if __name__ == "__main__":
    main()
