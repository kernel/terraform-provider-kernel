#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

cd "$root"

command -v python3 >/dev/null || {
	echo "python3 is required to check Markdown links" >&2
	exit 1
}

python3 - <<'PY'
import pathlib
import re
import subprocess
import sys
import urllib.parse

root = pathlib.Path.cwd()
tracked_markdown = subprocess.check_output(["git", "ls-files", "*.md"], text=True)
files = [pathlib.Path(p) for p in tracked_markdown.splitlines()]
# Matches both links and images; a missing local image target is just as
# broken a reference as a missing link target.
link_re = re.compile(r"!?\[[^\]]*\]\(([^)]+)\)")
title_re = re.compile(r'^(\S+)\s+"[^"]*"$')
errors: list[str] = []

for path in files:
    text = path.read_text(encoding="utf-8")
    for match in link_re.finditer(text):
        target = match.group(1).strip()
        # Drop an optional quoted title: [text](path "title").
        titled = title_re.match(target)
        if titled:
            target = titled.group(1)
        if not target or target.startswith(("#", "http://", "https://", "mailto:")):
            continue

        target = target.split("#", 1)[0]
        target = urllib.parse.unquote(target)
        if not target:
            continue

        resolved = (root / path.parent / target).resolve()
        try:
            resolved.relative_to(root)
        except ValueError:
            errors.append(f"{path}: link leaves repository: {target}")
            continue

        if not resolved.exists():
            errors.append(f"{path}: missing link target: {target}")

if errors:
    print("\n".join(errors), file=sys.stderr)
    sys.exit(1)
PY
