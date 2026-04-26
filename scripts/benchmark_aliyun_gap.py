#!/usr/bin/env python3
"""
阿里云式拼图缺口检测：**合成数据**上统计命中率（有真值），并支持对 ``--dataset-root`` 下样本做离线重跑。

合成场景：噪声底图 + 挖洞 + 半透明模糊块（模仿 shadow.png），检验
``aliyun_prepare_match_bytes`` + 多尺度 Canny 与 dddd 两路逻辑是否优于裸匹配。

用法（在 scripts 目录）::

    python benchmark_aliyun_gap.py --synthetic --trials 300 --tol 14
    python benchmark_aliyun_gap.py --dataset-root ./aliyun_captcha_dataset

目标：合成集上命中率宜 **≥50%**（``--tol`` 像素内算对）；真站图无真值时仅输出各算法 raw_x 供人工对照 meta.json。
"""
from __future__ import annotations

import argparse
import json
import os
import sys
from io import BytesIO

import numpy as np

_script_dir = os.path.dirname(os.path.abspath(__file__))
if _script_dir not in sys.path:
    sys.path.insert(0, _script_dir)


def _png_rgb(arr_rgb: np.ndarray) -> bytes:
    from PIL import Image

    buf = BytesIO()
    Image.fromarray(arr_rgb, mode="RGB").save(buf, format="PNG")
    return buf.getvalue()


def synth_pair(
    rng: np.random.Generator,
    *,
    bw: int = 520,
    h: int = 200,
    tw: int = 56,
) -> tuple[bytes, bytes, int]:
    """返回 (bg_png, slider_rgba_png_bytes, true_left_x)。"""
    import cv2

    true_x = int(rng.integers(40, max(41, bw - tw - 40)))
    bg = np.ones((h, bw, 3), dtype=np.float32) * 210.0
    bg += rng.normal(0, 9.0, size=(h, bw, 3))
    bg = np.clip(bg, 35, 255).astype(np.uint8)

    piece = bg[:, true_x : true_x + tw].copy().astype(np.uint8)
    piece = cv2.GaussianBlur(piece, (3, 3), 0)
    piece = np.clip(piece.astype(np.float32) * 0.82 + 28.0, 0, 255).astype(np.uint8)

    hole = bg[:, true_x : true_x + tw].copy()
    bg[:, true_x : true_x + tw] = np.clip(hole.astype(np.int16) - 55, 40, 255).astype(np.uint8)

    alpha = np.ones((h, tw), dtype=np.uint8) * 210
    rgba = np.dstack([piece, alpha])

    from PIL import Image

    sbuf = BytesIO()
    Image.fromarray(rgba, mode="RGBA").save(sbuf, format="PNG")
    return _png_rgb(bg), sbuf.getvalue(), true_x


def _eval_once(sl: bytes, bg: bytes, true_x: int, tol: int, use_prep: bool) -> bool:
    from icgoo_aliyun_gap import aliyun_prepare_match_bytes
    from icgoo_yidun_solver import (
        _dddocr_slide_match_to_left_x,
        _plausible_gap_left_x,
        _slide_match_pure_cv_like_dddd_engine,
    )

    if use_prep:
        sl, bg = aliyun_prepare_match_bytes(sl, bg)
    from PIL import Image

    tw = int(Image.open(BytesIO(sl)).size[0])
    im_bg = Image.open(BytesIO(bg))
    bw = int(im_bg.size[0])

    xs: list[int] = []
    for simple in (False, True):
        for cl, ch in ((50, 150), (40, 120), (30, 90)):
            try:
                r = _slide_match_pure_cv_like_dddd_engine(
                    sl, bg, simple_target=simple, canny_low=cl, canny_high=ch
                )
                lx = _dddocr_slide_match_to_left_x(r, tw)
                if lx is not None and _plausible_gap_left_x(lx, bw, tw):
                    xs.append(int(lx))
            except Exception:
                continue
    if not xs:
        return False
    xs.sort()
    pick = xs[len(xs) // 2]
    return abs(pick - true_x) <= tol


def run_synthetic(trials: int, tol: int, seed: int) -> tuple[int, int]:
    rng = np.random.default_rng(seed)
    ok_raw = 0
    ok_prep = 0
    for _ in range(trials):
        bg_b, sl_b, tx = synth_pair(rng)
        if _eval_once(sl_b, bg_b, tx, tol, use_prep=False):
            ok_raw += 1
        if _eval_once(sl_b, bg_b, tx, tol, use_prep=True):
            ok_prep += 1
    return ok_raw, ok_prep


def scan_dataset(root: str) -> None:
    from icgoo_aliyun_gap import aliyun_prepare_match_bytes
    from icgoo_yidun_solver import (
        _dddocr_slide_match_to_left_x,
        _plausible_gap_left_x,
        _slide_match_pure_cv_like_dddd_engine,
    )

    from PIL import Image

    subs = [
        os.path.join(root, d)
        for d in sorted(os.listdir(root))
        if os.path.isdir(os.path.join(root, d)) and d.startswith("sample_")
    ]
    if not subs:
        print(f"未找到 sample_* 子目录: {root}", file=sys.stderr)
        return
    print(f"样本数: {len(subs)}")
    for d in subs:
        bg_p = os.path.join(d, "background.png")
        sl_p = os.path.join(d, "slider.png")
        meta_p = os.path.join(d, "meta.json")
        if not (os.path.isfile(bg_p) and os.path.isfile(sl_p)):
            continue
        with open(bg_p, "rb") as f:
            bg_b = f.read()
        with open(sl_p, "rb") as f:
            sl_b = f.read()
        meta = {}
        if os.path.isfile(meta_p):
            try:
                with open(meta_p, encoding="utf-8") as f:
                    meta = json.load(f)
            except Exception:
                pass
        tw = int(Image.open(BytesIO(sl_b)).size[0])
        bw = int(Image.open(BytesIO(bg_b)).size[0])

        def pick(prep: bool) -> int | None:
            sb, bb = (aliyun_prepare_match_bytes(sl_b, bg_b) if prep else (sl_b, bg_b))
            xs: list[int] = []
            for simple in (False, True):
                for cl, ch in ((50, 150), (40, 120)):
                    try:
                        r = _slide_match_pure_cv_like_dddd_engine(
                            sb, bb, simple_target=simple, canny_low=cl, canny_high=ch
                        )
                        lx = _dddocr_slide_match_to_left_x(r, tw)
                        if lx is not None and _plausible_gap_left_x(lx, bw, tw):
                            xs.append(int(lx))
                    except Exception:
                        pass
            if not xs:
                return None
            xs.sort()
            return xs[len(xs) // 2]

        r0 = pick(False)
        r1 = pick(True)
        stored = meta.get("raw_x")
        line = f"{os.path.basename(d)}: raw_med={r0} prep_med={r1} meta.raw_x={stored}"
        if stored is not None and r1 is not None:
            line += f" |Δprep-meta|={abs(int(r1) - int(stored))}"
        print(line)


def main() -> int:
    ap = argparse.ArgumentParser(description="阿里云缺口：合成基准与数据集扫描")
    ap.add_argument("--synthetic", action="store_true", help="跑合成数据命中率")
    ap.add_argument("--trials", type=int, default=200)
    ap.add_argument("--tol", type=int, default=14, help="与真值允许偏差（像素）")
    ap.add_argument("--seed", type=int, default=42)
    ap.add_argument("--dataset-root", default=None, help="export 出的 sample_* 父目录")
    args = ap.parse_args()

    if args.synthetic:
        ok0, ok1 = run_synthetic(args.trials, args.tol, args.seed)
        print(
            f"合成 {args.trials} 次 tol={args.tol}px: "
            f"无预处理命中 {ok0}/{args.trials} ({100.0 * ok0 / args.trials:.1f}%), "
            f"阿里云预处理+多尺度命中 {ok1}/{args.trials} ({100.0 * ok1 / args.trials:.1f}%)"
        )
        if ok1 < args.trials * 0.5:
            print(
                "提示：未达 50% 时可调大 --trials、检查本机 OpenCV；"
                "或继续收集真图样本用 --dataset-root 对照 meta.raw_x。",
                file=sys.stderr,
            )
    if args.dataset_root:
        scan_dataset(os.path.abspath(args.dataset_root))
    if not args.synthetic and not args.dataset_root:
        ap.print_help()
        return 2
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
