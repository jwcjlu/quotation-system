#!/usr/bin/env python3
"""
离线复盘 ICGOO 滑块缺口检测：对 captcha 调试目录里的背景图 + 滑块图重跑与线上相同的模板匹配逻辑。

用法（在 scripts 目录下）::

    python replay_icgoo_gap_detect.py icgoo_captcha_snapshots/run_xxx
    python replay_icgoo_gap_detect.py --bg a.png --slider b.png

依赖与 ``icgoo_yidun_solver`` 一致（PIL；可选 ddddocr）。
"""
from __future__ import annotations

import argparse
import os
import sys
from io import BytesIO

from PIL import Image

_script_dir = os.path.dirname(os.path.abspath(__file__))
if _script_dir not in sys.path:
    sys.path.insert(0, _script_dir)

from icgoo_yidun_solver import (  # noqa: E402
    _dddocr_slide_match_to_left_x,
    _plausible_gap_left_x,
    _slide_match_pure_cv_like_dddd_engine,
)


def _load_pair(bg_path: str, slider_path: str) -> tuple[bytes, bytes, int]:
    with open(bg_path, "rb") as f:
        bg = f.read()
    with open(slider_path, "rb") as f:
        sl = f.read()
    im = Image.open(BytesIO(sl))
    tw = int(im.size[0])
    return bg, sl, tw


def _report(label: str, bg: bytes, sl: bytes, tw: int) -> None:
    r0 = _slide_match_pure_cv_like_dddd_engine(sl, bg, simple_target=False)
    r1 = _slide_match_pure_cv_like_dddd_engine(sl, bg, simple_target=True)
    x0 = _dddocr_slide_match_to_left_x(r0, tw)
    x1 = _dddocr_slide_match_to_left_x(r1, tw)
    im_bg = Image.open(BytesIO(bg))
    bg_w = int(im_bg.size[0])
    print(f"=== {label} ===")
    print(f"  piece_w={tw}px  bg_w={bg_w}px")
    print(
        f"  Canny(simple=False)  left_x={x0}  conf={r0.get('confidence')!r}"
    )
    print(
        f"  gray(simple=True)    left_x={x1}  conf={r1.get('confidence')!r}"
    )
    if x0 is not None and x1 is not None:
        print(f"  |Δx|={abs(x0 - x1)}")
    for name, x, res in (
        ("Canny", x0, r0),
        ("gray", x1, r1),
    ):
        ok = (
            x is not None
            and _plausible_gap_left_x(x, bg_w, tw)
            and (res.get("confidence") is not None)
        )
        print(f"  plausible({name})={ok}")
    print()


def main() -> int:
    p = argparse.ArgumentParser(description="离线重放缺口模板匹配")
    p.add_argument(
        "snapshot_dir",
        nargs="?",
        help="含 00_background.png / 00_slider.png 的目录（或 round_NN 前缀）",
    )
    p.add_argument("--bg", help="背景图路径")
    p.add_argument("--slider", help="滑块/拼图块图路径")
    args = p.parse_args()

    pairs: list[tuple[str, str, str]] = []

    if args.bg and args.slider:
        pairs.append(("cli", args.bg, args.slider))
    elif args.snapshot_dir:
        d = os.path.abspath(args.snapshot_dir)
        if not os.path.isdir(d):
            print(f"不是目录: {d}", file=sys.stderr)
            return 2
        # 根目录
        bg0 = os.path.join(d, "00_background.png")
        sl0 = os.path.join(d, "00_slider.png")
        if os.path.isfile(bg0) and os.path.isfile(sl0):
            pairs.append((os.path.basename(d), bg0, sl0))
        # round_NN_detect.txt 同目录可能有图（若保存了）
        for name in sorted(os.listdir(d)):
            if not name.startswith("round_") or not name.endswith("_detect.txt"):
                continue
            prefix = name.replace("_detect.txt", "")
            bgp = os.path.join(d, f"{prefix}_background.png")
            slp = os.path.join(d, f"{prefix}_slider.png")
            if os.path.isfile(bgp) and os.path.isfile(slp):
                pairs.append((prefix, bgp, slp))
        if not pairs:
            print(
                "未找到 00_background.png + 00_slider.png，"
                "或 round_NN_{background,slider}.png。"
                "请用 --bg/--slider 指定文件。",
                file=sys.stderr,
            )
            return 2
    else:
        p.print_help()
        return 2

    for label, bgp, slp in pairs:
        bg, sl, tw = _load_pair(bgp, slp)
        _report(label, bg, sl, tw)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
