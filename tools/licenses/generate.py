#!/usr/bin/env python3
"""Collect pinned module licenses and reviewed asset/native notices.

Run from any directory: python tools/licenses/generate.py [--check]
Windows release builds may supply --native-root <mingw-prefix>/share/licenses.
This collects notices; dependency/license changes still require human review.
"""

import argparse
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[2]
SEPARATOR = "\n\n" + "=" * 72 + "\n"
NOTICE_NAME = re.compile(r"^(licen[cs]e|copying|notice|authors|patents|ofl)([._-]|$)", re.I)
SKIP_DIRS = {".git", "testdata", "tests", "examples", "cmd", "tools"}
NATIVE_FILES = {
    "MinGW-w64-runtime.txt": "crt/COPYING.MinGW-w64-runtime.txt",
    "GCC-Runtime-Exception-3.1.txt": "gcc-libs/COPYING.RUNTIME",
    "GPL-3.0.txt": "gcc-libs/COPYING3",
    "Winpthreads.txt": "libwinpthread/COPYING",
}
EXTRA_HEADERS = {
    "github.com/go-gl/gl": [
        ("v3.2-core/gl/package.go", "go"),
        ("v3.2-core/gl/KHR/khrplatform.h", "c"),
    ],
    "github.com/go-gl/glfw/v3.4/glfw": [
        ("glfw/deps/mingw/dinput.h", "c"),
        ("glfw/deps/mingw/xinput.h", "c"),
        ("glfw/deps/mingw/_mingw_dxhelper.h", "c"),
    ],
}


def go(*args):
    executable = os.environ.get("GO") or shutil.which("go.exe") or shutil.which("go")
    if not executable:
        raise RuntimeError("Go is required to inspect the pinned dependency versions")
    return subprocess.check_output([executable, *args], cwd=ROOT, text=True, encoding="utf-8")


def json_stream(text):
    decoder = json.JSONDecoder()
    while text.strip():
        value, end = decoder.raw_decode(text.lstrip())
        yield value
        text = text.lstrip()[end:]


def read(path):
    return path.read_text(encoding="utf-8-sig").replace("\r\n", "\n").rstrip() + "\n"


def license_files(directory):
    result = []
    for current, dirs, files in os.walk(directory):
        dirs[:] = sorted(d for d in dirs if d not in SKIP_DIRS)
        for name in sorted(files):
            if NOTICE_NAME.match(name):
                result.append(Path(current) / name)
    return sorted(result, key=lambda p: p.relative_to(directory).as_posix())


def header_notice(path, kind):
    text = read(path)
    if kind == "go":
        return text.split("\npackage ", 1)[0].rstrip() + "\n"
    start = text.index("/*")
    end = text.index("*/", start) + 2
    return text[start:end] + "\n"


def generate(native_root):
    requirements = json.loads(go("mod", "edit", "-json"))["Require"]
    paths = sorted(m["Path"] for m in requirements)
    modules = {m["Path"]: m for m in json_stream(go("list", "-m", "-json", *paths))}
    sections = [read(ROOT / "licenses/overview.txt")]

    def section(title, content):
        sections.append(title + "\n\n" + content.rstrip() + "\n")

    goroot = Path(go("env", "GOROOT").strip())
    section("Go runtime and standard library", "Source: https://go.dev/LICENSE\n\n" + read(goroot / "LICENSE") + "\nPATENTS\n\n" + read(goroot / "PATENTS"))
    for requirement in sorted(requirements, key=lambda m: m["Path"]):
        path, version = requirement["Path"], requirement["Version"]
        module = modules[path]
        if module.get("Replace"):
            raise RuntimeError(f"Review license handling for replacement module {path}")
        if not module.get("Dir") or module.get("Version") != version:
            raise RuntimeError(f"Missing source for {path}@{version}; run go mod download")
        directory = Path(module["Dir"])
        files = license_files(directory)
        if not any(NOTICE_NAME.match(p.name) and p.name.lower().startswith(("license", "licence", "copying")) for p in files):
            raise RuntimeError(f"No license found for {path}@{version}; review before distribution")
        content = f"Source: https://pkg.go.dev/{path}@{version}\n"
        for file in files:
            content += "\n--- " + file.relative_to(directory).as_posix() + " ---\n\n" + read(file)
        for relative, kind in EXTRA_HEADERS.get(path, []):
            content += "\n--- Notice from " + relative + " ---\n\n" + header_notice(directory / relative, kind)
        section(path + " " + version, content)

    section("SQLite core (public domain)", "Source: https://sqlite.org/copyright.html\n\nThe bundled SQLite core is dedicated to the public domain. Its Go binding is\ncovered separately by the go-sqlite3 license reproduced above.\n")
    unicode = "Unicode data used by the Go/text segmentation and shaping dependencies.\nSources and scope: licenses/README.md\n"
    for file in sorted((ROOT / "licenses/data").glob("*.txt")):
        unicode += "\n--- " + file.name + " ---\n\n" + read(file)
    section("Unicode data notices", unicode)
    graphics = "Sources and scope: licenses/README.md\n"
    for file in sorted((ROOT / "licenses/graphics").glob("*.txt")):
        graphics += "\n--- " + file.name + " ---\n\n" + read(file)
    section("Additional graphics and header license texts", graphics)
    native = "Sources and scope: licenses/README.md\n"
    for name, relative in sorted(NATIVE_FILES.items()):
        file = native_root / relative if native_root else ROOT / "licenses/native" / name
        native += "\n--- " + name + " ---\n\n" + read(file)
    section("Windows native runtime notices", native)
    return SEPARATOR.join(sections)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true", help="fail if committed notices are stale")
    parser.add_argument("--native-root", type=Path, help="read native notices from the actual MinGW build installation")
    args = parser.parse_args()
    if args.check and args.native_root:
        parser.error("--check uses the reviewed native snapshots; omit --native-root")
    notices = generate(args.native_root)
    output = ROOT / "THIRD_PARTY_NOTICES.txt"
    if args.check:
        if not output.exists() or read(output) != notices:
            raise RuntimeError("THIRD_PARTY_NOTICES.txt is stale; review changes and run python tools/licenses/generate.py")
        print("Third-party notices match the pinned modules and reviewed license texts.")
    else:
        with output.open("w", encoding="utf-8", newline="\n") as file:
            file.write(notices)
        print(f"Wrote {output.name} ({len(notices.encode('utf-8'))} bytes)")


if __name__ == "__main__":
    try:
        main()
    except (OSError, RuntimeError, subprocess.CalledProcessError) as error:
        print(error, file=sys.stderr)
        sys.exit(1)
