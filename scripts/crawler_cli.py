"""
与 ickey_crawler.py 对齐的 CLI JSON 输出（stdout 仅 JSON，错误写 stderr）。
供 find_chips / hqchip / szlcsc / icgoo 等爬虫复用。
"""

from __future__ import annotations

import json
import sys
from typing import Any


def emit_json_stdout(results: list[dict[str, Any]]) -> None:
    out = json.dumps(results, ensure_ascii=False, indent=0)
    try:
        sys.stdout.reconfigure(encoding="utf-8")
    except AttributeError:
        pass
    sys.stdout.buffer.write((out + "\n").encode("utf-8"))


def emit_json_stderr_error(message: str) -> None:
    err = json.dumps({"error": message, "results": []}, ensure_ascii=False)
    try:
        sys.stderr.buffer.write((err + "\n").encode("utf-8"))
    except Exception:
        sys.stderr.write(err + "\n")


# 与 ickey_crawler 每条记录字段一致（CSV 导出可参考）
UNIFIED_RESULT_KEYS = (
    "seq",
    "model",
    "manufacturer",
    "package",
    "desc",
    "stock",
    "moq",
    "price_tiers",
    "hk_price",
    "mainland_price",
    "lead_time",
    "query_model",
)
