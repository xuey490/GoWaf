#!/usr/bin/env python3
"""把 Markdown 方案编译为自包含 HTML（侧边栏目录 + 浅色主题）。

用法：
    python3 docs/build_html.py

依赖：pandoc（>= 2.0）
"""
import re
import shutil
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent
SRC = ROOT / "Go-WAF-开发方案.md"
TMP = ROOT / "_build.md"
OUT = ROOT / "Go-WAF-开发方案.html"
TPL = ROOT / "_template.html"


def strip_for_html(md: str) -> str:
    lines = md.splitlines()

    # 1) 去掉文档大标题与紧随其后的副标题（hero 里已有）
    if lines and lines[0].startswith("# "):
        lines = lines[1:]
        while lines and not lines[0].strip():
            lines.pop(0)
        if lines and lines[0].startswith("**") and lines[0].rstrip().endswith("**"):
            lines.pop(0)

    # 2) 去掉 Markdown 内的“目录”章节（HTML 用侧边栏 TOC）
    text = "\n".join(lines)
    text = re.sub(r"\n## 目录\n.*?\n---\n", "\n", text, flags=re.S)

    return text.lstrip("\n")


def main() -> int:
    if not shutil.which("pandoc"):
        print("未找到 pandoc，请先安装：apt install pandoc", file=sys.stderr)
        return 1

    md = SRC.read_text(encoding="utf-8")
    TMP.write_text(strip_for_html(md), encoding="utf-8")

    cmd = [
        "pandoc", str(TMP),
        "-o", str(OUT),
        "--standalone",
        "--template", str(TPL),
        "--toc",
        "--toc-depth=2",
        "--metadata", "title=Go-WAF 开发方案",
        "--metadata", "lang=zh-CN",
        "--highlight-style=tango",
    ]
    subprocess.run(cmd, check=True)
    TMP.unlink(missing_ok=True)

    size = OUT.stat().st_size / 1024
    print(f"已生成 {OUT.name} ({size:.0f} KB)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
