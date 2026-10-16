#!/usr/bin/env python3
"""Check prwale color tokens for WCAG contrast.

Usage: contrast.py [path/to/tokens.json]

Every foreground token is measured against the backgrounds it is drawn on
(surface, selected, code-bg) and every badge color against on-fill text.
Exits non-zero when a token that carries meaning falls below 4.5:1 on the
terminal surface. `subtle` is exempt by design (3:1 floor): it is reserved
for hints a user never needs to read to act.
"""
import json
import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
DEFAULT = os.path.join(HERE, "..", "..", "..", "docs", "design-system", "tokens.json")


def luminance(hex_color: str) -> float:
    h = hex_color.lstrip("#")
    channels = [int(h[i:i + 2], 16) / 255 for i in (0, 2, 4)]
    lin = [c / 12.92 if c <= 0.03928 else ((c + 0.055) / 1.055) ** 2.4 for c in channels]
    return 0.2126 * lin[0] + 0.7152 * lin[1] + 0.0722 * lin[2]


def ratio(a: str, b: str) -> float:
    la, lb = sorted((luminance(a), luminance(b)), reverse=True)
    return (la + 0.05) / (lb + 0.05)


def main() -> int:
    path = sys.argv[1] if len(sys.argv) > 1 else DEFAULT
    tokens = {t["name"]: t["value"] for t in json.load(open(path))["color"]["tokens"]}
    backgrounds = ["surface", "selected", "code-bg"]
    foregrounds = ["bright", "ink", "muted", "subtle", "accent", "red", "amber", "green", "teal", "violet"]
    floors = {"subtle": 3.0}
    failed = False
    print(f"{'token':<8}" + "".join(f"{bg:>12}" for bg in backgrounds))
    for fg in foregrounds:
        cells = []
        for bg in backgrounds:
            r = ratio(tokens[fg], tokens[bg])
            floor = floors.get(fg, 4.5)
            mark = " " if r >= floor else "!"
            if bg == "surface" and r < floor:
                failed = True
            cells.append(f"{r:>10.2f}:1{mark}")
        print(f"{fg:<8}" + "".join(cells))
    print("\nbadges (on-fill text on a filled state color)")
    for fill in ["red", "amber", "green", "accent"]:
        r = ratio(tokens["on-fill"], tokens[fill])
        mark = "" if r >= 4.5 else "  !"
        failed |= r < 4.5
        print(f"  {fill:<7}{r:>6.2f}:1{mark}")
    print("\nborder on surface: %.2f:1 (decorative; 1.5:1 or more keeps rules visible)" % ratio(tokens["border"], tokens["surface"]))
    print("FAIL" if failed else "OK")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
