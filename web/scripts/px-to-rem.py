#!/usr/bin/env python3
"""One-shot codemod: convert hardcoded px font sizes to rem so the global
font-size stepper (which sets :root font-size) rescales all UI text.

Scope (font-size only — never touches widths, borders, radii, icon sizes):
  - Tailwind utilities:  text-[13px]            -> text-[0.8125rem]
  - CSS rules:           font-size: 13px        -> font-size: 0.8125rem
  - Inline JSX styles:   fontSize: '13px'       -> fontSize: '0.8125rem'
                         fontSize: 9            -> fontSize: '0.5625rem'
                         fontSize: a ? '1px':'2px' (both literals converted)

Left untouched on purpose: template literals (`${...}px`), 'inherit',
and any identifier-valued fontSize (e.g. fontSize: size).
"""
import os
import re
import sys

ROOT = os.path.join(os.path.dirname(__file__), "..", "src")


def px_to_rem(px: str) -> str:
    rem = float(px) / 16.0
    # /16 always terminates; trim trailing zeros and dangling dot.
    s = f"{rem:.6f}".rstrip("0").rstrip(".")
    return s if s else "0"


def conv_text_util(m: re.Match) -> str:
    return f"text-[{px_to_rem(m.group(1))}rem]"


def conv_css_fontsize(m: re.Match) -> str:
    return f"font-size: {px_to_rem(m.group(1))}rem"


PX_STR = re.compile(r"(['\"])(\d+(?:\.\d+)?)px\1")
BARE_NUM = re.compile(r"^\s*(\d+(?:\.\d+)?)\s*$")


def conv_inline_fontsize(m: re.Match) -> str:
    rhs = m.group(1)
    if "${" in rhs or "`" in rhs or "inherit" in rhs:
        return m.group(0)
    bare = BARE_NUM.match(rhs)
    if bare:
        return f"fontSize: '{px_to_rem(bare.group(1))}rem'"
    new_rhs = PX_STR.sub(lambda x: f"'{px_to_rem(x.group(2))}rem'", rhs)
    return f"fontSize:{new_rhs}" if new_rhs != rhs else m.group(0)


TEXT_UTIL = re.compile(r"text-\[(\d+(?:\.\d+)?)px\]")
CSS_FONTSIZE = re.compile(r"font-size:\s*(\d+(?:\.\d+)?)px")
# RHS captured up to the next comma / closing brace / newline.
INLINE_FONTSIZE = re.compile(r"fontSize:([^,\n}]+)")


def process(path: str) -> bool:
    with open(path, "r", encoding="utf-8") as f:
        original = f.read()
    text = original
    if path.endswith((".ts", ".tsx")):
        text = TEXT_UTIL.sub(conv_text_util, text)
        text = INLINE_FONTSIZE.sub(conv_inline_fontsize, text)
    elif path.endswith(".css"):
        text = CSS_FONTSIZE.sub(conv_css_fontsize, text)
    if text != original:
        with open(path, "w", encoding="utf-8") as f:
            f.write(text)
        return True
    return False


def main() -> int:
    changed = 0
    for dirpath, _dirs, files in os.walk(ROOT):
        for name in files:
            if name.endswith((".ts", ".tsx", ".css")):
                if process(os.path.join(dirpath, name)):
                    changed += 1
    print(f"Modified {changed} files")
    return 0


if __name__ == "__main__":
    sys.exit(main())
