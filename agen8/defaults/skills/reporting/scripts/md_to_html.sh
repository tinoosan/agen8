#!/usr/bin/env bash
set -euo pipefail
#
# Convert a Markdown file to styled HTML.
#
# Usage:
#   ./md_to_html.sh report.md                         # → report.html
#   ./md_to_html.sh report.md --output page.html      # → page.html
#   ./md_to_html.sh report.md --style github          # GitHub-like theme
#   ./md_to_html.sh report.md --style dark            # dark mode theme
#   ./md_to_html.sh report.md --title "Q2 Report"     # custom <title>
#
# Requirements (one of):
#   pandoc (brew install pandoc)
#   python markdown (pip install markdown)

INPUT=""
OUTPUT=""
STYLE="default"
TITLE=""

usage() {
    echo "Usage: $0 <input.md> [--output <out.html>] [--style default|github|dark|minimal] [--title <title>]"
    exit 1
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --output) OUTPUT="$2"; shift 2 ;;
        --style)  STYLE="$2"; shift 2 ;;
        --title)  TITLE="$2"; shift 2 ;;
        -h|--help) usage ;;
        *)
            if [ -z "$INPUT" ]; then INPUT="$1"; else echo "ERROR: Unexpected: $1" >&2; usage; fi
            shift ;;
    esac
done

[ -z "$INPUT" ] && usage
[ ! -f "$INPUT" ] && { echo "ERROR: File not found: $INPUT" >&2; exit 1; }
[ -z "$OUTPUT" ] && OUTPUT="${INPUT%.md}.html"
[ -z "$TITLE" ] && TITLE=$(basename "${INPUT%.md}")

# --- CSS themes ---------------------------------------------------------------
css_default() {
    cat << 'CSS'
:root{--bg:#fff;--fg:#1a1a1a;--accent:#2563eb;--border:#e5e7eb;--code-bg:#f3f4f6;--heading:#111827}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;max-width:800px;margin:0 auto;padding:40px 24px;line-height:1.7;color:var(--fg);background:var(--bg)}
h1{color:var(--heading);border-bottom:2px solid var(--accent);padding-bottom:8px;margin-top:2em}
h2{color:var(--heading);border-bottom:1px solid var(--border);padding-bottom:6px}
h3{color:var(--heading)}
code{background:var(--code-bg);padding:2px 6px;border-radius:4px;font-size:0.9em;font-family:'SF Mono',Menlo,monospace}
pre{background:var(--code-bg);padding:16px;border-radius:8px;overflow-x:auto;border:1px solid var(--border)}
pre code{background:none;padding:0}
table{border-collapse:collapse;width:100%;margin:16px 0}
th,td{border:1px solid var(--border);padding:10px 14px;text-align:left}
th{background:var(--code-bg);font-weight:600}
blockquote{border-left:4px solid var(--accent);margin:16px 0;padding:8px 16px;background:var(--code-bg);border-radius:0 8px 8px 0}
a{color:var(--accent);text-decoration:none}a:hover{text-decoration:underline}
CSS
}

css_github() {
    cat << 'CSS'
body{font-family:-apple-system,BlinkMacSystemFont,Segoe UI,Helvetica,Arial,sans-serif;max-width:980px;margin:0 auto;padding:45px;font-size:16px;line-height:1.5;color:#24292f}
h1,h2{border-bottom:1px solid hsla(210,18%,87%,1);padding-bottom:.3em}
h1{font-size:2em}h2{font-size:1.5em}
code{padding:.2em .4em;margin:0;font-size:85%;background:rgba(175,184,193,.2);border-radius:6px;font-family:ui-monospace,SFMono-Regular,SF Mono,Menlo,monospace}
pre{padding:16px;overflow:auto;font-size:85%;line-height:1.45;background:#f6f8fa;border-radius:6px}
pre code{background:transparent;padding:0}
table{border-spacing:0;border-collapse:collapse;width:100%}
th,td{padding:6px 13px;border:1px solid #d0d7de}
th{background:#f6f8fa;font-weight:600}
blockquote{padding:0 1em;color:#57606a;border-left:.25em solid #d0d7de;margin:0 0 16px}
CSS
}

css_dark() {
    cat << 'CSS'
:root{--bg:#0d1117;--fg:#c9d1d9;--accent:#58a6ff;--border:#30363d;--code-bg:#161b22;--heading:#f0f6fc}
body{font-family:-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif;max-width:800px;margin:0 auto;padding:40px 24px;line-height:1.7;color:var(--fg);background:var(--bg)}
h1,h2,h3{color:var(--heading)}
h1{border-bottom:1px solid var(--border);padding-bottom:8px}
h2{border-bottom:1px solid var(--border);padding-bottom:6px}
code{background:var(--code-bg);padding:2px 6px;border-radius:4px;font-size:0.9em;color:#ff7b72;font-family:'SF Mono',Menlo,monospace}
pre{background:var(--code-bg);padding:16px;border-radius:8px;overflow-x:auto;border:1px solid var(--border)}
pre code{color:var(--fg);background:none}
table{border-collapse:collapse;width:100%;margin:16px 0}
th,td{border:1px solid var(--border);padding:10px 14px}
th{background:var(--code-bg);font-weight:600}
blockquote{border-left:4px solid var(--accent);padding:8px 16px;color:#8b949e;background:var(--code-bg);border-radius:0 6px 6px 0}
a{color:var(--accent)}
CSS
}

css_minimal() {
    cat << 'CSS'
body{font-family:Georgia,serif;max-width:640px;margin:60px auto;padding:0 20px;line-height:1.8;color:#333;font-size:18px}
h1{font-size:1.8em;margin-top:2em}h2{font-size:1.4em}
code{font-family:Menlo,monospace;font-size:0.85em;background:#f5f5f5;padding:1px 4px}
pre{background:#f5f5f5;padding:12px;overflow-x:auto}
table{width:100%;border-collapse:collapse}th,td{border-bottom:1px solid #ddd;padding:8px;text-align:left}
CSS
}

get_css() {
    case "$STYLE" in
        github)  css_github ;;
        dark)    css_dark ;;
        minimal) css_minimal ;;
        *)       css_default ;;
    esac
}

# --- Conversion ---------------------------------------------------------------
CSS_CONTENT=$(get_css)

# Try pandoc first
if command -v pandoc &>/dev/null; then
    TMPSTYLE=$(mktemp /tmp/md_style_XXXXX.css)
    echo "$CSS_CONTENT" > "$TMPSTYLE"
    pandoc "$INPUT" -f markdown -t html5 --standalone --css="$TMPSTYLE" \
        --metadata title="$TITLE" -o "$OUTPUT" 2>/dev/null
    rm -f "$TMPSTYLE"
    echo "{\"status\": \"ok\", \"output\": \"$OUTPUT\", \"converter\": \"pandoc\", \"style\": \"$STYLE\"}"
    exit 0
fi

# Fallback: python markdown
if python3 -c "import markdown" 2>/dev/null; then
    python3 << PYEOF
import markdown

with open('$INPUT') as f:
    content = f.read()

html = markdown.markdown(content, extensions=['tables', 'fenced_code', 'toc', 'codehilite'])
css = '''$CSS_CONTENT'''

page = f"""<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>$TITLE</title>
<style>{css}</style>
</head>
<body>
{html}
</body>
</html>"""

with open('$OUTPUT', 'w') as f:
    f.write(page)
PYEOF
    echo "{\"status\": \"ok\", \"output\": \"$OUTPUT\", \"converter\": \"python-markdown\", \"style\": \"$STYLE\"}"
    exit 0
fi

echo "{\"status\": \"error\", \"error\": \"No Markdown converter available. Install: brew install pandoc OR pip install markdown\"}"
exit 1
