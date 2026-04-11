"""
ICGOO 滑块验证码：**网易易盾**与缺口识别引擎（``ddddocr`` / 纯 OpenCV shim，见 ``IcgooYidunSliderSolver``）。

**阿里云 aliyunCaptcha** 的 DOM/拉图/轨道比例见独立模块 ``icgoo_aliyun_captcha``；本文件通过
``aliyun_*`` 函数调用该模块，与易盾共用同一套 slide_match 流程。

供 ``icgoo_crawler`` 调用的生产逻辑（无页面/HTML 落盘、无调试 print）。

与 ``icgoo_crawler_dev.py`` 算法一致。

关键步骤写入当前工作目录 ``icgoo_yidun_solver.log``（懒初始化，仅文件 Handler，不抢 stdout）。

``try_auto_solve_icgoo_yidun_slider`` 返回 ``YidunAutoSolveResult``：``auto_solved=True`` 表示本次自动滑动成功，供 ``icgoo_crawler`` 写业务日志。
"""
from __future__ import annotations

import base64
import binascii
from dataclasses import dataclass
import json
import logging
import math
import os
import random
import re
import shutil
import tempfile
import time
import urllib.error
import urllib.request
from collections.abc import Callable
from io import BytesIO

from DrissionPage import ChromiumPage
from PIL import Image

from icgoo_aliyun_gap import (
    aliyun_resolve_detect_inputs,
    aliyun_shadow_multiscale_match_x,
    aliyun_shadow_multiscale_match_x_edges,
    aliyun_try_consensus_median,
    append_aliyun_calibration_jsonl,
)
from icgoo_aliyun_captcha import (
    _XP_ALIYUN_BG_IMG,
    _XP_ALIYUN_PUZZLE_IMG,
    _XP_ALIYUN_REFRESH,
    _XP_ALIYUN_SLIDER_HANDLE,
    _XP_ALIYUN_SLIDING_BODY,
    _aliyun_captcha_layer_visible,
    _aliyun_host_context,
    aliyun_captcha_cleared_for_success,
    ALIYUN_CAPTCHA_PUZZLE_WIDTH_NAT_PX,
    aliyun_challenge_active,
    aliyun_drag_scale_x,
    aliyun_get_images,
    aliyun_puzzle_image_fingerprint,
    aliyun_puzzle_src_compare_key,
    aliyun_verification_success_visible,
    aliyun_slide_human_slow_enabled,
    aliyun_slide_pre_release_extra_hold_sec,
    aliyun_post_release_decoy_click,
    aliyun_post_release_decoy_click_enabled,
    aliyun_slide_y_jitter_enabled,
    aliyun_slide_y_jitter_new_state,
    aliyun_slide_y_jitter_sigma_px,
    aliyun_slide_y_jitter_cap_px,
    aliyun_slide_y_jitter_step,
    aliyun_raw_x_segment_drag_adjust,
    aliyun_fail_same_image_retry_mouse_wiggle_enabled,
    aliyun_fail_same_image_retry_settle_sec,
    aliyun_fail_same_image_retry_wait_slider_sec,
    aliyun_fail_src_churn_retry_settle_sec,
    get_aliyun_drag_scale_mult,
    get_aliyun_systematic_drag_extra_px,
)

# 易盾拼图：须限定 img，避免匹配到带同类名的容器；背景 class 含连字符，XPath 比 @class:contains 更稳
_XP_YIDUN_JIGSAW_IMG = 'xpath://img[contains(@class,"yidun_jigsaw")]'
_XP_YIDUN_BG_IMG = 'xpath://img[contains(@class,"yidun_bg-img")]'
# 拖动条：须排除 span.yidun_slider__icon（class 含 yidun_slider 子串但不是手柄）；真正可拖多为 div.yidun_slider
_XP_YIDUN_SLIDER_CANDIDATES = (
    'xpath://div[contains(@class,"yidun_slider") and not(contains(@class,"yidun_slider__"))]',
    'xpath://div[contains(@class,"yidun_slide_indicator")]',
    'xpath://div[contains(@class,"yidun_control")]',
    'xpath://span[contains(@class,"yidun_slider") and not(contains(@class,"yidun_slider__"))]',
)
_XP_YIDUN_REFRESH_CANDIDATES = (
    'xpath://span[contains(@class,"yidun_refresh")]',
    'xpath://i[contains(@class,"yidun_refresh")]',
    'xpath://*[contains(@class,"yidun_refresh")]',
)
# 实测模板匹配 + 轨道换算略偏短时，对最终拖动像素做乘法补偿（可用 --drag-boost 覆盖）
_DEFAULT_DRAG_DISTANCE_BOOST = 1.10
# 换算后拖动距离仍常略短时，在基准拖动像素上直接累加（可用 --drag-extra-px 覆盖；实测约 +16px）
_DEFAULT_DRAG_EXTRA_PX = 17
# 同一张图上多次滑动：相对基准距离的系数（宜覆盖偏短/偏长，略加跨度便于识别略偏时仍有机会命中）
# 实测易盾上 1.02 略短时 1.08 常能过，故将 1.08 提前，减少无效尝试次数
_SLIDE_DISTANCE_FACTORS = (1.02, 1.08, 1.0, 0.92, 0.86, 1.14, 0.96, 1.04)
_DEFAULT_SLIDE_ATTEMPTS = 7
# slide_match 边缘 vs 灰度 两路 raw_x 超过此差值（图内像素）时不用辅基准（另一峰常为误匹配）。
# 与阿里云拼块固定宽同值（``ALIYUN_CAPTCHA_PUZZLE_WIDTH_NAT_PX``）；易盾路径沿用该上界作经验值。
_SLIDE_MATCH_ALT_MAX_RAW_DELTA_PX = ALIYUN_CAPTCHA_PUZZLE_WIDTH_NAT_PX
# 两路置信度均低于此值且 raw 相差超过下一项时，主 raw_x 取两路中点
_SLIDE_MATCH_BLEND_MAX_CONF = 0.35
_SLIDE_MATCH_BLEND_MIN_DELTA_PX = 48
# 与 scripts/test_ddddocr_slide.py 一致：simple=True 与 simple=False 左缘相差超过此阈值时，
# 勿只信 simple=True 的高 confidence（易盾常见假峰）；缺口类优先采信 simple_target=False（Canny）
_SLIDE_MATCH_SIMPLE_DIVERGE_MIN_PX = 48.0
_SLIDE_MATCH_SIMPLE_DIVERGE_FRAC_DIAG = 0.08
# 刷新/换图后重新拉拼图并识别缺口的最大轮数（避免死循环）
_MAX_CAPTCHA_REIDENTIFY_ROUNDS = 8
# 阿里云：simple_target=True 常在左侧纹理出假峰；xt 超过此绝对值或占背景宽比例则不作「左假峰」修正
_ALIYUN_REPAIR_XT_MAX_ABS_PX = 74
_ALIYUN_REPAIR_XT_MAX_BW_FRAC = 0.25
# xf(Canny) 须比 xt 至少靠右这么多像素才触发修正
_ALIYUN_REPAIR_XF_MINUS_XT_MIN_PX = 55
# shadow_ms_edges 与 xf 距离小于此且比离 xt 更近时视为佐证
_ALIYUN_REPAIR_EDGES_NEAR_XF_PX = 44

_LOG = logging.getLogger("icgoo_yidun_solver")
_yidun_logging_ready = False


def _ensure_yidun_file_logging() -> None:
    """当前目录 ``icgoo_yidun_solver.log``；仅配置一次，不往 stderr 打避免干扰 JSON 模式。"""
    global _yidun_logging_ready
    if _yidun_logging_ready:
        return
    if _LOG.handlers:
        _yidun_logging_ready = True
        return
    _yidun_logging_ready = True
    _LOG.setLevel(logging.INFO)
    _LOG.propagate = False
    fmt = logging.Formatter(
        "%(asctime)s [%(levelname)s] %(message)s",
        datefmt="%Y-%m-%d %H:%M:%S",
    )
    path = os.path.join(os.getcwd(), "icgoo_yidun_solver.log")
    fh = logging.FileHandler(path, encoding="utf-8")
    fh.setLevel(logging.INFO)
    fh.setFormatter(fmt)
    _LOG.addHandler(fh)


def _yidun_info(msg: str) -> None:
    _ensure_yidun_file_logging()
    _LOG.info(msg)


def _yidun_warning(msg: str) -> None:
    _ensure_yidun_file_logging()
    _LOG.warning(msg)


def _yidun_error(msg: str) -> None:
    _ensure_yidun_file_logging()
    _LOG.error(msg)


def _image_natural_wh_from_bytes(data: bytes) -> tuple[int, int]:
    """PIL 打不开时用 cv2.imdecode 取宽高，避免 tw=0 导致中心 x 未减半块宽。"""
    try:
        im = Image.open(BytesIO(data))
        w, h = im.size
        if w > 0 and h > 0:
            return int(w), int(h)
    except Exception:
        pass
    try:
        import cv2
        import numpy as np

        arr = np.frombuffer(data, dtype=np.uint8)
        dec = cv2.imdecode(arr, cv2.IMREAD_UNCHANGED)
        if dec is not None and dec.size > 0:
            return int(dec.shape[1]), int(dec.shape[0])
    except Exception:
        pass
    return 0, 0


def _pil_to_rgb_composite_rgba(im: Image.Image, fill_rgb: tuple[int, int, int] = (245, 245, 245)) -> Image.Image:
    """易盾滑块图常为 RGBA；直接转 RGB 会把透明区变成黑/杂色，模板匹配易找错位置。"""
    if im.mode == "RGBA":
        canvas = Image.new("RGB", im.size, fill_rgb)
        canvas.paste(im, mask=im.split()[3])
        return canvas
    return im.convert("RGB")


def _cv2_read_bgr(path: str):
    """用 PIL 处理 RGBA 后再转 BGR，供 OpenCV 模板匹配。"""
    import cv2
    import numpy as np

    try:
        im = Image.open(path)
    except OSError:
        return None
    rgb = np.asarray(_pil_to_rgb_composite_rgba(im), dtype=np.uint8)
    return cv2.cvtColor(rgb, cv2.COLOR_RGB2BGR)


def _template_top_y_at_left_x(bg_gray, tpl_gray, left_x: int) -> int:
    """
    固定背景图内横坐标 ``left_x``，在合法纵移范围内扫 ``y``，取与 ``tpl_gray`` 归一化互相关最大者。
    用于画框：ensemble 只保证左缘 x，未统一落盘 target_y 时估算竖直位置。
    """
    import numpy as np

    th, tw = int(tpl_gray.shape[0]), int(tpl_gray.shape[1])
    bh, bw = int(bg_gray.shape[0]), int(bg_gray.shape[1])
    if tw < 1 or th < 1 or bh < th or bw < tw:
        return max(0, (bh - th) // 2)
    lx = max(0, min(int(left_x), bw - tw))
    best_y, best_s = 0, -1.0
    tpl_f = tpl_gray.astype(np.float64).ravel()
    tpl_m = tpl_f.mean()
    tpl_c = tpl_f - tpl_m
    dn = float(np.linalg.norm(tpl_c))
    if dn < 1e-6:
        return max(0, (bh - th) // 2)
    for y in range(0, bh - th + 1):
        patch = bg_gray[y : y + th, lx : lx + tw].astype(np.float64).ravel()
        pm = patch.mean()
        pc = patch - pm
        pn = float(np.linalg.norm(pc))
        if pn < 1e-6:
            continue
        s = float(np.dot(tpl_c, pc) / (dn * pn))
        if s > best_s:
            best_s, best_y = s, y
    return int(best_y)


def write_gap_overlay_png(
    bg_path: str,
    slider_path: str,
    raw_left_x: int,
    out_path: str,
    *,
    raw_detected: int | None = None,
    gap_offset_image_px: int = 0,
    show_imshow: bool = False,
    imshow_title: str = "icgoo gap overlay",
) -> bool:
    """
    在背景图上用矩形标出识别得到的拼图落点区域（左缘 x 与 ``detect_gap`` 使用的图内坐标一致）。

    竖直位置按固定 x 下灰度模板相关最大估计；若与真实槽位有上下偏差，以横坐标对照为主。
    ``show_imshow=True`` 时用 ``cv2.imshow`` 弹出窗口（按任意键关闭），需本地 GUI。

    返回是否成功写出 ``out_path``。
    """
    import cv2
    import numpy as np

    try:
        bg_bgr = _cv2_read_bgr(bg_path)
        if bg_bgr is None or bg_bgr.size == 0:
            return False
        try:
            sim = Image.open(slider_path)
        except OSError:
            return False
        tw, th = int(sim.size[0]), int(sim.size[1])
        if tw < 2 or th < 2:
            return False
        tpl_rgb = np.asarray(_pil_to_rgb_composite_rgba(sim), dtype=np.uint8)
        tpl_gray = cv2.cvtColor(tpl_rgb, cv2.COLOR_RGB2GRAY)
        bg_gray = cv2.cvtColor(bg_bgr, cv2.COLOR_BGR2GRAY)
        bh, bw = int(bg_gray.shape[0]), int(bg_gray.shape[1])
        lx = max(0, min(int(raw_left_x), max(0, bw - 1)))
        ty = _template_top_y_at_left_x(bg_gray, tpl_gray, lx)
        tw_c = min(tw, max(0, bw - lx))
        th_c = min(th, max(0, bh - ty))
        if tw_c < 2 or th_c < 2:
            return False

        vis = bg_bgr.copy()
        overlay = vis.copy()
        pt1 = (lx, ty)
        pt2 = (lx + tw_c - 1, ty + th_c - 1)
        cv2.rectangle(overlay, pt1, pt2, (0, 220, 255), -1)
        cv2.addWeighted(overlay, 0.22, vis, 0.78, 0, vis)
        cv2.rectangle(vis, pt1, pt2, (0, 200, 255), 2)

        lines = [
            f"raw_x={int(raw_left_x)} piece={tw}x{th}",
        ]
        if raw_detected is not None:
            lines.append(
                f"detected={int(raw_detected)} off={int(gap_offset_image_px)}"
            )
        y0 = 22
        for i, line in enumerate(lines):
            cv2.putText(
                vis,
                line,
                (8, y0 + i * 20),
                cv2.FONT_HERSHEY_SIMPLEX,
                0.55,
                (0, 0, 255),
                2,
                cv2.LINE_AA,
            )
        out_dir = os.path.dirname(out_path)
        if out_dir:
            os.makedirs(out_dir, exist_ok=True)
        ok_write = bool(cv2.imwrite(out_path, vis))
        if show_imshow:
            try:
                cv2.namedWindow(imshow_title, cv2.WINDOW_NORMAL)
                cv2.imshow(imshow_title, vis)
                cv2.waitKey(0)
                cv2.destroyWindow(imshow_title)
            except cv2.error:
                pass
        if not ok_write:
            return False
    except Exception:
        return False
    return True


def _opencv_alpha_mask_match_left_x(
    bg_path: str,
    slider_path: str,
    *,
    min_alpha: int = 12,
    min_mask_ratio: float = 0.04,
) -> tuple[int, float] | None:
    """
    仅当滑块图为 **BGRA** 时：用 Alpha 作 ``matchTemplate`` 的 mask，灰度参与匹配（透明区不参与）。
    背景与 ``_cv2_read_bgr`` 一致，坐标系与现有 gray/Canny 路对齐。预处理成 RGB 无 Alpha 时自动跳过。
    """
    import cv2
    import numpy as np

    bg = _cv2_read_bgr(bg_path)
    if bg is None:
        return None
    bg_gray = cv2.cvtColor(bg, cv2.COLOR_BGR2GRAY)

    sl = cv2.imread(slider_path, cv2.IMREAD_UNCHANGED)
    if sl is None or sl.size == 0 or len(sl.shape) != 3 or sl.shape[2] != 4:
        return None
    b_chan, g_chan, r_chan, a_chan = cv2.split(sl)
    mask = np.where(a_chan >= min_alpha, 255, 0).astype(np.uint8)
    h0, w0 = int(mask.shape[0]), int(mask.shape[1])
    if h0 < 2 or w0 < 2:
        return None
    if cv2.countNonZero(mask) < int(h0 * w0 * min_mask_ratio):
        return None
    slide_gray = cv2.cvtColor(cv2.merge([b_chan, g_chan, r_chan]), cv2.COLOR_BGR2GRAY)
    th, tw = int(slide_gray.shape[0]), int(slide_gray.shape[1])
    bh, bw = int(bg_gray.shape[0]), int(bg_gray.shape[1])
    if tw >= bw or th >= bh:
        return None
    try:
        res = cv2.matchTemplate(bg_gray, slide_gray, cv2.TM_CCORR_NORMED, mask=mask)
    except (cv2.error, TypeError, ValueError):
        return None
    _mn, max_v, _min_loc, max_loc = cv2.minMaxLoc(res)
    return int(max_loc[0]), float(max_v)


def _puzzle_slider_captcha_left_x_score(
    bg_path: str,
    slider_path: str,
) -> tuple[int, float] | None:
    """
    可选依赖 `puzzle-slider-captcha <https://pypi.org/project/puzzle-slider-captcha/>`_：
    Normalize + Canny(150,250) 边缘图上做 ``TM_CCOEFF_NORMED``，与库默认流程一致；返回（左缘 x, 峰值作置信度）。
    输入用 ``_cv2_read_bgr`` 得三通道 BGR，与阿里云预处理后 PNG 对齐。
    """
    try:
        import importlib

        importlib.import_module("puzzle_slider_captcha")
        from puzzle_slider_captcha import PuzzleCaptchaSolver
        from puzzle_slider_captcha._transforms import EdgeTransform, NormalizeTransform
    except ImportError:
        return None
    import cv2

    bg = _cv2_read_bgr(bg_path)
    sl = _cv2_read_bgr(slider_path)
    if bg is None or sl is None:
        return None
    bh, bw = int(bg.shape[0]), int(bg.shape[1])
    sh, sw = int(sl.shape[0]), int(sl.shape[1])
    if sw >= bw or sh >= bh or sw < 4 or sh < 4:
        return None
    try:
        solver = PuzzleCaptchaSolver()
        result = solver.handle_image(bg, sl)
    except (ValueError, cv2.error, Exception):
        return None
    x = int(result.x)
    if not _plausible_gap_left_x(x, bw, sw):
        return None
    tr = (NormalizeTransform(), EdgeTransform(150, 250))

    def _apply(img):
        p = img.copy()
        for t in tr:
            p = t.transform(p)
        return p

    pb, ps = _apply(bg), _apply(sl)
    res = cv2.matchTemplate(pb, ps, cv2.TM_CCOEFF_NORMED)
    _mn, max_v, _min_loc, _max_loc = cv2.minMaxLoc(res)
    return x, float(max_v)


def _ddd_auto_skip_opencv_ensemble(
    xf: int | None,
    xt: int | None,
    *,
    is_aliyun_preprocessed: bool,
) -> bool:
    """
    ``detect_gap_position`` 在 auto 下、且 slide_match 两路均已得到 plausible 左缘时：
    若返回 True，则**不**调用 ``_ensemble_auto_best_gap``，仅走下方 dddd 融合/决选。

    易盾（非阿里云预处理图）：恒 True，保留「双 plausible 时不与 OpenCV 比分数」以防纹理假峰。

    阿里云：**恒 False**（只要两路均有值）。浅底阴影拼图下两路 dddd 横差小也可能**同错**，
    若跳过 ensemble 则 OpenCV 灰度/Canny 无法参与决选，线上识别率明显下降；故强制与 OpenCV 按分竞争。
    """
    if not is_aliyun_preprocessed:
        return True
    if xf is None or xt is None:
        return True
    return False


def _plausible_gap_left_x(left: int, bg_w: int, piece_w: int) -> bool:
    """
    模板匹配得到的拼图左缘 x：须在 (0, bg_w - piece_w) 内；0 常为误匹配，排除。
    """
    try:
        x = int(left)
    except (TypeError, ValueError):
        return False
    if x < 2:
        return False
    if bg_w <= 0 or piece_w <= 0:
        return x < 4096
    pw = max(4, int(piece_w))
    if pw >= bg_w:
        return False
    return x <= bg_w - pw - 1


def _aliyun_lowconf_dddd_shadow_override(
    *,
    chosen_left: int,
    chosen_etag: str,
    chosen_score: float,
    bw: int,
    tw: int,
    shadow_ms_x: int | None,
    shadow_ms_score: float | None,
    shadow_ms_edges_x: int | None,
    shadow_ms_edges_score: float | None,
) -> tuple[int, float, str] | None:
    """
    阿里云：ensemble 已决选 ``dddd_true`` / ``dddd_false``，但 dddd 分数偏低且与多尺度 shadow 匹配 x 明显分歧时，
    改采 ``aliyun_shadow_ms``（优先）或 ``aliyun_shadow_ms_edges``，减轻「低分假峰」误选（见 run_20260410_222318 R4）。

    环境变量见 ``icgoo_aliyun_captcha`` 模块说明。
    """
    off = (os.environ.get("ICGOO_ALIYUN_LOWCONF_SHADOW_OVERRIDE") or "").strip().lower()
    if off in ("0", "false", "no", "off"):
        return None
    if chosen_etag not in ("dddd_true", "dddd_false"):
        return None
    try:
        conf_max = float(
            (os.environ.get("ICGOO_ALIYUN_LOWCONF_SHADOW_CONF_MAX") or "0.45").strip()
        )
    except ValueError:
        conf_max = 0.45
    conf_max = max(0.12, min(0.82, conf_max))
    try:
        div_min = float(
            (os.environ.get("ICGOO_ALIYUN_LOWCONF_SHADOW_DIVERGE_MIN_PX") or "32").strip()
        )
    except ValueError:
        div_min = 32.0
    div_min = max(10.0, min(140.0, div_min))

    cs = float(chosen_score)
    if cs >= conf_max and cs >= 0.0:
        return None

    d_x = int(chosen_left)
    opts: list[tuple[int, float, str]] = []
    if (
        shadow_ms_x is not None
        and shadow_ms_score is not None
        and _plausible_gap_left_x(int(shadow_ms_x), bw, tw)
    ):
        opts.append((int(shadow_ms_x), float(shadow_ms_score), "aliyun_lowconf_shadow_ms"))
    if (
        shadow_ms_edges_x is not None
        and shadow_ms_edges_score is not None
        and _plausible_gap_left_x(int(shadow_ms_edges_x), bw, tw)
    ):
        opts.append(
            (
                int(shadow_ms_edges_x),
                float(shadow_ms_edges_score),
                "aliyun_lowconf_shadow_ms_edges",
            )
        )
    qualified = [t for t in opts if abs(int(t[0]) - d_x) >= int(div_min)]
    if not qualified:
        return None
    ms = [t for t in qualified if t[2] == "aliyun_lowconf_shadow_ms"]
    pick = ms[0] if ms else max(qualified, key=lambda t: abs(int(t[0]) - d_x))
    sx, ss, tag = pick
    return int(sx), float(ss), str(tag)


def _aliyun_repair_dddd_small_true_false_peak(
    *,
    bw: int,
    tw: int,
    xf: int | None,
    xt: int | None,
    chosen_left: int,
    shadow_ms_edges_x: int | None,
    shadow_ms_edges_score: float | None,
    dddd_conf_false: float | None,
) -> tuple[int, float] | None:
    """
    阿里云浅底拼图：dddd ``simple_target=True`` 的左缘 xt 偶落在最左纹理假峰，``simple_target=False``(Canny) xf
    在真槽附近。若当前决选仍落在 xt 一侧，且 ``aliyun_shadow_ms_edges`` 与 xf 更一致（或 xf 远超 xt），
    则改采 xf。易盾不调用。
    """
    if bw < 100 or tw < 4 or xf is None or xt is None:
        return None
    xi, xt_i = int(xf), int(xt)
    if not _plausible_gap_left_x(xi, bw, tw) or not _plausible_gap_left_x(xt_i, bw, tw):
        return None
    if abs(xi - xt_i) < _SLIDE_MATCH_ALT_MAX_RAW_DELTA_PX:
        return None
    xt_cap = max(40, min(_ALIYUN_REPAIR_XT_MAX_ABS_PX, int(bw * _ALIYUN_REPAIR_XT_MAX_BW_FRAC)))
    if xt_i > xt_cap:
        return None
    if xi < xt_i + _ALIYUN_REPAIR_XF_MINUS_XT_MIN_PX:
        return None
    ch = int(chosen_left)
    if abs(ch - xt_i) > 10:
        return None
    corroborate = False
    if shadow_ms_edges_x is not None and _plausible_gap_left_x(int(shadow_ms_edges_x), bw, tw):
        se = int(shadow_ms_edges_x)
        near_xf = abs(se - xi) <= _ALIYUN_REPAIR_EDGES_NEAR_XF_PX
        closer_to_xf = abs(se - xt_i) > abs(se - xi) + 6
        if near_xf and closer_to_xf:
            corroborate = True
    if not corroborate and (xi - xt_i) >= 95:
        corroborate = True
    if not corroborate:
        return None
    sc = 0.42
    if dddd_conf_false is not None and float(dddd_conf_false) >= 0.0:
        sc = max(sc, float(dddd_conf_false))
    if shadow_ms_edges_score is not None and float(shadow_ms_edges_score) >= 0.0:
        sc = max(sc, float(shadow_ms_edges_score) * 0.88)
    return (xi, sc)


def _dddocr_slide_match_to_left_x(res: dict, slide_w: int) -> int | None:
    """
    将 ddddocr.DdddOcr.slide_match 的返回 dict 转为「拼图块左缘在背景图内的 x」（自然像素）。
    与 ddddocr 组件一致：
    - 新版 SlideEngine：target 为 [center_x, center_y]，target_x 为匹配中心横坐标，须减滑块半宽得左缘。
    - 旧版部分返回 target 为 [x, y, w, h]：x 为模板左上角在背景上的位置，直接作左缘。
    """
    if not res:
        return None
    targ = res.get("target")
    if isinstance(targ, (list, tuple)) and len(targ) >= 4:
        try:
            return max(0, int(targ[0]))
        except (TypeError, ValueError):
            return None
    cx: int | None = None
    if "target_x" in res and res["target_x"] is not None:
        try:
            cx = int(res["target_x"])
        except (TypeError, ValueError):
            cx = None
    if cx is None and isinstance(targ, (list, tuple)) and len(targ) >= 1:
        try:
            cx = int(targ[0])
        except (TypeError, ValueError):
            cx = None
    if cx is None:
        return None
    if isinstance(targ, (list, tuple)) and len(targ) == 2:
        if slide_w > 0:
            return max(0, cx - slide_w // 2)
        return max(0, cx)
    if slide_w > 0:
        return max(0, cx - slide_w // 2)
    return max(0, cx)


def _slide_match_pure_cv_like_dddd_engine(
    target_img: bytes,
    background_img: bytes,
    *,
    simple_target: bool = False,
    canny_low: int = 50,
    canny_high: int = 150,
) -> dict:
    """
    与 ddddocr.core.slide_engine.SlideEngine 内逻辑一致：RGB→灰度，simple_target 时直接模板匹配，
    否则 Canny 后匹配，TM_CCOEFF_NORMED，返回 target/target_x/target_y/confidence。
    ``canny_low``/``canny_high`` 仅 ``simple_target=False`` 时生效（阿里云多尺度共识用）。
    不执行 ``import ddddocr``，从而避开其 ``core.base`` 对 onnxruntime 的顶层导入（Windows 上常见 DLL 初始化失败）。
    """
    import cv2
    import numpy as np

    def _rgb_u8(data: bytes):
        im = Image.open(BytesIO(data))
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
        lo = int(max(1, min(canny_low, canny_high - 1)))
        hi = int(max(lo + 1, canny_high))
        te = cv2.Canny(target_gray, lo, hi)
        be = cv2.Canny(background_gray, lo, hi)
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
    """与 ``DdddOcr.slide_match`` 相同调用方式；底层为上述纯 OpenCV，与官方 SlideEngine 对齐。"""

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


def _data_url_base64_payload(src: str | None) -> str:
    """从 img src（data:image/...;base64,...）取出逗号后的 payload，容错大小写与空格。"""
    if not src:
        return ""
    s = src.strip()
    low = s.lower()
    if "base64," in low:
        i = low.index("base64,")
        return s[i + len("base64,") :].strip()
    if "," in s:
        return s.split(",", 1)[1].strip()
    return s


def _decode_base64_image_payload(b64: str) -> bytes:
    """
    解码易盾图片 data URL 中的 base64：去空白、支持 url-safe、补 padding。
    优先尝试标准解码，若失败则尝试去除末尾一个字符再解码（应对站点偶发的异常长度）。
    """
    s = re.sub(r"\s+", "", (b64 or "").strip())
    if not s:
        raise ValueError("base64 为空")
    s = s.replace("-", "+").replace("_", "/")
    rem = len(s) % 4
    # 标准做法：补等号
    if rem:
        s += "=" * (4 - rem)
    try:
        return base64.b64decode(s, validate=True)
    except (binascii.Error, ValueError):
        # 尝试去除最后一个字符再解码（某些站点会多一个异常字符）
        if len(s) > 1:
            s_stripped = s[:-1]
            rem2 = len(s_stripped) % 4
            if rem2:
                s_stripped += "=" * (4 - rem2)
            try:
                return base64.b64decode(s_stripped, validate=False)
            except Exception:
                pass
        # 最后尝试不验证直接解码
        return base64.b64decode(s, validate=False)


def _captcha_img_src_to_b64_payload(src: str | None, referer: str) -> str:
    """
    将易盾背景/滑块图的 src 转为可交给 save_base64_image 的 base64 字符串。
    站点可能使用 data: URL，也可能使用 necaptcha.nosdn.127.net 等 https 直链。
    """
    if not src or not str(src).strip():
        raise ValueError("验证码图片 src 为空")
    s = str(src).strip()
    low = s.lower()
    if "base64," in low or (low.startswith("data:") and "," in s):
        pl = _data_url_base64_payload(s)
        if not pl:
            raise ValueError("data URL 中无有效 base64")
        return pl
    if low.startswith("http://") or low.startswith("https://"):
        req = urllib.request.Request(
            s,
            headers={
                "User-Agent": (
                    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 "
                    "(KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
                ),
                "Referer": (referer or "").strip() or "https://www.icgoo.net/",
            },
        )
        try:
            with urllib.request.urlopen(req, timeout=30) as resp:
                raw = resp.read()
        except urllib.error.URLError as e:
            raise ValueError(f"下载验证码图片失败: {e}") from e
        if not raw:
            raise ValueError("验证码图片响应为空")
        return base64.b64encode(raw).decode("ascii")
    raise ValueError(f"不支持的验证码图片地址: {s[:120]}")


class IcgooBrowserDisconnectedError(RuntimeError):
    """DrissionPage 与浏览器连接已断开；不应再调用 page.refresh()。"""


def _read_yidun_img_src(ele) -> str:
    """
    易盾拼图 img 的地址：DOM 可能先出现节点再异步写 src，或放在 data-src；
    部分环境下需读 JS 的 ``src`` / ``currentSrc``。
    """
    if not ele:
        return ""
    for attr in ("src", "data-src", "data-original", "data-lazy-src"):
        try:
            v = (ele.attr(attr) or "").strip()
            if v:
                return v
        except Exception:
            pass
    try:
        raw = ele.run_js(
            "return (this.src || this.currentSrc || this.getAttribute('data-src') || '').trim();"
        )
        if raw is not None:
            s = str(raw).strip()
            if s:
                return s
    except Exception:
        pass
    return ""


def build_gap_calibration_record(
    solver: object,
    *,
    captcha_round: int,
    drag_scale_x: float,
    raw_x: int,
    base_drag_px: int,
    distance_alt_px: int | None,
    page_url: str,
    drag_boost: float,
    gap_offset_image_px: int,
    drag_extra_px_user: int,
    bg_path: str | None = None,
    slider_path: str | None = None,
) -> dict:
    """单轮识别后的 JSONL 一行，供与 ``benchmark_aliyun_gap`` / 手工标定对照。"""
    bw = bh = tw = th = None
    if bg_path:
        try:
            im = Image.open(bg_path)
            bw, bh = int(im.size[0]), int(im.size[1])
        except Exception:
            pass
    if slider_path:
        try:
            im = Image.open(slider_path)
            tw, th = int(im.size[0]), int(im.size[1])
        except Exception:
            pass
    rec: dict = {
        "event": "gap_detected",
        "captcha_round": int(captcha_round),
        "page_url": (page_url or "")[:500],
        "gap_backend": getattr(solver, "_last_gap_backend", None),
        "gap_raw_detected": getattr(solver, "_gap_raw_detected", None),
        "raw_x_image_px": int(raw_x),
        "gap_offset_image_px": int(gap_offset_image_px),
        "drag_scale_x": float(drag_scale_x),
        "drag_boost": float(drag_boost),
        "drag_extra_px_user": int(drag_extra_px_user),
        "drag_extra_effective_px": getattr(solver, "_effective_drag_extra_px", None),
        "aliyun_drag_extra_applied": getattr(solver, "_aliyun_drag_extra_applied", None),
        "aliyun_systematic_constant_px": get_aliyun_systematic_drag_extra_px(),
        "aliyun_drag_scale_mult": get_aliyun_drag_scale_mult(),
        "aliyun_raw_x_seg_mult": getattr(solver, "_aliyun_raw_x_seg_mult", None),
        "aliyun_raw_x_seg_extra_px": getattr(solver, "_aliyun_raw_x_seg_extra_px", None),
        "aliyun_raw_x_seg_label": getattr(solver, "_aliyun_raw_x_seg_label", None),
        "base_drag_px": int(base_drag_px),
        "distance_alt_px": distance_alt_px,
        "bg_nat_w": bw,
        "bg_nat_h": bh,
        "slider_nat_w": tw,
        "slider_nat_h": th,
        "slide_match_blended": getattr(solver, "_slide_match_blended", None),
        "slide_match_diverge_prefer_false": getattr(solver, "_slide_match_diverge_prefer_false", None),
        "dddd_simple_target": getattr(solver, "_dddd_simple_target", None),
        "dddd_confidence": getattr(solver, "_dddd_confidence", None),
        "dddd_raw_x_simple_false": getattr(solver, "_dddd_raw_left_simple_false", None),
        "dddd_raw_x_simple_true": getattr(solver, "_dddd_raw_left_simple_true", None),
        "opencv_x_gray": getattr(solver, "_opencv_raw_x_gray", None),
        "opencv_x_canny": getattr(solver, "_opencv_raw_x_canny", None),
        "opencv_score_gray": getattr(solver, "_opencv_score_gray", None),
        "opencv_score_canny": getattr(solver, "_opencv_score_canny", None),
        "opencv_x_shadow_ms": getattr(solver, "_opencv_raw_x_shadow_ms", None),
        "opencv_score_shadow_ms": getattr(solver, "_opencv_score_shadow_ms", None),
        "opencv_x_shadow_ms_edges": getattr(solver, "_opencv_raw_x_shadow_ms_edges", None),
        "opencv_score_shadow_ms_edges": getattr(solver, "_opencv_score_shadow_ms_edges", None),
        "opencv_mode_primary": getattr(solver, "_last_opencv_match_mode", None),
        "opencv_score_primary": getattr(solver, "_last_opencv_match_score", None),
        "gap_ensemble_pick": getattr(solver, "_gap_ensemble_pick", None),
    }
    try:
        rec["is_aliyun_active"] = solver._aliyun_challenge_active(timeout=0.35)  # type: ignore[attr-defined]
    except Exception:
        rec["is_aliyun_active"] = None
    return rec


# 与 ``SliderCaptchaSolver._captcha_debug_apply_release_residual`` 写入的字段同名（JSON 键）
_SLIDE_ATTEMPT_RELEASE_CALIB_KEYS: tuple[tuple[str, str], ...] = (
    ("_cal_release_residual_css_x", "release_residual_css_x"),
    ("_cal_release_slot_left_css_x_approx", "release_slot_left_css_x_approx"),
    ("_cal_release_puzzle_left_css_x", "release_puzzle_left_css_x"),
    ("_cal_release_scale_css_px_per_nat_x", "release_scale_css_px_per_nat_x"),
    ("_cal_release_raw_x_image_px", "release_raw_x_image_px"),
    ("_cal_release_residual_note", "release_residual_note"),
)


def _reset_slide_release_calibration_fields(solver: object) -> None:
    """每轮滑动尝试开始前清空，避免上一轮松手残差误入本条 ``slide_attempt``。"""
    for attr, _ in _SLIDE_ATTEMPT_RELEASE_CALIB_KEYS:
        setattr(solver, attr, None)
    setattr(solver, "_cal_aliyun_y_jitter_bundle", None)


def build_slide_attempt_calibration_record(
    *,
    captcha_round: int,
    attempt_index: int,
    factor: float,
    base_tag: str,
    base_px: int,
    d_try: int,
    tracks_sum: int,
    ok: bool,
    outcome: str,
    verify_note: str = "",
    src_before_key: str = "",
    src_after_key: str = "",
    puzzle_replaced: bool = False,
    solver: object | None = None,
) -> dict:
    rec: dict = {
        "event": "slide_attempt",
        "captcha_round": int(captcha_round),
        "attempt_index": int(attempt_index),
        "factor": float(factor),
        "base_tag": base_tag,
        "base_px": int(base_px),
        "d_try_px": int(d_try),
        "tracks_sum": int(tracks_sum),
        "check_success_ok": bool(ok),
        "outcome": outcome,
        "verify_note": (verify_note or "")[:240],
        "puzzle_src_before_key": (src_before_key or "")[:220],
        "puzzle_src_after_key": (src_after_key or "")[:220],
        "puzzle_replaced": bool(puzzle_replaced),
    }
    if solver is not None:
        for attr, jkey in _SLIDE_ATTEMPT_RELEASE_CALIB_KEYS:
            v = getattr(solver, attr, None)
            if v is not None:
                rec[jkey] = v
        jb = getattr(solver, "_cal_aliyun_y_jitter_bundle", None)
        if isinstance(jb, dict):
            rec.update(jb)
    return rec


class IcgooYidunSliderSolver:
    def __init__(
        self,
        page: ChromiumPage,
        *,
        slider_gap_backend: str = "auto",
        quiet: bool = True,
        drag_boost: float = _DEFAULT_DRAG_DISTANCE_BOOST,
        drag_extra_px: int = _DEFAULT_DRAG_EXTRA_PX,
        gap_offset_image_px: int = 0,
        calibration_jsonl_path: str | None = None,
    ):
        self.page = page
        self._quiet = quiet
        p = (calibration_jsonl_path or "").strip()
        self.calibration_jsonl_path: str | None = p if p else None
        self._drag_boost = float(drag_boost) if drag_boost and drag_boost > 0 else 1.0
        self._drag_extra_px = int(drag_extra_px)
        self._gap_offset_image_px = int(gap_offset_image_px)
        # auto：优先 ddddocr.slide_match，失败再 OpenCV；ddddocr：仅 ddddocr；opencv：仅 OpenCV（调试）；
        # slidercracker：可选 PyPI 包，内部等同 simple_target=True 的 slide_match（依赖版本较老，慎用）
        self._slider_gap_backend = slider_gap_backend if slider_gap_backend in (
            "auto",
            "ddddocr",
            "opencv",
            "slidercracker",
        ) else "auto"
        # None=未尝试；False=ddddocr 不可用（未安装或导入失败等）；否则为 DdddOcr 实例
        self._dddd_slide = None
        self._dddd_init_error: str | None = None
        self._slide_backend_warned = False
        self._last_opencv_match_score: float | None = None
        self._last_opencv_match_mode: str | None = None
        self._opencv_raw_x_gray: int | None = None
        self._opencv_raw_x_canny: int | None = None
        self._opencv_score_gray: float | None = None
        self._opencv_score_canny: float | None = None
        self._opencv_raw_x_alpha_mask: int | None = None
        self._opencv_score_alpha_mask: float | None = None
        self._opencv_raw_x_puzzle_psc: int | None = None
        self._opencv_score_puzzle_psc: float | None = None
        self._opencv_raw_x_shadow_ms: int | None = None
        self._opencv_score_shadow_ms: float | None = None
        self._opencv_raw_x_shadow_ms_edges: int | None = None
        self._opencv_score_shadow_ms_edges: float | None = None
        # ddddocr 路径：与官方「仅滑块」模式一致（不加载 OCR/检测 ONNX，仅 SlideEngine）
        self._last_gap_backend: str = ""
        self._dddd_confidence: float | None = None
        self._dddd_simple_target: bool | None = None
        # slide_match 两路的图内左缘 x（simple_target=False / True），用于与 OpenCV 类似的 distance_alt
        self._dddd_raw_left_simple_false: int | None = None
        self._dddd_raw_left_simple_true: int | None = None
        self._dddd_conf_simple_false: float | None = None
        self._dddd_conf_simple_true: float | None = None
        self._slide_match_blended = False
        # True：两路分歧大时已按 test_ddddocr_slide 策略采用 simple_target=False 的左缘，而非置信度最高路
        self._slide_match_diverge_prefer_false = False
        # auto 下四路 ensemble 胜出时的来源标签（如 opencv_canny），供 00_detect 记录
        self._gap_ensemble_pick: str | None = None
        # True：未 import 成 ddddocr 包，正使用本文件内与 SlideEngine 等价的纯 OpenCV shim
        self._slide_match_is_shim = False
        self._gap_raw_detected: int | None = None
        # detect_gap 内写入：含阿里云系统性加量后的拖动加量，供 _compute_slide_distance_alt 与日志一致
        self._effective_drag_extra_px = int(drag_extra_px)
        self._aliyun_drag_extra_applied = 0

    def _emit_calibration(self, record: dict) -> None:
        if not self.calibration_jsonl_path:
            return
        try:
            append_aliyun_calibration_jsonl(self.calibration_jsonl_path, dict(record))
        except Exception:
            pass

    def _verbose(self, msg: str) -> None:
        if not self._quiet:
            print(msg, flush=True)

    def _aliyun_challenge_active(self, timeout: float = 0.85) -> bool:
        """当前是否处于阿里云滑块挑战（与 ``icgoo_aliyun_captcha`` 内可见性探测一致）。"""
        return aliyun_challenge_active(self.page, timeout, read_img_src=_read_yidun_img_src)

    def _aliyun_drag_scale_x(self, bg_ele, natural_width: int) -> float:
        """阿里云：轨道可行程 / 背景图自然宽度。"""
        return aliyun_drag_scale_x(
            self.page,
            bg_ele,
            natural_width,
            fallback_display_scale=self._yidun_bg_display_scale_x,
        )

    def _aliyun_get_images(self) -> tuple[str, str, float]:
        """拉取阿里云背景图、拼图块；返回 (bg_b64, puzzle_b64, drag_scale_x)。"""
        return aliyun_get_images(
            self.page,
            log_warning=_yidun_warning,
            log_info=_yidun_info,
            fallback_display_scale=self._yidun_bg_display_scale_x,
            read_img_src=_read_yidun_img_src,
        )

    def _get_dddd_slide(self):
        if self._dddd_slide is False:
            return None
        if self._dddd_slide is not None:
            return self._dddd_slide
        self._slide_match_is_shim = False
        try:
            import ddddocr

            try:
                inst = ddddocr.DdddOcr(ocr=False, det=False, show_ad=False)
            except TypeError:
                try:
                    inst = ddddocr.DdddOcr(det=False, ocr=False, show_ad=False)
                except TypeError:
                    inst = ddddocr.DdddOcr(det=False, ocr=False)
            self._dddd_slide = inst
            return inst
        except Exception as e:
            self._dddd_init_error = f"{type(e).__name__}: {e}"
            try:
                import cv2  # noqa: F401
            except ImportError:
                self._dddd_slide = False
                return None
            self._dddd_slide = _DdddSlideEngineShim()
            self._slide_match_is_shim = True
            if not self._slide_backend_warned:
                self._verbose(
                    "ddddocr 不可用，已改用与 SlideEngine 等价的纯 OpenCV 滑块（无需 ONNX）。"
                    f" 原因: {self._dddd_init_error}"
                )
                _yidun_info(
                    "缺口识别后端: ddddocr 不可用，已切换 OpenCV SlideEngine shim；"
                    f"原因={self._dddd_init_error}"
                )
                self._slide_backend_warned = True
            return self._dddd_slide

    def _fill_opencv_gap_scores(
        self,
        bg_path: str,
        slider_path: str,
        *,
        include_aliyun_aux: bool = False,
        original_slider_path: str | None = None,
    ) -> bool:
        """
        计算灰度/Canny 模板匹配的左缘 x 与 TM_CCOEFF_NORMED 峰值；供 ensemble 与回退。
        ``include_aliyun_aux``：仅 **阿里云预处理**（``tmp_aliyun``）时 True，额外尝试 Alpha-mask、
        可选 ``puzzle-slider-captcha``、以及 ``gap-position-detection.md`` 风格的多尺度 shadow 匹配；
        易盾恒 False。
        ``original_slider_path``：磁盘上**未预处理**的滑块 PNG（保留 Alpha），供 shadow 紧裁；省略则不跑该路。
        """
        import cv2

        self._opencv_raw_x_gray = None
        self._opencv_raw_x_canny = None
        self._opencv_score_gray = None
        self._opencv_score_canny = None
        self._opencv_raw_x_alpha_mask = None
        self._opencv_score_alpha_mask = None
        self._opencv_raw_x_puzzle_psc = None
        self._opencv_score_puzzle_psc = None
        self._opencv_raw_x_shadow_ms = None
        self._opencv_score_shadow_ms = None
        self._opencv_raw_x_shadow_ms_edges = None
        self._opencv_score_shadow_ms_edges = None
        bg = _cv2_read_bgr(bg_path)
        slide = _cv2_read_bgr(slider_path)
        if bg is None or slide is None:
            return False
        bg_gray = cv2.cvtColor(bg, cv2.COLOR_BGR2GRAY)
        slide_gray = cv2.cvtColor(slide, cv2.COLOR_BGR2GRAY)
        res1 = cv2.matchTemplate(bg_gray, slide_gray, cv2.TM_CCOEFF_NORMED)
        _, max_v1, _, loc1 = cv2.minMaxLoc(res1)

        bg_e = cv2.Canny(bg_gray, 50, 150)
        sl_e = cv2.Canny(slide_gray, 50, 150)
        res2 = cv2.matchTemplate(bg_e, sl_e, cv2.TM_CCOEFF_NORMED)
        _, max_v2, _, loc2 = cv2.minMaxLoc(res2)

        self._opencv_raw_x_gray = int(loc1[0])
        self._opencv_raw_x_canny = int(loc2[0])
        self._opencv_score_gray = float(max_v1)
        self._opencv_score_canny = float(max_v2)
        if include_aliyun_aux:
            am = _opencv_alpha_mask_match_left_x(bg_path, slider_path)
            if am is not None:
                self._opencv_raw_x_alpha_mask, self._opencv_score_alpha_mask = am
            psc = _puzzle_slider_captcha_left_x_score(bg_path, slider_path)
            if psc is not None:
                self._opencv_raw_x_puzzle_psc, self._opencv_score_puzzle_psc = psc
            if original_slider_path:
                sms = aliyun_shadow_multiscale_match_x(bg_path, original_slider_path)
                if sms is not None:
                    self._opencv_raw_x_shadow_ms, self._opencv_score_shadow_ms = sms
                sme = aliyun_shadow_multiscale_match_x_edges(bg_path, original_slider_path)
                if sme is not None:
                    self._opencv_raw_x_shadow_ms_edges, self._opencv_score_shadow_ms_edges = sme
        return True

    def _detect_gap_position_opencv(self, bg_path: str, slider_path: str) -> int:
        """OpenCV 回退：同时算灰度与 Canny 的 x；主结果取更可信者，二者均保留供交替试拖。"""
        self._last_opencv_match_score = None
        self._last_opencv_match_mode = None
        if not self._fill_opencv_gap_scores(bg_path, slider_path):
            return 100
        max_v1 = float(self._opencv_score_gray or 0.0)
        max_v2 = float(self._opencv_score_canny or 0.0)
        xc = int(self._opencv_raw_x_canny or 0)
        xg = int(self._opencv_raw_x_gray or 0)
        if max_v2 > max(0.28, max_v1 - 0.12):
            self._last_opencv_match_mode = "canny"
            self._last_opencv_match_score = max_v2
            return xc
        self._last_opencv_match_mode = "gray"
        self._last_opencv_match_score = max_v1
        return xg

    def _detect_gap_position_slidercracker(
        self,
        bg_path: str,
        slider_path: str,
        *,
        bw: int,
        tw: int,
    ) -> int:
        """
        可选依赖 `slidercracker <https://pypi.org/project/slidercracker/>`_：
        库内实现为 ``ddddocr.slide_match(..., simple_target=True)`` 加路径封装；
        包声明依赖 ``ddddocr==1.4.7``、``opencv-python==4.5.5.64``，易与当前环境冲突，建议单独 venv。
        """
        import contextlib
        import importlib
        import io

        try:
            slidercracker = importlib.import_module("slidercracker")
        except ImportError as e:
            raise RuntimeError(
                "滑块后端 slidercracker 未安装。可尝试: pip install slidercracker "
                "（注意包内固定 ddddocr==1.4.7、opencv-python==4.5.5.64，可能与现有依赖冲突）"
            ) from e
        self._last_gap_backend = "slidercracker"
        self._gap_ensemble_pick = None
        self._slide_match_blended = False
        self._slide_match_diverge_prefer_false = False
        self._dddd_simple_target = True
        self._dddd_confidence = None
        self._dddd_conf_simple_false = None
        self._dddd_conf_simple_true = None
        self._dddd_raw_left_simple_false = None
        self._dddd_raw_left_simple_true = None
        with contextlib.redirect_stdout(io.StringIO()):
            out = slidercracker.identify_gap_locations(
                background_img_dir=bg_path,
                slider_img_dir=slider_path,
                retain=False,
            )
        x = int(out.get("identify_w", 0))
        if not _plausible_gap_left_x(x, bw, tw):
            raise RuntimeError(
                f"slidercracker 返回缺口 x={x} 不合理（background_w={bw}, slider_w={tw}）"
            )
        self._dddd_raw_left_simple_true = x
        return x

    def _ensemble_auto_best_gap(
        self,
        bw: int,
        tw: int,
        *,
        aliyun_include_opencv_in_ensemble: bool = False,
    ) -> tuple[int, float, str] | None:
        """
        auto 专用：在 dddd 两路与 OpenCV 灰度/Canny（阿里云时尚可有 Alpha-mask、``puzzle-slider-captcha``）的
        plausible 候选中，取**分数最高**者。

        **易盾**：任一路 dddd plausible 时**不**与 OpenCV 比分数；阿里云辅助路仅当
        ``aliyun_include_opencv_in_ensemble`` 为 True 时进入候选。
        **阿里云**：dddd 与各路 OpenCV（含可选 PSC）按同一排序规则比分数决选。
        """
        cand: list[tuple[int, float, str]] = []
        xf, xt = self._dddd_raw_left_simple_false, self._dddd_raw_left_simple_true
        cf, ct = self._dddd_conf_simple_false, self._dddd_conf_simple_true
        if xf is not None and cf is not None and cf >= 0.0 and _plausible_gap_left_x(int(xf), bw, tw):
            cand.append((int(xf), float(cf), "dddd_false"))
        if xt is not None and ct is not None and ct >= 0.0 and _plausible_gap_left_x(int(xt), bw, tw):
            cand.append((int(xt), float(ct), "dddd_true"))
        xg, sg = self._opencv_raw_x_gray, self._opencv_score_gray
        if xg is not None and sg is not None and _plausible_gap_left_x(int(xg), bw, tw):
            cand.append((int(xg), float(sg), "opencv_gray"))
        xc, sc = self._opencv_raw_x_canny, self._opencv_score_canny
        if xc is not None and sc is not None and _plausible_gap_left_x(int(xc), bw, tw):
            cand.append((int(xc), float(sc), "opencv_canny"))
        xa, sa = getattr(self, "_opencv_raw_x_alpha_mask", None), getattr(
            self, "_opencv_score_alpha_mask", None
        )
        if (
            aliyun_include_opencv_in_ensemble
            and xa is not None
            and sa is not None
            and _plausible_gap_left_x(int(xa), bw, tw)
        ):
            cand.append((int(xa), float(sa), "opencv_alpha_mask"))
        xp, sp = getattr(self, "_opencv_raw_x_puzzle_psc", None), getattr(
            self, "_opencv_score_puzzle_psc", None
        )
        if (
            aliyun_include_opencv_in_ensemble
            and xp is not None
            and sp is not None
            and _plausible_gap_left_x(int(xp), bw, tw)
        ):
            cand.append((int(xp), float(sp), "puzzle_psc"))
        xs, ss = getattr(self, "_opencv_raw_x_shadow_ms", None), getattr(
            self, "_opencv_score_shadow_ms", None
        )
        xse, sse = getattr(self, "_opencv_raw_x_shadow_ms_edges", None), getattr(
            self, "_opencv_score_shadow_ms_edges", None
        )
        # aliyun_shadow_ms 使用 TM_CCOEFF_NORMED，数值常落在 ~0.35～0.65，与 dddd 的 confidence、
        # 灰度路 0.1～0.4 不可直接横比；若无脑取 max，会压过「分低但位置更稳」的灰度/Canny（见复盘
        # 日志：shadow x=10/22/4 仍因高分胜出）。故：对 shadow 分数乘权重，且在灰度与 Canny 已分歧时
        # 若 shadow 远离二者中点则剔除该候选。权重可用环境变量 ICGOO_ALIYUN_SHADOW_ENSEMBLE_WEIGHT（默认 0.72）。
        # aliyun_shadow_ms_edges：在同 ROI 上对 Canny 边缘做多尺度 TM（与常见开源阿里云滑块脚本同思路），
        # 权重默认略低于灰度多尺度路，见 ICGOO_ALIYUN_SHADOW_MS_EDGES_ENSEMBLE_WEIGHT。
        skip_shadow = False
        skip_shadow_edges = False
        if (
            xg is not None
            and xc is not None
            and sg is not None
            and sc is not None
            and _plausible_gap_left_x(int(xg), bw, tw)
            and _plausible_gap_left_x(int(xc), bw, tw)
        ):
            ixg, ixc = int(xg), int(xc)
            if abs(ixg - ixc) >= 22:
                mid_gc = (ixg + ixc) // 2
                far = max(36, bw // 14)
                if (
                    xs is not None
                    and ss is not None
                    and _plausible_gap_left_x(int(xs), bw, tw)
                    and abs(int(xs) - mid_gc) > far
                ):
                    skip_shadow = True
                if (
                    xse is not None
                    and sse is not None
                    and _plausible_gap_left_x(int(xse), bw, tw)
                    and abs(int(xse) - mid_gc) > far
                ):
                    skip_shadow_edges = True
        if (
            aliyun_include_opencv_in_ensemble
            and xs is not None
            and ss is not None
            and _plausible_gap_left_x(int(xs), bw, tw)
            and not skip_shadow
        ):
            try:
                w = float((os.environ.get("ICGOO_ALIYUN_SHADOW_ENSEMBLE_WEIGHT") or "0.72").strip())
            except ValueError:
                w = 0.72
            w = max(0.35, min(1.0, w))
            cand.append((int(xs), float(ss) * w, "aliyun_shadow_ms"))
        if (
            aliyun_include_opencv_in_ensemble
            and xse is not None
            and sse is not None
            and _plausible_gap_left_x(int(xse), bw, tw)
            and not skip_shadow_edges
        ):
            try:
                we = float(
                    (os.environ.get("ICGOO_ALIYUN_SHADOW_MS_EDGES_ENSEMBLE_WEIGHT") or "0.62").strip()
                )
            except ValueError:
                we = 0.62
            we = max(0.3, min(1.0, we))
            cand.append((int(xse), float(sse) * we, "aliyun_shadow_ms_edges"))
        if not cand:
            return None
        if not aliyun_include_opencv_in_ensemble:
            # 易盾：任一路 dddd plausible 时只信 dddd，避免 OpenCV 灰度因纹理得更高假峰
            if any(t[2].startswith("dddd") for t in cand):
                cand = [t for t in cand if t[2].startswith("dddd")]
        cand.sort(key=lambda t: t[1], reverse=True)
        return cand[0][0], cand[0][1], cand[0][2]

    def captcha_exists(self, timeout: float = 2.0) -> bool:
        """判断页面是否存在易盾或阿里云滑块验证码。"""
        t_aliyun = min(2.0, timeout) if timeout > 0 else timeout
        if self._aliyun_challenge_active(timeout=t_aliyun):
            return True
        # 与 _no_visible_yidun_panel_means_passed 对齐：仅主文档可锚时仍应认为有验证码
        try:
            if _aliyun_host_context(self.page) is not None:
                return True
        except Exception:
            pass
        try:
            if self.page.ele("css:#aliyunCaptcha-window-embed", timeout=min(0.85, timeout)):
                return True
        except Exception:
            pass
        try:
            ele = self.page.ele(_XP_YIDUN_JIGSAW_IMG, timeout=timeout)
            return ele is not None
        except Exception:
            return False

    def _yidun_jigsaw_src(self) -> str:
        """易盾拼图小块 img 的 src（失败换图后通常会变），用于判断是否仍为同一张拼图。"""
        try:
            el = self.page.ele(_XP_YIDUN_JIGSAW_IMG, timeout=0.85)
            if el:
                return _read_yidun_img_src(el)
        except Exception:
            pass
        return ""

    def _puzzle_piece_src(self) -> str:
        """当前挑战（阿里云或易盾）拼图小块 src，用于滑动前后比对是否换图。"""
        if self._aliyun_challenge_active(timeout=0.5):
            ctx = _aliyun_host_context(self.page) or self.page
            try:
                el = ctx.ele(_XP_ALIYUN_PUZZLE_IMG, timeout=0.85)
                if el:
                    return _read_yidun_img_src(el)
            except Exception:
                pass
            return ""
        return self._yidun_jigsaw_src()

    def get_images(self) -> tuple[str, str, float]:
        """获取滑块和背景图（data: 或 https）；第三项为拖动换算比例（优先 滑轨可行程/图自然宽）。"""
        if self._aliyun_challenge_active(timeout=min(2.0, 1.5)):
            return self._aliyun_get_images()
        referer = (self.page.url or "").strip() or "https://www.icgoo.net/"
        # 阶段 1：等待两个 img 节点出现在 DOM（易盾壳先渲染，图后注入）
        t1 = time.time() + 22.0
        slider_ele = None
        bg_ele = None
        while time.time() < t1:
            try:
                self.page.wait.eles_loaded(_XP_YIDUN_JIGSAW_IMG, timeout=2)
            except Exception:
                pass
            slider_ele = self.page.ele(_XP_YIDUN_JIGSAW_IMG, timeout=2)
            bg_ele = self.page.ele(_XP_YIDUN_BG_IMG, timeout=2)
            if slider_ele and bg_ele:
                break
            time.sleep(0.28)

        if not slider_ele or not bg_ele:
            _yidun_warning("get_images: 超时仍未找到滑块或背景图 img")
            raise ValueError("未找到滑块或背景图 img，请确认验证码已完整弹出")

        # 阶段 2：节点已有后仍可能延迟写入 src（异步 / 懒加载）；单独拉长等待并刷新读取
        t2 = time.time() + 40.0
        slider_src, bg_src = "", ""
        n = 0
        while time.time() < t2:
            try:
                slider_ele = self.page.ele(_XP_YIDUN_JIGSAW_IMG, timeout=1.5)
                bg_ele = self.page.ele(_XP_YIDUN_BG_IMG, timeout=1.5)
            except Exception:
                slider_ele, bg_ele = None, None
            if not slider_ele or not bg_ele:
                time.sleep(0.35)
                continue
            slider_src = _read_yidun_img_src(slider_ele)
            bg_src = _read_yidun_img_src(bg_ele)
            if slider_src and bg_src:
                break
            n += 1
            if n % 5 == 0:
                try:
                    bg_ele.scroll.to_see(center=True)
                    time.sleep(0.15)
                except Exception:
                    pass
            time.sleep(0.35)

        if not slider_src or not bg_src:
            _yidun_warning(
                f"get_images: img 已存在但 src 在 40s 内仍为空 "
                f"(slider={bool(slider_src)} bg={bool(bg_src)})。"
                "若 Chromium 启用了 imagesEnabled=false，验证码图不会加载，请关闭该 blink-settings。"
            )
            raise ValueError("滑块或背景图尚未加载出 src，请稍后重试")

        slider_base64 = _captcha_img_src_to_b64_payload(slider_src, referer)
        bg_base64 = _captcha_img_src_to_b64_payload(bg_src, referer)

        # 易盾常返回 @2x 等资源，图内像素 > 页面上 <img> 的 CSS 宽度；须按比例换算到视口拖动像素
        nat_w = 0
        try:
            raw_bg = _decode_base64_image_payload(bg_base64)
            im_bg = Image.open(BytesIO(raw_bg))
            nat_w = int(im_bg.size[0])
        except Exception:
            nat_w = 0
        drag_scale_x = self._yidun_drag_scale_x(bg_ele, nat_w)

        _yidun_info(
            f"get_images ok: nat_bg_w={nat_w} drag_scale_x={drag_scale_x:.4f} referer={referer[:100]!r}"
        )
        return bg_base64, slider_base64, drag_scale_x

    def _yidun_bg_display_scale_x(self, bg_ele, natural_width: int) -> float:
        """
        背景图在页面上实际显示宽度 / 解码后的图像宽度。
        """
        if natural_width <= 0:
            return 1.0
        try:
            w_disp, _h = bg_ele.rect.size
            if w_disp <= 0:
                return 1.0
            s = w_disp / float(natural_width)
            if s < 0.08 or s > 12.0:
                return 1.0
            return s
        except Exception:
            return 1.0

    def _yidun_drag_scale_x(self, bg_ele, natural_width: int) -> float:
        """
        将图内缺口 x（自然像素）换算为鼠标水平拖动量：优先用 (滑轨宽 - 手柄宽) / 图自然宽。
        易盾手柄占用滑轨宽度，若仅用「背景图显示宽/自然宽」会系统性偏大或偏小。
        """
        if natural_width <= 0:
            return self._yidun_bg_display_scale_x(bg_ele, natural_width)
        try:
            ctrl = self.page.ele('xpath://div[contains(@class,"yidun_control")]', timeout=0.8)
            if not ctrl:
                raise ValueError("no control")
            w_ctrl, _ = ctrl.rect.size
            handle_el = None
            for xp in _XP_YIDUN_SLIDER_CANDIDATES:
                try:
                    handle_el = self.page.ele(xp, timeout=0.35)
                    if handle_el:
                        break
                except Exception:
                    continue
            if not handle_el:
                raise ValueError("no handle")
            w_h, _ = handle_el.rect.size
            travel = float(w_ctrl) - float(w_h)
            if travel < 1.0:
                travel = float(w_ctrl) * 0.85
            s = travel / float(natural_width)
            if 0.04 <= s <= 4.0:
                return s
        except Exception:
            pass
        return self._yidun_bg_display_scale_x(bg_ele, natural_width)

    def save_base64_image(self, base64_str, filename):
        try:
            img_data = _decode_base64_image_payload(base64_str)
        except (binascii.Error, ValueError) as e:
            raise ValueError(
                "滑块图 base64 无效（常见于易盾未完全加载或等待超时后 DOM 不完整）。"
                "请完成验证/限流解除后重试。"
            ) from e
        with open(filename, "wb") as f:
            f.write(img_data)
        return filename

    def detect_gap_position(self, bg_path: str, slider_path: str) -> int:
        """
        滑块缺口 x：按 ddddocr README，``DdddOcr(det=False, ocr=False)`` 后调用
        ``slide_match(滑块字节, 背景字节)``（默认边缘匹配 / ``simple_target=False``），
        无透明底拼图再试 ``simple_target=True``；API 形态见 README「滑块验证码处理」。
        ``_slider_gap_backend`` 为 opencv 时跳过 ddddocr；为 ddddocr 时仅 ddddocr、失败抛错；
        为 slidercracker 时调用 PyPI ``slidercracker``（内部 ``simple_target=True``）；为 auto 时 ddddocr 失败再回退 OpenCV。

        **算法局限（易盾拼图常见）：** 本质是「整块拼图小图」在「带缺口的大图」上做模板匹配，
        相关峰不一定是真实缺口位置（纹理重复、阴影、圆角都会引入假峰）；Canny 阈值固定为 50/150，
        对部分底图不理想。与 ``scripts/test_ddddocr_slide.py`` 一致：两路 ``slide_match`` 左缘相差较大时，
        **不会**仅凭 ``simple_target=True`` 的较高 confidence 定案（该路常虚高但仍错），而优先采信
        ``simple_target=False``（边缘/Canny）；两路置信度均低且相差仍大时仍取中点融合。
        拖动像素还依赖 ``drag_scale_x``（DOM 量轨）与 ``drag_boost``，任一环偏差都会表现为「总偏一点」。
        可配合 ``detect_gap(..., gap_offset_image_px=…)`` 做图像空间微调，或修复 onnx 后对比 ddddocr 官方模型。

        **阿里云**：见 ``icgoo_aliyun_gap`` — 拼块/缺口特征约 52×52 自然像素（
        ``ALIYUN_CAPTCHA_PUZZLE_SIZE_NAT_PX``）；预处理拼图 RGB、OpenCV/dddd 统一读临时 PNG；两路分歧大时
        对**原始**图做多尺度 Canny 共识（中位数）。另：若 ``simple_target=True`` 左缘过靠左而 Canny 路在右侧且
        ``shadow_ms_edges`` 与 Canny 一致，则 ``_aliyun_repair_dddd_small_true_false_peak`` 改采 Canny 左缘。
        ensemble 决选为 dddd 且分数低、与 ``aliyun_shadow_ms`` 分歧大时，``_aliyun_lowconf_dddd_shadow_override``
        可改采 shadow（见环境变量 ``ICGOO_ALIYUN_LOWCONF_SHADOW_*``）。
        """
        self._last_opencv_match_score = None
        self._last_opencv_match_mode = None
        self._opencv_raw_x_gray = None
        self._opencv_raw_x_canny = None
        self._opencv_score_gray = None
        self._opencv_score_canny = None
        self._opencv_raw_x_alpha_mask = None
        self._opencv_score_alpha_mask = None
        self._opencv_raw_x_puzzle_psc = None
        self._opencv_score_puzzle_psc = None
        self._opencv_raw_x_shadow_ms = None
        self._opencv_score_shadow_ms = None
        self._opencv_raw_x_shadow_ms_edges = None
        self._opencv_score_shadow_ms_edges = None
        self._last_gap_backend = ""
        self._dddd_confidence = None
        self._dddd_simple_target = None
        self._dddd_raw_left_simple_false = None
        self._dddd_raw_left_simple_true = None
        self._dddd_conf_simple_false = None
        self._dddd_conf_simple_true = None
        self._slide_match_blended = False
        self._slide_match_diverge_prefer_false = False
        self._gap_ensemble_pick = None
        tmp_aliyun: str | None = None
        try:
            bg_eff, sl_eff, slide_bytes, bg_bytes, tmp_aliyun = aliyun_resolve_detect_inputs(
                bg_path,
                slider_path,
                lambda: self._aliyun_challenge_active(timeout=0.45),
            )
        except OSError:
            return 100

        try:
            tw = int(Image.open(BytesIO(slide_bytes)).size[0])
        except Exception:
            tw = 0
        if tw <= 0:
            tw, _ = _image_natural_wh_from_bytes(slide_bytes)
        if tmp_aliyun is not None:
            # ``aliyun_prepare_match_bytes`` 已 Alpha 紧裁；用预处理 PNG 实际宽做中心→左缘，避免与模板不一致。
            if tw < 36 or tw > 72:
                tw = ALIYUN_CAPTCHA_PUZZLE_WIDTH_NAT_PX
        try:
            im_bg = Image.open(BytesIO(bg_bytes))
            bw, bh = int(im_bg.size[0]), int(im_bg.size[1])
        except Exception:
            bw, bh = 0, 0
        if bw <= 0 or bh <= 0:
            bw, bh = _image_natural_wh_from_bytes(bg_bytes)

        try:
            if self._slider_gap_backend == "opencv":
                self._last_gap_backend = "opencv"
                return self._detect_gap_position_opencv(bg_eff, sl_eff)

            if self._slider_gap_backend == "slidercracker":
                self._fill_opencv_gap_scores(
                    bg_eff,
                    sl_eff,
                    include_aliyun_aux=(tmp_aliyun is not None),
                    original_slider_path=(slider_path if tmp_aliyun else None),
                )
                return self._detect_gap_position_slidercracker(
                    bg_eff, sl_eff, bw=bw, tw=tw
                )

            if self._slider_gap_backend == "auto":
                self._fill_opencv_gap_scores(
                    bg_eff,
                    sl_eff,
                    include_aliyun_aux=(tmp_aliyun is not None),
                    original_slider_path=(slider_path if tmp_aliyun else None),
                )

            ocr = self._get_dddd_slide()
            if ocr is None:
                detail = self._dddd_init_error or "请执行: pip install ddddocr"
                if self._slider_gap_backend == "ddddocr":
                    raise RuntimeError(f"已要求仅用 ddddocr 识别滑块，但无法初始化: {detail}")
                if not self._slide_backend_warned:
                    self._verbose(f"ddddocr 不可用（{detail}），已改用 OpenCV 模板匹配。")
                    _yidun_info(f"detect_gap_position: ddddocr 不可用（{detail}），回退 OpenCV 模板匹配")
                    self._slide_backend_warned = True
                self._last_gap_backend = "opencv"
                return self._detect_gap_position_opencv(bg_eff, sl_eff)

            for simple in (False, True):
                try:
                    # README：slide.slide_match(target_bytes, background_bytes[, simple_target=...])
                    res = ocr.slide_match(slide_bytes, bg_bytes, simple_target=simple)
                except Exception:
                    continue
                left = _dddocr_slide_match_to_left_x(res, tw)
                if left is None or not _plausible_gap_left_x(left, bw, tw):
                    continue
                conf = res.get("confidence")
                try:
                    conf_f = float(conf) if conf is not None else -1.0
                except (TypeError, ValueError):
                    conf_f = -1.0
                if simple:
                    self._dddd_raw_left_simple_true = int(left)
                    self._dddd_conf_simple_true = conf_f
                else:
                    self._dddd_raw_left_simple_false = int(left)
                    self._dddd_conf_simple_false = conf_f

            xf = self._dddd_raw_left_simple_false
            xt = self._dddd_raw_left_simple_true
            cf = self._dddd_conf_simple_false
            ct = self._dddd_conf_simple_true
            xf_ok = xf is not None and _plausible_gap_left_x(xf, bw, tw)
            xt_ok = xt is not None and _plausible_gap_left_x(xt, bw, tw)
            # 两路 dddd plausible 时：易盾仍可仅走 dddd；阿里云恒先跑 ensemble（见 _ddd_auto_skip_opencv_ensemble）
            _auto_use_dddd_only = (
                self._slider_gap_backend == "auto"
                and xf_ok
                and xt_ok
                and _ddd_auto_skip_opencv_ensemble(
                    xf, xt, is_aliyun_preprocessed=(tmp_aliyun is not None)
                )
            )

            if self._slider_gap_backend == "auto" and not _auto_use_dddd_only:
                ens = self._ensemble_auto_best_gap(
                    bw,
                    tw,
                    aliyun_include_opencv_in_ensemble=(tmp_aliyun is not None),
                )
                if ens is not None:
                    left_x, escore, etag = ens
                    self._slide_match_blended = False
                    self._slide_match_diverge_prefer_false = False
                    self._gap_ensemble_pick = etag
                    if tmp_aliyun is not None and xf_ok and xt_ok:
                        rep = _aliyun_repair_dddd_small_true_false_peak(
                            bw=bw,
                            tw=tw,
                            xf=xf,
                            xt=xt,
                            chosen_left=left_x,
                            shadow_ms_edges_x=getattr(
                                self, "_opencv_raw_x_shadow_ms_edges", None
                            ),
                            shadow_ms_edges_score=getattr(
                                self, "_opencv_score_shadow_ms_edges", None
                            ),
                            dddd_conf_false=cf,
                        )
                        if rep is not None:
                            left_x, escore = int(rep[0]), float(rep[1])
                            self._gap_ensemble_pick = "aliyun_repair_prefer_dddd_false"
                            self._slide_match_diverge_prefer_false = True
                            etag = "aliyun_repair_prefer_dddd_false"
                    if tmp_aliyun is not None and etag in ("dddd_true", "dddd_false"):
                        ovr = _aliyun_lowconf_dddd_shadow_override(
                            chosen_left=int(left_x),
                            chosen_etag=str(etag),
                            chosen_score=float(escore),
                            bw=int(bw),
                            tw=int(tw),
                            shadow_ms_x=getattr(self, "_opencv_raw_x_shadow_ms", None),
                            shadow_ms_score=getattr(self, "_opencv_score_shadow_ms", None),
                            shadow_ms_edges_x=getattr(
                                self, "_opencv_raw_x_shadow_ms_edges", None
                            ),
                            shadow_ms_edges_score=getattr(
                                self, "_opencv_score_shadow_ms_edges", None
                            ),
                        )
                        if ovr is not None:
                            left_x, escore, etag = int(ovr[0]), float(ovr[1]), str(ovr[2])
                            self._gap_ensemble_pick = etag
                    if etag in (
                        "opencv_gray",
                        "opencv_canny",
                        "opencv_alpha_mask",
                        "puzzle_psc",
                        "aliyun_shadow_ms",
                        "aliyun_shadow_ms_edges",
                        "aliyun_lowconf_shadow_ms",
                        "aliyun_lowconf_shadow_ms_edges",
                    ):
                        self._last_gap_backend = "opencv"
                        if etag == "opencv_gray":
                            self._last_opencv_match_mode = "gray"
                        elif etag == "opencv_canny":
                            self._last_opencv_match_mode = "canny"
                        elif etag == "opencv_alpha_mask":
                            self._last_opencv_match_mode = "alpha_mask"
                        elif etag == "puzzle_psc":
                            self._last_opencv_match_mode = "puzzle_psc"
                        elif etag in ("aliyun_shadow_ms_edges", "aliyun_lowconf_shadow_ms_edges"):
                            self._last_opencv_match_mode = "shadow_ms_edges"
                        else:
                            self._last_opencv_match_mode = "shadow_ms"
                        self._last_opencv_match_score = escore
                        self._dddd_simple_target = None
                    else:
                        self._last_gap_backend = "ddddocr_shim" if self._slide_match_is_shim else "ddddocr"
                        self._dddd_simple_target = etag == "dddd_true"
                    self._dddd_confidence = escore
                    return int(left_x)

            if xf_ok or xt_ok:
                left_x: int
                simple_for_meta: bool | None
                conf_f: float | None
                self._slide_match_blended = False
                self._slide_match_diverge_prefer_false = False

                if xf_ok and xt_ok:
                    delta = abs(int(xf) - int(xt))
                    diag = math.hypot(float(bw), float(bh)) if bw > 0 and bh > 0 else 0.0
                    diverge_thresh = max(
                        _SLIDE_MATCH_SIMPLE_DIVERGE_MIN_PX,
                        _SLIDE_MATCH_SIMPLE_DIVERGE_FRAC_DIAG * diag,
                    )
                    # 任一路左缘 <2 与 _plausible_gap_left_x 一致视为无效峰；显式禁止参与中点（避免 0 等脏值拉偏）
                    if (
                        cf is not None
                        and ct is not None
                        and int(xf) >= 2
                        and int(xt) >= 2
                        and delta >= _SLIDE_MATCH_BLEND_MIN_DELTA_PX
                        and cf < _SLIDE_MATCH_BLEND_MAX_CONF
                        and ct < _SLIDE_MATCH_BLEND_MAX_CONF
                    ):
                        left_x = (int(xf) + int(xt)) // 2
                        self._slide_match_blended = True
                        simple_for_meta = None
                        conf_f = max(cf, ct)
                    elif delta >= diverge_thresh:
                        # 与 test_ddddocr_slide：分歧大时 simple=True 的 confidence 常虚高，优先 Canny 路
                        left_x = int(xf)
                        simple_for_meta = False
                        conf_f = cf if cf is not None else -1.0
                        self._slide_match_diverge_prefer_false = True
                        if tmp_aliyun is not None:
                            cons = aliyun_try_consensus_median(
                                bg_path, slider_path, bw, tw, int(xf), int(xt)
                            )
                            if cons is not None:
                                left_x = cons
                                self._gap_ensemble_pick = "aliyun_cv_consensus"
                                self._slide_match_diverge_prefer_false = False
                    else:
                        ctn = ct if ct is not None else -1.0
                        cfn = cf if cf is not None else -1.0
                        if ctn > cfn:
                            left_x, simple_for_meta, conf_f = int(xt), True, ct
                        else:
                            left_x, simple_for_meta, conf_f = int(xf), False, cf
                elif xf_ok:
                    left_x, simple_for_meta, conf_f = int(xf), False, cf
                else:
                    left_x, simple_for_meta, conf_f = int(xt), True, ct

                if tmp_aliyun is not None and xf_ok and xt_ok:
                    rep2 = _aliyun_repair_dddd_small_true_false_peak(
                        bw=bw,
                        tw=tw,
                        xf=xf,
                        xt=xt,
                        chosen_left=left_x,
                        shadow_ms_edges_x=getattr(
                            self, "_opencv_raw_x_shadow_ms_edges", None
                        ),
                        shadow_ms_edges_score=getattr(
                            self, "_opencv_score_shadow_ms_edges", None
                        ),
                        dddd_conf_false=cf,
                    )
                    if rep2 is not None:
                        left_x = int(rep2[0])
                        conf_f = float(rep2[1])
                        simple_for_meta = False
                        self._gap_ensemble_pick = "aliyun_repair_prefer_dddd_false"
                        self._slide_match_diverge_prefer_false = True

                if (
                    tmp_aliyun is not None
                    and not self._slide_match_blended
                    and self._gap_ensemble_pick is None
                    and simple_for_meta is not None
                ):
                    te = "dddd_true" if simple_for_meta else "dddd_false"
                    ovr2 = _aliyun_lowconf_dddd_shadow_override(
                        chosen_left=int(left_x),
                        chosen_etag=te,
                        chosen_score=float(conf_f if conf_f is not None else -1.0),
                        bw=int(bw),
                        tw=int(tw),
                        shadow_ms_x=getattr(self, "_opencv_raw_x_shadow_ms", None),
                        shadow_ms_score=getattr(self, "_opencv_score_shadow_ms", None),
                        shadow_ms_edges_x=getattr(
                            self, "_opencv_raw_x_shadow_ms_edges", None
                        ),
                        shadow_ms_edges_score=getattr(
                            self, "_opencv_score_shadow_ms_edges", None
                        ),
                    )
                    if ovr2 is not None:
                        left_x = int(ovr2[0])
                        conf_f = float(ovr2[1])
                        self._gap_ensemble_pick = str(ovr2[2])
                        simple_for_meta = None

                gp = self._gap_ensemble_pick
                if gp in ("aliyun_lowconf_shadow_ms", "aliyun_lowconf_shadow_ms_edges"):
                    self._last_gap_backend = "opencv"
                    self._last_opencv_match_mode = (
                        "shadow_ms_edges" if str(gp).endswith("_edges") else "shadow_ms"
                    )
                    self._last_opencv_match_score = (
                        float(conf_f) if conf_f is not None else None
                    )
                    self._dddd_simple_target = None
                else:
                    self._last_gap_backend = (
                        "ddddocr_shim" if self._slide_match_is_shim else "ddddocr"
                    )
                    self._dddd_simple_target = simple_for_meta
                self._dddd_confidence = (
                    float(conf_f) if conf_f is not None and float(conf_f) >= 0.0 else None
                )
                return int(left_x)

            if self._slider_gap_backend == "ddddocr":
                raise RuntimeError(
                    "已要求仅用 ddddocr：slide_match（simple_target=False/True）均未得到有效缺口坐标，"
                    "请检查滑块/背景图或升级 ddddocr。"
                )
            if not self._slide_backend_warned:
                self._verbose("ddddocr.slide_match 未得到有效结果，已改用 OpenCV 模板匹配。")
                _yidun_info("detect_gap_position: slide_match 无有效结果，回退 OpenCV 模板匹配")
                self._slide_backend_warned = True
            self._last_gap_backend = "opencv"
            return self._detect_gap_position_opencv(bg_eff, sl_eff)
        finally:
            if tmp_aliyun:
                shutil.rmtree(tmp_aliyun, ignore_errors=True)

    def detect_gap(
        self,
        bg_path: str,
        slider_path: str,
        drag_scale_x: float = 1.0,
        drag_boost: float = 1.0,
        gap_offset_image_px: int = 0,
        drag_extra_px: int = _DEFAULT_DRAG_EXTRA_PX,
    ) -> tuple[int, int]:
        """
        返回 (用于换算的图内 x 像素, 折合页面拖动像素)。
        drag_scale_x 为 get_images 第三项；drag_boost 为经验补偿（略偏小时可 >1）。
        gap_offset_image_px 在识别结果上叠加（图像自然像素），用于系统性偏左/偏右时手工校准。
        drag_extra_px 在 ``round(raw_x * scale * boost)`` 之后累加（页面拖动像素），与比例无关。
        当前为 **阿里云** 挑战时，另叠加 ``get_aliyun_systematic_drag_extra_px()``（默认约 +20px，可环境变量覆盖）；
        并对 ``drag_scale_x`` 乘 ``get_aliyun_drag_scale_mult()``（默认约 1.02）；最后再按图内 ``raw_x`` 做
        ``aliyun_raw_x_segment_drag_adjust``（分段乘子与加量，见 ``icgoo_aliyun_captcha``）。**易盾**不叠加、不乘、不分段。
        """
        raw_detected = self.detect_gap_position(bg_path, slider_path)
        self._gap_raw_detected = int(raw_detected)
        raw_x = int(raw_detected) + int(gap_offset_image_px)
        if raw_x < 1:
            raw_x = 1
        is_aliyun = self._aliyun_challenge_active(timeout=0.45)
        self._aliyun_raw_x_seg_mult = None
        self._aliyun_raw_x_seg_extra_px = None
        self._aliyun_raw_x_seg_label = None
        s = drag_scale_x if drag_scale_x and drag_scale_x > 0 else 1.0
        if is_aliyun:
            s *= get_aliyun_drag_scale_mult()
        b = drag_boost if drag_boost and drag_boost > 0 else 1.0
        aliyun_adj = get_aliyun_systematic_drag_extra_px() if is_aliyun else 0
        self._aliyun_drag_extra_applied = aliyun_adj
        self._effective_drag_extra_px = int(drag_extra_px) + aliyun_adj
        extra = self._effective_drag_extra_px
        drag_x = max(1, int(round(raw_x * s * b)) + extra)
        if is_aliyun:
            sm, se, lbl = aliyun_raw_x_segment_drag_adjust(raw_x)
            self._aliyun_raw_x_seg_mult = sm
            self._aliyun_raw_x_seg_extra_px = se
            self._aliyun_raw_x_seg_label = lbl
            drag_x = max(1, int(round(drag_x * sm)) + se)
        return raw_x, drag_x

    def get_tracks(self, distance: int) -> list[int]:
        """
        生成拟人化滑动轨迹；各步之和尽量接近期望 distance（易盾对落点敏感，避免末尾额外偏移）。
        """
        tracks = []
        real_distance = max(1, int(distance))
        if real_distance < 5:
            real_distance = max(1, distance)

        mid = int(real_distance * 0.7)
        current = 0
        v = 0.0
        t = 0.2

        while current < real_distance:
            if current < mid:
                a = random.uniform(1.5, 2.5)
            else:
                a = random.uniform(-2.5, -1.5)
            v += a * t
            if v < 0:
                v = 0.0
            move = v * t + 0.5 * a * t * t
            step = int(round(move))
            if step < 1:
                step = 1
            if random.random() < 0.08:
                step += random.randint(-1, 1)
            step = max(1, step)
            tracks.append(step)
            current += step
            if current > real_distance + 5:
                break

        overshoot = current - real_distance
        if overshoot > 0:
            tracks.append(-overshoot)

        # 保证水平位移总和等于 distance（修正整数步误差）
        total = sum(tracks)
        fix = real_distance - total
        if fix != 0:
            tracks.append(fix)

        return tracks

    def _find_yidun_drag_handle(self, timeout_each: float = 1.5):
        """定位可拖动节点：优先阿里云手柄，其次易盾。"""
        if self._aliyun_challenge_active(timeout=0.35):
            ctx = _aliyun_host_context(self.page) or self.page
            try:
                el = ctx.ele(_XP_ALIYUN_SLIDER_HANDLE, timeout=timeout_each)
                if el:
                    return el
            except Exception:
                pass
        for xp in _XP_YIDUN_SLIDER_CANDIDATES:
            try:
                el = self.page.ele(xp, timeout=timeout_each)
                if el:
                    return el
            except Exception:
                continue
        return None

    def _prepare_yidun_drag_handle_for_actions(self, retries: int = 6):
        """
        多次查找手柄并确认 rect 有效；滚动到视区内，降低 DrissionPage「该元素没有位置及大小」概率。
        """
        for _ in range(retries):
            el = self._find_yidun_drag_handle(timeout_each=1.2)
            if not el:
                time.sleep(0.35)
                continue
            try:
                w, h = el.rect.size
                if w >= 3 and h >= 3:
                    try:
                        el.scroll.to_see(center=True)
                        time.sleep(0.15)
                    except Exception:
                        pass
                    w2, h2 = el.rect.size
                    if w2 >= 3 and h2 >= 3:
                        return el
            except Exception:
                pass
            time.sleep(0.4)
        return None

    def _wait_yidun_slider_ready(self, total_sec: float = 14.0) -> bool:
        """拼图 img 已有时，手柄可能晚一点才可点；轮询等待避免误判为「找不到滑块」。"""
        deadline = time.time() + total_sec
        while time.time() < deadline:
            el = self._prepare_yidun_drag_handle_for_actions(retries=2)
            if el:
                return True
            try:
                if self._aliyun_challenge_active(timeout=0.3):
                    actx = _aliyun_host_context(self.page) or self.page
                    actx.wait.eles_loaded(_XP_ALIYUN_PUZZLE_IMG, timeout=1.2)
                else:
                    self.page.wait.eles_loaded(_XP_YIDUN_JIGSAW_IMG, timeout=1.2)
            except Exception:
                pass
            time.sleep(0.5)
        return False

    def _aliyun_arm_after_fail_same_image_or_src_churn(self, *, short_settle: bool) -> None:
        """
        阿里云：同图继续试拖前，将滑轨滚入视口、短停、可选鼠标微移，并轮询手柄就绪，
        降低验证失败后手柄短暂无 rect 导致下一轮误判「无滑块」的概率。
        ``short_settle=True``：URL 变、指纹仍同图（仅短停）；``False``：src 字符串未变。
        """
        try:
            here = (_aliyun_host_context(self.page) is not None) or self._aliyun_challenge_active(
                timeout=0.35
            )
            if not here:
                return
        except Exception:
            return
        try:
            ctx = _aliyun_host_context(self.page) or self.page
            track = ctx.ele(_XP_ALIYUN_SLIDING_BODY, timeout=1.0)
            if track:
                track.scroll.to_see(center=True)
                time.sleep(0.1 + random.uniform(0, 0.16))
        except Exception:
            pass
        if short_settle:
            sec = aliyun_fail_src_churn_retry_settle_sec()
        else:
            sec = aliyun_fail_same_image_retry_settle_sec()
        if sec > 0:
            time.sleep(random.uniform(sec * 0.82, sec * 1.22))
        if not short_settle and aliyun_fail_same_image_retry_mouse_wiggle_enabled():
            try:
                ac = self.page.actions
                ac.move(
                    float(random.randint(-22, 22)),
                    float(random.randint(-14, 14)),
                    duration=random.uniform(0.06, 0.14),
                )
                time.sleep(random.uniform(0.05, 0.14))
            except Exception:
                pass
        if not short_settle:
            wsec = aliyun_fail_same_image_retry_wait_slider_sec()
            if wsec > 0:
                self._wait_yidun_slider_ready(wsec)

    def _find_yidun_refresh(self, timeout_each: float = 1.0):
        if self._aliyun_challenge_active(timeout=0.35):
            ctx = _aliyun_host_context(self.page) or self.page
            try:
                el = ctx.ele(_XP_ALIYUN_REFRESH, timeout=timeout_each)
                if el:
                    return el
            except Exception:
                pass
        for xp in _XP_YIDUN_REFRESH_CANDIDATES:
            try:
                el = self.page.ele(xp, timeout=timeout_each)
                if el:
                    return el
            except Exception:
                continue
        return None

    def _no_visible_yidun_panel_means_passed(self) -> bool:
        """
        「验证码相关节点仍在 DOM，但页面上没有可见的 .yidun 面板」在 **网易易盾** 收起后
        常表示业务已放行（残留隐藏节点）。

        **阿里云** 宿主若在 DOM 中，不得以「无可见 .yidun」冒充通过（surf 对阿里云恒为假）；
        须由 ``aliyun_verification_success_visible`` / 图层 ``False`` 等路径认定。

        若阿里云在 **跨域 iframe** 内，主文档可能一时找不到 ``_aliyun_host_context``，但 HTML 或
        主文档上的 ``#aliyunCaptcha-window-embed`` 仍可锚定——此时 **不得** 因 ``surf is False`` 误判为已通过。
        """
        if _aliyun_host_context(self.page) is not None:
            return False
        if self._aliyun_challenge_active(timeout=0.3):
            return False
        try:
            if self.page.ele("css:#aliyunCaptcha-window-embed", timeout=0.35):
                return False
        except Exception:
            pass
        try:
            low = (self.page.html or "").lower()
            if "aliyuncaptcha" in low or "aliyuncaptcha-window-embed" in low:
                return False
        except Exception:
            pass
        surf = self._yidun_challenge_surface_visible_js()
        return surf is False

    def _slide_passed_or_captcha_gone(self) -> tuple[bool, str]:
        """
        是否已无需再拖：易盾成功态出现，或拼图 img 已不在 DOM（常见于上一滑已通过但
        check_success 略晚、或 UI 收起瞬间手柄先消失）。
        返回 (True, 说明文案) 表示应直接视为成功退出 slide_verification。
        """
        try:
            ax = aliyun_verification_success_visible(self.page)
            if ax is True:
                return True, "阿里云验证码 UI 认定通过"
        except Exception:
            pass
        if self._yidun_success_visible():
            return True, "检测到易盾已通过态"
        try:
            jp = self._yidun_passed_js_probe()
            if jp is True:
                return True, "JS DOM 探测认定易盾已通过（成功态/文案/拼图块不可见）"
        except Exception:
            pass
        try:
            if not self.captcha_exists(timeout=0.45):
                if aliyun_captcha_cleared_for_success(self.page):
                    return True, "拼图验证码已从页面消失"
        except Exception:
            pass
        try:
            if self.captcha_exists(timeout=0.35):
                if self._no_visible_yidun_panel_means_passed():
                    return True, "易盾仍在 DOM 但页面无可见滑块层（业务已收起、数据已加载时常出现）"
        except Exception:
            pass
        return False, ""

    def slide_verification(
        self,
        distance,
        max_attempts: int = _DEFAULT_SLIDE_ATTEMPTS,
        distance_alt: int | None = None,
        *,
        captcha_round: int = 0,
        after_slide_attempt: Callable[
            [object, int, int, float, list[int], bool],
            None,
        ]
        | None = None,
        after_slide_release: Callable[
            [object, int, int, float, list[int], int],
            None,
        ]
        | None = None,
    ) -> tuple[bool, bool]:
        """
        在同一张验证码上多次滑动：微调距离试探。
        distance_alt 若给出且与 distance 不同，则奇数次用主基准、偶数次用辅基准
        （应对 OpenCV 灰度/Canny 或 slide_match 边缘/灰度模板两路 x 不一致）。

        ``after_slide_release`` 在 ``release()`` 之后、**阿里云 decoy 点击（若开启）之前**、校验前长等待
        **之前**调用（最后一项参数为 ``captcha_round``）。避免 decoy 先点导致拼图 DOM 回弹，使松手截图/
        ``release_*`` 几何与 JSONL 残差失真。

        返回 ``(是否验证成功, 是否已换图/刷新页)``。
        第二项为 True 时拼图已非当前 distance 对应的那一张，**必须**重新 get_images + detect_gap，
        不得继续用旧拖动距离。

        易盾在**验证失败后常会直接下发新拼图**（与手动点刷新类似）。每次失败后比对拼图 ``src``，
        若已变化则第二项返回 True，避免仍按「同一张图」换系数空滑。
        """
        factors = _SLIDE_DISTANCE_FACTORS
        use_alt = distance_alt is not None and distance_alt > 0 and distance_alt != distance
        for attempt in range(max_attempts):
            _reset_slide_release_calibration_fields(self)
            try:
                ok_early, why = self._slide_passed_or_captcha_gone()
                if ok_early:
                    self._verbose(f"{why}，不再寻找滑块手柄（视为验证成功）。")
                    _yidun_info(f"slide_verification: 无需拖动即通过 ({why})")
                    self._emit_calibration(
                        {
                            "event": "slide_early_pass",
                            "captcha_round": int(captcha_round),
                            "attempt_index": int(attempt),
                            "reason": (why or "")[:240],
                        }
                    )
                    return True, False

                f = factors[attempt] if attempt < len(factors) else 1.0
                base = distance_alt if (use_alt and attempt % 2 == 1) else distance
                base_tag = "辅" if base == distance_alt and use_alt else "主"
                d_try = max(1, int(round(base * f)))
                src_before = self._puzzle_piece_src()
                self._verbose(
                    f"第{attempt + 1}次尝试滑动（{base_tag}基准，目标位移≈{d_try}px，系数{f:.2f}）…"
                )
                slider = self._prepare_yidun_drag_handle_for_actions()
                if not slider:
                    ok_early, why = self._slide_passed_or_captcha_gone()
                    if ok_early:
                        self._verbose(f"{why}，跳过等待手柄（视为验证成功）。")
                        _yidun_info(f"slide_verification: 无手柄仍判定通过 ({why})")
                        self._emit_calibration(
                            {
                                "event": "slide_early_pass",
                                "captcha_round": int(captcha_round),
                                "attempt_index": int(attempt),
                                "reason": (why or "")[:240],
                                "phase": "no_handle_immediate",
                            }
                        )
                        return True, False
                    self._verbose("未立即找到滑块手柄，等待易盾控件与手柄渲染…")
                    if self._wait_yidun_slider_ready(12.0):
                        slider = self._prepare_yidun_drag_handle_for_actions(retries=10)
                if not slider:
                    ok_early, why = self._slide_passed_or_captcha_gone()
                    if ok_early:
                        self._verbose(f"{why}，等待结束仍无手柄（视为验证成功，不点刷新）。")
                        _yidun_info(f"slide_verification: 等待后无手柄仍判定通过 ({why})")
                        self._emit_calibration(
                            {
                                "event": "slide_early_pass",
                                "captcha_round": int(captcha_round),
                                "attempt_index": int(attempt),
                                "reason": (why or "")[:240],
                                "phase": "no_handle_after_wait",
                            }
                        )
                        return True, False
                    self._verbose(
                        "等待后仍无手柄：仅点击验证码「刷新」换拼图（避免整页 refresh 导致连接断开）。"
                    )
                    _yidun_warning("slide_verification: 无滑块手柄，点击刷新换图")
                    refresh_btn = self._find_yidun_refresh()
                    if refresh_btn:
                        try:
                            refresh_btn.click()
                        except Exception as ex:
                            self._verbose(f"点击验证码刷新失败: {ex}")
                            _yidun_warning(f"验证码刷新按钮点击失败: {ex}")
                    time.sleep(2.8)
                    self._emit_calibration(
                        {
                            "event": "slide_no_handle_refresh",
                            "captcha_round": int(captcha_round),
                            "attempt_index": int(attempt),
                        }
                    )
                    return False, True

                is_aliyun_slide = False
                human_slow = False
                try:
                    is_aliyun_slide = (_aliyun_host_context(self.page) is not None) or self._aliyun_challenge_active(
                        timeout=0.25
                    )
                    if is_aliyun_slide:
                        human_slow = aliyun_slide_human_slow_enabled()
                except Exception:
                    pass

                ac = self.page.actions
                ac.hold(slider)
                time.sleep(random.uniform(0.12, 0.28))

                tracks = self.get_tracks(d_try)
                move_dur = random.uniform(0.034, 0.052) if human_slow else 0.02
                step_lo, step_hi = (0.014, 0.032) if human_slow else (0.008, 0.018)
                pre_release_lo, pre_release_hi = (
                    (0.22, 0.48) if human_slow else (0.12, 0.28)
                )
                post_release_lo, post_release_hi = (
                    (2.45, 3.35) if human_slow else (2.0, 2.75)
                )

                y_jitter_on = bool(is_aliyun_slide and aliyun_slide_y_jitter_enabled())
                y_state = aliyun_slide_y_jitter_new_state()
                sum_abs_dy = 0

                for track in tracks:
                    if track != 0:
                        dy = 0
                        if y_jitter_on:
                            dy = aliyun_slide_y_jitter_step(
                                y_state, human_slow=human_slow
                            )
                            sum_abs_dy += abs(int(dy))
                        ac.move(track, dy, duration=move_dur)
                    time.sleep(random.uniform(step_lo, step_hi))

                if is_aliyun_slide:
                    self._cal_aliyun_y_jitter_bundle = {
                        "aliyun_y_jitter_enabled": y_jitter_on,
                        "aliyun_y_jitter_sum_abs_dy": int(sum_abs_dy),
                        "aliyun_y_jitter_cap_px": float(aliyun_slide_y_jitter_cap_px()),
                        "aliyun_y_jitter_sigma_px": float(
                            aliyun_slide_y_jitter_sigma_px()
                        ),
                        "aliyun_y_jitter_human_slow": bool(human_slow),
                    }

                time.sleep(random.uniform(pre_release_lo, pre_release_hi))
                if is_aliyun_slide:
                    extra_hold = aliyun_slide_pre_release_extra_hold_sec()
                    if extra_hold > 0:
                        time.sleep(extra_hold)
                ac.release()
                if after_slide_release is not None:
                    try:
                        after_slide_release(
                            self, attempt, d_try, f, tracks, int(captcha_round)
                        )
                    except Exception:
                        pass
                if is_aliyun_slide and aliyun_post_release_decoy_click_enabled():
                    try:
                        aliyun_post_release_decoy_click(self.page)
                    except Exception:
                        pass
                time.sleep(random.uniform(post_release_lo, post_release_hi))

                verify_note = ""
                ok = self.check_success()
                if not ok:
                    # 易盾/Vue/ICGOO 偶在轮询结束后才挂 success、撤层或仅保留隐藏 DOM；分段延迟复核
                    time.sleep(2.0)
                    late_ok, late_why = self._slide_passed_or_captcha_gone()
                    if late_ok:
                        ok = True
                        verify_note = f"late1:{(late_why or '')[:100]}"
                        self._verbose(f"首轮轮询未命中，延迟复核：{late_why}，视为验证成功。")
                    if not ok:
                        time.sleep(2.5)
                        late_ok2, late_why2 = self._slide_passed_or_captcha_gone()
                        if late_ok2:
                            ok = True
                            verify_note = f"late2:{(late_why2 or '')[:100]}"
                            self._verbose(f"二次延迟复核：{late_why2}，视为验证成功。")
                tracks_sum = sum(tracks)
                if after_slide_attempt is not None:
                    try:
                        after_slide_attempt(self, attempt, d_try, f, tracks, ok)
                    except Exception:
                        pass
                if ok:
                    self._verbose("验证成功！")
                    _yidun_info(
                        f"slide_verification: 拖动成功 attempt={attempt + 1} d_try={d_try} factor={f:.3f}"
                    )
                    self._emit_calibration(
                        build_slide_attempt_calibration_record(
                            captcha_round=captcha_round,
                            attempt_index=attempt,
                            factor=f,
                            base_tag=base_tag,
                            base_px=int(base),
                            d_try=d_try,
                            tracks_sum=tracks_sum,
                            ok=True,
                            outcome="success",
                            verify_note=verify_note,
                            src_before_key=aliyun_puzzle_src_compare_key(src_before) or "",
                            src_after_key="",
                            puzzle_replaced=False,
                            solver=self,
                        )
                    )
                    return True, False

                # 阿里云：松手后拼图 img.src 可能短暂处于过渡态；过早读 src_after 易与指纹复核打架。
                post_wait = 0.9 if attempt < max_attempts - 1 else 0.65
                try:
                    if (_aliyun_host_context(self.page) is not None) or self._aliyun_challenge_active(
                        timeout=0.3
                    ):
                        post_wait += 0.5
                except Exception:
                    pass
                time.sleep(post_wait)
                src_after = self._puzzle_piece_src()
                kb = aliyun_puzzle_src_compare_key(src_before)
                ka = aliyun_puzzle_src_compare_key(src_after)
                if kb and ka and kb != ka:
                    # 阿里云：松手后可能先更新拼图资源 URL，成功文案 / 弹层收起略晚于换 src；
                    # 若立即判「换题失败」会漏掉其实已经通过的一轮（用户可见已对准缺口）。
                    resolved_after_src_change = False
                    verify_delayed = ""
                    try:
                        aliyun_here = (_aliyun_host_context(self.page) is not None) or (
                            self._aliyun_challenge_active(timeout=0.35)
                        )
                        if aliyun_here:
                            for tag, sec in (("delay_a", 1.15), ("delay_b", 1.85)):
                                time.sleep(sec)
                                lo, lw = self._slide_passed_or_captcha_gone()
                                if lo:
                                    resolved_after_src_change = True
                                    verify_delayed = f"{tag}:{lw}"
                                    break
                                if self.check_success():
                                    resolved_after_src_change = True
                                    verify_delayed = f"{tag}:check_success"
                                    break
                    except Exception:
                        pass
                    if resolved_after_src_change:
                        self._verbose(
                            "阿里云：拼图资源 URL 已更新，但延迟复核认定验证成功（成功态晚于换图，不按换题处理）。"
                        )
                        _yidun_info(
                            "slide_verification: Aliyun puzzle src changed but delayed pass "
                            f"({verify_delayed[:120]!r})"
                        )
                        self._emit_calibration(
                            build_slide_attempt_calibration_record(
                                captcha_round=captcha_round,
                                attempt_index=attempt,
                                factor=f,
                                base_tag=base_tag,
                                base_px=int(base),
                                d_try=d_try,
                                tracks_sum=tracks_sum,
                                ok=True,
                                outcome="success",
                                verify_note=verify_delayed,
                                src_before_key=kb,
                                src_after_key=ka,
                                puzzle_replaced=False,
                                solver=self,
                            )
                        )
                        return True, False

                    # 阿里云：失败后 DOM 可能改写 src（路径/query/data 重编码）但仍是同一拼图；仅字符串键不同
                    # 时误判换题会导致每轮只试 1 个系数就重识别。用图像字节指纹二次确认。
                    aliyun_src_churn_same_image = False
                    try:
                        referer_fp = (self.page.url or "").strip() or "https://www.icgoo.net/"
                        aliyun_here = (_aliyun_host_context(self.page) is not None) or (
                            self._aliyun_challenge_active(timeout=0.35)
                        )
                        if aliyun_here:
                            fb = aliyun_puzzle_image_fingerprint(src_before, referer_fp)
                            fa = aliyun_puzzle_image_fingerprint(src_after, referer_fp)
                            if fb and fa and fb == fa:
                                aliyun_src_churn_same_image = True
                    except Exception:
                        pass
                    if aliyun_src_churn_same_image:
                        self._verbose(
                            "阿里云：拼图地址与滑动前不同，但图像指纹一致，按同一张图继续试拖（非换题）。"
                        )
                        _yidun_info(
                            "slide_verification: Aliyun puzzle src key changed but image fingerprint "
                            "matches, same challenge"
                        )
                        self._emit_calibration(
                            build_slide_attempt_calibration_record(
                                captcha_round=captcha_round,
                                attempt_index=attempt,
                                factor=f,
                                base_tag=base_tag,
                                base_px=int(base),
                                d_try=d_try,
                                tracks_sum=tracks_sum,
                                ok=False,
                                outcome="fail_same_image",
                                verify_note="aliyun_src_churn_same_fingerprint",
                                src_before_key=kb,
                                src_after_key=ka,
                                puzzle_replaced=False,
                                solver=self,
                            )
                        )
                        if attempt < max_attempts - 1:
                            self._aliyun_arm_after_fail_same_image_or_src_churn(short_settle=True)
                        continue
                    else:
                        self._verbose("服务端未返回成功（校验接口或页面未进入通过态）。")
                        self._verbose(
                            "同时检测到拼图小块已更换（滑动后资源与滑动前不同），将重新拉图并识别缺口。"
                        )
                        _yidun_info(
                            "slide_verification: server not ok, puzzle src changed, re-identify gap"
                        )
                        self._emit_calibration(
                            build_slide_attempt_calibration_record(
                                captcha_round=captcha_round,
                                attempt_index=attempt,
                                factor=f,
                                base_tag=base_tag,
                                base_px=int(base),
                                d_try=d_try,
                                tracks_sum=tracks_sum,
                                ok=False,
                                outcome="fail_new_image",
                                src_before_key=kb,
                                src_after_key=ka,
                                puzzle_replaced=True,
                                solver=self,
                            )
                        )
                        return False, True
                if attempt < max_attempts - 1:
                    self._verbose("服务端未返回成功；拼图小块未换题，将同图换距离重试…")
                self._emit_calibration(
                    build_slide_attempt_calibration_record(
                        captcha_round=captcha_round,
                        attempt_index=attempt,
                        factor=f,
                        base_tag=base_tag,
                        base_px=int(base),
                        d_try=d_try,
                        tracks_sum=tracks_sum,
                        ok=False,
                        outcome="fail_same_image",
                        src_before_key=kb,
                        src_after_key=ka,
                        puzzle_replaced=False,
                        solver=self,
                    )
                )
                if attempt < max_attempts - 1:
                    self._aliyun_arm_after_fail_same_image_or_src_churn(short_settle=False)

            except Exception as e:
                msg = str(e).strip() or repr(e)
                self._verbose(f"滑动过程中出错: {msg}")
                _yidun_warning(f"slide_verification: 滑动异常 attempt={attempt + 1} {type(e).__name__}: {msg}")
                low = msg.lower()
                rect_lost = (
                    "没有位置" in msg
                    or "没有大小" in msg
                    or "位置及大小" in msg
                    or "has_rect" in low
                    or "no rect" in low
                )
                if rect_lost and attempt < max_attempts - 1:
                    self._verbose("手柄暂时无有效尺寸，等待后同图重试（不刷新页面）…")
                    time.sleep(0.85)
                    continue
                disconnect = (
                    "连接已断开" in msg
                    or ("断开" in msg and "连接" in msg)
                    or "disconnected" in low
                )
                if disconnect:
                    self._verbose("检测到与浏览器连接已断开，不再执行页面刷新。")
                    _yidun_error(f"slide_verification: 浏览器连接断开 {msg}")
                    raise IcgooBrowserDisconnectedError(msg) from e
                self._verbose("非常规错误，已刷新页面；将重新拉取拼图并识别缺口。")
                _yidun_warning(f"slide_verification: 非常规错误将整页刷新: {msg}")
                try:
                    self.page.refresh()
                    time.sleep(3)
                except Exception as ex:
                    self._verbose(f"刷新页面失败: {ex}")
                    _yidun_warning(f"整页 refresh 失败: {ex}")
                self._emit_calibration(
                    {
                        "event": "slide_exception_refresh",
                        "captcha_round": int(captcha_round),
                        "attempt_index": int(attempt),
                        "error": msg[:300],
                    }
                )
                return False, True

        self._verbose(f"经过{max_attempts}次尝试均未通过验证（同一张拼图，未换图）")
        _yidun_warning(
            f"slide_verification: 同图已试满 {max_attempts} 次仍未通过，captcha_replaced=False"
        )
        self._emit_calibration(
            {
                "event": "slide_exhausted_same_image",
                "captcha_round": int(captcha_round),
                "max_attempts": int(max_attempts),
            }
        )
        return False, False

    def _yidun_passed_js_probe(self) -> bool | None:
        """
        在浏览器文档（含同源 iframe）内判断是否已通过滑块验证。
        ``True``：出现 success 类、tips 成功文案、或 DOM 仍有 jigsaw 节点但 **computed 下不可见**
        （易盾通过后常保留 img，仅 ``display:none``，导致 ``page.ele`` 仍命中、``captcha_exists`` 长期为真）。
        ``False``：拼图小块仍可见，明确未通过。
        ``None``：无法判断（跨域 iframe、异常等），交给原有 ele 轮询。
        """
        js = """
        return (function(){
          function cls(el){
            try {
              var c = el.getAttribute && el.getAttribute('class');
              if (c) return String(c);
              return String(el.className || '');
            } catch (e) { return ''; }
          }
          function tipsOk(doc){
            try {
              var nodes = doc.querySelectorAll('[class*="yidun_tips"]');
              for (var i = 0; i < nodes.length; i++) {
                var t = (nodes[i].innerText || '').trim();
                if (/成功|验证通过|通过验证|拼图.{0,4}成功|验证.{0,3}成功/.test(t)) return true;
              }
            } catch (e) {}
            return false;
          }
          function rootYidunSuccess(doc){
            try {
              var roots = doc.querySelectorAll('.yidun, [class*="yidun yidun"], [class*="yidun--"]');
              for (var i = 0; i < roots.length; i++) {
                var c = cls(roots[i]);
                if (c.indexOf('yidun--success') < 0) continue;
                if (c.indexOf('yidun--error') >= 0) continue;
                var st = getComputedStyle(roots[i]);
                if (st.display === 'none' || st.visibility === 'hidden') continue;
                return true;
              }
            } catch (e) {}
            return false;
          }
          function successClass(doc){
            try {
              if (doc.querySelector('.yidun--success')) return true;
              var nodes = doc.querySelectorAll('[class*="yidun"]');
              for (var i = 0; i < nodes.length; i++) {
                if (cls(nodes[i]).indexOf('yidun--success') >= 0) {
                  var st = getComputedStyle(nodes[i]);
                  if (st.display !== 'none' && st.visibility !== 'hidden') return true;
                }
              }
            } catch (e) {}
            return false;
          }
          function jigsawVisible(doc){
            try {
              var imgs = doc.querySelectorAll('img.yidun_jigsaw, img[class*="yidun_jigsaw"]');
              for (var j = 0; j < imgs.length; j++) {
                var im = imgs[j];
                var st = getComputedStyle(im);
                if (st.display === 'none' || st.visibility === 'hidden') continue;
                var op = parseFloat(st.opacity);
                if (!isNaN(op) && op < 0.04) continue;
                var r = im.getBoundingClientRect();
                if (r.width < 2 || r.height < 2) continue;
                if (im.offsetWidth < 2 || im.offsetHeight < 2) continue;
                return true;
              }
            } catch (e) {}
            return false;
          }
          function jigsawModeResolved(doc){
            try {
              var box = doc.querySelector('.yidun.yidun--jigsaw') ||
                  doc.querySelector('[class*="yidun--jigsaw"]');
              if (!box) return false;
              var bc = cls(box);
              if (bc.indexOf('yidun--loading') >= 0) return false;
              if (bc.indexOf('yidun--error') >= 0) return false;
              return !jigsawVisible(doc);
            } catch (e) {}
            return false;
          }
          function walk(doc, depth){
            if (!doc || depth > 10) return null;
            try {
              if (tipsOk(doc) || successClass(doc) || rootYidunSuccess(doc)) return true;
              if (jigsawModeResolved(doc)) return true;
              if (jigsawVisible(doc)) return false;
            } catch (e) {}
            var frs = doc.querySelectorAll('iframe');
            for (var k = 0; k < frs.length; k++) {
              try {
                var sub = walk(frs[k].contentDocument, depth + 1);
                if (sub === true || sub === false) return sub;
              } catch (e) {}
            }
            return null;
          }
          var r = walk(document, 0);
          if (r === true || r === false) return {p: r, r: 'walk'};
          try {
            var topImgs = document.querySelectorAll('img.yidun_jigsaw, img[class*="yidun_jigsaw"]');
            if (topImgs.length && !jigsawVisible(document)) return {p: true, r: 'top_hidden'};
          } catch (e) {}
          return {p: null, r: 'unk'};
        })();
        """
        try:
            raw = self.page.run_js(js)
        except Exception:
            return None
        if isinstance(raw, str):
            try:
                raw = json.loads(raw)
            except Exception:
                return None
        if not isinstance(raw, dict):
            return None
        p = raw.get("p")
        if p is True:
            return True
        if p is False:
            return False
        return None

    def _yidun_challenge_surface_visible_js(self) -> bool | None:
        """
        同源文档树内是否存在 **用户可见** 的 ``.yidun`` 面板（足够宽高且在视口内）。
        ICGOO 等场景：验证通过后业务层收起弹层，但 ``img.yidun_jigsaw`` 仍留在 DOM，
        ``captcha_exists``/``ele`` 仍真；此时本方法为 ``False``，应视为已放行。
        ``None``：``run_js`` 失败；跨域 iframe 内验证码时主文档可能恒为 ``False``，勿单独依赖。
        """
        js = """
        return (function(){
          function surfaceVisibleInDoc(doc, depth){
            if (!doc || depth > 10) return false;
            var mv = doc.defaultView || window;
            var vh = mv.innerHeight || 800;
            var vw = mv.innerWidth || 1200;
            try {
              var nodes = doc.querySelectorAll('.yidun');
              for (var i = 0; i < nodes.length; i++) {
                var el = nodes[i];
                var st = mv.getComputedStyle(el);
                if (st.display === 'none' || st.visibility === 'hidden') continue;
                var op = parseFloat(st.opacity);
                if (!isNaN(op) && op < 0.08) continue;
                var r = el.getBoundingClientRect();
                if (r.width < 40 || r.height < 28) continue;
                if (r.bottom <= 2 || r.right <= 2) continue;
                if (r.top >= vh - 2 || r.left >= vw - 2) continue;
                return true;
              }
            } catch (e) {}
            var frs = doc.querySelectorAll('iframe');
            for (var k = 0; k < frs.length; k++) {
              try {
                var fd = frs[k].contentDocument;
                if (fd && surfaceVisibleInDoc(fd, depth + 1)) return true;
              } catch (e) {}
            }
            return false;
          }
          return {surf: surfaceVisibleInDoc(document, 0)};
        })();
        """
        try:
            raw = self.page.run_js(js)
        except Exception:
            return None
        if isinstance(raw, str):
            try:
                raw = json.loads(raw)
            except Exception:
                return None
        if not isinstance(raw, dict):
            return None
        s = raw.get("surf")
        if s is True:
            return True
        if s is False:
            return False
        return None

    def _yidun_success_visible(self) -> bool:
        """易盾通过态：``yidun--success`` 常与 ``yidun`` 不在同一 DOM 节点，不能只查 ``.yidun.yidun--success``。"""
        for sel in (
            "css:.yidun.yidun--success",
            "css:.yidun--success",
            'xpath://*[contains(@class,"yidun--success")]',
            # ICGOO 等 skin：根节点常为 ``yidun yidun--light yidun--jigsaw``，通过后再挂 ``yidun--success``
            'xpath://div[contains(@class,"yidun") and contains(@class,"yidun--success")]',
        ):
            try:
                if self.page.ele(sel, timeout=0.5):
                    return True
            except Exception:
                continue
        return False

    def check_success(self) -> bool:
        """
        松手后在数秒内轮询，**任一**成立即视为通过：
        1) 拼图 ``yidun_jigsaw`` img 已不存在（站点撤验证码）；
        2) 出现易盾 success 相关 class（含子节点上的 ``yidun--success``）。

        旧实现要求「根上同时 .yidun + .yidun--success，且 3s 内拼图必消失」，易漏判：
        class 挂在子节点、或拼图节点晚于 3s 才移除、或仅展示成功态不重绘拼图。

        实测：松手后易盾/业务层偶在 **首轮约 6.5s 结束后** 才挂上 success 或撤掉拼图节点，故首轮未命中后 **静候约 1s 再轮询约 5s**，避免已通过仍报「验证失败」。

        另：通过后 DOM 内可能仍保留 ``img.yidun_jigsaw``（仅样式隐藏），``captcha_exists`` 用 ``ele`` 仍会命中，
        故每轮优先 ``_yidun_passed_js_probe()``（可见性 + tips + 同源 iframe）。

        **阿里云**无 .yidun：须优先 ``aliyun_verification_success_visible``（成功文案 / 弹层收起），
        勿仅依赖「无可见易盾层」启发式。
        """

        def _one_poll_window(deadline_end: float) -> bool:
            while time.time() < deadline_end:
                try:
                    ax = aliyun_verification_success_visible(self.page)
                    if ax is True:
                        return True
                except Exception:
                    pass
                try:
                    jp = self._yidun_passed_js_probe()
                    if jp is True:
                        return True
                except Exception:
                    pass
                try:
                    if self.captcha_exists(timeout=0.35):
                        if self._no_visible_yidun_panel_means_passed():
                            return True
                except Exception:
                    pass
                try:
                    if not self.captcha_exists(timeout=0.35):
                        if aliyun_captcha_cleared_for_success(self.page):
                            return True
                except Exception:
                    pass
                if self._yidun_success_visible():
                    time.sleep(0.45)
                    try:
                        if not self.captcha_exists(timeout=0.55):
                            if aliyun_captcha_cleared_for_success(self.page):
                                return True
                    except Exception:
                        pass
                    return True
                time.sleep(0.18)
            return False

        if _one_poll_window(time.time() + 6.5):
            return True
        time.sleep(1.05)
        return _one_poll_window(time.time() + 6.5)


def _compute_slide_distance_alt(
    solver: IcgooYidunSliderSolver,
    raw_x: int,
    distance: int,
    drag_scale_x: float,
    drag_boost: float,
    drag_extra_px: int,
) -> int | None:
    """与 dev 脚本一致：OpenCV 灰度/Canny 或 slide_match 两路分歧时交替试拖。"""
    ds = drag_scale_x if drag_scale_x and drag_scale_x > 0 else 1.0
    try:
        if solver._aliyun_challenge_active(timeout=0.35):
            ds *= get_aliyun_drag_scale_mult()
    except Exception:
        pass
    distance_alt: int | None = None
    xg, xc = solver._opencv_raw_x_gray, solver._opencv_raw_x_canny
    sg, scn = solver._opencv_score_gray, solver._opencv_score_canny
    aliyun_here = False
    try:
        aliyun_here = bool(solver._aliyun_challenge_active(timeout=0.35))
    except Exception:
        pass
    if (
        xg is not None
        and xc is not None
        and sg is not None
        and scn is not None
        and abs(xg - xc) >= 6
        and abs(xg - xc) <= 72
    ):
        alt_raw = xg if raw_x == xc else xc
        base_alt = int(round(alt_raw * ds * drag_boost)) + drag_extra_px
        if aliyun_here:
            sm, se, _ = aliyun_raw_x_segment_drag_adjust(alt_raw)
            distance_alt = max(1, int(round(base_alt * sm)) + se)
        else:
            distance_alt = max(1, base_alt)
        if distance_alt == distance:
            distance_alt = None
    if distance_alt is None and not getattr(solver, "_slide_match_blended", False):
        xf = solver._dddd_raw_left_simple_false
        xt = solver._dddd_raw_left_simple_true
        ddx = abs(xf - xt) if xf is not None and xt is not None else 0
        if (
            solver._last_gap_backend in ("ddddocr", "ddddocr_shim", "slidercracker")
            and xf is not None
            and xt is not None
            and 4 <= ddx <= _SLIDE_MATCH_ALT_MAX_RAW_DELTA_PX
        ):
            alt_raw = xt if raw_x == xf else xf
            base_alt = int(round(alt_raw * ds * drag_boost)) + drag_extra_px
            if aliyun_here:
                sm, se, _ = aliyun_raw_x_segment_drag_adjust(alt_raw)
                distance_alt = max(1, int(round(base_alt * sm)) + se)
            else:
                distance_alt = max(1, base_alt)
            if distance_alt == distance:
                distance_alt = None
    return distance_alt


def _yidun_jigsaw_piece_visible_js(page: ChromiumPage) -> bool | None:
    """
    拼图小块 img 是否在布局上**可见**（同源文档树含 iframe）。
    与 ``_yidun_passed_js_probe`` 内 ``jigsawVisible`` 一致；勿用 ``captcha_exists``（隐藏节点仍会被 ele 命中）。
    """
    js = """
    return (function(){
      function pieceVisible(doc){
        if (!doc) return false;
        try {
          var imgs = doc.querySelectorAll('img.yidun_jigsaw, img[class*="yidun_jigsaw"]');
          for (var j = 0; j < imgs.length; j++) {
            var im = imgs[j];
            var st = getComputedStyle(im);
            if (st.display === 'none' || st.visibility === 'hidden') continue;
            var op = parseFloat(st.opacity);
            if (!isNaN(op) && op < 0.04) continue;
            var r = im.getBoundingClientRect();
            if (r.width < 2 || r.height < 2) continue;
            if (im.offsetWidth < 2 || im.offsetHeight < 2) continue;
            return true;
          }
        } catch (e) {}
        return false;
      }
      function walk(doc, depth){
        if (!doc || depth > 10) return false;
        if (pieceVisible(doc)) return true;
        var frs = doc.querySelectorAll('iframe');
        for (var k = 0; k < frs.length; k++) {
          try {
            if (walk(frs[k].contentDocument, depth + 1)) return true;
          } catch (e) {}
        }
        return false;
      }
      return walk(document, 0);
    })();
    """
    try:
        raw = page.run_js(js)
        if raw is True:
            return True
        if raw is False:
            return False
    except Exception:
        pass
    return None


def yidun_still_requires_user_action(page: ChromiumPage) -> bool:
    """
    页面是否仍需要用户处理易盾或阿里云滑块（可见滑块层 / 可见拼图）。
    与 ``IcgooYidunSliderSolver`` 内 JS 探测一致：已通过或业务收起层、仅残留隐藏 DOM 时返回 False。

    供 ``icgoo_crawler`` 使用，避免 ``page.ele('.yidun')`` 在验证通过后仍命中隐藏节点而误进手动等待。
    """
    av = _aliyun_captcha_layer_visible(page)
    if av is True:
        return True
    if av is False:
        try:
            ax = aliyun_verification_success_visible(page)
            if ax is True:
                return False
            if ax is False:
                return True
        except Exception:
            pass
    s = IcgooYidunSliderSolver(page, quiet=True)
    jp = s._yidun_passed_js_probe()
    if jp is True:
        return False
    if jp is False:
        return True
    surf = s._yidun_challenge_surface_visible_js()
    if surf is False:
        # 阿里云无 .yidun：surf 恒 False，不能当作「易盾已收起」
        if s._no_visible_yidun_panel_means_passed():
            return False
        return True
    if surf is True:
        return True
    jv = _yidun_jigsaw_piece_visible_js(page)
    if jv is True:
        return True
    if jv is False:
        return False
    # JS 不可用时保守：仍可能需人工（如跨域 iframe 内验证码）
    return True


@dataclass(frozen=True)
class YidunAutoSolveResult:
    """
    ``try_auto_solve_icgoo_yidun_slider`` 的返回值。

    - ``ok``：是否可继续业务（未出现拼图视为已通过；自动成功亦为 True；自动失败为 False）。
    - ``auto_solved``：本次是否**实际跑过**自动滑动且**成功**（供 ``icgoo_crawler`` 打日志等）。

    布尔判断与原先 ``bool`` 返回值一致：``if result:`` 等价于 ``if result.ok:``。
    """

    ok: bool
    auto_solved: bool = False

    def __bool__(self) -> bool:
        return self.ok


def try_auto_solve_icgoo_yidun_slider(
    page: ChromiumPage,
    *,
    quiet: bool = True,
    slider_gap_backend: str = "auto",
    slide_attempts: int = _DEFAULT_SLIDE_ATTEMPTS,
    drag_boost: float = _DEFAULT_DRAG_DISTANCE_BOOST,
    drag_extra_px: int = _DEFAULT_DRAG_EXTRA_PX,
    gap_offset_image_px: int = 0,
    calibration_jsonl_path: str | None = None,
) -> YidunAutoSolveResult:
    """
    若当前页无易盾/阿里云拼图，返回 ``YidunAutoSolveResult(ok=True)``（``auto_solved=False``）。
    否则在临时目录落盘拼图、识别缺口并拖动；成功则 ``ok=True, auto_solved=True``。
    同图试尽或达到换图重识别轮数上限仍失败则 ``ok=False``。
    浏览器与页面连接断开时抛出 ``IcgooBrowserDisconnectedError``。

    ``calibration_jsonl_path``：若未传，可读环境变量 ``ICGOO_CAPTCHA_CALIBRATION_JSONL``；
    指向单文件路径时，每轮识别与各次滑动会追加一行 JSON（与 ``icgoo_aliyun_gap.append_aliyun_calibration_jsonl`` 格式一致）。
    """
    if slide_attempts < 1:
        slide_attempts = 1
    cal_path = (calibration_jsonl_path or "").strip() or None
    if cal_path is None:
        cal_path = (os.environ.get("ICGOO_CAPTCHA_CALIBRATION_JSONL") or "").strip() or None
    solver = IcgooYidunSliderSolver(
        page,
        slider_gap_backend=slider_gap_backend,
        quiet=quiet,
        drag_boost=drag_boost,
        drag_extra_px=drag_extra_px,
        gap_offset_image_px=gap_offset_image_px,
        calibration_jsonl_path=cal_path,
    )
    if not solver.captcha_exists(timeout=3):
        return YidunAutoSolveResult(ok=True, auto_solved=False)

    tmpdir = tempfile.mkdtemp(prefix="icgoo_yidun_")
    try:
        _yidun_info(
            "try_auto_solve: 检测到滑块验证码（易盾或阿里云），开始自动破解 "
            f"backend={slider_gap_backend} slide_attempts={slide_attempts} "
            f"drag_boost={drag_boost} drag_extra_px={drag_extra_px} gap_offset_image_px={gap_offset_image_px} "
            f"page_url={(page.url or '')[:180]!r}"
        )
        captcha_round = 0
        while solver.captcha_exists(timeout=3) and captcha_round < _MAX_CAPTCHA_REIDENTIFY_ROUNDS:
            captcha_round += 1
            if captcha_round > 1:
                time.sleep(1.0)
            _yidun_info(
                f"try_auto_solve: 第 {captcha_round}/{_MAX_CAPTCHA_REIDENTIFY_ROUNDS} 轮（拉图并识别）"
            )
            bg_base64, slider_base64, drag_scale_x = solver.get_images()
            bg_path = os.path.join(tmpdir, "background.png")
            slider_path = os.path.join(tmpdir, "slider.png")
            solver.save_base64_image(bg_base64, bg_path)
            solver.save_base64_image(slider_base64, slider_path)
            raw_x, distance = solver.detect_gap(
                bg_path,
                slider_path,
                drag_scale_x,
                drag_boost=drag_boost,
                gap_offset_image_px=gap_offset_image_px,
                drag_extra_px=drag_extra_px,
            )
            distance_alt = _compute_slide_distance_alt(
                solver,
                raw_x,
                distance,
                drag_scale_x,
                drag_boost,
                solver._effective_drag_extra_px,
            )
            _yidun_info(
                "try_auto_solve: 缺口与拖动 "
                f"gap_backend={solver._last_gap_backend} raw_detected={solver._gap_raw_detected} "
                f"raw_x={raw_x} drag_px={distance} drag_scale_x={drag_scale_x:.4f} "
                f"drag_extra_effective={solver._effective_drag_extra_px} "
                f"aliyun_extra_applied={solver._aliyun_drag_extra_applied} "
                f"distance_alt_px={distance_alt!s}"
            )
            solver._emit_calibration(
                build_gap_calibration_record(
                    solver,
                    captcha_round=captcha_round,
                    drag_scale_x=drag_scale_x,
                    raw_x=raw_x,
                    base_drag_px=distance,
                    distance_alt_px=distance_alt,
                    page_url=page.url or "",
                    drag_boost=drag_boost,
                    gap_offset_image_px=gap_offset_image_px,
                    drag_extra_px_user=drag_extra_px,
                    bg_path=bg_path,
                    slider_path=slider_path,
                )
            )
            solver._captcha_debug_residual_raw_x = int(raw_x)
            slide_ok, captcha_replaced = solver.slide_verification(
                distance,
                max_attempts=slide_attempts,
                distance_alt=distance_alt,
                captcha_round=captcha_round,
            )
            _yidun_info(
                f"try_auto_solve: 本轮滑动结束 slide_ok={slide_ok} captcha_replaced={captcha_replaced}"
            )
            if slide_ok:
                _yidun_info("try_auto_solve: 滑块自动破解成功")
                time.sleep(0.8)
                return YidunAutoSolveResult(ok=True, auto_solved=True)
            if not captcha_replaced:
                _yidun_warning("try_auto_solve: 同一张拼图已试尽仍未通过，结束自动破解")
                return YidunAutoSolveResult(ok=False, auto_solved=False)
        _yidun_warning(
            f"try_auto_solve: 已达换图重试上限({_MAX_CAPTCHA_REIDENTIFY_ROUNDS})或仍存在拼图，返回失败"
        )
        return YidunAutoSolveResult(ok=False, auto_solved=False)
    except IcgooBrowserDisconnectedError as e:
        _yidun_error(f"try_auto_solve: 浏览器连接断开，中止: {e}")
        raise
    except Exception as e:
        _yidun_error(f"try_auto_solve: 未预期异常 {type(e).__name__}: {e}")
        raise
    finally:
        shutil.rmtree(tmpdir, ignore_errors=True)