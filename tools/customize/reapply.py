#!/usr/bin/env python3
"""Reapply/check FullPack's naming overlay after an upstream source merge."""
import argparse
import json
import re
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SPEC = json.loads(Path(__file__).with_name("fullpack.json").read_text(encoding="utf-8"))


def transform(path, source):
    """Protect compatibility/provenance before changing product identifiers."""
    protected = []

    def hold(match):
        protected.append(match.group(0))
        return f"\x01{len(protected) - 1}\x02"

    if path == "README.md":
        source = re.sub(r"### Name & logo[\s\S]*", hold, source)
    if path == "README_FA.md":
        source = re.sub(r"- \*\*از نام خودت استفاده کن\.\*\*[\s\S]*", hold, source)
    # The attribution's URL must continue to point at the original author.
    source = re.sub(r"Based on BackPack by Amin Mohammadi \(AminMGMT\)[\s\S]{0,100}?https://github.com/AminMGMT/BackPack", hold, source)
    for literal in sorted(SPEC["preserve_literals"], key=len, reverse=True):
        source = re.sub(re.escape(literal), hold, source)
    for pattern in SPEC["preserve_patterns"]:
        source = re.sub(pattern, hold, source)
    identity_path = path.replace("_test.go", ".go")
    if identity_path in SPEC["preserve_bare_username_files"]:
        source = re.sub(r'"backpack"', hold, source)
    if path == "internal/manage/update.go":
        source = source.replace('"backpack", cfg.Server.Token', '"\x03username\x03", cfg.Server.Token')
    # Keep an existing real repository address rather than inventing FullPack on GitHub.
    source = source.replace(SPEC["upstream_repository"], SPEC["repository"])
    source = source.replace('RepoOwner = "AminMGMT"', 'RepoOwner = "firegoood"')
    source = re.sub(r'RepoName\s+= "(?:BackPack|FullPack)"', 'RepoName  = "\x03repository\x03"', source)
    source = source.replace(SPEC["repository"], "\x03fork\x03")
    source = re.sub(r"\b(?:BackPack|Backpack|BACKPACK)\b", SPEC["display_name"], source)
    source = re.sub(r"(?<![A-Za-z])backpack(?![A-Za-z])", SPEC["command"], source)
    source = re.sub(r"بک[‌ -]?پک", SPEC["display_name"], source)
    source = source.replace("\x03repository\x03", "BackPack").replace("\x03fork\x03", SPEC["repository"])
    source = source.replace("\x03username\x03", "backpack")
    source = re.sub(r"\x01(\d+)\x02", lambda m: protected[int(m.group(1))], source)
    if path in {"README.md", "README_FA.md"}:
        source = source.replace('src="img/image.png"', 'src="img/fullpack.svg"')
        source = source.replace('src="img/cover.png"', 'src="img/fullpack.svg"')
    if path in {".github/workflows/ci.yml", ".github/workflows/release.yml"}:
        if "Verify FullPack customizations" not in source:
            newline = "\r\n" if "\r\n" in source else "\n"
            hook = "\n\n      - name: Verify FullPack customizations\n        run: |\n          python3 tools/customize/reapply.py --check\n          python3 -m unittest discover -s tools/customize -p 'test_*.py'"
            source = source.replace("      - uses: actions/checkout@v7", "      - uses: actions/checkout@v7" + hook.replace("\n", newline), 1)
    if path == "internal/tui/tui.go":
        # The old ASCII wordmark is not searchable text; replace only its banner.
        newline = "\r\n" if "\r\n" in source else "\n"
        banner = "`\n  ╔══════════════════════╗\n  ║       FullPack       ║\n  ╚══════════════════════╝`)".replace("\n", newline)
        source = re.sub(r"(func Logo\(version string\) \{\s*fmt.Print\(Red\)\s*fmt.Println\()`[\s\S]*?`\)[ \t]*",
                        lambda m: m.group(1) + banner, source, count=1)
        source = source.replace("TeleGram : @BlackProtocols  |  GitHub : https://github.com/AminMGMT",
                                "GitHub : https://github.com/" + SPEC["repository"])
    return source


def files():
    result = subprocess.run(["git", "ls-files", "-z"], cwd=ROOT, check=True, capture_output=True)
    excluded = set(SPEC["preserve_files"])
    paths = set(result.stdout.decode("utf-8").split("\0")) | set(SPEC["required"])
    for path in sorted(paths):
        if not path or path in excluded or path.startswith("tools/customize/") or path in {"AGENTS.md", "CUSTOMIZATIONS.md"}:
            continue
        yield path


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument("--apply", action="store_true", help="reapply the local naming overlay")
    mode.add_argument("--check", action="store_true", help="report drift without changing files")
    args = parser.parse_args()
    changed = []
    for path in files():
        target = ROOT / path
        if not target.is_file():
            continue # Required missing files receive a diagnostic below.
        raw = target.read_bytes()
        if b"\0" in raw:
            continue
        try:
            source = raw.decode("utf-8")
        except UnicodeDecodeError:
            continue
        updated = transform(path, source)
        if source != updated:
            changed.append(path)
            if args.apply:
                target.write_bytes(updated.encode("utf-8"))
    errors = []
    for path, snippets in SPEC["required"].items():
        target = ROOT / path
        if not target.is_file():
            errors.append(f"Missing customization surface: {path}")
            continue
        source = target.read_text(encoding="utf-8")
        for snippet in snippets:
            if snippet not in source:
                errors.append(f"{path}: missing {snippet!r}")
    if args.check and changed:
        errors.extend(f"Naming drift: {path}" for path in changed)
    print(f"{'Updated' if args.apply else 'Checked'} FullPack naming; {len(changed)} file(s) {'changed' if args.apply else 'need reapplication'}.")
    for error in errors:
        print(error)
    return 1 if errors else 0


if __name__ == "__main__":
    raise SystemExit(main())
