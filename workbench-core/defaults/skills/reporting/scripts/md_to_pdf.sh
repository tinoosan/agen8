#!/usr/bin/env bash
set -euo pipefail
#
# Convert a Markdown file to PDF.
#
# Usage:
#   ./md_to_pdf.sh report.md                       # → report.pdf
#   ./md_to_pdf.sh report.md --output custom.pdf   # → custom.pdf
#   ./md_to_pdf.sh report.md --style github        # GitHub-flavored styling
#
# Requirements (auto-detected):
#   Option A: pandoc + latex    (brew install pandoc basictex)
#   Option B: weasyprint        (pip install weasyprint)
#   Option C: python + markdown (pip install markdown)  [basic fallback]
#
# The script tries each converter in order and uses the first available.

INPUT=""
OUTPUT=""
STYLE="default"

usage() {
    echo "Usage: $0 <input.md> [--output <output.pdf>] [--style default|github|minimal]"
    exit 1
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --output) OUTPUT="$2"; shift 2 ;;
        --style)  STYLE="$2"; shift 2 ;;
        -h|--help) usage ;;
        *)
            if [ -z "$INPUT" ]; then
                INPUT="$1"
            else
                echo "ERROR: Unexpected argument: $1" >&2; usage
            fi
            shift ;;
    esac
done

[ -z "$INPUT" ] && usage
[ ! -f "$INPUT" ] && { echo "ERROR: File not found: $INPUT" >&2; exit 1; }

if [ -z "$OUTPUT" ]; then
    OUTPUT="${INPUT%.md}.pdf"
fi

# --- Pandoc + LaTeX -----------------------------------------------------------
if command -v pandoc &>/dev/null; then
    PANDOC_ARGS=(-f markdown -o "$OUTPUT" --pdf-engine=xelatex -V geometry:margin=1in)
    if [ "$STYLE" = "github" ]; then
        PANDOC_ARGS+=(-V mainfont="Helvetica" -V fontsize=11pt)
    elif [ "$STYLE" = "minimal" ]; then
        PANDOC_ARGS+=(-V fontsize=10pt -V pagestyle=empty)
    fi
    if pandoc "$INPUT" "${PANDOC_ARGS[@]}" 2>/dev/null; then
        echo "{\"status\": \"ok\", \"output\": \"$OUTPUT\", \"converter\": \"pandoc\", \"pages\": \"unknown\"}"
        exit 0
    fi
    echo "WARN: pandoc failed (missing LaTeX?), trying fallbacks..." >&2
fi

# --- WeasyPrint ---------------------------------------------------------------
if python3 -c "import weasyprint" 2>/dev/null; then
    # Convert md → html → pdf
    TMPHTML=$(mktemp /tmp/md_to_pdf_XXXXX.html)
    trap "rm -f $TMPHTML" EXIT

    CSS=""
    if [ "$STYLE" = "github" ]; then
        CSS="body{font-family:Helvetica,Arial,sans-serif;max-width:800px;margin:40px auto;padding:0 20px;font-size:14px;line-height:1.6;color:#333}h1,h2,h3{border-bottom:1px solid #eee;padding-bottom:6px}code{background:#f5f5f5;padding:2px 6px;border-radius:3px}pre{background:#f5f5f5;padding:16px;border-radius:6px;overflow-x:auto}table{border-collapse:collapse;width:100%}th,td{border:1px solid #ddd;padding:8px;text-align:left}th{background:#f5f5f5}"
    elif [ "$STYLE" = "minimal" ]; then
        CSS="body{font-family:serif;max-width:700px;margin:20px auto;font-size:12px;line-height:1.5}"
    else
        CSS="body{font-family:-apple-system,system-ui,sans-serif;max-width:800px;margin:40px auto;padding:0 20px;font-size:14px;line-height:1.6;color:#1a1a1a}h1{color:#2c3e50}h2{color:#34495e}code{background:#f0f0f0;padding:2px 5px;border-radius:3px;font-size:13px}pre{background:#f8f8f8;padding:16px;border-radius:6px;overflow-x:auto;border:1px solid #e1e1e1}table{border-collapse:collapse;width:100%;margin:16px 0}th,td{border:1px solid #ddd;padding:10px}th{background:#f5f7fa;font-weight:600}"
    fi

    python3 -c "
import markdown, sys
with open('$INPUT') as f:
    content = f.read()
html = markdown.markdown(content, extensions=['tables', 'fenced_code', 'toc'])
with open('$TMPHTML', 'w') as f:
    f.write(f'<html><head><style>$CSS</style></head><body>{html}</body></html>')
" 2>/dev/null

    python3 -c "
import weasyprint
weasyprint.HTML(filename='$TMPHTML').write_pdf('$OUTPUT')
" 2>/dev/null

    echo "{\"status\": \"ok\", \"output\": \"$OUTPUT\", \"converter\": \"weasyprint\"}"
    exit 0
fi

# --- Fallback: python markdown → html, then suggest manual conversion ---------
if python3 -c "import markdown" 2>/dev/null; then
    HTML_OUT="${OUTPUT%.pdf}.html"
    python3 -c "
import markdown
with open('$INPUT') as f:
    content = f.read()
html = markdown.markdown(content, extensions=['tables', 'fenced_code', 'toc'])
with open('$HTML_OUT', 'w') as f:
    f.write(f'<html><head><style>body{{font-family:sans-serif;max-width:800px;margin:40px auto;padding:0 20px;line-height:1.6}}</style></head><body>{html}</body></html>')
"
    echo "{\"status\": \"partial\", \"output\": \"$HTML_OUT\", \"converter\": \"markdown_fallback\", \"note\": \"Generated HTML instead of PDF. Install pandoc or weasyprint for PDF output.\"}"
    exit 0
fi

echo "{\"status\": \"error\", \"error\": \"No PDF converter available. Install: brew install pandoc basictex OR pip install weasyprint\"}"
exit 1
