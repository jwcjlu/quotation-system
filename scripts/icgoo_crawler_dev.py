"""
ICGOO 易盾滑块调试脚本 — Cookie 与正式爬虫 icgoo_crawler.py 共用同一 JSON 格式。

用法示例:
  # 首次：浏览器里登录/过验证后保存（与 icgoo_crawler 相同文件即可共享）
  python icgoo_crawler_dev.py --save-cookies icgoo_cookies.json --url "https://www.icgoo.net/search/XXX/1"

  # 下次：先注入 Cookie 再打开搜索页，减少易盾
  python icgoo_crawler_dev.py --cookies-file icgoo_cookies.json --url "https://www.icgoo.net/search/XXX/1"

  # 滑块缺口：ddddocr 用法见 README「滑块验证码处理」
  # https://github.com/sml2h3/ddddocr/blob/master/README.md#滑块验证码处理
  python icgoo_crawler_dev.py --slider-backend ddddocr --url "..."

  # 破解/验证通过后：保存页面 HTML，并按与 ickey_crawler 对齐的字段解析表格 JSON 到 stdout
  python icgoo_crawler_dev.py --url "https://www.icgoo.net/search/XXX/1" \\
    --save-html-after ./icgoo_after_captcha.html --parse-as-ickey

环境变量 ICGOO_COOKIES_FILE 可指定默认 Cookie 路径；本脚本目录下存在 icgoo_cookies.json 时也会自动尝试加载。

若打开目标 URL 后检测到未登录（文案/跳转登录页等，与 icgoo_crawler 同源逻辑），且已配置
ICGOO_USERNAME / ICGOO_PASSWORD 或 ``--user`` / ``--password``，将自动调用与正式爬虫相同的 ``login_icgoo``。
可用 ``--skip-auto-login`` 关闭。
"""
from __future__ import annotations

import argparse
import base64
import binascii
import json
import math
import os
import random
import re
import sys
import time
import urllib.error
import urllib.request
from io import BytesIO
from urllib.parse import unquote, urlparse

from DrissionPage import ChromiumPage
from PIL import Image

# 与同目录 icgoo_crawler 共用加载/保存逻辑
_script_dir = os.path.dirname(os.path.abspath(__file__))
if _script_dir not in sys.path:
    sys.path.insert(0, _script_dir)

from icgoo_crawler import (  # noqa: E402
    DEFAULT_LOGIN_URL,
    _default_cookies_file_path,
    _get_credentials,
    icgoo_suggests_not_logged_in,
    load_icgoo_cookies_from_file,
    login_icgoo,
    parse_search_results,
    save_icgoo_cookies_to_file,
    wait_for_manual_captcha_or_rate_limit,
    wait_for_results,
)

_DEFAULT_LOCAL_COOKIES = os.path.join(_script_dir, "icgoo_cookies.json")


def _keyword_from_icgoo_search_url(url: str) -> str:
    """从 ``https://www.icgoo.net/search/{型号}/1`` 提取型号，供 ``parse_search_results`` 的 query_model。"""
    try:
        path = (urlparse(url or "").path or "").strip("/")
        parts = [p for p in path.split("/") if p]
        if len(parts) >= 2 and parts[0].lower() == "search":
            return unquote(parts[1]) or "UNKNOWN"
    except Exception:
        pass
    return "UNKNOWN"

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
_DEFAULT_DRAG_EXTRA_PX = 16
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


def _new_captcha_debug_run_dir(base_dir: str) -> str:
    """在 base_dir 下创建 run_时间戳_随机 子目录。"""
    base_dir = os.path.abspath(base_dir)
    os.makedirs(base_dir, exist_ok=True)
    ts = time.strftime("%Y%m%d_%H%M%S")
    rand = f"{random.randint(0, 0xFFFFFF):06x}"
    run = os.path.join(base_dir, f"run_{ts}_{rand}")
    os.makedirs(run, exist_ok=True)
    return run


def _debug_write_text(path: str, text: str) -> None:
    try:
        with open(path, "w", encoding="utf-8") as f:
            f.write(text)
    except OSError:
        pass


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


def _resolve_load_cookies_path(explicit: str | None, no_cookies: bool) -> str | None:
    if no_cookies:
        return None
    if explicit:
        return explicit if os.path.isfile(explicit) else None
    env = _default_cookies_file_path(None)
    if env and os.path.isfile(env):
        return env
    if os.path.isfile(_DEFAULT_LOCAL_COOKIES):
        return _DEFAULT_LOCAL_COOKIES
    return None


def _resolve_save_cookies_path(save_arg: str | None, load_path: str | None) -> str | None:
    if save_arg is None:
        return None
    if save_arg == "" or save_arg == "__default__":
        return load_path or os.environ.get("ICGOO_COOKIES_FILE", "").strip() or _DEFAULT_LOCAL_COOKIES
    return save_arg


def ensure_icgoo_auto_login(
    page: ChromiumPage,
    target_url: str,
    username: str | None,
    password: str | None,
    login_url: str,
    captcha_wait_sec: float,
    save_path: str | None,
    skip_auto_login: bool,
) -> bool:
    """
    若页面显示未登录且提供了账号密码，则执行 login_icgoo 并回到 target_url。
    不在此函数内对搜索页调用 wait_for_manual（易盾/限流判断），由调用方在登录流程结束后再调。
    返回是否实际执行了账号密码登录。
    """
    if skip_auto_login:
        return False
    u, p = _get_credentials(username, password)
    if not icgoo_suggests_not_logged_in(page):
        return False
    if not u or not p:
        print(
            "检测到未登录，但未设置 ICGOO_USERNAME/ICGOO_PASSWORD 或 --user/--password，无法自动登录",
            flush=True,
        )
        return False
    print("检测到未登录，正在自动登录…", flush=True)
    login_icgoo(
        page,
        u,
        p,
        login_url=login_url,
        quiet=False,
        captcha_wait_sec=captcha_wait_sec,
        # 登录常跳到首页 ?code=…，易盾/限流在搜索页再处理（见 main 里 wait_for_manual）
        wait_captcha_after_login=False,
    )
    if save_path:
        save_icgoo_cookies_to_file(page, save_path, quiet=False)
    page.get(target_url)
    time.sleep(2)
    return True


class IcgooBrowserDisconnectedError(RuntimeError):
    """DrissionPage 与浏览器连接已断开；不应再调用 page.refresh()。"""


class SliderCaptchaSolver:
    def __init__(
        self,
        cookies_load_path: str | None = None,
        cookies_save_path: str | None = None,
        *,
        slider_gap_backend: str = "auto",
        offline_gap_eval: bool = False,
    ):
        # 仅跑 detect_gap_position 等离线逻辑时可 True，避免拉起浏览器（其它方法需 page）
        self.page = None if offline_gap_eval else ChromiumPage()
        self._cookies_load_path = cookies_load_path
        self._cookies_save_path = cookies_save_path
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
                print(
                    "提示：ddddocr 包因 onnxruntime 等依赖无法导入（常见于 DLL 初始化失败），"
                    "已自动使用与 ddddocr 官方 SlideEngine 相同的纯 OpenCV 滑块算法（无需 ONNX）。",
                    flush=True,
                )
                print(f"  （原始异常: {self._dddd_init_error}）", flush=True)
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

    def load_shared_cookies(self) -> bool:
        """在首次访问 icgoo 前注入 JSON Cookie；与 icgoo_crawler 行为一致。"""
        if not self._cookies_load_path:
            return False
        ok = load_icgoo_cookies_from_file(self.page, self._cookies_load_path)
        if ok:
            print(f"已加载共享 Cookie: {self._cookies_load_path}", flush=True)
        else:
            print(f"未加载 Cookie（文件不存在或格式错误）: {self._cookies_load_path}", flush=True)
        return ok

    def save_shared_cookies(self) -> bool:
        """将当前浏览器中 icgoo.net Cookie 写入 JSON，供下次或其它脚本复用。"""
        path = self._cookies_save_path or self._cookies_load_path or _DEFAULT_LOCAL_COOKIES
        return save_icgoo_cookies_to_file(self.page, path, quiet=False)

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
                return (el.attr("src") or "").strip()
        except Exception:
            pass
        return ""

    def get_images(self, debug_run_dir: str | None = None) -> tuple[str, str, float]:
        """获取滑块和背景图（data: 或 https）；第三项为拖动换算比例（优先 滑轨可行程/图自然宽）。"""
        try:
            html_content = self.page.html or ""
            if debug_run_dir:
                hp = os.path.join(debug_run_dir, "00_page_initial.html")
                with open(hp, "w", encoding="utf-8") as f:
                    f.write(html_content)
                print(f"已保存页面 HTML: {hp}", flush=True)
            else:
                with open("icgoo_captcha_page_dump.html", "w", encoding="utf-8") as f:
                    f.write(html_content)
                print("已保存当前页面为 icgoo_captcha_page_dump.html", flush=True)
        except Exception as e:
            print(f"保存页面 HTML 失败: {e}", flush=True)

        referer = (self.page.url or "").strip() or "https://www.icgoo.net/"
        deadline = time.time() + 15.0
        slider_ele = None
        bg_ele = None
        while time.time() < deadline:
            try:
                self.page.wait.eles_loaded(_XP_YIDUN_JIGSAW_IMG, timeout=2)
            except Exception:
                pass
            slider_ele = self.page.ele(_XP_YIDUN_JIGSAW_IMG, timeout=2)
            bg_ele = self.page.ele(_XP_YIDUN_BG_IMG, timeout=2)
            s_sl = (slider_ele.attr("src") if slider_ele else None) or ""
            s_bg = (bg_ele.attr("src") if bg_ele else None) or ""
            if slider_ele and bg_ele and s_sl.strip() and s_bg.strip():
                break
            time.sleep(0.25)

        if not slider_ele or not bg_ele:
            raise ValueError("未找到滑块或背景图 img，请确认验证码已完整弹出")

        slider_src = slider_ele.attr("src")
        bg_src = bg_ele.attr("src")
        if not (slider_src or "").strip() or not (bg_src or "").strip():
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
                print(
                    f"ddddocr 不可用（{detail}），已改用 OpenCV 模板匹配。",
                    flush=True,
                )
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
            print(
                "ddddocr.slide_match 未得到有效结果，已改用 OpenCV 模板匹配。",
                flush=True,
            )
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

    def _captcha_debug_save_after_attempt(
        self,
        debug_run_dir: str | None,
        attempt: int,
        d_try: int,
        f: float,
        tracks: list[int],
        verified_ok: bool,
    ) -> None:
        """每次滑动后保存视口截图、整页 HTML、易盾区域截图（若可定位）、轨迹元数据。"""
        if not debug_run_dir:
            return
        n = attempt + 1
        try:
            self.page.get_screenshot(path=debug_run_dir, name=f"{n:02d}_viewport_d{d_try}.png")
        except Exception as e:
            _debug_write_text(os.path.join(debug_run_dir, f"{n:02d}_viewport_error.txt"), str(e))
        try:
            _debug_write_text(
                os.path.join(debug_run_dir, f"{n:02d}_page_after.html"),
                self.page.html or "",
            )
        except Exception as e:
            _debug_write_text(os.path.join(debug_run_dir, f"{n:02d}_page_html_error.txt"), str(e))
        try:
            y = self.page.ele("css:.yidun", timeout=0.6)
            if y:
                y.get_screenshot(path=debug_run_dir, name=f"{n:02d}_yidun_widget_d{d_try}.png")
        except Exception:
            pass
        ts = sum(tracks)
        meta = (
            f"attempt={n}\n"
            f"factor={f}\n"
            f"d_try_px={d_try}\n"
            f"tracks_sum={ts}\n"
            f"tracks_len={len(tracks)}\n"
            f"verified_ok={verified_ok}\n"
            f"tracks={tracks!r}\n"
        )
        _debug_write_text(os.path.join(debug_run_dir, f"{n:02d}_slide_meta.txt"), meta)
        try:
            log_path = os.path.join(debug_run_dir, "attempts.tsv")
            with open(log_path, "a", encoding="utf-8") as log:
                log.write(f"{n}\t{d_try}\t{f}\t{ts}\t{verified_ok}\n")
        except OSError:
            pass

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
        debug_run_dir: str | None = None,
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
                    print(f"{why}，不再寻找滑块手柄（视为验证成功）。", flush=True)
                    return True, False

                f = factors[attempt] if attempt < len(factors) else 1.0
                base = distance_alt if (use_alt and attempt % 2 == 1) else distance
                base_tag = "辅" if base == distance_alt and use_alt else "主"
                d_try = max(1, int(round(base * f)))
                src_before = self._yidun_jigsaw_src()
                print(
                    f"第{attempt + 1}次尝试滑动（{base_tag}基准，目标位移≈{d_try}px，系数{f:.2f}）…",
                    flush=True,
                )
                slider = self._prepare_yidun_drag_handle_for_actions()
                if not slider:
                    ok_early, why = self._slide_passed_or_captcha_gone()
                    if ok_early:
                        print(f"{why}，跳过等待手柄（视为验证成功）。", flush=True)
                        return True, False
                    print("未立即找到滑块手柄，等待易盾控件与手柄渲染…", flush=True)
                    if self._wait_yidun_slider_ready(12.0):
                        slider = self._prepare_yidun_drag_handle_for_actions(retries=10)
                if not slider:
                    ok_early, why = self._slide_passed_or_captcha_gone()
                    if ok_early:
                        print(f"{why}，等待结束仍无手柄（视为验证成功，不点刷新）。", flush=True)
                        return True, False
                    print(
                        "等待后仍无手柄：仅点击易盾「刷新」换拼图（避免整页 refresh 导致连接断开）。",
                        flush=True,
                    )
                    refresh_btn = self._find_yidun_refresh()
                    if refresh_btn:
                        try:
                            refresh_btn.click()
                        except Exception as ex:
                            print(f"点击易盾刷新失败: {ex}", flush=True)
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
                        print(f"首轮轮询未命中，延迟复核：{late_why}，视为验证成功。", flush=True)
                    if not ok:
                        time.sleep(2.5)
                        late_ok2, late_why2 = self._slide_passed_or_captcha_gone()
                        if late_ok2:
                            ok = True
                            print(f"二次延迟复核：{late_why2}，视为验证成功。", flush=True)
                self._captcha_debug_save_after_attempt(
                    debug_run_dir, attempt, d_try, f, tracks, ok
                )
                if ok:
                    print("验证成功！", flush=True)
                    return True, False

                time.sleep(0.9 if attempt < max_attempts - 1 else 0.65)
                src_after = self._yidun_jigsaw_src()
                if src_before and src_after and src_before != src_after:
                    print(
                        "验证失败：检测到拼图已更换（与滑动前非同一 src），将重新拉图并识别缺口。",
                        flush=True,
                    )
                    return False, True
                if attempt < max_attempts - 1:
                    print("验证失败，拼图 src 未变，同图换距离重试…", flush=True)

            except Exception as e:
                msg = str(e).strip() or repr(e)
                print(f"滑动过程中出错: {msg}", flush=True)
                if debug_run_dir:
                    _debug_write_text(
                        os.path.join(debug_run_dir, f"attempt{attempt + 1:02d}_exception.txt"),
                        f"{type(e).__name__}: {msg}\n",
                    )
                low = msg.lower()
                rect_lost = (
                    "没有位置" in msg
                    or "没有大小" in msg
                    or "位置及大小" in msg
                    or "has_rect" in low
                    or "no rect" in low
                )
                if rect_lost and attempt < max_attempts - 1:
                    print("手柄暂时无有效尺寸，等待后同图重试（不刷新页面）…", flush=True)
                    time.sleep(0.85)
                    continue
                disconnect = (
                    "连接已断开" in msg
                    or ("断开" in msg and "连接" in msg)
                    or "disconnected" in low
                )
                if disconnect:
                    print("检测到与浏览器连接已断开，不再执行页面刷新。", flush=True)
                    raise IcgooBrowserDisconnectedError(msg) from e
                print("非常规错误，已刷新页面；将重新拉取拼图并识别缺口。", flush=True)
                try:
                    self.page.refresh()
                    time.sleep(3)
                except Exception as ex:
                    print(f"刷新页面失败: {ex}", flush=True)
                return False, True

        print(f"经过{max_attempts}次尝试均未通过验证（同一张拼图，未换图）", flush=True)
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


def main():
    parser = argparse.ArgumentParser(description="ICGOO 易盾调试（Cookie 与 icgoo_crawler 共享）")
    parser.add_argument(
        "--url",
        default="https://www.icgoo.net/search/APDS-9306-065/1",
        help="打开的目标 URL",
    )
    parser.add_argument(
        "--cookies-file",
        default=None,
        help="从 JSON 加载 Cookie（也可用环境变量 ICGOO_COOKIES_FILE；默认同目录 icgoo_cookies.json）",
    )
    parser.add_argument(
        "--no-cookies",
        action="store_true",
        help="不加载 Cookie",
    )
    parser.add_argument(
        "--save-cookies",
        nargs="?",
        const="__default__",
        default=None,
        metavar="PATH",
        help="流程结束后保存 icgoo.net Cookie；省略 PATH 时写入 --cookies-file 或 icgoo_cookies.json",
    )
    parser.add_argument(
        "--user",
        default="18025478083",
        help="登录账号（也可用环境变量 ICGOO_USERNAME）",
    )
    parser.add_argument(
        "--password",
        default="jw123456",
        help="登录密码（也可用环境变量 ICGOO_PASSWORD）",
    )
    parser.add_argument(
        "--login-url",
        default=DEFAULT_LOGIN_URL,
        help=f"登录页 URL，默认 {DEFAULT_LOGIN_URL}",
    )
    parser.add_argument(
        "--captcha-wait",
        type=float,
        default=300.0,
        help="易盾/限流时等待手动操作的最长时间（秒）",
    )
    parser.add_argument(
        "--skip-auto-login",
        action="store_true",
        help="检测到未登录时不自动填表登录（仅依赖 Cookie）",
    )
    parser.add_argument(
        "--drag-boost",
        type=float,
        default=_DEFAULT_DRAG_DISTANCE_BOOST,
        metavar="K",
        help=f"拖动距离总体系数（略偏小时调大），默认 {_DEFAULT_DRAG_DISTANCE_BOOST}；设为 1 关闭补偿",
    )
    parser.add_argument(
        "--gap-offset-image-px",
        type=int,
        default=0,
        metavar="DX",
        help="在识别出的图内 raw_x 上叠加的偏移（自然像素，正数向右）；用于系统性偏左/偏右时校准",
    )
    parser.add_argument(
        "--drag-extra-px",
        type=int,
        default=_DEFAULT_DRAG_EXTRA_PX,
        metavar="D",
        help=f"换算得到基准拖动像素后再累加的量（正数多拖），默认 {_DEFAULT_DRAG_EXTRA_PX}；"
        f"若仍偏短/偏长可再调；设为 0 关闭",
    )
    parser.add_argument(
        "--captcha-debug-dir",
        default=os.path.join(_script_dir, "icgoo_captcha_snapshots"),
        metavar="DIR",
        help="验证码调试图父目录，每次运行新建 run_时间戳_* 子目录；配合 --no-captcha-debug 可关闭",
    )
    parser.add_argument(
        "--no-captcha-debug",
        action="store_true",
        help="不写入验证码快照（不建目录、不截图）",
    )
    parser.add_argument(
        "--slide-attempts",
        type=int,
        default=_DEFAULT_SLIDE_ATTEMPTS,
        metavar="N",
        help=f"单轮内、**同一张**拼图上的滑动系数试探次数（默认 {_DEFAULT_SLIDE_ATTEMPTS}）；"
        f"若易盾在失败后自动换图（拼图 src 变化），会提前结束本轮并重新识别，不必等试满 N 次",
    )
    parser.add_argument(
        "--slider-backend",
        choices=("auto", "ddddocr", "opencv"),
        default="auto",
        metavar="MODE",
        help="滑块缺口识别：auto=默认用 ddddocr.slide_match，失败再 OpenCV；"
        "ddddocr=仅 ddddocr（不可用或未识别成功则报错）；opencv=仅用 OpenCV（调试）",
    )
    parser.add_argument(
        "--save-html-after",
        default=None,
        metavar="FILE",
        help="易盾/限流处理结束后，将当前页完整 HTML 写入该文件，便于对照解析调试",
    )
    parser.add_argument(
        "--parse-as-ickey",
        action="store_true",
        help="处理完验证后，按 icgoo_crawler.parse_search_results（字段与 ickey_crawler 一致）解析表格，"
        "将 JSON 数组打印到 stdout",
    )
    parser.add_argument(
        "--query-model",
        default=None,
        metavar="PN",
        help="解析时的 query_model 提示（默认从 --url 的 /search/{型号}/ 段提取）",
    )
    args = parser.parse_args()
    if args.slide_attempts < 1:
        args.slide_attempts = 1

    load_path = _resolve_load_cookies_path(args.cookies_file, args.no_cookies)
    save_path = _resolve_save_cookies_path(args.save_cookies, load_path)

    solver = SliderCaptchaSolver(
        cookies_load_path=load_path,
        cookies_save_path=save_path,
        slider_gap_backend=args.slider_backend,
    )

    if load_path:
        solver.load_shared_cookies()

    # 首次打开目标页面
    solver.page.get(args.url)
    time.sleep(2)

    # 自动登录（如果需要）
    did_auto_login = ensure_icgoo_auto_login(
        solver.page,
        target_url=args.url,
        username=args.user,
        password=args.password,
        login_url=args.login_url or DEFAULT_LOGIN_URL,
        captcha_wait_sec=args.captcha_wait,
        save_path=save_path,
        skip_auto_login=args.skip_auto_login,
    )

    # 等待页面稳定后，判断是否有验证码
    time.sleep(2)

    # 如果存在验证码，先尝试自动破解
    auto_success = False
    if solver.captcha_exists(timeout=3):
        print("检测到易盾滑块验证码，开始自动破解...")
        if args.slider_backend == "opencv":
            print("缺口定位：OpenCV（--slider-backend opencv）", flush=True)
        elif args.slider_backend == "ddddocr":
            print("缺口定位：仅 ddddocr（DdddOcr.slide_match）", flush=True)
        else:
            print("缺口定位：ddddocr.slide_match 优先，失败则 OpenCV", flush=True)
        captcha_run_dir = None
        if not args.no_captcha_debug and args.captcha_debug_dir:
            captcha_run_dir = _new_captcha_debug_run_dir(args.captcha_debug_dir)
            print(f"验证码调试图目录: {captcha_run_dir}", flush=True)
            _debug_write_text(
                os.path.join(captcha_run_dir, "00_session.txt"),
                f"url\t{args.url}\n"
                f"slider_backend\t{args.slider_backend}\n"
                f"drag_boost\t{args.drag_boost}\n"
                f"drag_extra_px\t{args.drag_extra_px}\n"
                f"time\t{time.strftime('%Y-%m-%d %H:%M:%S')}\n",
            )
            with open(os.path.join(captcha_run_dir, "attempts.tsv"), "w", encoding="utf-8") as log:
                log.write("attempt\td_try\tfactor\ttracks_sum\tverified_ok\n")
            _debug_write_text(
                os.path.join(captcha_run_dir, "00_README.txt"),
                "00_page_initial.html  拉取拼图时的整页 HTML\n"
                "00_background.png / 00_slider.png  识别用图\n"
                "00_detect.txt         缺口与拖动换算参数\n"
                "NN_viewport_d*.png    第 N 次滑动后视口截图\n"
                "NN_yidun_widget_*.png 第 N 次后 .yidun 区域截图（若可取）\n"
                "NN_page_after.html    第 N 次后整页 HTML\n"
                "NN_slide_meta.txt     第 N 次轨迹与是否判定成功\n"
                "attempts.tsv          各次汇总\n",
            )
        try:
            captcha_round = 0
            while solver.captcha_exists(timeout=3) and captcha_round < _MAX_CAPTCHA_REIDENTIFY_ROUNDS:
                captcha_round += 1
                if captcha_round > 1:
                    print(
                        f"第 {captcha_round} 轮：拼图/页面已更换，重新拉取图片并识别缺口…",
                        flush=True,
                    )
                    if captcha_run_dir:
                        try:
                            with open(
                                os.path.join(captcha_run_dir, "attempts.tsv"),
                                "a",
                                encoding="utf-8",
                            ) as log:
                                log.write(f"\n# captcha_round\t{captcha_round}\n")
                        except OSError:
                            pass
                    time.sleep(1.0)

                bg_base64, slider_base64, drag_scale_x = solver.get_images(debug_run_dir=captcha_run_dir)
                if captcha_run_dir:
                    bg_path = os.path.join(captcha_run_dir, "00_background.png")
                    slider_path = os.path.join(captcha_run_dir, "00_slider.png")
                else:
                    bg_path = "background.png"
                    slider_path = "slider.png"
                solver.save_base64_image(bg_base64, bg_path)
                solver.save_base64_image(slider_base64, slider_path)
                raw_x, distance = solver.detect_gap(
                    bg_path,
                    slider_path,
                    drag_scale_x,
                    drag_boost=args.drag_boost,
                    gap_offset_image_px=args.gap_offset_image_px,
                    drag_extra_px=args.drag_extra_px,
                )
                rd = getattr(solver, "_gap_raw_detected", None)
                if args.gap_offset_image_px != 0 and rd is not None:
                    print(
                        f"识别图内 x≈{rd}px，叠加 --gap-offset-image-px={args.gap_offset_image_px} → "
                        f"用于换算的 x≈{raw_x}px",
                        flush=True,
                    )
                print(
                    f"检测到缺口: 图内 x≈{raw_x}px（主算法选用），换算比例 {drag_scale_x:.4f}，"
                    f"补偿系数 {args.drag_boost:.3f}，拖动加量 {args.drag_extra_px}px，基准拖动≈{distance}px",
                    flush=True,
                )
                if getattr(solver, "_slide_match_blended", False):
                    print(
                        "说明：边缘与灰度两路置信度均偏低且横向相差较大，主 raw_x 已取两路中点（不再用极端辅基准）。",
                        flush=True,
                    )
                if getattr(solver, "_slide_match_diverge_prefer_false", False):
                    print(
                        "说明：slide_match 两路左缘相差较大，已按与 test_ddddocr_slide 相同策略优先采用 "
                        "simple_target=False（Canny），未采用 simple=True 的较高 confidence（该路易虚高）。",
                        flush=True,
                    )
                if solver._last_gap_backend == "ddddocr":
                    dc = solver._dddd_confidence
                    ds = solver._dddd_simple_target
                    st = "中点融合" if ds is None else str(ds)
                    print(
                        "缺口识别: ddddocr.slide_match（官方 DdddOcr + SlideEngine），"
                        f"simple_target={st}，"
                        + (f"confidence={dc:.3f}" if dc is not None else "confidence=（无）"),
                        flush=True,
                    )
                elif solver._last_gap_backend == "ddddocr_shim":
                    dc = solver._dddd_confidence
                    ds = solver._dddd_simple_target
                    st = "中点融合" if ds is None else str(ds)
                    print(
                        "缺口识别: 与 ddddocr SlideEngine 等价的 slide_match（纯 OpenCV，未加载 onnxruntime），"
                        f"simple_target={st}，"
                        + (f"confidence={dc:.3f}" if dc is not None else "confidence=（无）"),
                        flush=True,
                    )
                elif solver._last_gap_backend == "opencv" and args.slider_backend == "auto":
                    print(
                        "缺口识别：已回退双通道 OpenCV（灰度+Canny）；"
                        "修复 onnxruntime DLL 后可使用完整 ddddocr 包。",
                        flush=True,
                    )
                if solver._last_gap_backend in ("ddddocr", "ddddocr_shim"):
                    _rxf = solver._dddd_raw_left_simple_false
                    _rxt = solver._dddd_raw_left_simple_true
                    if _rxf is not None and _rxt is not None:
                        print(
                            f"slide_match 两路图内左缘 x：边缘(Canny)={_rxf}，灰度模板={_rxt}（|差|={abs(_rxf - _rxt)}px）",
                            flush=True,
                        )
                distance_alt = None
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
                        int(round(alt_raw * drag_scale_x * args.drag_boost)) + args.drag_extra_px,
                    )
                    if distance_alt == distance:
                        distance_alt = None
                    else:
                        print(
                            f"OpenCV 灰度 x={xg}（{sg:.3f}） Canny x={xc}（{scn:.3f}）相差 {abs(xg - xc)}px → "
                            f"第1/3/5…次用主拖动≈{distance}px，第2/4/6…次用辅拖动≈{distance_alt}px",
                            flush=True,
                        )
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
                            int(round(alt_raw * drag_scale_x * args.drag_boost)) + args.drag_extra_px,
                        )
                        if distance_alt == distance:
                            distance_alt = None
                        else:
                            print(
                                f"slide_match 边缘 x={xf}px，灰度模板 x={xt}px（相差 {ddx}）→ "
                                f"第1/3/5…次主≈{distance}px，第2/4/6…次辅≈{distance_alt}px",
                                flush=True,
                            )
                    elif (
                        solver._last_gap_backend in ("ddddocr", "ddddocr_shim")
                        and xf is not None
                        and xt is not None
                        and ddx > _SLIDE_MATCH_ALT_MAX_RAW_DELTA_PX
                    ):
                        print(
                            f"slide_match 两路相差 {ddx}px（>{_SLIDE_MATCH_ALT_MAX_RAW_DELTA_PX}），"
                            "不启用辅基准，避免另一路为误峰。",
                            flush=True,
                        )
                if solver._last_opencv_match_mode:
                    sc = solver._last_opencv_match_score
                    print(
                        f"OpenCV 主选: mode={solver._last_opencv_match_mode} "
                        f"score={sc:.3f}" + ("（置信偏低，缺口 x 可能不准）" if sc is not None and sc < 0.35 else ""),
                        flush=True,
                    )
                det_extra = f"captcha_round\t{captcha_round}\n"
                det_extra += f"gap_backend\t{solver._last_gap_backend or 'unknown'}\n"
                if solver._last_gap_backend in ("ddddocr", "ddddocr_shim"):
                    det_extra += f"slide_match_blended\t{getattr(solver, '_slide_match_blended', False)}\n"
                    det_extra += (
                        f"slide_match_diverge_prefer_false\t"
                        f"{getattr(solver, '_slide_match_diverge_prefer_false', False)}\n"
                    )
                    if solver._dddd_simple_target is not None:
                        det_extra += f"dddd_simple_target\t{solver._dddd_simple_target}\n"
                    if solver._dddd_confidence is not None:
                        det_extra += f"dddd_confidence\t{solver._dddd_confidence}\n"
                    if solver._dddd_conf_simple_false is not None:
                        det_extra += f"dddd_conf_simple_false\t{solver._dddd_conf_simple_false}\n"
                    if solver._dddd_conf_simple_true is not None:
                        det_extra += f"dddd_conf_simple_true\t{solver._dddd_conf_simple_true}\n"
                    if solver._dddd_raw_left_simple_false is not None:
                        det_extra += f"dddd_raw_x_simple_false\t{solver._dddd_raw_left_simple_false}\n"
                    if solver._dddd_raw_left_simple_true is not None:
                        det_extra += f"dddd_raw_x_simple_true\t{solver._dddd_raw_left_simple_true}\n"
                    if getattr(solver, "_gap_ensemble_pick", None):
                        det_extra += f"gap_ensemble_pick\t{solver._gap_ensemble_pick}\n"
                if solver._last_opencv_match_mode:
                    det_extra += (
                        f"opencv_mode_primary\t{solver._last_opencv_match_mode}\n"
                        f"opencv_score_primary\t{solver._last_opencv_match_score}\n"
                    )
                if xg is not None and xc is not None:
                    det_extra += f"opencv_x_gray\t{xg}\nopencv_x_canny\t{xc}\n"
                    if sg is not None and scn is not None:
                        det_extra += f"opencv_score_gray\t{sg}\nopencv_score_canny\t{scn}\n"
                if distance_alt is not None:
                    det_extra += f"base_drag_alt_px\t{distance_alt}\n"
                if captcha_run_dir:
                    det_name = "00_detect.txt" if captcha_round <= 1 else f"round_{captcha_round:02d}_detect.txt"
                    _debug_write_text(
                        os.path.join(captcha_run_dir, det_name),
                        f"raw_x_px\t{raw_x}\n"
                        f"raw_x_detected\t{getattr(solver, '_gap_raw_detected', '')}\n"
                        f"gap_offset_image_px\t{args.gap_offset_image_px}\n"
                        f"drag_scale_x\t{drag_scale_x}\n"
                        f"drag_boost\t{args.drag_boost}\n"
                        f"drag_extra_px\t{args.drag_extra_px}\n"
                        f"base_drag_px\t{distance}\n"
                        f"slide_attempts\t{args.slide_attempts}\n"
                        f"slide_factors\t{','.join(str(x) for x in _SLIDE_DISTANCE_FACTORS)}\n"
                        f"{det_extra}"
                        f"background\t00_background.png\n"
                        f"slider\t00_slider.png\n",
                    )
                slide_ok, captcha_replaced = solver.slide_verification(
                    distance,
                    max_attempts=args.slide_attempts,
                    debug_run_dir=captcha_run_dir,
                    distance_alt=distance_alt,
                )
                if slide_ok:
                    time.sleep(0.8)
                    # slide_verification 已为真即视为通过；外层勿再用「仍能找到拼图 img」否定结果
                    # （易盾/ICGOO 常保留隐藏 DOM，captcha_exists 仍真但业务已放行）
                    auto_success = True
                    if solver.captcha_exists(timeout=2.0):
                        print(
                            "自动破解成功（轨道与轮询已判定通过；拼图节点若仍短暂留在 DOM 不影响）。",
                            flush=True,
                        )
                    else:
                        print("自动破解成功！", flush=True)
                    break
                elif captcha_replaced:
                    print("已换图或整页刷新，将用新拼图重新识别缺口并再试滑动。", flush=True)
                    continue
                else:
                    print("自动破解失败（同一张拼图已试尽），将尝试手动等待...", flush=True)
                    break
            if (
                not auto_success
                and captcha_round >= _MAX_CAPTCHA_REIDENTIFY_ROUNDS
                and solver.captcha_exists(timeout=1.5)
            ):
                print(
                    f"已达到换图重识别上限（{_MAX_CAPTCHA_REIDENTIFY_ROUNDS} 轮），仍未通过，将尝试手动等待。",
                    flush=True,
                )
        except IcgooBrowserDisconnectedError as e:
            print(f"浏览器连接已断开，自动破解中止：{e}", flush=True)
            if captcha_run_dir:
                _debug_write_text(
                    os.path.join(captcha_run_dir, "00_browser_disconnected.txt"),
                    f"{e}\n",
                )
            auto_success = False
        except Exception as e:
            print(f"自动破解过程出错: {e}")
            if captcha_run_dir:
                _debug_write_text(
                    os.path.join(captcha_run_dir, "00_exception.txt"),
                    f"{type(e).__name__}: {e}\n",
                )
            auto_success = False
    else:
        print("未检测到滑块验证码，继续执行。")

    # 如果自动破解失败，或者一开始就没有验证码但后续触发了限流/其他验证，则调用手动等待
    # 注意：wait_for_manual_captcha_or_rate_limit 内部会持续检测易盾或限流提示，并等待用户手动处理
    if not auto_success:
        captcha_cleared = wait_for_manual_captcha_or_rate_limit(
            solver.page, timeout_sec=args.captcha_wait, quiet=False
        )
        if not captcha_cleared:
            print(
                "易盾/限流等待超时，已结束运行；请在浏览器中完成验证或稍后再试，然后重新执行本脚本。",
                flush=True,
            )
            sys.exit(1)
        else:
            print("手动验证已通过或限流已解除。")

    # 检查登录状态（仅当执行过自动登录且仍显示未登录时给出提示）
    if did_auto_login and icgoo_suggests_not_logged_in(solver.page):
        print("自动登录后页面仍显示未登录，请检查账号、密码或手动完成验证", flush=True)

    # 最终保存 Cookie（如果指定了保存路径）
    if save_path:
        solver.save_shared_cookies()
        print(f"已保存 Cookie 至 {save_path}", flush=True)

    query_model = (args.query_model or "").strip() or _keyword_from_icgoo_search_url(args.url)

    if args.save_html_after:
        outp = os.path.abspath(args.save_html_after)
        parent = os.path.dirname(outp)
        if parent and not os.path.isdir(parent):
            os.makedirs(parent, exist_ok=True)
        try:
            html_out = solver.page.html or ""
            with open(outp, "w", encoding="utf-8") as f:
                f.write(html_out)
            print(f"已保存破解/验证后页面 HTML（{len(html_out)} 字符）: {outp}", flush=True)
        except OSError as ex:
            print(f"保存 HTML 失败: {ex}", flush=True)

    if args.parse_as_ickey:
        print("等待列表区域加载后按 ickey 对齐字段解析…", flush=True)
        if not wait_for_results(solver.page, timeout=25.0, quiet=False):
            print("等待表格超时，仍尝试解析当前 DOM…", flush=True)
            time.sleep(2)
        else:
            time.sleep(1)
        rows = parse_search_results(solver.page, query_model, quiet=False)
        print(json.dumps(rows, ensure_ascii=False, indent=2), flush=True)
        print(f"共解析 {len(rows)} 条（query_model={query_model!r}）", flush=True)
    rows = parse_search_results(solver.page, query_model, quiet=False)
    print(json.dumps(rows, ensure_ascii=False, indent=2), flush=True)
    print(f"共解析 {len(rows)} 条（query_model={query_model!r}）", flush=True)
    print("脚本执行完毕，浏览器保持打开。如需关闭，请手动关闭。")


if __name__ == "__main__":
    main()