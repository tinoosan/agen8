#!/usr/bin/env bash
set -euo pipefail
#
# Convert a Markdown file to DOCX (Word).
#
# Usage:
#   ./md_to_docx.sh report.md                       # → report.docx
#   ./md_to_docx.sh report.md --output custom.docx  # → custom.docx
#   ./md_to_docx.sh report.md --toc                 # include table of contents
#
# Requirements:
#   pandoc (brew install pandoc)
#   OR python-docx (pip install python-docx markdown)

INPUT=""
OUTPUT=""
TOC=false

usage() {
    echo "Usage: $0 <input.md> [--output <output.docx>] [--toc]"
    exit 1
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --output) OUTPUT="$2"; shift 2 ;;
        --toc)    TOC=true; shift ;;
        -h|--help) usage ;;
        *)
            if [ -z "$INPUT" ]; then INPUT="$1"; else echo "ERROR: Unexpected: $1" >&2; usage; fi
            shift ;;
    esac
done

[ -z "$INPUT" ] && usage
[ ! -f "$INPUT" ] && { echo "ERROR: File not found: $INPUT" >&2; exit 1; }
[ -z "$OUTPUT" ] && OUTPUT="${INPUT%.md}.docx"

# --- Pandoc -------------------------------------------------------------------
if command -v pandoc &>/dev/null; then
    ARGS=(-f markdown -t docx -o "$OUTPUT")
    $TOC && ARGS+=(--toc)
    pandoc "$INPUT" "${ARGS[@]}"
    echo "{\"status\": \"ok\", \"output\": \"$OUTPUT\", \"converter\": \"pandoc\"}"
    exit 0
fi

# --- python-docx fallback -----------------------------------------------------
if python3 -c "import docx, markdown" 2>/dev/null; then
    python3 << 'PYEOF'
import sys, markdown, re
from docx import Document
from docx.shared import Pt, Inches
from docx.enum.text import WD_ALIGN_PARAGRAPH

input_file = sys.argv[1]
output_file = sys.argv[2]

with open(input_file) as f:
    lines = f.readlines()

doc = Document()
style = doc.styles['Normal']
font = style.font
font.name = 'Calibri'
font.size = Pt(11)

for line in lines:
    stripped = line.rstrip()
    if stripped.startswith('# '):
        doc.add_heading(stripped[2:], level=1)
    elif stripped.startswith('## '):
        doc.add_heading(stripped[3:], level=2)
    elif stripped.startswith('### '):
        doc.add_heading(stripped[4:], level=3)
    elif stripped.startswith('#### '):
        doc.add_heading(stripped[5:], level=4)
    elif stripped.startswith('- ') or stripped.startswith('* '):
        doc.add_paragraph(stripped[2:], style='List Bullet')
    elif re.match(r'^\d+\.\s', stripped):
        text = re.sub(r'^\d+\.\s*', '', stripped)
        doc.add_paragraph(text, style='List Number')
    elif stripped.startswith('> '):
        p = doc.add_paragraph(stripped[2:])
        p.style = doc.styles['Quote'] if 'Quote' in [s.name for s in doc.styles] else doc.styles['Normal']
    elif stripped == '---' or stripped == '***':
        doc.add_paragraph('─' * 50)
    elif stripped:
        doc.add_paragraph(stripped)

doc.save(output_file)
PYEOF
    python3 -c "" "$INPUT" "$OUTPUT"  # dummy for syntax
    # Actually run:
    python3 -c "
import sys, markdown, re
from docx import Document
from docx.shared import Pt

with open('$INPUT') as f:
    lines = f.readlines()

doc = Document()
style = doc.styles['Normal']
style.font.name = 'Calibri'
style.font.size = Pt(11)

for line in lines:
    s = line.rstrip()
    if s.startswith('# '): doc.add_heading(s[2:], level=1)
    elif s.startswith('## '): doc.add_heading(s[3:], level=2)
    elif s.startswith('### '): doc.add_heading(s[4:], level=3)
    elif s.startswith('- ') or s.startswith('* '): doc.add_paragraph(s[2:], style='List Bullet')
    elif re.match(r'^\d+\.\s', s): doc.add_paragraph(re.sub(r'^\d+\.\s*', '', s), style='List Number')
    elif s: doc.add_paragraph(s)

doc.save('$OUTPUT')
"
    echo "{\"status\": \"ok\", \"output\": \"$OUTPUT\", \"converter\": \"python-docx\"}"
    exit 0
fi

echo "{\"status\": \"error\", \"error\": \"No DOCX converter available. Install: brew install pandoc OR pip install python-docx markdown\"}"
exit 1
