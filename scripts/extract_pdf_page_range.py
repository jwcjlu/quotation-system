# -*- coding: utf-8 -*-
"""从 PDF 中按页码范围（1-based，含首尾）导出为新 PDF。"""
from __future__ import annotations

import argparse
import sys
from pathlib import Path


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Extract inclusive page range from a PDF.",
        epilog="示例: %(prog)s doc.pdf 1044 1278 -o out.pdf\n"
        "  或: %(prog)s --src doc.pdf --start 1044 --end 1278 -o out.pdf",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument("src_pos", nargs="?", type=Path, default=None, metavar="src", help="源 PDF 路径（可与 --src 二选一）")
    parser.add_argument("start_pos", nargs="?", type=int, default=None, metavar="start", help="起始页 1-based（可与 --start 二选一）")
    parser.add_argument("end_pos", nargs="?", type=int, default=None, metavar="end", help="结束页含首尾（可与 --end 二选一）")
    parser.add_argument("--src", dest="src_opt", type=Path, default=None, help="源 PDF 路径")
    parser.add_argument("--start", dest="start_opt", type=int, default=None, help="起始页（从 1 开始）")
    parser.add_argument("--end", dest="end_opt", type=int, default=None, help="结束页（含该页）")
    parser.add_argument("-o", "--output", type=Path, required=True, help="输出 PDF 路径")
    args = parser.parse_args()

    src = args.src_opt or args.src_pos
    start = args.start_opt if args.start_opt is not None else args.start_pos
    end = args.end_opt if args.end_opt is not None else args.end_pos
    if src is None:
        parser.error("必须提供源 PDF：位置参数 src 或 --src")
    if start is None:
        parser.error("必须提供起始页：位置参数 start 或 --start")
    if end is None:
        parser.error("必须提供结束页：位置参数 end 或 --end")

    try:
        from pypdf import PdfReader, PdfWriter
    except ImportError:
        print("请先安装: pip install pypdf", file=sys.stderr)
        return 1

    src = src.expanduser().resolve()
    if not src.is_file():
        print(f"源文件不存在: {src}", file=sys.stderr)
        return 1

    if start < 1 or end < start:
        print("页码无效：要求 start>=1 且 end>=start", file=sys.stderr)
        return 1

    reader = PdfReader(str(src))
    total = len(reader.pages)
    if end > total:
        print(f"结束页 {end} 超过文档总页数 {total}", file=sys.stderr)
        return 1

    writer = PdfWriter()
    for i in range(start - 1, end):
        writer.add_page(reader.pages[i])

    out = args.output.expanduser().resolve()
    out.parent.mkdir(parents=True, exist_ok=True)
    with open(out, "wb") as f:
        writer.write(f)

    print(f"已写入: {out}")
    print(f"页码范围: {start}-{end}（共 {end - start + 1} 页），源文件总页数: {total}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
