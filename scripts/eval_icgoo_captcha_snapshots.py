"""
离线对比 SliderCaptchaSolver 在快照图上的缺口 raw_x：auto（含 ensemble）vs 仅 ddddocr。

参考标签（与 ``detect_gap_position`` 一致，图内像素、不含 gap_offset）：
  - 优先 ``raw_x_detected``；
  - 否则 ``raw_x_px - gap_offset_image_px``（旧快照仅有 raw_x_px 时，避免与手工偏移混淆）。

**重要**：多轮验证码会反复覆盖 ``00_background.png``，仅 ``00_detect.txt`` 记录的是第 1 轮识别结果，
二者常错位。评估时：
  - 若存在 ``round_NN_background.png``（爬虫每轮已复制），则按轮用 ``round_NN_detect.txt`` 配对；
  - 否则仅当**不存在** ``round_02_detect.txt`` 时用 ``00_*.png`` + ``00_detect.txt``。

用法:
  python scripts/eval_icgoo_captcha_snapshots.py
  python scripts/eval_icgoo_captcha_snapshots.py --root scripts/icgoo_captcha_snapshots --tol 5
"""
from __future__ import annotations

import argparse
import os
import re
import sys

_SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
if _SCRIPT_DIR not in sys.path:
    sys.path.insert(0, _SCRIPT_DIR)

from icgoo_crawler_dev import SliderCaptchaSolver  # noqa: E402

_ROUND_DET_RE = re.compile(r"^round_(\d+)_detect\.txt$", re.I)


def _parse_detect(path: str) -> tuple[int | None, int | None, int]:
    """返回 (raw_x_detected, raw_x_px, gap_offset_image_px)。"""
    rd: int | None = None
    rx: int | None = None
    gap_off = 0
    try:
        with open(path, encoding="utf-8") as f:
            for line in f:
                line = line.strip()
                if line.startswith("raw_x_detected\t"):
                    try:
                        v = line.split("\t", 1)[1].strip()
                        if v:
                            rd = int(v)
                    except ValueError:
                        pass
                elif line.startswith("raw_x_px\t"):
                    try:
                        rx = int(line.split("\t", 1)[1].strip())
                    except ValueError:
                        pass
                elif line.startswith("gap_offset_image_px\t"):
                    try:
                        gap_off = int(line.split("\t", 1)[1].strip())
                    except ValueError:
                        gap_off = 0
    except OSError:
        pass
    return rd, rx, gap_off


def _reference_x(rd: int | None, rx: int | None, gap_off: int) -> int | None:
    if rd is not None:
        return rd
    if rx is not None:
        return int(rx) - int(gap_off)
    return None


def _list_eval_samples(run_dir: str) -> list[tuple[str, str, str, int]]:
    """
    返回 [(det_path, bg_path, slider_path, round_id), ...]。
    det_path 用于解析参考 x；round_id 仅用于命名配对。
    """
    out: list[tuple[str, str, str, int]] = []
    p00_det = os.path.join(run_dir, "00_detect.txt")
    p00_bg = os.path.join(run_dir, "00_background.png")
    p00_sl = os.path.join(run_dir, "00_slider.png")
    has_r2_det = os.path.isfile(os.path.join(run_dir, "round_02_detect.txt"))

    def pair_for_round(rid: int) -> tuple[str, str] | None:
        rb = os.path.join(run_dir, f"round_{rid:02d}_background.png")
        rs = os.path.join(run_dir, f"round_{rid:02d}_slider.png")
        if os.path.isfile(rb) and os.path.isfile(rs):
            return rb, rs
        return None

    # 带 per-round 图：每一轮 detect 与 round_NN 图对齐（新爬虫落盘）
    for name in sorted(os.listdir(run_dir)):
        m = _ROUND_DET_RE.match(name)
        if not m:
            continue
        rid = int(m.group(1))
        det_p = os.path.join(run_dir, name)
        pr = pair_for_round(rid)
        if pr is None:
            continue
        bg, sl = pr
        out.append((det_p, bg, sl, rid))

    if os.path.isfile(p00_det):
        pr1 = pair_for_round(1)
        if pr1 is not None:
            out.append((p00_det, pr1[0], pr1[1], 1))
        elif not has_r2_det and os.path.isfile(p00_bg) and os.path.isfile(p00_sl):
            # 仅单轮会话：00 图与 00_detect 一致（旧数据）
            out.append((p00_det, p00_bg, p00_sl, 1))

    # 同一 run 内按 det 路径去重（round_01 与 00_detect 可能同内容时保留一条）
    seen: set[tuple[str, str, str]] = set()
    dedup: list[tuple[str, str, str, int]] = []
    for det_p, bg, sl, rid in out:
        key = (os.path.normpath(det_p), os.path.normpath(bg), os.path.normpath(sl))
        if key in seen:
            continue
        seen.add(key)
        dedup.append((det_p, bg, sl, rid))
    return dedup


def collect_samples(root: str) -> list[tuple[str, str, str, str, int]]:
    """(run_name, det_path, bg, sl, ref_x)"""
    rows: list[tuple[str, str, str, str, int]] = []
    for name in sorted(os.listdir(root)):
        if not name.startswith("run_"):
            continue
        d = os.path.join(root, name)
        if not os.path.isdir(d):
            continue
        for det_p, bg, sl, _rid in _list_eval_samples(d):
            rd, rx, go = _parse_detect(det_p)
            ref = _reference_x(rd, rx, go)
            if ref is None:
                continue
            rows.append((name, det_p, bg, sl, ref))
    return rows


def main() -> int:
    ap = argparse.ArgumentParser(description="快照目录下 auto vs ddddocr 缺口 raw_x 对比（图/标签对齐）")
    ap.add_argument(
        "--root",
        default=os.path.join(_SCRIPT_DIR, "icgoo_captcha_snapshots"),
        help="含 run_* 子目录的根路径",
    )
    ap.add_argument("--tol", type=int, default=5, help="与参考 x 相差不超过此像素算命中")
    args = ap.parse_args()
    root = os.path.abspath(args.root)
    tol = max(0, int(args.tol))

    runs = collect_samples(root)
    if not runs:
        print(
            f"未找到可用快照（需 detect 与图对齐："
            f"round_NN_detect + round_NN_*.png，或单轮时 00_detect + 00_*.png）: {root}"
        )
        return 1

    sol_auto = SliderCaptchaSolver(offline_gap_eval=True, slider_gap_backend="auto")
    sol_dddd = SliderCaptchaSolver(offline_gap_eval=True, slider_gap_backend="ddddocr")
    sol_cv = SliderCaptchaSolver(offline_gap_eval=True, slider_gap_backend="opencv")

    hit_a = hit_d = hit_o = n = 0
    rows_out: list[str] = []
    for run_name, det_p, bg, sl, ref in runs:
        tag = os.path.basename(det_p)
        try:
            xa = sol_auto.detect_gap_position(bg, sl)
        except Exception as e:
            rows_out.append(f"{run_name}\t{tag}\tauto_err\t{e!r}\tref={ref}")
            continue
        try:
            xd = sol_dddd.detect_gap_position(bg, sl)
        except Exception as e:
            xd = -99999
            rows_out.append(f"{run_name}\t{tag}\tdddd_err\t{e!r}\tref={ref}\tauto={xa}")
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
        rows_out.append(
            f"{run_name}\t{tag}\tref={ref}\tauto={xa}\tdddd={xd}\topencv={xo}\t"
            f"da={xa - ref}\tdd={xd - ref}\tdo={xo - ref}\tensemble={pick}"
        )

    ra = hit_a / n if n else 0.0
    rd_ = hit_d / n if n else 0.0
    ro = hit_o / n if n else 0.0
    imp_vs_dddd = (ra - rd_) / max(rd_, 1e-9) * 100.0
    imp_vs_cv = (ra - ro) / max(ro, 1e-9) * 100.0

    print(f"samples\t{n}\ttol_px\t{tol}\t(root={root})")
    print(f"hit_rate_auto\t{ra:.4f}\t({hit_a}/{n})")
    print(f"hit_rate_ddddocr_only\t{rd_:.4f}\t({hit_d}/{n})")
    print(f"hit_rate_opencv_only\t{ro:.4f}\t({hit_o}/{n})")
    print(f"relative_improvement_vs_dddd_pct\t{imp_vs_dddd:.2f}%")
    print(f"relative_improvement_vs_opencv_pct\t{imp_vs_cv:.2f}%")
    print("--- detail ---")
    for line in rows_out:
        print(line)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
