"""
离线对比 SliderCaptchaSolver 在快照图上的缺口 raw_x：auto（含 ensemble）vs 仅 ddddocr。

参考标签优先取 00_detect.txt 的 raw_x_detected，否则 raw_x_px（历史一次识别/换算结果）。
指标：在容差 tol 像素内与参考一致的比例；改进率 = (rate_auto - rate_dddd) / max(rate_dddd, 1e-6)。

用法:
  python scripts/eval_icgoo_captcha_snapshots.py
  python scripts/eval_icgoo_captcha_snapshots.py --root scripts/icgoo_captcha_snapshots --tol 5
"""
from __future__ import annotations

import argparse
import os
import sys

_SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
if _SCRIPT_DIR not in sys.path:
    sys.path.insert(0, _SCRIPT_DIR)

from icgoo_crawler_dev import SliderCaptchaSolver  # noqa: E402


def _parse_detect(path: str) -> tuple[int | None, int | None]:
    """返回 (raw_x_detected, raw_x_px)。"""
    rd: int | None = None
    rx: int | None = None
    try:
        with open(path, encoding="utf-8") as f:
            for line in f:
                line = line.strip()
                if line.startswith("raw_x_detected\t"):
                    try:
                        rd = int(line.split("\t", 1)[1].strip())
                    except ValueError:
                        pass
                elif line.startswith("raw_x_px\t"):
                    try:
                        rx = int(line.split("\t", 1)[1].strip())
                    except ValueError:
                        pass
    except OSError:
        pass
    return rd, rx


def _reference_x(rd: int | None, rx: int | None) -> int | None:
    if rd is not None:
        return rd
    return rx


def main() -> int:
    ap = argparse.ArgumentParser(description="快照目录下 auto vs ddddocr 缺口 raw_x 对比")
    ap.add_argument(
        "--root",
        default=os.path.join(_SCRIPT_DIR, "icgoo_captcha_snapshots"),
        help="含 run_* 子目录的根路径",
    )
    ap.add_argument("--tol", type=int, default=5, help="与参考 x 相差不超过此像素算命中")
    args = ap.parse_args()
    root = os.path.abspath(args.root)
    tol = max(0, int(args.tol))

    runs = []
    for name in sorted(os.listdir(root)):
        if not name.startswith("run_"):
            continue
        d = os.path.join(root, name)
        if not os.path.isdir(d):
            continue
        bg = os.path.join(d, "00_background.png")
        sl = os.path.join(d, "00_slider.png")
        det = os.path.join(d, "00_detect.txt")
        if not (os.path.isfile(bg) and os.path.isfile(sl) and os.path.isfile(det)):
            continue
        rd, rx = _parse_detect(det)
        ref = _reference_x(rd, rx)
        if ref is None:
            continue
        runs.append((name, bg, sl, ref))

    if not runs:
        print(f"未找到可用快照（需 00_background.png + 00_slider.png + 00_detect.txt 且含 raw_x）: {root}")
        return 1

    sol_auto = SliderCaptchaSolver(offline_gap_eval=True, slider_gap_backend="auto")
    sol_dddd = SliderCaptchaSolver(offline_gap_eval=True, slider_gap_backend="ddddocr")
    sol_cv = SliderCaptchaSolver(offline_gap_eval=True, slider_gap_backend="opencv")

    hit_a = hit_d = hit_o = n = 0
    rows: list[str] = []
    for name, bg, sl, ref in runs:
        try:
            xa = sol_auto.detect_gap_position(bg, sl)
        except Exception as e:
            rows.append(f"{name}\tauto_err\t{e!r}\tref={ref}")
            continue
        try:
            xd = sol_dddd.detect_gap_position(bg, sl)
        except Exception as e:
            xd = -99999
            rows.append(f"{name}\tdddd_err\t{e!r}\tref={ref}\tauto={xa}")
        try:
            xo = sol_cv.detect_gap_position(bg, sl)
        except Exception as e:
            xo = -99999

        n += 1
        if abs(int(xa) - int(ref)) <= tol:
            hit_a += 1
        if abs(int(xd) - int(ref)) <= tol:
            hit_d += 1
        if abs(int(xo) - int(ref)) <= tol:
            hit_o += 1
        pick = getattr(sol_auto, "_gap_ensemble_pick", None) or ""
        rows.append(
            f"{name}\tref={ref}\tauto={xa}\tdddd={xd}\topencv={xo}\tda={xa - ref}\tdd={xd - ref}\tdo={xo - ref}\tensemble={pick}"
        )

    ra = hit_a / n if n else 0.0
    rd_ = hit_d / n if n else 0.0
    ro = hit_o / n if n else 0.0
    imp_vs_dddd = (ra - rd_) / max(rd_, 1e-9) * 100.0
    imp_vs_cv = (ra - ro) / max(ro, 1e-9) * 100.0

    print(f"samples\t{n}\ttol_px\t{tol}")
    print(f"hit_rate_auto\t{ra:.4f}\t({hit_a}/{n})")
    print(f"hit_rate_ddddocr_only\t{rd_:.4f}\t({hit_d}/{n})")
    print(f"hit_rate_opencv_only\t{ro:.4f}\t({hit_o}/{n})")
    print(f"relative_improvement_vs_dddd_pct\t{imp_vs_dddd:.2f}%")
    print(f"relative_improvement_vs_opencv_pct\t{imp_vs_cv:.2f}%")
    print("--- detail ---")
    for line in rows:
        print(line)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
