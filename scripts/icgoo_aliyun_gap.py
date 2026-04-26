"""
阿里云 aliyunCaptcha 拼图与易盾视觉差异大：RGBA 阴影块、浅灰底、缺口边缘弱。
缺口/拼块图像特征约 **52×52** 自然像素（见 ``icgoo_aliyun_captcha.ALIYUN_CAPTCHA_PUZZLE_SIZE_NAT_PX``）。
**主路** dddd/OpenCV 所用 ``aliyun_prepare_match_bytes`` 默认对滑块 **Alpha 紧裁** 后再合成 RGB（与 shadow 路一致）。
本模块提供：**拉图后预处理**（按背景色合成拼图）、**多阈值 Canny 共识**（分歧大时取中位数），
及 **数据集落盘** 辅助，供离线 ``benchmark_aliyun_gap.py`` 复盘。

**标定 / 拖动链路复盘**：``append_aliyun_calibration_jsonl`` 向单文件追加一行 JSON（JSONL），
与 ``export_aliyun_dataset_sample`` 的 ``sample_*/meta.json`` 互补——后者按轮存图，前者便于
按时间轴 grep / pandas 读入分析 ``drag_scale_x``、raw_x、实拖像素与成败。

**边缘多尺度路**：``aliyun_shadow_multiscale_match_x_edges`` 在同 ROI 上对 Canny 边缘做
多尺度模板匹配，供 ensemble 标签 ``aliyun_shadow_ms_edges``；环境变量见 ``gap-position-detection.md`` §7。

不在此 import ``icgoo_yidun_solver`` 顶层，避免循环依赖；共识函数内懒加载。
"""
from __future__ import annotations

import json
import os
import shutil
import tempfile
import time
import uuid
from datetime import datetime, timezone
from io import BytesIO

import cv2
import numpy as np
from PIL import Image

# 与 icgoo_crawler_dev 调试图目录、IcgooYidunSliderSolver 共用文件名约定
ALIYUN_CALIBRATION_JSONL_BASENAME = "aliyun_calibration.jsonl"


def _pil_to_rgb_composite_rgba(im: Image.Image, fill_rgb: tuple[int, int, int]) -> Image.Image:
    if im.mode == "RGBA":
        canvas = Image.new("RGB", im.size, fill_rgb)
        canvas.paste(im, mask=im.split()[3])
        return canvas
    return im.convert("RGB")


def _mean_corner_rgb(im_rgb: Image.Image) -> tuple[int, int, int]:
    w, h = im_rgb.size
    px = im_rgb.load()
    corners = ((0, 0), (w - 1, 0), (0, h - 1), (w - 1, h - 1))
    rs, gs, bs = [], [], []
    for x, y in corners:
        r, g, b = px[x, y]
        rs.append(r)
        gs.append(g)
        bs.append(b)
    return (sum(rs) // 4, sum(gs) // 4, sum(bs) // 4)


def _rgba_tight_crop_pil(im_rgba: Image.Image, *, min_alpha: int = 12) -> Image.Image:
    """裁掉近似全透明外边，与 ``gap-position-detection.md`` 中 shadow 预处理一致。"""
    if im_rgba.mode != "RGBA":
        im_rgba = im_rgba.convert("RGBA")
    a = np.array(im_rgba.split()[3], dtype=np.uint8)
    ys, xs = np.where(a >= int(min_alpha))
    if xs.size == 0:
        return im_rgba
    x0, y0 = int(xs.min()), int(ys.min())
    x1, y1 = int(xs.max()), int(ys.max())
    return im_rgba.crop((x0, y0, x1 + 1, y1 + 1))


def _aliyun_prepare_tight_crop_enabled() -> bool:
    """主路 dddd/OpenCV 与 ``aliyun_prepare_match_bytes`` 是否对滑块做 Alpha 紧裁（与 shadow 路一致）。"""
    v = (os.environ.get("ICGOO_ALIYUN_PREPARE_TIGHT_CROP") or "1").strip().lower()
    return v not in ("0", "false", "no", "off")


def aliyun_prepare_match_bytes(slider_b: bytes, bg_b: bytes) -> tuple[bytes, bytes]:
    """
    将阿里云常见 RGBA 阴影块按**背景四角均值**合成 RGB，减少透明底默认灰与底图色差导致的模板假峰。
    背景图亦规范为 RGB。

    滑块默认先做 **Alpha 紧裁**（与 ``_aliyun_shadow_ms_gray_arrays`` / shadow 多尺度路一致），
    去掉大透明边，使 ddddocr.slide_match 与 OpenCV 主路模板接近实体拼块（约 52×52）。
    设 ``ICGOO_ALIYUN_PREPARE_TIGHT_CROP=0`` 可关闭以对照回归。
    """
    im_b = Image.open(BytesIO(bg_b))
    im_s = Image.open(BytesIO(slider_b))
    if im_b.mode == "RGBA":
        im_b_rgb = _pil_to_rgb_composite_rgba(im_b, (245, 245, 245))
    else:
        im_b_rgb = im_b.convert("RGB")
    fill = _mean_corner_rgb(im_b_rgb)
    im_s_rgba = im_s if im_s.mode == "RGBA" else im_s.convert("RGBA")
    if _aliyun_prepare_tight_crop_enabled():
        cropped = _rgba_tight_crop_pil(im_s_rgba, min_alpha=12)
        cw, ch = cropped.size
        if cw >= 4 and ch >= 4:
            im_s_rgba = cropped
    im_s_rgb = _pil_to_rgb_composite_rgba(im_s_rgba, fill_rgb=fill)
    out_s = BytesIO()
    out_b = BytesIO()
    im_s_rgb.save(out_s, format="PNG")
    im_b_rgb.save(out_b, format="PNG")
    return out_s.getvalue(), out_b.getvalue()


def _aliyun_shadow_match_scales_from_env() -> tuple[float, ...]:
    raw = (os.environ.get("ICGOO_ALIYUN_SHADOW_SCALES") or "").strip()
    if not raw:
        return (0.92, 0.96, 1.0, 1.03, 1.07)
    out: list[float] = []
    for part in raw.replace(";", ",").split(","):
        p = part.strip()
        if not p:
            continue
        try:
            f = float(p)
            if 0.75 <= f <= 1.28:
                out.append(f)
        except ValueError:
            continue
    return tuple(out) if out else (0.92, 0.96, 1.0, 1.03, 1.07)


def _aliyun_shadow_blur_ksize() -> int:
    """0 表示不做高斯模糊；否则为奇数核宽，默认 3。"""
    v = (os.environ.get("ICGOO_ALIYUN_SHADOW_BLUR") or "3").strip().lower()
    if v in ("0", "no", "false", "off"):
        return 0
    try:
        k = int(v)
        if k < 1:
            return 0
        if k % 2 == 0:
            k += 1
        return k
    except ValueError:
        return 3


def _aliyun_shadow_search_y_fracs() -> tuple[float, float]:
    """纵向搜索条带 (y0_frac, y1_frac)，相对背景高度。"""
    def _one(key: str, default: float) -> float:
        s = (os.environ.get(key) or "").strip()
        if not s:
            return default
        try:
            f = float(s)
            return max(0.0, min(1.0, f))
        except ValueError:
            return default

    y0 = _one("ICGOO_ALIYUN_SHADOW_Y0_FRAC", 0.06)
    y1 = _one("ICGOO_ALIYUN_SHADOW_Y1_FRAC", 0.995)
    if y1 <= y0 + 0.02:
        y0, y1 = 0.06, 0.995
    return y0, y1


def _aliyun_shadow_ms_gray_arrays(
    preprocessed_bg_path: str,
    original_slider_path: str,
) -> tuple[np.ndarray, np.ndarray, int, int, int, int] | None:
    """
    与 ``aliyun_shadow_multiscale_match_x`` 相同：读预处理背景 + 原始 RGBA 滑块 → 灰度数组（可选模糊），
    返回 ``(bg_gray, tpl_gray, bw, bh, tw0, th0)``；几何非法时返回 None。
    """
    im_bg = Image.open(preprocessed_bg_path).convert("RGB")
    fill = _mean_corner_rgb(im_bg)
    bg_gray = np.array(im_bg.convert("L"), dtype=np.uint8)

    im_s = Image.open(original_slider_path)
    if im_s.mode != "RGBA":
        im_s = im_s.convert("RGBA")
    im_s = _rgba_tight_crop_pil(im_s, min_alpha=12)
    im_s_rgb = _pil_to_rgb_composite_rgba(im_s, fill_rgb=fill)
    tpl_gray = np.array(im_s_rgb.convert("L"), dtype=np.uint8)
    if tpl_gray.size < 4:
        return None

    bk = _aliyun_shadow_blur_ksize()
    if bk >= 3:
        bg_gray = cv2.GaussianBlur(bg_gray, (bk, bk), 0)
        tpl_gray = cv2.GaussianBlur(tpl_gray, (bk, bk), 0)

    bh, bw = int(bg_gray.shape[0]), int(bg_gray.shape[1])
    th0, tw0 = int(tpl_gray.shape[0]), int(tpl_gray.shape[1])
    if tw0 >= bw or th0 >= bh:
        return None
    return (bg_gray, tpl_gray, bw, bh, tw0, th0)


def _aliyun_shadow_ms_canny_pairs_from_env() -> tuple[tuple[int, int], ...]:
    """
    边缘多尺度路 Canny 阈值组；借鉴常见滑块脚本（如 50/150）并加略低阈值以适配浅槽。
    可用 ``ICGOO_ALIYUN_SHADOW_MS_EDGES_CANNY=50,150;40,120`` 覆盖（分号或逗号分隔多组，每组 low,high）。
    """
    raw = (os.environ.get("ICGOO_ALIYUN_SHADOW_MS_EDGES_CANNY") or "").strip()
    if not raw:
        return ((50, 150), (40, 120), (35, 100), (65, 190))
    out: list[tuple[int, int]] = []
    for chunk in raw.replace(";", ",").split(","):
        chunk = chunk.strip()
        if not chunk:
            continue
        parts = chunk.split(":")
        if len(parts) == 2:
            try:
                lo, hi = int(parts[0].strip()), int(parts[1].strip())
                if 1 <= lo < hi <= 400:
                    out.append((lo, hi))
            except ValueError:
                continue
    # 兼容 "50,150,40,120" 两对
    if not out:
        nums: list[int] = []
        for p in raw.replace(";", " ").replace(",", " ").split():
            try:
                nums.append(int(p))
            except ValueError:
                continue
        for i in range(0, len(nums) - 1, 2):
            lo, hi = nums[i], nums[i + 1]
            if 1 <= lo < hi <= 400:
                out.append((lo, hi))
    return tuple(out) if out else ((50, 150), (40, 120), (35, 100), (65, 190))


def aliyun_shadow_multiscale_match_x(
    preprocessed_bg_path: str,
    original_slider_path: str,
) -> tuple[int, float] | None:
    """
    参照 ``scripts/gap-position-detection.md``：**预处理后的背景** + **原始 RGBA 滑块**（Alpha 紧裁、
    按背景四角色合成 RGB）→ 灰度 → 可选高斯模糊 → 在纵向 ROI 内 **多尺度** ``TM_CCOEFF_NORMED``，
    取全局最大响应的左缘 ``x`` 与 ``score``。

    仅用于阿里云辅助路径；可用 ``ICGOO_ALIYUN_SHADOW_MS=0`` 关闭。
    """
    flag = (os.environ.get("ICGOO_ALIYUN_SHADOW_MS") or "").strip().lower()
    if flag in ("0", "false", "no", "off"):
        return None
    try:
        loaded = _aliyun_shadow_ms_gray_arrays(preprocessed_bg_path, original_slider_path)
        if loaded is None:
            return None
        bg_gray, tpl_gray, bw, bh, tw0, th0 = loaded
        y0f, y1f = _aliyun_shadow_search_y_fracs()
        y0 = max(0, min(int(bh * y0f), bh - 2))
        y1 = max(y0 + 2, min(int(bh * y1f), bh))
        roi = bg_gray[y0:y1, :]
        rh, rw = int(roi.shape[0]), int(roi.shape[1])
        if rw < tw0 + 2 or rh < th0 + 2:
            return None

        scales = _aliyun_shadow_match_scales_from_env()
        best_x, best_s = 0, -1.0
        for sc in scales:
            ntw = max(2, int(round(tw0 * sc)))
            nth = max(2, int(round(th0 * sc)))
            if ntw >= rw or nth >= rh:
                continue
            interp = cv2.INTER_AREA if sc < 1.0 else cv2.INTER_LINEAR
            templ = cv2.resize(tpl_gray, (ntw, nth), interpolation=interp)
            res = cv2.matchTemplate(roi, templ, cv2.TM_CCOEFF_NORMED)
            _mn, max_v, _ml, max_loc = cv2.minMaxLoc(res)
            if float(max_v) > best_s:
                best_s = float(max_v)
                best_x = int(max_loc[0])

        if best_s < 0.12:
            return None
        return (best_x, best_s)
    except Exception:
        return None


def aliyun_shadow_multiscale_match_x_edges(
    preprocessed_bg_path: str,
    original_slider_path: str,
) -> tuple[int, float] | None:
    """
    与 ``aliyun_shadow_multiscale_match_x`` 同图与同 ROI，但在 **Canny 边缘图** 上做多尺度
    ``TM_CCOEFF_NORMED``（与常见开源阿里云滑块脚本思路一致：边缘 + 模板匹配），作为独立 ensemble 候选。

    依赖与灰度多尺度路相同的预处理；若 ``ICGOO_ALIYUN_SHADOW_MS=0`` 则本路不跑。
    另可用 ``ICGOO_ALIYUN_SHADOW_MS_EDGES=0`` 单独关闭边缘路。
    """
    ms_off = (os.environ.get("ICGOO_ALIYUN_SHADOW_MS") or "").strip().lower()
    if ms_off in ("0", "false", "no", "off"):
        return None
    ed = (os.environ.get("ICGOO_ALIYUN_SHADOW_MS_EDGES") or "").strip().lower()
    if ed in ("0", "false", "no", "off"):
        return None
    try:
        loaded = _aliyun_shadow_ms_gray_arrays(preprocessed_bg_path, original_slider_path)
        if loaded is None:
            return None
        bg_gray, tpl_gray, _bw, bh, tw0, th0 = loaded
        y0f, y1f = _aliyun_shadow_search_y_fracs()
        y0 = max(0, min(int(bh * y0f), bh - 2))
        y1 = max(y0 + 2, min(int(bh * y1f), bh))
        roi_gray = bg_gray[y0:y1, :]
        rh, rw = int(roi_gray.shape[0]), int(roi_gray.shape[1])
        if rw < tw0 + 2 or rh < th0 + 2:
            return None

        scales = _aliyun_shadow_match_scales_from_env()
        pairs = _aliyun_shadow_ms_canny_pairs_from_env()
        best_x, best_s = 0, -1.0
        for sc in scales:
            ntw = max(2, int(round(tw0 * sc)))
            nth = max(2, int(round(th0 * sc)))
            if ntw >= rw or nth >= rh:
                continue
            interp = cv2.INTER_AREA if sc < 1.0 else cv2.INTER_LINEAR
            templ_gray = cv2.resize(tpl_gray, (ntw, nth), interpolation=interp)
            for cl, ch in pairs:
                roi_e = cv2.Canny(roi_gray, int(cl), int(ch))
                te = cv2.Canny(templ_gray, int(cl), int(ch))
                if int(cv2.countNonZero(te)) < 8:
                    continue
                res = cv2.matchTemplate(roi_e, te, cv2.TM_CCOEFF_NORMED)
                _mn, max_v, _ml, max_loc = cv2.minMaxLoc(res)
                if float(max_v) > best_s:
                    best_s = float(max_v)
                    best_x = int(max_loc[0])
        # 边缘 NCC 峰值整体偏低，阈值略低于灰度路
        if best_s < 0.085:
            return None
        return (best_x, best_s)
    except Exception:
        return None


def aliyun_resolve_detect_inputs(
    bg_path: str,
    slider_path: str,
    challenge_active_fn,
) -> tuple[str, str, bytes, bytes, str | None]:
    """
    若当前为阿里云挑战：返回临时目录内预处理后的路径 + 对应字节（供 ddddocr）；
    否则返回原始路径与字节。返回第五项为临时目录路径，调用方须在 finally 中 ``shutil.rmtree``。
    """
    with open(slider_path, "rb") as f:
        slide_bytes = f.read()
    with open(bg_path, "rb") as f:
        bg_bytes = f.read()
    try:
        active = bool(challenge_active_fn())
    except Exception:
        active = False
    if not active:
        return bg_path, slider_path, slide_bytes, bg_bytes, None
    try:
        slide_bytes, bg_bytes = aliyun_prepare_match_bytes(slide_bytes, bg_bytes)
        tmp = tempfile.mkdtemp(prefix="icgoo_aliyun_gap_")
        bp = os.path.join(tmp, "bg.png")
        sp = os.path.join(tmp, "sl.png")
        with open(bp, "wb") as f:
            f.write(bg_bytes)
        with open(sp, "wb") as f:
            f.write(slide_bytes)
        return bp, sp, slide_bytes, bg_bytes, tmp
    except Exception:
        return bg_path, slider_path, slide_bytes, bg_bytes, None


def aliyun_try_consensus_median(
    bg_orig_path: str,
    slider_orig_path: str,
    bw: int,
    tw: int,
    xf: int,
    xt: int,
) -> int | None:
    """
    dddd 两路 Canny/灰度分歧大时，在**原始**图上用多组 Canny 阈值 + 预处理后再跑纯 CV slide_match，
    若多峰聚拢则取中位数，否则返回 None（保持原优先 Canny 路）。
    """
    from icgoo_yidun_solver import (
        _dddocr_slide_match_to_left_x,
        _plausible_gap_left_x,
        _slide_match_pure_cv_like_dddd_engine,
    )

    try:
        with open(slider_orig_path, "rb") as f:
            sb = f.read()
        with open(bg_orig_path, "rb") as f:
            bb = f.read()
    except OSError:
        return None

    xs: list[int] = []
    for v in (xf, xt):
        if _plausible_gap_left_x(int(v), bw, tw):
            xs.append(int(v))

    canny_triples = (
        (25, 70),
        (35, 100),
        (50, 150),
        (65, 190),
    )
    for cl, ch in canny_triples:
        for simple in (False, True):
            try:
                r = _slide_match_pure_cv_like_dddd_engine(
                    sb,
                    bb,
                    simple_target=simple,
                    canny_low=cl,
                    canny_high=ch,
                )
                lx = _dddocr_slide_match_to_left_x(r, tw)
                if lx is not None and _plausible_gap_left_x(lx, bw, tw):
                    xs.append(int(lx))
            except Exception:
                continue

    try:
        sb2, bb2 = aliyun_prepare_match_bytes(sb, bb)
        for simple in (False, True):
            for cl, ch in ((50, 150), (40, 120)):
                try:
                    r = _slide_match_pure_cv_like_dddd_engine(
                        sb2,
                        bb2,
                        simple_target=simple,
                        canny_low=cl,
                        canny_high=ch,
                    )
                    lx = _dddocr_slide_match_to_left_x(r, tw)
                    if lx is not None and _plausible_gap_left_x(lx, bw, tw):
                        xs.append(int(lx))
                except Exception:
                    continue
    except Exception:
        pass

    # 阿里云多峰略散时仍希望收束到中位数；过严会导致几乎永不触发共识
    if len(xs) < 3:
        return None
    xs.sort()
    span = xs[-1] - xs[0]
    if span > 88:
        return None
    return xs[len(xs) // 2]


def append_aliyun_calibration_jsonl(log_path: str, record: dict) -> None:
    """
    向 ``log_path`` 追加一行 JSON（UTF-8）。``record`` 应为可 ``json.dumps`` 的对象；
    自动写入 ``ts_utc``（ISO8601 Z）、若尚无 ``schema`` 则置 1。
    """
    if not log_path or not str(log_path).strip():
        return
    rec = dict(record)
    rec.setdefault("schema", 1)
    rec.setdefault("ts_utc", datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"))
    line = json.dumps(rec, ensure_ascii=False) + "\n"
    try:
        d = os.path.dirname(os.path.abspath(log_path))
        if d:
            os.makedirs(d, exist_ok=True)
        with open(log_path, "a", encoding="utf-8") as f:
            f.write(line)
    except OSError:
        pass


def export_aliyun_dataset_sample(
    dataset_root: str,
    *,
    bg_src_path: str,
    slider_src_path: str,
    meta: dict,
) -> str | None:
    """
    将一轮识别用图 + meta.json 写入 ``dataset_root/sample_<uuid>/``。
    ``meta`` 建议含：captcha_round, raw_x, base_drag_px, slide_ok, captcha_replaced, drag_scale_x, url, time。
    """
    if not dataset_root:
        return None
    try:
        os.makedirs(dataset_root, exist_ok=True)
    except OSError:
        return None
    try:
        if not meta.get("is_aliyun"):
            return None
    except Exception:
        return None
    sid = f"{time.strftime('%Y%m%d_%H%M%S')}_{uuid.uuid4().hex[:8]}"
    d = os.path.join(dataset_root, f"sample_{sid}")
    os.makedirs(d, exist_ok=True)
    shutil.copy2(bg_src_path, os.path.join(d, "background.png"))
    shutil.copy2(slider_src_path, os.path.join(d, "slider.png"))
    with open(os.path.join(d, "meta.json"), "w", encoding="utf-8") as f:
        json.dump(meta, f, ensure_ascii=False, indent=2)
    return d
