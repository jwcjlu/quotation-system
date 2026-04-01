"""
独立测试 ddddocr 滑块匹配（写法对齐官方 README「滑块验证码处理」一节）。

- 文档：<https://github.com/sml2h3/ddddocr/blob/master/README.md#滑块验证码处理>
  镜像：<https://gitcode.com/gh_mirrors/dd/ddddocr/blob/master/README.md#%E2%85%B3-%E6%BB%91%E5%9D%97%E6%A3%80%E6%B5%8B>
- 初始化：``DdddOcr(det=False, ocr=False)``；匹配：``slide_match(target_bytes, background_bytes)``
  默认边缘匹配；无透明底滑块用 ``simple_target=True``。
- 勿命名为 ddddocr.py（会遮蔽 pip 包）。
- 网上随意找的「滑块 URL + 背景 URL」往往不是同一次验证码，匹配会失败或落在 (0,0)。
- ``--slide-engine shim`` 与 ``scripts/icgoo_crawler_dev.py`` 纯 OpenCV 回退一致，便于对比。
"""
from __future__ import annotations

import argparse
import math
import os
import sys
import urllib.request
from io import BytesIO

import cv2
import ddddocr
import numpy as np
from PIL import Image

# 默认同目录或仓库根目录下的背景图（用于合成自测）
_SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
_REPO_ROOT = os.path.abspath(os.path.join(_SCRIPT_DIR, ".."))


def _fetch(url: str) -> bytes:
    req = urllib.request.Request(
        url,
        headers={"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120.0.0.0"},
    )
    with urllib.request.urlopen(req, timeout=30) as r:
        return r.read()


def _pil_to_rgb_composite_rgba(
    im: Image.Image, fill_rgb: tuple[int, int, int] = (245, 245, 245)
) -> Image.Image:
    """与 icgoo_crawler_dev 一致：易盾滑块图常为 RGBA，直接转 RGB 会把透明区变成黑/杂色。"""
    if im.mode == "RGBA":
        canvas = Image.new("RGB", im.size, fill_rgb)
        canvas.paste(im, mask=im.split()[3])
        return canvas
    return im.convert("RGB")


def _bytes_bgr(data: bytes) -> np.ndarray | None:
    """与 icgoo_crawler_dev._slide_match 输入解码一致（PIL 合成 RGBA → BGR），非裸 cv2.imdecode。"""
    try:
        im = Image.open(BytesIO(data))
    except Exception:
        return None
    rgb = np.asarray(_pil_to_rgb_composite_rgba(im), dtype=np.uint8)
    return cv2.cvtColor(rgb, cv2.COLOR_RGB2BGR)


def _read_bgr_path(path: str) -> np.ndarray | None:
    """与 icgoo_crawler_dev._cv2_read_bgr 一致。"""
    try:
        im = Image.open(path)
    except OSError:
        return None
    rgb = np.asarray(_pil_to_rgb_composite_rgba(im), dtype=np.uint8)
    return cv2.cvtColor(rgb, cv2.COLOR_RGB2BGR)


def _normalize_captcha_png_bytes(data: bytes) -> bytes:
    """
    先按爬虫同款管线解码再统一为 PNG 字节，使传入 slide_match 的像素与 icgoo 落盘再读取一致。
    """
    bgr = _bytes_bgr(data)
    if bgr is None:
        return data
    ok, buf = cv2.imencode(".png", bgr)
    return buf.tobytes() if ok else data


def _slide_match_pure_cv_like_dddd_engine(
    target_img: bytes,
    background_img: bytes,
    *,
    simple_target: bool = False,
) -> dict:
    """
    与 icgoo_crawler_dev 内实现一致：RGB→灰度，simple_target 时直接模板匹配，否则 Canny(50,150)。
    """
    def _rgb_u8(raw: bytes):
        im = Image.open(BytesIO(raw))
        return np.asarray(_pil_to_rgb_composite_rgba(im), dtype=np.uint8)

    target = _rgb_u8(target_img)
    background = _rgb_u8(background_img)
    target_gray = cv2.cvtColor(target, cv2.COLOR_RGB2GRAY)
    background_gray = cv2.cvtColor(background, cv2.COLOR_RGB2GRAY)
    th, tw = target_gray.shape
    if simple_target:
        m = cv2.matchTemplate(background_gray, target_gray, cv2.TM_CCOEFF_NORMED)
        _, max_val, _, max_loc = cv2.minMaxLoc(m)
    else:
        te = cv2.Canny(target_gray, 50, 150)
        be = cv2.Canny(background_gray, 50, 150)
        m = cv2.matchTemplate(be, te, cv2.TM_CCOEFF_NORMED)
        _, max_val, _, max_loc = cv2.minMaxLoc(m)
    center_x = int(max_loc[0] + tw // 2)
    center_y = int(max_loc[1] + th // 2)
    return {
        "target": [center_x, center_y],
        "target_x": center_x,
        "target_y": center_y,
        "confidence": float(max_val),
    }


class _DdddSlideEngineShim:
    """与 icgoo_crawler_dev._DdddSlideEngineShim 一致，供 --slide-engine shim。"""

    def slide_match(
        self,
        target_img,
        background_img,
        simple_target: bool = False,
    ) -> dict:
        if not isinstance(target_img, (bytes, bytearray)):
            raise TypeError("target_img 须为 PNG/JPEG 字节")
        if not isinstance(background_img, (bytes, bytearray)):
            raise TypeError("background_img 须为 PNG/JPEG 字节")
        return _slide_match_pure_cv_like_dddd_engine(
            bytes(target_img),
            bytes(background_img),
            simple_target=simple_target,
        )


def _encode_png_bgr(bgr: np.ndarray) -> bytes:
    ok, buf = cv2.imencode(".png", bgr)
    if not ok:
        raise RuntimeError("imencode png failed")
    return buf.tobytes()


def pick_crop_window_max_laplacian(
    bg_bgr: np.ndarray, tw: int, th: int, stride: int = 5
) -> tuple[int, int]:
    """仅按拉普拉斯方差选窗（易被整片黄瓜等重复纹理误导，作备用策略）。"""
    gray = cv2.cvtColor(bg_bgr, cv2.COLOR_BGR2GRAY)
    h, w = gray.shape[:2]
    best_v = -1.0
    best_xy = (0, 0)
    for y in range(0, max(1, h - th + 1), stride):
        for x in range(0, max(1, w - tw + 1), stride):
            roi = gray[y : y + th, x : x + tw]
            v = float(cv2.Laplacian(roi, cv2.CV_64F).var())
            if v > best_v:
                best_v = v
                best_xy = (x, y)
    return best_xy


def pick_crop_window_unique_match(
    bg_bgr: np.ndarray,
    tw: int,
    th: int,
    stride: int = 6,
    top_k: int = 50,
) -> tuple[int, int]:
    """
    合成自测选窗：先按拉普拉斯方差取 Top-K 候选，再对每块在整图上 matchTemplate，
    用「主峰 /（邻域外的次峰）」衡量唯一性；黄瓜等自相似块次峰仍很高，会被压低。
    """
    gray = cv2.cvtColor(bg_bgr, cv2.COLOR_BGR2GRAY)
    h, w = gray.shape[:2]
    candidates: list[tuple[float, int, int]] = []
    for y in range(0, max(1, h - th + 1), stride):
        for x in range(0, max(1, w - tw + 1), stride):
            roi = gray[y : y + th, x : x + tw]
            lap = float(cv2.Laplacian(roi, cv2.CV_64F).var())
            candidates.append((lap, x, y))
    if not candidates:
        return (0, 0)
    candidates.sort(key=lambda t: t[0], reverse=True)
    margin = max(4, min(tw, th) // 6)
    best_score = -1.0
    best_xy = (candidates[0][1], candidates[0][2])
    for lap, x, y in candidates[:top_k]:
        if lap < 1e-6:
            break
        tpl = gray[y : y + th, x : x + tw]
        res = cv2.matchTemplate(gray, tpl, cv2.TM_CCOEFF_NORMED)
        hr, wr = res.shape[:2]
        if y >= hr or x >= wr:
            continue
        first = float(res[y, x])
        masked = res.copy()
        y0, y1 = max(0, y - margin), min(hr, y + margin + 1)
        x0, x1 = max(0, x - margin), min(wr, x + margin + 1)
        masked[y0:y1, x0:x1] = -1.0
        second = float(np.max(masked))
        ratio = first / (second + 1e-6)
        score = lap * ratio
        if score > best_score:
            best_score = score
            best_xy = (x, y)
    return best_xy


def make_synthetic_pair_from_bg(
    bg_bgr: np.ndarray,
    tw: int = 72,
    th: int = 72,
    x0: int | None = None,
    y0: int | None = None,
    crop_strategy: str = "unique",
) -> tuple[bytes, bytes, tuple[int, int, int, int]]:
    """
    从大图切一块当作「滑块图」，整图作背景，保证是同一张图里的内容，匹配应落在切口附近。
    未指定 (x0,y0) 时默认用 unique：拉普拉斯 Top-K + matchTemplate 唯一性，减轻黄瓜等重复纹理误匹配。
    crop_strategy: "unique" | "laplacian"。
    返回 (target_bytes, background_bytes, (x0, y0, tw, th))。
    """
    h, w = bg_bgr.shape[:2]
    tw = max(24, min(tw, w // 3))
    th = max(24, min(th, h - 4))
    if x0 is None or y0 is None:
        if crop_strategy == "laplacian":
            px, py = pick_crop_window_max_laplacian(bg_bgr, tw, th)
        else:
            px, py = pick_crop_window_unique_match(bg_bgr, tw, th)
        x0 = px if x0 is None else x0
        y0 = py if y0 is None else y0
    x0 = max(0, min(int(x0), w - tw))
    y0 = max(0, min(int(y0), h - th))
    tpl = bg_bgr[y0 : y0 + th, x0 : x0 + tw].copy()
    # 与 icgoo 调试图一致：滑块与背景均用 PNG，避免 JPG 与爬虫落盘格式不一致导致分数偏差
    return _encode_png_bgr(tpl), _encode_png_bgr(bg_bgr), (x0, y0, tw, th)


def target_xy_from_result(res: dict) -> tuple[int, int] | None:
    """
    从 slide_match 返回中取用于绘图/比对的中心点。
    README 示例中 target 可能为 [x1,y1,x2,y2]；新版常见 [cx,cy] 或 target_x/target_y。
    """
    t = res.get("target") or []
    if len(t) >= 4:
        x1, y1, x2, y2 = int(t[0]), int(t[1]), int(t[2]), int(t[3])
        return ((x1 + x2) // 2, (y1 + y2) // 2)
    if len(t) >= 2:
        return (int(t[0]), int(t[1]))
    tx, ty = res.get("target_x"), res.get("target_y")
    if tx is not None and ty is not None:
        return (int(tx), int(ty))
    return None


def run_match(
    slide,
    target_bytes: bytes,
    bg_bytes: bytes,
    simple_modes: list[bool] | None = None,
) -> list[tuple[bool, dict]]:
    # README：先默认边缘匹配（simple_target=False），再无透明底时用 True；与 icgoo 爬虫顺序一致
    modes = simple_modes if simple_modes is not None else [False, True]
    out: list[tuple[bool, dict]] = []
    for simple in modes:
        try:
            # 与 README 一致：slide.slide_match(target_bytes, background_bytes[, simple_target=...])
            r = slide.slide_match(target_bytes, bg_bytes, simple_target=simple)
            out.append((simple, r))
        except Exception as e:
            out.append((simple, {"error": str(e)}))
    return out


def divergence_hint(results: list[tuple[bool, dict]], bg_shape: tuple[int, int]) -> str | None:
    """若 simple=True / False 坐标相差很大，返回中文提示（勿只信高置信度）。"""
    by_simple: dict[bool, tuple[tuple[int, int], float | None]] = {}
    for simple, res in results:
        if "error" in res:
            continue
        p = target_xy_from_result(res)
        if p is None:
            continue
        c = res.get("confidence")
        cf = float(c) if c is not None else None
        by_simple[simple] = (p, cf)
    if True not in by_simple or False not in by_simple:
        return None
    (p0, _c0) = by_simple[True]
    (p1, c1) = by_simple[False]
    d = math.hypot(p0[0] - p1[0], p0[1] - p1[1])
    h, w = bg_shape[0], bg_shape[1]
    thresh = max(48.0, 0.08 * math.hypot(w, h))
    if d < thresh:
        return None
    c1s = f"{c1:.3f}" if c1 is not None else "n/a"
    return (
        f"两路相差≈{d:.0f}px：simple=True 置信度常偏高但仍可能错；"
        f"缺口类场景可优先试 simple_target=False（simple=False conf≈{c1s}）。"
    )


def draw_and_save(
    bg_bytes: bytes,
    results: list[tuple[bool, dict]],
    out_path: str,
    expect_box: tuple[int, int, int, int] | None,
) -> None:
    bg = _bytes_bgr(bg_bytes)
    if bg is None:
        print("无法解码背景字节为图像", file=sys.stderr)
        return
    if expect_box:
        ex, ey, etw, eth = expect_box
        cv2.rectangle(bg, (ex, ey), (ex + etw - 1, ey + eth - 1), (0, 165, 255), 2)
        cv2.putText(
            bg,
            "expected crop",
            (ex, max(12, ey - 4)),
            cv2.FONT_HERSHEY_SIMPLEX,
            0.5,
            (0, 165, 255),
            1,
            cv2.LINE_AA,
        )
    for simple, res in results:
        if "error" in res:
            continue
        targ = res.get("target") or []
        conf = res.get("confidence")
        label = f"simple={simple} conf={conf}"
        if len(targ) >= 4:
            # README：x1,y1,x2,y2 画矩形
            x1, y1, x2, y2 = int(targ[0]), int(targ[1]), int(targ[2]), int(targ[3])
            color = (0, 255, 0) if simple else (0, 0, 255)
            cv2.rectangle(bg, (x1, y1), (x2, y2), color, 2)
            cx, cy = (x1 + x2) // 2, (y1 + y2) // 2
            cv2.putText(
                bg,
                label[:48],
                (max(5, cx - 100), min(bg.shape[0] - 8, cy + 22)),
                cv2.FONT_HERSHEY_SIMPLEX,
                0.45,
                (0, 200, 0) if simple else (200, 0, 200),
                1,
                cv2.LINE_AA,
            )
        elif len(targ) >= 2:
            cx, cy = int(targ[0]), int(targ[1])
            cv2.circle(bg, (cx, cy), 10, (0, 255, 0) if simple else (255, 0, 0), 2)
            cv2.putText(
                bg,
                label[:48],
                (max(5, cx - 100), min(bg.shape[0] - 8, cy + 22)),
                cv2.FONT_HERSHEY_SIMPLEX,
                0.45,
                (0, 200, 0) if simple else (200, 0, 200),
                1,
                cv2.LINE_AA,
            )
        elif res.get("target_x") is not None and res.get("target_y") is not None:
            cv2.circle(
                bg,
                (int(res["target_x"]), int(res["target_y"])),
                10,
                (0, 255, 255),
                2,
            )
    hint = divergence_hint(results, bg.shape)
    if hint:
        y0 = max(24, bg.shape[0] - 36)
        for i, line in enumerate(hint.split("；")):
            if not line.strip():
                continue
            cv2.putText(
                bg,
                line.strip()[:70],
                (8, y0 + i * 18),
                cv2.FONT_HERSHEY_SIMPLEX,
                0.42,
                (0, 255, 255),
                1,
                cv2.LINE_AA,
            )
    cv2.imwrite(out_path, bg)
    print(f"已写出可视化: {out_path}")


def main() -> None:
    p = argparse.ArgumentParser(
        description="测试 ddddocr.slide_match（对齐 README 滑块验证码处理）"
    )
    p.add_argument(
        "--mode",
        choices=("synthetic", "local", "url"),
        default="synthetic",
        help="synthetic=从本地背景图切一块自测（推荐）；local=指定同一次验证码的滑块+背景文件；url=用内置演示 URL（易非同会话则对不上）",
    )
    p.add_argument("--bg", default="", help="背景图路径（synthetic/local）")
    p.add_argument("--target", default="", help="滑块小图路径（仅 local）")
    p.add_argument("--tw", type=int, default=72, help="synthetic：滑块模板宽（像素）")
    p.add_argument("--th", type=int, default=72, help="synthetic：滑块模板高（像素）")
    p.add_argument(
        "--crop-strategy",
        choices=("unique", "laplacian"),
        default="unique",
        help="synthetic 自动选窗：unique=拉普拉斯Top-K+全图匹配唯一性（默认，减轻黄瓜等重复纹理）；laplacian=仅最大拉普拉斯",
    )
    p.add_argument(
        "--crop-x0",
        type=int,
        default=None,
        help="synthetic：裁剪左上角 x（未指定时由 --crop-strategy 自动选窗）",
    )
    p.add_argument(
        "--crop-y0",
        type=int,
        default=None,
        help="synthetic：裁剪左上角 y",
    )
    p.add_argument("--out", default="slide_result.jpg", help="输出标注图路径")
    p.add_argument("--no-show", action="store_true", help="不弹 matplotlib（默认已不用 show）")
    p.add_argument(
        "--simple-target",
        choices=("both", "true", "false"),
        default="both",
        help="slide_match 的 simple_target：both=两路都跑并画在图上（默认）；true/false=只跑一路",
    )
    p.add_argument(
        "--slide-engine",
        choices=("ddddocr", "shim"),
        default="ddddocr",
        help="ddddocr=官方 DdddOcr.slide_match（默认）；shim=与 icgoo_crawler_dev 相同的纯 OpenCV SlideEngine（无 onnx）",
    )
    args = p.parse_args()

    if args.slide_engine == "shim":
        slide = _DdddSlideEngineShim()
    else:
        # README：slide = ddddocr.DdddOcr(det=False, ocr=False)；show_ad=False 减少控制台广告
        try:
            slide = ddddocr.DdddOcr(det=False, ocr=False, show_ad=False)
        except TypeError:
            slide = ddddocr.DdddOcr(det=False, ocr=False)

    expect_center: tuple[int, int] | None = None
    box: tuple[int, int, int, int] | None = None

    if args.mode == "url":
        # 演示 URL 多为随意摘录，不保证成对，匹配常失败
        t_url = "https://necaptcha.nosdn.127.net/f33e83c2a5564ffea41beb0d288b5751@2x.png"
        b_url = "https://necaptcha.nosdn.127.net/1987dfff868d4f9395683f3e0cba4c42@2x.jpg"
        #t_url = "https://necaptcha.nosdn.127.net/01d139591d6349daaafda1c8d846eb7c@2x.png"
        #b_url= "https://necaptcha.nosdn.127.net/5966c727ec484494baf6e0e7172a4462@2x.jpg"
        print("警告: url 模式下两张图通常不是同一次验证码，匹配可能为 (0,0) 或乱位。", file=sys.stderr)
        target_bytes = _normalize_captcha_png_bytes(_fetch(t_url))
        background_bytes = _normalize_captcha_png_bytes(_fetch(b_url))
    elif args.mode == "local":
        tp = args.target or ""
        bp = args.bg or os.path.join(_REPO_ROOT, "background.png")
        if not tp or not os.path.isfile(tp):
            print("local 模式需要 --target 滑块图 且 --bg 背景图（同一次验证码导出）", file=sys.stderr)
            sys.exit(2)
        with open(tp, "rb") as f:
            target_bytes = _normalize_captcha_png_bytes(f.read())
        with open(bp, "rb") as f:
            background_bytes = _normalize_captcha_png_bytes(f.read())
    else:
        bp = args.bg or os.path.join(_REPO_ROOT, "background.png")
        if not os.path.isfile(bp):
            print(f"未找到背景图: {bp} ，请用 --bg 指定，或改用 --mode local --target ... --bg ...", file=sys.stderr)
            sys.exit(2)
        bg = _read_bgr_path(bp)
        if bg is None:
            print(f"无法读取: {bp}", file=sys.stderr)
            sys.exit(2)
        target_bytes, background_bytes, box = make_synthetic_pair_from_bg(
            bg,
            tw=args.tw,
            th=args.th,
            x0=args.crop_x0,
            y0=args.crop_y0,
            crop_strategy=args.crop_strategy,
        )
        x0, y0, tw, th = box
        expect_center = (x0 + tw // 2, y0 + th // 2)
        auto_crop = args.crop_x0 is None and args.crop_y0 is None
        if auto_crop:
            how = (
                "拉普拉斯Top-K + 匹配唯一性选窗"
                if args.crop_strategy == "unique"
                else "拉普拉斯方差自动选窗"
            )
        else:
            how = "手动/半自动裁剪坐标"
        print(
            f"合成自测: {how}，切 ({x0},{y0}) 大小 {tw}x{th}，期望中心约 {expect_center}",
            flush=True,
        )

    if args.simple_target == "both":
        simple_modes = None
    elif args.simple_target == "true":
        simple_modes = [True]
    else:
        simple_modes = [False]

    results = run_match(
        slide, target_bytes, background_bytes, simple_modes=simple_modes
    )
    for simple, res in results:
        print(f"simple_target={simple!s:5} -> {res}")

    bg_decoded = _bytes_bgr(background_bytes)
    if bg_decoded is not None and len(results) >= 2:
        hint = divergence_hint(results, bg_decoded.shape)
        if hint:
            print(hint, file=sys.stderr)

    best_conf = -1.0
    best_res = None
    for simple, res in results:
        if "error" in res:
            continue
        c = res.get("confidence")
        if c is not None and float(c) > best_conf:
            best_conf = float(c)
            best_res = res

    if best_res:
        t = best_res.get("target") or []
        if len(t) >= 2 and int(t[0]) == 0 and int(t[1]) == 0 and best_conf < 0.15:
            print(
                "\n提示: 结果接近 (0,0) 且置信度很低，多半是「滑块图与背景图不是一对」。"
                "请用 --mode synthetic（默认）或同一次验证码导出的 local 文件。",
                file=sys.stderr,
            )

    draw_and_save(background_bytes, results, args.out, box)

    if args.mode == "synthetic" and expect_center and best_res and box:
        _x0, _y0, tw, th = box
        t = best_res.get("target") or []
        if len(t) >= 2:
            dx = abs(int(t[0]) - expect_center[0])
            dy = abs(int(t[1]) - expect_center[1])
            if dx + dy > (tw + th) // 3:
                print(
                    f"与期望中心偏差较大 (Δ≈{dx},{dy})："
                    "可能是模板在背景中多处自相似；可换 --bg、手动指定裁剪坐标，或检查 ddddocr/onnx。",
                    file=sys.stderr,
                )


if __name__ == "__main__":
    main()
