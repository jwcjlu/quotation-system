"""
阿里云拼图 **背景图** 缺口位置的实验性 OpenCV 探测（未接入 icgoo 主流程）。

主站识别请用 ``icgoo_yidun_solver``（ddddocr.slide_match + OpenCV 模板 + 阿里云预处理）。

本脚本用阈值 + 轮廓做快速肉眼看图；ICGOO 上缺口往往不是「高亮白块」，
固定 threshold=200 容易误检 —— 请用 ``--thresh`` / ``--invert`` 调参，或改用
``scripts/replay_icgoo_gap_detect.py`` 复现正式算法。
"""
from __future__ import annotations

import argparse
import io
import sys
from pathlib import Path

import cv2
import numpy as np
import requests
from PIL import Image

_SCRIPT_DIR = Path(__file__).resolve().parent
_DEFAULT_OUT_DIR = str(_SCRIPT_DIR / "aliyun_yolo_out")


def _default_input_background() -> str | None:
    """``scripts/icgoo_captcha_snapshots/**/00_background.png`` 中修改时间最新的一条。"""
    snap = _SCRIPT_DIR / "icgoo_captcha_snapshots"
    if not snap.is_dir():
        return None
    found = list(snap.glob("**/00_background.png"))
    if not found:
        return None
    best = max(found, key=lambda p: p.stat().st_mtime)
    return str(best)


def _load_image(path: str | None, url: str | None) -> np.ndarray:
    if path:
        p = Path(path)
        if not p.is_file():
            raise FileNotFoundError(f"找不到文件: {p.resolve()}")
        img = cv2.imread(str(p), cv2.IMREAD_COLOR)
        if img is None:
            raise ValueError(f"无法解码图像: {p}")
        return img
    if url:
        print("下载:", url, flush=True)
        r = requests.get(url, timeout=30)
        r.raise_for_status()
        img_pil = Image.open(io.BytesIO(r.content)).convert("RGB")
        return cv2.cvtColor(np.array(img_pil), cv2.COLOR_RGB2BGR)
    raise ValueError("请指定 --input 或 --url")


def detect_gap_bbox(
    img: np.ndarray,
    *,
    thresh: int = 200,
    invert: bool = False,
    blur_ksize: int = 5,
    morph_ksize: int = 5,
) -> tuple[int, int, int, int] | None:
    gray = cv2.cvtColor(img, cv2.COLOR_BGR2GRAY)
    k = max(3, blur_ksize | 1)
    blurred = cv2.GaussianBlur(gray, (k, k), 0)
    _, binary = cv2.threshold(blurred, thresh, 255, cv2.THRESH_BINARY)
    if invert:
        binary = cv2.bitwise_not(binary)

    mk = max(3, morph_ksize | 1)
    kernel = np.ones((mk, mk), np.uint8)
    closed = cv2.morphologyEx(binary, cv2.MORPH_CLOSE, kernel)

    contours, _ = cv2.findContours(closed, cv2.RETR_EXTERNAL, cv2.CHAIN_APPROX_SIMPLE)
    h_img, w_img = img.shape[:2]
    total_area = h_img * w_img
    min_area = total_area * 0.005
    max_area = total_area * 0.08

    candidates: list[tuple[float, tuple[int, int, int, int]]] = []
    for cnt in contours:
        area = cv2.contourArea(cnt)
        if area < min_area or area > max_area:
            continue
        x, y, w, h = cv2.boundingRect(cnt)
        aspect = w / h if h else 0.0
        if 0.5 < aspect < 3.0:
            candidates.append((area, (x, y, w, h)))

    if not candidates and contours:
        largest = max(contours, key=cv2.contourArea)
        x, y, w, h = cv2.boundingRect(largest)
        candidates.append((cv2.contourArea(largest), (x, y, w, h)))

    if not candidates:
        return None
    _, (x, y, w, h) = max(candidates, key=lambda t: t[0])
    return x, y, w, h


def main(argv: list[str] | None = None) -> int:
    p = argparse.ArgumentParser(
        description="实验性：背景图缺口轮廓探测（调试用）",
        formatter_class=argparse.ArgumentDefaultsHelpFormatter,
    )
    p.add_argument(
        "--input",
        "-i",
        default=None,
        metavar="PATH",
        help="本地背景图；省略则自动选 icgoo_captcha_snapshots 下最新的 00_background.png",
    )
    p.add_argument(
        "--url",
        "-u",
        default=None,
        metavar="URL",
        help="直接下载背景图（与 -i 二选一；都不给且无默认快照时退出码 2）",
    )
    p.add_argument("--thresh", type=int, default=200, help="固定阈值 0-255")
    p.add_argument(
        "--invert",
        action="store_true",
        default=False,
        help="二值化后反色（缺口偏暗时用）",
    )
    p.add_argument(
        "--out-dir",
        "-o",
        default=_DEFAULT_OUT_DIR,
        help="输出 debug_*.png 的目录",
    )
    p.add_argument(
        "--show",
        action="store_true",
        default=False,
        help="弹窗显示（无桌面/SSH 时不要加）",
    )
    args = p.parse_args(argv)

    img_path = (args.input or "").strip() or None
    url = (args.url or "").strip() or None
    if not img_path and not url:
        img_path = _default_input_background()
    if not img_path and not url:
        print(
            "未找到默认快照（scripts/icgoo_captcha_snapshots/**/00_background.png），请指定：\n"
            "  python scripts/aliyun_yolo.py -i path/to/00_background.png\n"
            "  python scripts/aliyun_yolo.py -u https://...\n",
            file=sys.stderr,
        )
        return 2
    if img_path:
        print("使用背景图:", img_path, flush=True)

    out_dir = Path(args.out_dir)
    out_dir.mkdir(parents=True, exist_ok=True)

    img = _load_image(img_path, url)
    cv2.imwrite(str(out_dir / "debug_input_back.png"), img)
    print("shape:", img.shape, flush=True)

    box = detect_gap_bbox(
        img,
        thresh=args.thresh,
        invert=args.invert,
    )

    gray = cv2.cvtColor(img, cv2.COLOR_BGR2GRAY)
    blurred = cv2.GaussianBlur(gray, (5, 5), 0)
    _, thresh_img = cv2.threshold(blurred, args.thresh, 255, cv2.THRESH_BINARY)
    if args.invert:
        thresh_img = cv2.bitwise_not(thresh_img)
    kernel = np.ones((5, 5), np.uint8)
    closed = cv2.morphologyEx(thresh_img, cv2.MORPH_CLOSE, kernel)
    cv2.imwrite(str(out_dir / "debug_thresh.png"), thresh_img)
    cv2.imwrite(str(out_dir / "debug_closed.png"), closed)

    result = img.copy()
    if box is not None:
        x, y, w, h = box
        print(f"bbox: x={x} y={y} w={w} h={h}  center=({x + w // 2}, {y + h // 2})")
        cv2.rectangle(result, (x, y), (x + w, y + h), (0, 0, 255), 2)
        cv2.putText(result, "gap", (x, max(0, y - 5)), cv2.FONT_HERSHEY_SIMPLEX, 0.6, (0, 0, 255), 2)
    else:
        print("未找到轮廓候选；请换 --thresh / --invert 或勿依赖本脚本（正式流程用 dddd+模板）。")

    cv2.imwrite(str(out_dir / "debug_gap_marked.png"), result)

    if args.show:
        try:
            cv2.imshow("Thresh", thresh_img)
            cv2.imshow("Closed", closed)
            cv2.imshow("Result", result)
            cv2.waitKey(0)
            cv2.destroyAllWindows()
        except cv2.error as e:
            print("无法显示窗口（无 GUI?）:", e, file=sys.stderr)

    return 0 if box is not None else 1


if __name__ == "__main__":
    raise SystemExit(main())
