"""
ICGOO 网易易盾滑块：供 ``icgoo_crawler`` 调用的生产逻辑（无页面/HTML 落盘、无调试 print）。

与 ``icgoo_crawler_dev.py`` 算法一致；依赖 ``ddddocr`` 或纯 OpenCV shim（见 ``IcgooYidunSliderSolver``）。

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
from io import BytesIO

from DrissionPage import ChromiumPage
from PIL import Image

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
_DEFAULT_DRAG_DISTANCE_BOOST = 1.07
# 换算后拖动距离仍常略短时，在基准拖动像素上直接累加（可用 --drag-extra-px 覆盖；实测约 +16px）
_DEFAULT_DRAG_EXTRA_PX = 17
# 同一张图上多次滑动：相对基准距离的系数（宜覆盖偏短/偏长，略加跨度便于识别略偏时仍有机会命中）
# 实测易盾上 1.02 略短时 1.08 常能过，故将 1.08 提前，减少无效尝试次数
_SLIDE_DISTANCE_FACTORS = (1.02, 1.08, 1.0, 0.92, 0.86, 1.14, 0.96, 1.04)
_DEFAULT_SLIDE_ATTEMPTS = 7
# slide_match 边缘 vs 灰度 两路 raw_x 超过此差值（图内像素）时不用辅基准（另一峰常为误匹配）
_SLIDE_MATCH_ALT_MAX_RAW_DELTA_PX = 52
# 两路置信度均低于此值且 raw 相差超过下一项时，主 raw_x 取两路中点
_SLIDE_MATCH_BLEND_MAX_CONF = 0.35
_SLIDE_MATCH_BLEND_MIN_DELTA_PX = 48
# 与 scripts/test_ddddocr_slide.py 一致：simple=True 与 simple=False 左缘相差超过此阈值时，
# 勿只信 simple=True 的高 confidence（易盾常见假峰）；缺口类优先采信 simple_target=False（Canny）
_SLIDE_MATCH_SIMPLE_DIVERGE_MIN_PX = 48.0
_SLIDE_MATCH_SIMPLE_DIVERGE_FRAC_DIAG = 0.08
# 刷新/换图后重新拉拼图并识别缺口的最大轮数（避免死循环）
_MAX_CAPTCHA_REIDENTIFY_ROUNDS = 8

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
) -> dict:
    """
    与 ddddocr.core.slide_engine.SlideEngine 内逻辑一致：RGB→灰度，simple_target 时直接模板匹配，
    否则 Canny(50,150) 后匹配，TM_CCOEFF_NORMED，返回 target/target_x/target_y/confidence。
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
    ):
        self.page = page
        self._quiet = quiet
        self._drag_boost = float(drag_boost) if drag_boost and drag_boost > 0 else 1.0
        self._drag_extra_px = int(drag_extra_px)
        self._gap_offset_image_px = int(gap_offset_image_px)
        # auto：优先 ddddocr.slide_match，失败再 OpenCV；ddddocr：仅 ddddocr；opencv：仅 OpenCV（调试）
        self._slider_gap_backend = slider_gap_backend if slider_gap_backend in (
            "auto",
            "ddddocr",
            "opencv",
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

    def _verbose(self, msg: str) -> None:
        if not self._quiet:
            print(msg, flush=True)

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

    def _fill_opencv_gap_scores(self, bg_path: str, slider_path: str) -> bool:
        """计算灰度/Canny 模板匹配的左缘 x 与 TM_CCOEFF_NORMED 峰值；供 ensemble 与回退。"""
        import cv2

        self._opencv_raw_x_gray = None
        self._opencv_raw_x_canny = None
        self._opencv_score_gray = None
        self._opencv_score_canny = None
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

    def _ensemble_auto_best_gap(self, bw: int, tw: int) -> tuple[int, float, str] | None:
        """
        auto 专用：在 dddd 两路与 OpenCV 灰度/Canny 的 plausible 候选中，取 TM 相关度最高者，
        缓解 shim 峰值整体偏低而 OpenCV 某通道更尖的情况。
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
        if not cand:
            return None
        # 任一路 dddd 已给出 plausible 峰时，不与 OpenCV 比分数（灰度模板易因纹理出现更高假峰）
        if any(t[2].startswith("dddd") for t in cand):
            cand = [t for t in cand if t[2].startswith("dddd")]
        cand.sort(key=lambda t: t[1], reverse=True)
        return cand[0][0], cand[0][1], cand[0][2]

    def captcha_exists(self, timeout: float = 2.0) -> bool:
        """判断页面是否存在易盾滑块验证码（通过查找拼图 img 等元素）。"""
        try:
            ele = self.page.ele(_XP_YIDUN_JIGSAW_IMG, timeout=timeout)
            return ele is not None
        except Exception:
            return False

    def _yidun_jigsaw_src(self) -> str:
        """拼图小块 img 的 src（失败换图后通常会变），用于判断是否仍为同一张拼图。"""
        try:
            el = self.page.ele(_XP_YIDUN_JIGSAW_IMG, timeout=0.85)
            if el:
                return _read_yidun_img_src(el)
        except Exception:
            pass
        return ""

    def get_images(self) -> tuple[str, str, float]:
        """获取滑块和背景图（data: 或 https）；第三项为拖动换算比例（优先 滑轨可行程/图自然宽）。"""
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
        为 auto 时 ddddocr 失败再回退 OpenCV。

        **算法局限（易盾拼图常见）：** 本质是「整块拼图小图」在「带缺口的大图」上做模板匹配，
        相关峰不一定是真实缺口位置（纹理重复、阴影、圆角都会引入假峰）；Canny 阈值固定为 50/150，
        对部分底图不理想。与 ``scripts/test_ddddocr_slide.py`` 一致：两路 ``slide_match`` 左缘相差较大时，
        **不会**仅凭 ``simple_target=True`` 的较高 confidence 定案（该路常虚高但仍错），而优先采信
        ``simple_target=False``（边缘/Canny）；两路置信度均低且相差仍大时仍取中点融合。
        拖动像素还依赖 ``drag_scale_x``（DOM 量轨）与 ``drag_boost``，任一环偏差都会表现为「总偏一点」。
        可配合 ``detect_gap(..., gap_offset_image_px=…)`` 做图像空间微调，或修复 onnx 后对比 ddddocr 官方模型。
        """
        self._last_opencv_match_score = None
        self._last_opencv_match_mode = None
        self._opencv_raw_x_gray = None
        self._opencv_raw_x_canny = None
        self._opencv_score_gray = None
        self._opencv_score_canny = None
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
        try:
            with open(slider_path, "rb") as f:
                slide_bytes = f.read()
            with open(bg_path, "rb") as f:
                bg_bytes = f.read()
        except OSError:
            return 100

        try:
            tw = int(Image.open(BytesIO(slide_bytes)).size[0])
        except Exception:
            tw = 0
        if tw <= 0:
            tw, _ = _image_natural_wh_from_bytes(slide_bytes)
        try:
            im_bg = Image.open(BytesIO(bg_bytes))
            bw, bh = int(im_bg.size[0]), int(im_bg.size[1])
        except Exception:
            bw, bh = 0, 0
        if bw <= 0 or bh <= 0:
            bw, bh = _image_natural_wh_from_bytes(bg_bytes)

        if self._slider_gap_backend == "opencv":
            self._last_gap_backend = "opencv"
            return self._detect_gap_position_opencv(bg_path, slider_path)

        if self._slider_gap_backend == "auto":
            self._fill_opencv_gap_scores(bg_path, slider_path)

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
            return self._detect_gap_position_opencv(bg_path, slider_path)

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
        # 两路 dddd 均有 plausible 峰时一律走融合/分歧逻辑；ensemble 仅用于「仅一路有效或两路皆无有效峰」时引入 OpenCV
        _auto_use_dddd_only = (
            self._slider_gap_backend == "auto" and xf_ok and xt_ok
        )

        if self._slider_gap_backend == "auto" and not _auto_use_dddd_only:
            ens = self._ensemble_auto_best_gap(bw, tw)
            if ens is not None:
                left_x, escore, etag = ens
                self._slide_match_blended = False
                self._slide_match_diverge_prefer_false = False
                self._gap_ensemble_pick = etag
                if etag in ("opencv_gray", "opencv_canny"):
                    self._last_gap_backend = "opencv"
                    self._last_opencv_match_mode = "gray" if etag == "opencv_gray" else "canny"
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

            self._last_gap_backend = "ddddocr_shim" if self._slide_match_is_shim else "ddddocr"
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
        return self._detect_gap_position_opencv(bg_path, slider_path)

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
        """
        raw_detected = self.detect_gap_position(bg_path, slider_path)
        self._gap_raw_detected = int(raw_detected)
        raw_x = int(raw_detected) + int(gap_offset_image_px)
        if raw_x < 1:
            raw_x = 1
        s = drag_scale_x if drag_scale_x and drag_scale_x > 0 else 1.0
        b = drag_boost if drag_boost and drag_boost > 0 else 1.0
        extra = int(drag_extra_px)
        drag_x = max(1, int(round(raw_x * s * b)) + extra)
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
        """定位易盾可拖动节点（拼图模式下多为 span.yidun_slider 或外层 yidun_slide_indicator）。"""
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
                self.page.wait.eles_loaded(_XP_YIDUN_JIGSAW_IMG, timeout=1.2)
            except Exception:
                pass
            time.sleep(0.5)
        return False

    def _find_yidun_refresh(self, timeout_each: float = 1.0):
        for xp in _XP_YIDUN_REFRESH_CANDIDATES:
            try:
                el = self.page.ele(xp, timeout=timeout_each)
                if el:
                    return el
            except Exception:
                continue
        return None

    def _slide_passed_or_captcha_gone(self) -> tuple[bool, str]:
        """
        是否已无需再拖：易盾成功态出现，或拼图 img 已不在 DOM（常见于上一滑已通过但
        check_success 略晚、或 UI 收起瞬间手柄先消失）。
        返回 (True, 说明文案) 表示应直接视为成功退出 slide_verification。
        """
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
                return True, "拼图验证码已从页面消失"
        except Exception:
            return True, "验证码节点不可访问（页面可能已变迁）"
        try:
            if self.captcha_exists(timeout=0.35):
                surf = self._yidun_challenge_surface_visible_js()
                if surf is False:
                    return True, "易盾仍在 DOM 但页面无可见滑块层（业务已收起、数据已加载时常出现）"
        except Exception:
            pass
        return False, ""

    def slide_verification(
        self,
        distance,
        max_attempts: int = _DEFAULT_SLIDE_ATTEMPTS,
        distance_alt: int | None = None,
    ) -> tuple[bool, bool]:
        """
        在同一张验证码上多次滑动：微调距离试探。
        distance_alt 若给出且与 distance 不同，则奇数次用主基准、偶数次用辅基准
        （应对 OpenCV 灰度/Canny 或 slide_match 边缘/灰度模板两路 x 不一致）。

        返回 ``(是否验证成功, 是否已换图/刷新页)``。
        第二项为 True 时拼图已非当前 distance 对应的那一张，**必须**重新 get_images + detect_gap，
        不得继续用旧拖动距离。

        易盾在**验证失败后常会直接下发新拼图**（与手动点刷新类似）。每次失败后比对拼图 ``src``，
        若已变化则第二项返回 True，避免仍按「同一张图」换系数空滑。
        """
        factors = _SLIDE_DISTANCE_FACTORS
        use_alt = distance_alt is not None and distance_alt > 0 and distance_alt != distance
        for attempt in range(max_attempts):
            try:
                ok_early, why = self._slide_passed_or_captcha_gone()
                if ok_early:
                    self._verbose(f"{why}，不再寻找滑块手柄（视为验证成功）。")
                    _yidun_info(f"slide_verification: 无需拖动即通过 ({why})")
                    return True, False

                f = factors[attempt] if attempt < len(factors) else 1.0
                base = distance_alt if (use_alt and attempt % 2 == 1) else distance
                base_tag = "辅" if base == distance_alt and use_alt else "主"
                d_try = max(1, int(round(base * f)))
                src_before = self._yidun_jigsaw_src()
                self._verbose(
                    f"第{attempt + 1}次尝试滑动（{base_tag}基准，目标位移≈{d_try}px，系数{f:.2f}）…"
                )
                slider = self._prepare_yidun_drag_handle_for_actions()
                if not slider:
                    ok_early, why = self._slide_passed_or_captcha_gone()
                    if ok_early:
                        self._verbose(f"{why}，跳过等待手柄（视为验证成功）。")
                        _yidun_info(f"slide_verification: 无手柄仍判定通过 ({why})")
                        return True, False
                    self._verbose("未立即找到滑块手柄，等待易盾控件与手柄渲染…")
                    if self._wait_yidun_slider_ready(12.0):
                        slider = self._prepare_yidun_drag_handle_for_actions(retries=10)
                if not slider:
                    ok_early, why = self._slide_passed_or_captcha_gone()
                    if ok_early:
                        self._verbose(f"{why}，等待结束仍无手柄（视为验证成功，不点刷新）。")
                        _yidun_info(f"slide_verification: 等待后无手柄仍判定通过 ({why})")
                        return True, False
                    self._verbose(
                        "等待后仍无手柄：仅点击易盾「刷新」换拼图（避免整页 refresh 导致连接断开）。"
                    )
                    _yidun_warning("slide_verification: 无滑块手柄，点击易盾刷新换图")
                    refresh_btn = self._find_yidun_refresh()
                    if refresh_btn:
                        try:
                            refresh_btn.click()
                        except Exception as ex:
                            self._verbose(f"点击易盾刷新失败: {ex}")
                            _yidun_warning(f"易盾刷新按钮点击失败: {ex}")
                    time.sleep(2.8)
                    return False, True

                ac = self.page.actions
                ac.hold(slider)
                time.sleep(random.uniform(0.12, 0.28))

                tracks = self.get_tracks(d_try)
                for track in tracks:
                    if track != 0:
                        ac.move(track, 0, duration=0.02)
                    time.sleep(random.uniform(0.008, 0.018))

                time.sleep(random.uniform(0.12, 0.28))
                ac.release()
                time.sleep(random.uniform(2.0, 2.75))

                ok = self.check_success()
                if not ok:
                    # 易盾/Vue/ICGOO 偶在轮询结束后才挂 success、撤层或仅保留隐藏 DOM；分段延迟复核
                    time.sleep(2.0)
                    late_ok, late_why = self._slide_passed_or_captcha_gone()
                    if late_ok:
                        ok = True
                        self._verbose(f"首轮轮询未命中，延迟复核：{late_why}，视为验证成功。")
                    if not ok:
                        time.sleep(2.5)
                        late_ok2, late_why2 = self._slide_passed_or_captcha_gone()
                        if late_ok2:
                            ok = True
                            self._verbose(f"二次延迟复核：{late_why2}，视为验证成功。")
                if ok:
                    self._verbose("验证成功！")
                    _yidun_info(
                        f"slide_verification: 拖动成功 attempt={attempt + 1} d_try={d_try} factor={f:.3f}"
                    )
                    return True, False

                time.sleep(0.9 if attempt < max_attempts - 1 else 0.65)
                src_after = self._yidun_jigsaw_src()
                if src_before and src_after and src_before != src_after:
                    self._verbose(
                        "验证失败：检测到拼图已更换（与滑动前非同一 src），将重新拉图并识别缺口。"
                    )
                    _yidun_info("slide_verification: 拼图 src 已变，需重新识别缺口")
                    return False, True
                if attempt < max_attempts - 1:
                    self._verbose("验证失败，拼图 src 未变，同图换距离重试…")

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
                return False, True

        self._verbose(f"经过{max_attempts}次尝试均未通过验证（同一张拼图，未换图）")
        _yidun_warning(
            f"slide_verification: 同图已试满 {max_attempts} 次仍未通过，captcha_replaced=False"
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
        """

        def _one_poll_window(deadline_end: float) -> bool:
            while time.time() < deadline_end:
                try:
                    jp = self._yidun_passed_js_probe()
                    if jp is True:
                        return True
                except Exception:
                    pass
                try:
                    if self.captcha_exists(timeout=0.35):
                        surf = self._yidun_challenge_surface_visible_js()
                        if surf is False:
                            return True
                except Exception:
                    pass
                try:
                    if not self.captcha_exists(timeout=0.35):
                        return True
                except Exception:
                    return True
                if self._yidun_success_visible():
                    time.sleep(0.45)
                    try:
                        if not self.captcha_exists(timeout=0.55):
                            return True
                    except Exception:
                        return True
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
    distance_alt: int | None = None
    xg, xc = solver._opencv_raw_x_gray, solver._opencv_raw_x_canny
    sg, scn = solver._opencv_score_gray, solver._opencv_score_canny
    if (
        xg is not None
        and xc is not None
        and sg is not None
        and scn is not None
        and abs(xg - xc) >= 6
        and abs(xg - xc) <= 72
    ):
        alt_raw = xg if raw_x == xc else xc
        distance_alt = max(
            1,
            int(round(alt_raw * drag_scale_x * drag_boost)) + drag_extra_px,
        )
        if distance_alt == distance:
            distance_alt = None
    if distance_alt is None and not getattr(solver, "_slide_match_blended", False):
        xf = solver._dddd_raw_left_simple_false
        xt = solver._dddd_raw_left_simple_true
        ddx = abs(xf - xt) if xf is not None and xt is not None else 0
        if (
            solver._last_gap_backend in ("ddddocr", "ddddocr_shim")
            and xf is not None
            and xt is not None
            and 4 <= ddx <= _SLIDE_MATCH_ALT_MAX_RAW_DELTA_PX
        ):
            alt_raw = xt if raw_x == xf else xf
            distance_alt = max(
                1,
                int(round(alt_raw * drag_scale_x * drag_boost)) + drag_extra_px,
            )
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
    页面是否仍需要用户处理易盾（可见滑块层 / 可见拼图）。
    与 ``IcgooYidunSliderSolver`` 内 JS 探测一致：已通过或业务收起层、仅残留隐藏 DOM 时返回 False。

    供 ``icgoo_crawler`` 使用，避免 ``page.ele('.yidun')`` 在验证通过后仍命中隐藏节点而误进手动等待。
    """
    s = IcgooYidunSliderSolver(page, quiet=True)
    jp = s._yidun_passed_js_probe()
    if jp is True:
        return False
    if jp is False:
        return True
    surf = s._yidun_challenge_surface_visible_js()
    if surf is False:
        return False
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
) -> YidunAutoSolveResult:
    """
    若当前页无易盾拼图 ``img.yidun_jigsaw``，返回 ``YidunAutoSolveResult(ok=True)``（``auto_solved=False``）。
    否则在临时目录落盘拼图、识别缺口并拖动；成功则 ``ok=True, auto_solved=True``。
    同图试尽或达到换图重识别轮数上限仍失败则 ``ok=False``。
    浏览器与页面连接断开时抛出 ``IcgooBrowserDisconnectedError``。
    """
    if slide_attempts < 1:
        slide_attempts = 1
    solver = IcgooYidunSliderSolver(
        page,
        slider_gap_backend=slider_gap_backend,
        quiet=quiet,
        drag_boost=drag_boost,
        drag_extra_px=drag_extra_px,
        gap_offset_image_px=gap_offset_image_px,
    )
    if not solver.captcha_exists(timeout=3):
        return YidunAutoSolveResult(ok=True, auto_solved=False)

    tmpdir = tempfile.mkdtemp(prefix="icgoo_yidun_")
    try:
        _yidun_info(
            "try_auto_solve: 检测到易盾拼图，开始自动破解 "
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
                solver, raw_x, distance, drag_scale_x, drag_boost, drag_extra_px
            )
            _yidun_info(
                "try_auto_solve: 缺口与拖动 "
                f"gap_backend={solver._last_gap_backend} raw_detected={solver._gap_raw_detected} "
                f"raw_x={raw_x} drag_px={distance} drag_scale_x={drag_scale_x:.4f} "
                f"distance_alt_px={distance_alt!s}"
            )
            slide_ok, captcha_replaced = solver.slide_verification(
                distance,
                max_attempts=slide_attempts,
                distance_alt=distance_alt,
            )
            _yidun_info(
                f"try_auto_solve: 本轮滑动结束 slide_ok={slide_ok} captcha_replaced={captcha_replaced}"
            )
            if slide_ok:
                _yidun_info("try_auto_solve: 易盾自动破解成功")
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