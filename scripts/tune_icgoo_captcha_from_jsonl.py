#!/usr/bin/env python3
"""
根据 ``aliyun_calibration.jsonl``（或合并多文件）里的 ``gap_detected`` / ``slide_attempt`` 事件，
统计阿里云滑块会话成败，并**给出可执行的环境变量建议**（提高过验率时可调 ``drag_scale_mult``、
``shadow ensemble`` 权重等）。

数据来自 ``icgoo_crawler_dev`` 验证码快照目录下的 ``aliyun_calibration.jsonl``，或
``ICGOO_CAPTCHA_CALIBRATION_JSONL`` 指向的文件。

用法::

    python scripts/tune_icgoo_captcha_from_jsonl.py path/to/aliyun_calibration.jsonl
    python scripts/tune_icgoo_captcha_from_jsonl.py scripts/icgoo_captcha_snapshots/run_20260405_*/aliyun_calibration.jsonl
    python scripts/tune_icgoo_captcha_from_jsonl.py a.jsonl b.jsonl --write-env icgoo_tuned_env.cmd

说明：脚本**不**改写系统配置，仅打印报告并可选写出 ``.cmd`` / ``.sh``，由你手工执行或合并到启动脚本。
"""
from __future__ import annotations

import argparse
import glob
import json
import os
import sys
from dataclasses import dataclass, field
from pathlib import Path


def _iter_jsonl(path: Path):
    with open(path, encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if not line or line.startswith("#"):
                continue
            try:
                yield json.loads(line)
            except json.JSONDecodeError:
                continue


def _load_all(paths: list[str]) -> list[tuple[str, dict]]:
    out: list[tuple[str, dict]] = []
    for pat in paths:
        for p in sorted(glob.glob(pat) if any(c in pat for c in "*?[]") else [pat]):
            pp = Path(p)
            if not pp.is_file():
                continue
            for rec in _iter_jsonl(pp):
                out.append((str(pp), rec))
    return out


def _is_aliyun_gap(g: dict) -> bool:
    if g.get("is_aliyun_active") is True:
        return True
    try:
        return int(g.get("aliyun_drag_extra_applied") or 0) > 0
    except (TypeError, ValueError):
        return False


@dataclass
class RoundAgg:
    gap: dict
    slides: list[dict] = field(default_factory=list)

    def success(self) -> bool:
        for s in self.slides:
            if s.get("check_success_ok") or s.get("outcome") == "success":
                return True
        return False

    def first_slide(self) -> dict | None:
        if not self.slides:
            return None
        return min(self.slides, key=lambda x: int(x.get("attempt_index", 99)))


def _segment_rounds(rows: list[tuple[str, dict]]) -> list[RoundAgg]:
    rounds: list[RoundAgg] = []
    current: RoundAgg | None = None
    for _src, rec in rows:
        ev = rec.get("event")
        if ev == "gap_detected":
            if current is not None:
                rounds.append(current)
            current = RoundAgg(gap=dict(rec))
        elif ev == "slide_attempt" and current is not None:
            cr = rec.get("captcha_round")
            if cr is not None and cr == current.gap.get("captcha_round"):
                current.slides.append(dict(rec))
    if current is not None:
        rounds.append(current)
    return rounds


def _report(aliyun: list[RoundAgg]) -> dict:
    n = len(aliyun)
    ok = sum(1 for r in aliyun if r.success())
    first = [r.first_slide() for r in aliyun if r.first_slide()]
    fail_new = sum(
        1
        for s in first
        if s.get("outcome") == "fail_new_image" and s.get("puzzle_replaced") is True
    )
    fail_same = sum(1 for s in first if s.get("outcome") == "fail_same_image")
    shadow_pick = sum(
        1 for r in aliyun if (r.gap.get("gap_ensemble_pick") or "") == "aliyun_shadow_ms"
    )
    op_shadow = sum(
        1 for r in aliyun if (r.gap.get("opencv_mode_primary") or "") == "shadow_ms"
    )
    last_mult = None
    last_boost = None
    for r in aliyun:
        g = r.gap
        if g.get("aliyun_drag_scale_mult") is not None:
            try:
                last_mult = float(g["aliyun_drag_scale_mult"])
            except (TypeError, ValueError):
                pass
        if g.get("drag_boost") is not None:
            try:
                last_boost = float(g["drag_boost"])
            except (TypeError, ValueError):
                pass
    return {
        "n_rounds": n,
        "success_count": ok,
        "success_rate": (ok / n) if n else 0.0,
        "first_fail_new_image": fail_new,
        "first_fail_same_image": fail_same,
        "ensemble_shadow_ms": shadow_pick,
        "opencv_primary_shadow_ms": op_shadow,
        "last_aliyun_drag_scale_mult": last_mult,
        "last_drag_boost": last_boost,
    }


def _suggest(st: dict) -> dict[str, str]:
    """返回建议的环境变量键值（字符串）。"""
    sug: dict[str, str] = {}
    n = st["n_rounds"]
    if n < 5:
        return sug
    sr = float(st["success_rate"])
    fni = int(st["first_fail_new_image"])
    mult = st["last_aliyun_drag_scale_mult"]
    boost = st["last_drag_boost"]

    # 首轮即换题失败多：常见为略少拖，略增 scale 或 boost
    if sr < 0.35 and fni >= max(4, int(n * 0.55)):
        if mult is not None:
            nm = min(2.0, float(mult) + 0.04)
            if nm > float(mult) + 1e-6:
                sug["ICGOO_ALIYUN_DRAG_SCALE_MULT"] = f"{nm:.4f}".rstrip("0").rstrip(".")
        if boost is not None:
            nb = min(1.22, float(boost) + 0.03)
            if nb > float(boost) + 1e-6:
                sug["SUGGEST_DRAG_BOOST"] = f"{nb:.4f}".rstrip("0").rstrip(".")

    # shadow 主选且成功率低：压低 ensemble 权重
    if (
        st["ensemble_shadow_ms"] >= max(3, n // 3)
        and sr < 0.4
        and st["opencv_primary_shadow_ms"] >= max(3, n // 3)
    ):
        sug["ICGOO_ALIYUN_SHADOW_ENSEMBLE_WEIGHT"] = "0.65"

    if sr < 0.25 and n >= 10:
        sug["ICGOO_ALIYUN_SYSTEMATIC_DRAG_EXTRA_PX"] = "24"

    return sug


def _print_report(st: dict, sug: dict[str, str]) -> None:
    print("=== ICGOO 验证码标定复盘（阿里云相关轮次）===")
    print(f"  轮次数: {st['n_rounds']}")
    print(f"  任一次滑动成功: {st['success_count']}  成功率: {st['success_rate']:.1%}")
    print(f"  首轮 outcome=fail_new_image 且换题: {st['first_fail_new_image']}")
    print(f"  首轮 outcome=fail_same_image: {st['first_fail_same_image']}")
    print(f"  gap_ensemble_pick=aliyun_shadow_ms: {st['ensemble_shadow_ms']}")
    print(f"  opencv_mode_primary=shadow_ms: {st['opencv_primary_shadow_ms']}")
    if st["last_aliyun_drag_scale_mult"] is not None:
        print(f"  记录中最近 aliyun_drag_scale_mult: {st['last_aliyun_drag_scale_mult']}")
    if st["last_drag_boost"] is not None:
        print(f"  记录中最近 drag_boost: {st['last_drag_boost']}")
    print()
    if not sug:
        print("（样本不足或模式不明显，暂不生成自动建议；可继续收集 JSONL 后再跑本脚本。）")
        return
    print("=== 建议环境变量（请人工确认后设置）===")
    for k, v in sug.items():
        if k == "SUGGEST_DRAG_BOOST":
            print(f"  # 对应 CLI: --drag-boost {v}")
            continue
        print(f"  {k}={v}")
    print()
    print("Windows CMD 示例:")
    for k, v in sug.items():
        if k == "SUGGEST_DRAG_BOOST":
            continue
        print(f"  set {k}={v}")
    if "SUGGEST_DRAG_BOOST" in sug:
        print(f"  # 或在 icgoo_crawler_dev 加: --drag-boost {sug['SUGGEST_DRAG_BOOST']}")


def _write_env_cmd(path: Path, sug: dict[str, str]) -> None:
    lines = ["@echo off", "REM auto-generated by tune_icgoo_captcha_from_jsonl.py", ""]
    for k, v in sug.items():
        if k == "SUGGEST_DRAG_BOOST":
            lines.append(f"REM drag_boost: use icgoo_crawler_dev --drag-boost {v}")
            continue
        lines.append(f"set {k}={v}")
    path.write_text("\n".join(lines) + "\n", encoding="utf-8")
    print(f"Wrote: {path}")


def _write_env_sh(path: Path, sug: dict[str, str]) -> None:
    lines = ["#!/bin/sh", "# auto-generated by tune_icgoo_captcha_from_jsonl.py", ""]
    for k, v in sug.items():
        if k == "SUGGEST_DRAG_BOOST":
            lines.append(f"# drag_boost: icgoo_crawler_dev --drag-boost {v}")
            continue
        lines.append(f"export {k}={v}")
    path.write_text("\n".join(lines) + "\n", encoding="utf-8")
    os.chmod(path, 0o755)
    print(f"Wrote: {path}")


def main(argv: list[str] | None = None) -> int:
    p = argparse.ArgumentParser(description="从 aliyun_calibration.jsonl 生成调参建议")
    p.add_argument(
        "jsonl",
        nargs="+",
        help="JSONL 文件路径，支持通配符",
    )
    p.add_argument(
        "--write-env-cmd",
        metavar="FILE",
        help="写入 Windows .cmd（set VAR=val）",
    )
    p.add_argument(
        "--write-env-sh",
        metavar="FILE",
        help="写入 POSIX shell（export VAR=val）",
    )
    args = p.parse_args(argv)

    rows = _load_all(args.jsonl)
    if not rows:
        print("未读到任何 JSON 行；请检查路径。", file=sys.stderr)
        return 2

    rounds = _segment_rounds(rows)
    aliyun = [r for r in rounds if _is_aliyun_gap(r.gap)]
    if not aliyun:
        print("未找到阿里云相关 gap_detected（is_aliyun_active 或 aliyun_drag_extra_applied>0）。", file=sys.stderr)
        return 1

    st = _report(aliyun)
    sug = _suggest(st)
    _print_report(st, sug)

    if args.write_env_cmd and sug:
        _write_env_cmd(Path(args.write_env_cmd), sug)
    if args.write_env_sh and sug:
        _write_env_sh(Path(args.write_env_sh), sug)

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
