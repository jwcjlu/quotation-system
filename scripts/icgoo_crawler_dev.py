"""
ICGOO 滑块验证码调试脚本（易盾 / 阿里云 aliyunCaptcha）— Cookie 与正式爬虫 icgoo_crawler.py 共用同一 JSON 格式。

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

  # 汇总多轮快照标定并生成环境变量（默认不写 --no-captcha-debug 即每轮落盘 PNG + JSONL）：
  #   python scripts/tune_icgoo_captcha_from_jsonl.py scripts/icgoo_captcha_snapshots/run_*/aliyun_calibration.jsonl \\
  #     --write-env-cmd scripts/icgoo_tuned_env.cmd
  # 然后在 CMD 先运行 scripts\\icgoo_tuned_env.cmd 再启动本脚本。

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
import shutil
import sys
import tempfile
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
from icgoo_aliyun_captcha import (  # noqa: E402
    _XP_ALIYUN_BG_IMG,
    _XP_ALIYUN_PUZZLE_IMG,
    _XP_ALIYUN_REFRESH,
    _XP_ALIYUN_SLIDER_HANDLE,
    _XP_ALIYUN_SLIDING_BODY,
    _aliyun_captcha_layer_visible,
    _aliyun_host_context,
    ALIYUN_CAPTCHA_PUZZLE_WIDTH_NAT_PX,
    aliyun_challenge_active,
    aliyun_drag_scale_x,
    aliyun_get_images,
    aliyun_puzzle_src_compare_key,
    aliyun_verification_success_visible,
    get_aliyun_drag_scale_mult,
    get_aliyun_systematic_drag_extra_px,
    aliyun_post_release_decoy_click_enabled,
    aliyun_fail_same_image_retry_mouse_wiggle_enabled,
    aliyun_fail_same_image_retry_settle_sec,
    aliyun_fail_same_image_retry_wait_slider_sec,
    aliyun_fail_src_churn_retry_settle_sec,
    aliyun_slide_pre_release_extra_hold_sec,
    aliyun_slide_y_jitter_cap_px,
    aliyun_slide_y_jitter_enabled,
    aliyun_slide_y_jitter_sigma_px,
)
from icgoo_aliyun_gap import ALIYUN_CALIBRATION_JSONL_BASENAME  # noqa: E402
from icgoo_yidun_solver import (  # noqa: E402
    _compute_slide_distance_alt,
    build_gap_calibration_record,
    write_gap_overlay_png,
)

_DEFAULT_LOCAL_COOKIES = os.path.join(_script_dir, "icgoo_cookies.json")

# 松手瞬间与视口截图同名的 ``*_geometry.json``：阿里云 iframe 内 DOM 几何，供将 raw_x（自然像素）换算到视口 CSS 像素并与拼图块位置对比。
_ALIYUN_RELEASE_GEOMETRY_JS = """
return (function(){
  function br(el) {
    if (!el) return null;
    var r = el.getBoundingClientRect();
    return {left: r.left, top: r.top, width: r.width, height: r.height};
  }
  var img = document.querySelector('#aliyunCaptcha-img');
  var puz = document.querySelector('#aliyunCaptcha-puzzle');
  var body = document.querySelector('#aliyunCaptcha-sliding-body');
  var handle = document.querySelector('#aliyunCaptcha-sliding-slider');
  var out = {
    bg_img: br(img),
    puzzle: br(puz),
    sliding_body: br(body),
    handle: br(handle),
    img_naturalWidth: img ? img.naturalWidth : null,
    img_naturalHeight: img ? img.naturalHeight : null,
    viewport_innerWidth: window.innerWidth,
    viewport_innerHeight: window.innerHeight,
  };
  try {
    if (img && out.bg_img && out.bg_img.width > 0 && img.naturalWidth > 0) {
      out.scale_css_px_per_nat_x = out.bg_img.width / img.naturalWidth;
    }
    if (body && handle) {
      var bb = body.getBoundingClientRect();
      var bh = handle.getBoundingClientRect();
      out.handle_left_minus_track_left = bh.left - bb.left;
      out.track_client_width = bb.width;
      out.handle_client_width = bh.width;
      out.travel_css_px = Math.max(0, bb.width - bh.width);
    }
  } catch (e) {}
  return out;
})();
"""


def _captcha_debug_release_screenshot_delay_sec() -> float:
    """
    松手后、截 ``*_viewport_release_*.png`` 与读 DOM 前的等待（秒）。
    过短：手柄/拼图 rect 偶未稳定（如 handle 全 0）；过长：阿里云失败时可能已换拼图 src，截到新品。
    环境变量 ``ICGOO_CAPTCHA_RELEASE_SCREENSHOT_DELAY_SEC`` 可覆盖，合法约 0～2.5。
    """
    raw = (os.environ.get("ICGOO_CAPTCHA_RELEASE_SCREENSHOT_DELAY_SEC") or "").strip()
    if raw:
        try:
            t = float(raw)
            if 0.0 <= t <= 2.5:
                return t
        except ValueError:
            pass
    return 0.22


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
# 实测阿里云/易盾常略少拖；标定汇总（tune_icgoo_captcha_from_jsonl）倾向略加大，默认 1.10（可用 --drag-boost 覆盖）
_DEFAULT_DRAG_DISTANCE_BOOST = 1.10
# 换算后拖动距离仍常略短时，在基准拖动像素上直接累加（可用 --drag-extra-px 覆盖；实测约 +16px）
_DEFAULT_DRAG_EXTRA_PX = 16
# 同一张图上多次滑动：相对基准距离的系数（宜覆盖偏短/偏长，略加跨度便于识别略偏时仍有机会命中）
# 实测易盾上 1.02 略短时 1.08 常能过，故将 1.08 提前，减少无效尝试次数
_SLIDE_DISTANCE_FACTORS = (1.02, 1.08, 1.0, 0.92, 0.86, 1.14, 0.96, 1.04)
_DEFAULT_SLIDE_ATTEMPTS = 7
# slide_match 边缘 vs 灰度 两路 raw_x 超过此差值（图内像素）时不用辅基准（另一峰常为误匹配）。
# 与阿里云拼块固定宽一致（见 ``ALIYUN_CAPTCHA_PUZZLE_WIDTH_NAT_PX``）；易盾路径沿用同数值作上界。
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


def _append_fail_new_image_review_row(
    run_dir: str,
    *,
    captcha_round: int,
    gap_overlay_png: str,
    raw_x_px: int,
    base_drag_px: int,
    page_url: str,
) -> None:
    """
    服务端未过且换题时追加一行，便于对照 ``*_background_gap_marked.png`` 人工判断：
    红框若已对准真缺口，则问题多在拖动/轨宽/轨迹；否则先调识别。
    """
    path = os.path.join(run_dir, "fail_new_image_review.tsv")
    need_header = not os.path.isfile(path)
    try:
        with open(path, "a", encoding="utf-8") as f:
            if need_header:
                f.write(
                    "captcha_round\tgap_overlay_png\traw_x_px\tbase_drag_px\tpage_url\t"
                    "manual_note_overlay_ok_yes_no\n"
                )
            go = (gap_overlay_png or "").replace("\t", " ")
            pu = (page_url or "").replace("\t", " ")
            f.write(
                f"{int(captcha_round)}\t{go}\t{int(raw_x_px)}\t{int(base_drag_px)}\t{pu}\t\n"
            )
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
        # auto：优先 ddddocr.slide_match，失败再 OpenCV；ddddocr：仅 ddddocr；opencv：仅 OpenCV（调试）；
        # slidercracker：可选 PyPI 包（内部 simple_target=True，依赖版本较老）
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
        self._effective_drag_extra_px = 0
        self._aliyun_drag_extra_applied = 0
        self.calibration_jsonl_path: str | None = None

    def _emit_calibration(self, record: dict) -> None:
        if not self.calibration_jsonl_path:
            return
        try:
            from icgoo_aliyun_gap import append_aliyun_calibration_jsonl

            append_aliyun_calibration_jsonl(self.calibration_jsonl_path, dict(record))
        except Exception:
            pass

    def _verbose(self, msg: str) -> None:
        print(msg, flush=True)

    def _yidun_info(self, msg: str) -> None:
        """与 ``IcgooYidunSliderSolver`` 对齐，供委托 ``detect_gap_position`` 打日志。"""
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
                print(
                    "提示：ddddocr 包因 onnxruntime 等依赖无法导入（常见于 DLL 初始化失败），"
                    "已自动使用与 ddddocr 官方 SlideEngine 相同的纯 OpenCV 滑块算法（无需 ONNX）。",
                    flush=True,
                )
                print(f"  （原始异常: {self._dddd_init_error}）", flush=True)
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
        """计算灰度/Canny；阿里云 aux 时另算 Alpha-mask、PSC、多尺度 shadow（见 gap-position-detection.md）。"""
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
            from icgoo_aliyun_gap import (
                aliyun_shadow_multiscale_match_x,
                aliyun_shadow_multiscale_match_x_edges,
            )
            from icgoo_yidun_solver import (
                _opencv_alpha_mask_match_left_x,
                _puzzle_slider_captcha_left_x_score,
            )

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

    def _ensemble_auto_best_gap(
        self,
        bw: int,
        tw: int,
        *,
        aliyun_include_opencv_in_ensemble: bool = False,
    ) -> tuple[int, float, str] | None:
        """与 ``IcgooYidunSliderSolver._ensemble_auto_best_gap`` 一致（含阿里云 OpenCV 竞争）。"""
        from icgoo_yidun_solver import IcgooYidunSliderSolver

        return IcgooYidunSliderSolver._ensemble_auto_best_gap(
            self,
            bw,
            tw,
            aliyun_include_opencv_in_ensemble=aliyun_include_opencv_in_ensemble,
        )

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

    def _aliyun_challenge_active(self, timeout: float = 0.85) -> bool:
        return aliyun_challenge_active(self.page, timeout)

    def _aliyun_drag_scale_x(self, bg_ele, natural_width: int) -> float:
        return aliyun_drag_scale_x(
            self.page,
            bg_ele,
            natural_width,
            fallback_display_scale=self._yidun_bg_display_scale_x,
        )

    def _aliyun_get_images(self, debug_run_dir: str | None = None) -> tuple[str, str, float]:
        _ = debug_run_dir
        return aliyun_get_images(
            self.page,
            log_warning=None,
            log_info=None,
            fallback_display_scale=self._yidun_bg_display_scale_x,
            err_not_found_nodes="未找到阿里云验证码 #aliyunCaptcha-img / #aliyunCaptcha-puzzle",
            err_src_not_ready="阿里云验证码图片 src 尚未就绪",
        )

    def captcha_exists(self, timeout: float = 2.0) -> bool:
        """判断页面是否存在易盾或阿里云滑块验证码。"""
        ta = min(1.0, timeout) if timeout > 0 else timeout
        if self._aliyun_challenge_active(timeout=ta):
            return True
        try:
            ele = self.page.ele(_XP_YIDUN_JIGSAW_IMG, timeout=timeout)
            return ele is not None
        except Exception:
            return False

    def _yidun_jigsaw_src(self) -> str:
        """易盾拼图小块 src。"""
        try:
            el = self.page.ele(_XP_YIDUN_JIGSAW_IMG, timeout=0.85)
            if el:
                return (el.attr("src") or "").strip()
        except Exception:
            pass
        return ""

    def _puzzle_piece_src(self) -> str:
        if self._aliyun_challenge_active(timeout=0.5):
            ctx = _aliyun_host_context(self.page) or self.page
            try:
                el = ctx.ele(_XP_ALIYUN_PUZZLE_IMG, timeout=0.85)
                if el:
                    return (el.attr("src") or "").strip()
            except Exception:
                pass
            return ""
        return self._yidun_jigsaw_src()

    def get_images(self, debug_run_dir: str | None = None) -> tuple[str, str, float]:
        """获取滑块和背景图（data: 或 https）；第三项为拖动换算比例（优先 滑轨可行程/图自然宽）。"""
        if self._aliyun_challenge_active(timeout=min(2.0, 1.5)):
            return self._aliyun_get_images(debug_run_dir=debug_run_dir)
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
        与 ``icgoo_yidun_solver.IcgooYidunSliderSolver.detect_gap_position`` 同一实现（含阿里云预处理与共识）。
        """
        from icgoo_yidun_solver import IcgooYidunSliderSolver

        return IcgooYidunSliderSolver.detect_gap_position(self, bg_path, slider_path)

    def detect_gap(
        self,
        bg_path: str,
        slider_path: str,
        drag_scale_x: float = 1.0,
        drag_boost: float = 1.0,
        gap_offset_image_px: int = 0,
        drag_extra_px: int = _DEFAULT_DRAG_EXTRA_PX,
    ) -> tuple[int, int]:
        """与 ``IcgooYidunSliderSolver.detect_gap`` 一致（含阿里云专用 scale 乘子与系统性加量）。"""
        from icgoo_yidun_solver import IcgooYidunSliderSolver

        return IcgooYidunSliderSolver.detect_gap(
            self,
            bg_path,
            slider_path,
            drag_scale_x,
            drag_boost=drag_boost,
            gap_offset_image_px=gap_offset_image_px,
            drag_extra_px=drag_extra_px,
        )

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

    def _captcha_debug_save_on_release(
        self,
        debug_run_dir: str | None,
        attempt: int,
        d_try: int,
        f: float,
        tracks: list[int],
        captcha_round: int,
    ) -> None:
        """
        松手后先短等浏览器排版/绘制，再截图并读 DOM（仍早于校验长等待与换图检测），减少
        「截太早 handle 未就绪」；默认延迟平衡「未换图前落点帧」与「不宜拖到服务端已下发新拼图」。
        """
        if not debug_run_dir:
            return
        n = attempt + 1
        rc = int(captcha_round) if captcha_round is not None else 0
        stem = (
            f"{n:02d}_viewport_release_r{rc}_d{d_try}"
            if rc > 0
            else f"{n:02d}_viewport_release_d{d_try}"
        )
        delay_sec = _captcha_debug_release_screenshot_delay_sec()
        try:
            time.sleep(delay_sec)
            self.page.get_screenshot(path=debug_run_dir, name=f"{stem}.png")
        except Exception as e:
            _debug_write_text(
                os.path.join(debug_run_dir, f"{stem}_error.txt"),
                str(e),
            )
        self._captcha_debug_write_aliyun_release_geometry(
            debug_run_dir, stem, rc, d_try
        )

    def _captcha_debug_write_aliyun_release_geometry(
        self,
        debug_run_dir: str,
        stem: str,
        captcha_round: int,
        d_try: int,
    ) -> None:
        """写出 ``{stem}_geometry.json``，与松手 PNG 同名；便于用 raw_x 与 puzzle/bg 的 getBoundingClientRect 复盘偏差。"""
        rec: dict = {
            "schema": 1,
            "purpose": "aliyun_release_dom",
            "captcha_round": int(captcha_round) if captcha_round else 0,
            "d_try_px": int(d_try),
            "release_screenshot_delay_sec": round(
                float(_captcha_debug_release_screenshot_delay_sec()), 4
            ),
            "capture_phase": "after_release_after_paint_before_verify_wait",
            "how_to_compare": (
                "同轮 round_NN_detect.txt 的 raw_x_px 为图内自然像素；若 dom.scale_css_px_per_nat_x 存在，"
                "则槽位左缘视口 x ≈ dom.bg_img.left + raw_x_px * scale_css_px_per_nat_x；"
                "拼图块视口左缘 ≈ dom.puzzle.left。差值(拼图-槽位)为正表示拖过头，为负表示少拖（近似，受阴影/子像素影响）。"
            ),
        }
        ctx = _aliyun_host_context(self.page)
        if ctx is None:
            rec["error"] = "no_aliyun_host_context"
        else:
            try:
                geo = ctx.run_js(_ALIYUN_RELEASE_GEOMETRY_JS)
                if geo is not None:
                    rec["dom"] = geo
            except Exception as e:
                rec["error"] = f"run_js:{type(e).__name__}:{e}"
        self._captcha_debug_apply_release_residual(rec)
        try:
            with open(
                os.path.join(debug_run_dir, f"{stem}_geometry.json"),
                "w",
                encoding="utf-8",
            ) as f:
                json.dump(rec, f, ensure_ascii=False, indent=2)
        except OSError:
            pass

    def _captcha_debug_apply_release_residual(self, rec: dict) -> None:
        """
        根据 ``_captcha_debug_residual_raw_x``（与 detect 的图内槽左缘一致）与 ``rec['dom']`` 估算
        视口 CSS 下「拼图块左缘 − 槽左缘」残差，写入 rec['residual_estimate'] 及 ``_cal_release_*``，
        供 ``build_slide_attempt_calibration_record(..., solver=self)`` 写入 aliyun_calibration.jsonl。
        """
        for name in (
            "_cal_release_residual_css_x",
            "_cal_release_slot_left_css_x_approx",
            "_cal_release_puzzle_left_css_x",
            "_cal_release_scale_css_px_per_nat_x",
            "_cal_release_raw_x_image_px",
            "_cal_release_residual_note",
        ):
            setattr(self, name, None)

        def _rq(x) -> float | None:
            if x is None:
                return None
            try:
                return round(float(x), 3)
            except (TypeError, ValueError):
                return None

        raw_any = getattr(self, "_captcha_debug_residual_raw_x", None)
        dom = rec.get("dom")
        note: str | None = None
        raw_i: int | None = None
        try:
            if raw_any is not None:
                raw_i = int(raw_any)
        except (TypeError, ValueError):
            note = "raw_x_invalid"

        if rec.get("error") and note is None:
            note = str(rec["error"])[:160]

        scale_v = slot_left = puzzle_left = residual = None
        if not isinstance(dom, dict):
            if note is None:
                note = "no_dom"
        elif raw_i is None and note is None:
            note = "no_raw_x_for_residual"
        elif raw_i is not None:
            scale_v = dom.get("scale_css_px_per_nat_x")
            bg = dom.get("bg_img") if isinstance(dom.get("bg_img"), dict) else {}
            pz = dom.get("puzzle") if isinstance(dom.get("puzzle"), dict) else {}
            try:
                if scale_v is not None and bg.get("left") is not None:
                    slot_left = float(bg["left"]) + float(raw_i) * float(scale_v)
                if pz.get("left") is not None:
                    puzzle_left = float(pz["left"])
            except (TypeError, ValueError):
                note = note or "dom_coerce_failed"
            if slot_left is not None and puzzle_left is not None:
                residual = puzzle_left - slot_left
            elif note is None:
                note = "missing_slot_or_puzzle_or_scale"

        sub = {
            "release_residual_css_x": _rq(residual),
            "release_slot_left_css_x_approx": _rq(slot_left),
            "release_puzzle_left_css_x": _rq(puzzle_left),
            "release_scale_css_px_per_nat_x": _rq(scale_v),
            "release_raw_x_image_px": raw_i,
            "release_residual_note": note,
        }
        rec["residual_estimate"] = sub
        self._cal_release_residual_css_x = sub["release_residual_css_x"]
        self._cal_release_slot_left_css_x_approx = sub["release_slot_left_css_x_approx"]
        self._cal_release_puzzle_left_css_x = sub["release_puzzle_left_css_x"]
        self._cal_release_scale_css_px_per_nat_x = sub["release_scale_css_px_per_nat_x"]
        self._cal_release_raw_x_image_px = sub["release_raw_x_image_px"]
        self._cal_release_residual_note = sub["release_residual_note"]

    def _captcha_debug_save_after_attempt(
        self,
        debug_run_dir: str | None,
        attempt: int,
        d_try: int,
        f: float,
        tracks: list[int],
        verified_ok: bool,
    ) -> None:
        """每次滑动在校验与等待之后保存视口截图、整页 HTML、易盾区域截图（若可定位）、轨迹元数据。"""
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

    def _no_visible_yidun_panel_means_passed(self) -> bool:
        """与 ``IcgooYidunSliderSolver`` 一致，避免 dev 与生产通过判定漂移。"""
        from icgoo_yidun_solver import IcgooYidunSliderSolver

        return IcgooYidunSliderSolver._no_visible_yidun_panel_means_passed(self)

    def _slide_passed_or_captcha_gone(self) -> tuple[bool, str]:
        """与 ``IcgooYidunSliderSolver`` 一致（含阿里云 ``captcha_exists`` 假阳性防护）。"""
        from icgoo_yidun_solver import IcgooYidunSliderSolver

        return IcgooYidunSliderSolver._slide_passed_or_captcha_gone(self)

    def slide_verification(
        self,
        distance,
        max_attempts: int = _DEFAULT_SLIDE_ATTEMPTS,
        debug_run_dir: str | None = None,
        distance_alt: int | None = None,
        *,
        captcha_round: int = 0,
    ) -> tuple[bool, bool]:
        """
        委托 ``IcgooYidunSliderSolver.slide_verification``，与生产共用滑动 / 标定 JSONL / 通过判定；
        ``debug_run_dir`` 非空时：松手后经短延迟再 ``*_viewport_release_*``（见 ``_captcha_debug_release_screenshot_delay_sec``），校验后再 ``*_viewport_d*``。
        """
        from icgoo_yidun_solver import IcgooYidunSliderSolver

        def _after_attempt(
            _solver: object,
            attempt: int,
            d_try: int,
            f: float,
            tracks: list[int],
            verified_ok: bool,
        ) -> None:
            self._captcha_debug_save_after_attempt(
                debug_run_dir, attempt, d_try, f, tracks, verified_ok
            )

        def _on_release(
            _solver: object,
            attempt: int,
            d_try: int,
            f: float,
            tracks: list[int],
            round_idx: int,
        ) -> None:
            self._captcha_debug_save_on_release(
                debug_run_dir, attempt, d_try, f, tracks, round_idx
            )

        return IcgooYidunSliderSolver.slide_verification(
            self,
            distance,
            max_attempts=max_attempts,
            distance_alt=distance_alt,
            captcha_round=captcha_round,
            after_slide_attempt=_after_attempt if debug_run_dir else None,
            after_slide_release=_on_release if debug_run_dir else None,
        )

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
        """与 ``IcgooYidunSliderSolver`` 一致，避免 dev 与生产通过判定漂移。"""
        from icgoo_yidun_solver import IcgooYidunSliderSolver

        return IcgooYidunSliderSolver.check_success(self)


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
        "--aliyun-drag-scale-mult",
        type=float,
        default=None,
        metavar="M",
        help="仅阿里云拼图：对 drag_scale_x 再乘 M（轨宽/图宽仍偏时调），默认 1、合法约 0.5～2；"
        "等同环境变量 ICGOO_ALIYUN_DRAG_SCALE_MULT；易盾忽略",
    )
    parser.add_argument(
        "--aliyun-systematic-extra-px",
        type=int,
        default=None,
        metavar="PX",
        help="仅阿里云：覆盖默认系统性拖动加量（像素，默认见 icgoo_aliyun_captcha）；等同环境变量 "
        "ICGOO_ALIYUN_SYSTEMATIC_DRAG_EXTRA_PX；易盾忽略",
    )
    parser.add_argument(
        "--aliyun-slide-human-slow",
        action="store_true",
        default=1,
        help="仅阿里云：放慢单步拖动、步间停顿与松手前静候，并略延长松手后再轮询成功态；"
        "减轻「落点已准仍失败」（轨迹/风控）。等同 ICGOO_ALIYUN_SLIDE_HUMAN_SLOW=1；易盾忽略",
    )
    parser.add_argument(
        "--aliyun-slide-pre-extra-hold-sec",
        type=float,
        default=0.5,
        metavar="SEC",
        help="仅阿里云：轨迹到位后、松手前再静止 SEC 秒（叠加在随机 pre_release 之后）；"
        "等同 ICGOO_ALIYUN_SLIDE_PRE_RELEASE_EXTRA_HOLD_SEC；易盾忽略",
    )
    parser.add_argument(
        "--aliyun-post-release-decoy-click",
        action="store_true",
        default=1,
        help="仅阿里云：松开后指针移到落点附近（随机半径）再左键点一下，非点在精确释放坐标；"
        "等同 ICGOO_ALIYUN_POST_RELEASE_DECOY_CLICK=1；易盾忽略",
    )
    parser.add_argument(
        "--aliyun-slide-y-jitter",
        action="store_true",
        default=1,
        help="仅阿里云：拖动段每步叠加有界纵向随机游走（非严格水平线），配合 --aliyun-slide-human-slow 更佳；"
        "等同 ICGOO_ALIYUN_SLIDE_Y_JITTER=1；可调 CAP/SIGMA 环境变量；易盾忽略",
    )
    parser.add_argument(
        "--no-aliyun-raw-x-segment",
        action="store_true",
        help="仅阿里云：关闭按图内 raw_x 分段的拖动乘子/加量（等同 ICGOO_ALIYUN_RAW_X_SEGMENT_ADJUST=0）",
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
        "--show-gap-overlay",
        action="store_true",
        help="识别缺口后用 OpenCV 窗口显示带矩形的背景图（按任意键关闭后再继续；需本地 GUI）",
    )
    parser.add_argument(
        "--aliyun-dataset-root",
        default=None,
        metavar="DIR",
        help="阿里云样本库：每轮识别后写入 sample_*/background.png、slider.png、meta.json，"
        "供 scripts/benchmark_aliyun_gap.py --dataset-root 复盘",
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
        choices=("auto", "ddddocr", "opencv", "slidercracker"),
        default="auto",
        metavar="MODE",
        help="滑块缺口识别：auto=默认用 ddddocr.slide_match，失败再 OpenCV；"
        "ddddocr=仅 ddddocr（不可用或未识别成功则报错）；opencv=仅用 OpenCV（调试）；"
        "slidercracker=PyPI slidercracker（内部等同 simple_target=True；依赖 ddddocr/opencv 老版本，慎用）",
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
    if args.aliyun_drag_scale_mult is not None:
        os.environ["ICGOO_ALIYUN_DRAG_SCALE_MULT"] = str(args.aliyun_drag_scale_mult)
    if args.aliyun_systematic_extra_px is not None:
        os.environ["ICGOO_ALIYUN_SYSTEMATIC_DRAG_EXTRA_PX"] = str(
            args.aliyun_systematic_extra_px
        )
    if args.aliyun_slide_human_slow:
        os.environ["ICGOO_ALIYUN_SLIDE_HUMAN_SLOW"] = "1"
    if args.aliyun_slide_pre_extra_hold_sec is not None:
        os.environ["ICGOO_ALIYUN_SLIDE_PRE_RELEASE_EXTRA_HOLD_SEC"] = str(
            args.aliyun_slide_pre_extra_hold_sec
        )
    if args.aliyun_post_release_decoy_click:
        os.environ["ICGOO_ALIYUN_POST_RELEASE_DECOY_CLICK"] = "1"
    if args.aliyun_slide_y_jitter:
        os.environ["ICGOO_ALIYUN_SLIDE_Y_JITTER"] = "1"
    if args.no_aliyun_raw_x_segment:
        os.environ["ICGOO_ALIYUN_RAW_X_SEGMENT_ADJUST"] = "0"
    if solver.captcha_exists(timeout=3):
        print("检测到滑块验证码（易盾或阿里云），开始自动破解...")
        if args.slider_backend == "opencv":
            print("缺口定位：OpenCV（--slider-backend opencv）", flush=True)
        elif args.slider_backend == "ddddocr":
            print("缺口定位：仅 ddddocr（DdddOcr.slide_match）", flush=True)
        elif args.slider_backend == "slidercracker":
            print(
                "缺口定位：PyPI slidercracker（内部为 ddddocr slide_match simple_target=True；"
                "未安装则报错）",
                flush=True,
            )
        else:
            print("缺口定位：ddddocr.slide_match 优先，失败则 OpenCV", flush=True)
        captcha_run_dir = None
        if not args.no_captcha_debug and args.captcha_debug_dir:
            captcha_run_dir = _new_captcha_debug_run_dir(args.captcha_debug_dir)
            print(f"验证码调试图目录: {captcha_run_dir}", flush=True)
            print(
                "快照含每轮 round_NN_background.png / round_NN_slider.png / *_gap_marked.png；"
                "汇总调参可运行: "
                f'python "{os.path.join(_script_dir, "tune_icgoo_captcha_from_jsonl.py")}" '
                f'"{os.path.join(args.captcha_debug_dir, "run_*", "aliyun_calibration.jsonl")}" '
                f'--write-env-cmd "{os.path.join(_script_dir, "icgoo_tuned_env.cmd")}"',
                flush=True,
            )
            _aliyun_sys_env = (os.environ.get("ICGOO_ALIYUN_SYSTEMATIC_DRAG_EXTRA_PX") or "").strip()
            _aliyun_mult_env = (os.environ.get("ICGOO_ALIYUN_DRAG_SCALE_MULT") or "").strip()
            _aliyun_hum_env = (os.environ.get("ICGOO_ALIYUN_SLIDE_HUMAN_SLOW") or "").strip()
            _aliyun_pre_hold_env = (
                os.environ.get("ICGOO_ALIYUN_SLIDE_PRE_RELEASE_EXTRA_HOLD_SEC") or ""
            ).strip()
            _aliyun_decoy_env = (os.environ.get("ICGOO_ALIYUN_POST_RELEASE_DECOY_CLICK") or "").strip()
            _aliyun_decoy_r_env = (
                os.environ.get("ICGOO_ALIYUN_POST_RELEASE_DECOY_RADIUS_PX") or ""
            ).strip()
            _aliyun_yjit_env = (os.environ.get("ICGOO_ALIYUN_SLIDE_Y_JITTER") or "").strip()
            _aliyun_yjit_cap_env = (
                os.environ.get("ICGOO_ALIYUN_SLIDE_Y_JITTER_CAP_PX") or ""
            ).strip()
            _aliyun_yjit_sig_env = (
                os.environ.get("ICGOO_ALIYUN_SLIDE_Y_JITTER_SIGMA_PX") or ""
            ).strip()
            _aliyun_rxseg_env = (os.environ.get("ICGOO_ALIYUN_RAW_X_SEGMENTS") or "").strip()
            _aliyun_rxseg_off_env = (
                os.environ.get("ICGOO_ALIYUN_RAW_X_SEGMENT_ADJUST") or ""
            ).strip()
            _aliyun_fsi_settle_env = (
                os.environ.get("ICGOO_ALIYUN_FAIL_SAME_IMAGE_SETTLE_SEC") or ""
            ).strip()
            _aliyun_fsi_wait_env = (
                os.environ.get("ICGOO_ALIYUN_FAIL_SAME_IMAGE_WAIT_SLIDER_SEC") or ""
            ).strip()
            _aliyun_fsi_wig_env = (
                os.environ.get("ICGOO_ALIYUN_FAIL_SAME_IMAGE_MOUSE_WIGGLE") or ""
            ).strip()
            _aliyun_fsc_settle_env = (
                os.environ.get("ICGOO_ALIYUN_FAIL_SRC_CHURN_SETTLE_SEC") or ""
            ).strip()
            _aliyun_lc_off_env = (
                os.environ.get("ICGOO_ALIYUN_LOWCONF_SHADOW_OVERRIDE") or ""
            ).strip()
            _aliyun_lc_conf_env = (
                os.environ.get("ICGOO_ALIYUN_LOWCONF_SHADOW_CONF_MAX") or ""
            ).strip()
            _aliyun_lc_div_env = (
                os.environ.get("ICGOO_ALIYUN_LOWCONF_SHADOW_DIVERGE_MIN_PX") or ""
            ).strip()
            _debug_write_text(
                os.path.join(captcha_run_dir, "00_session.txt"),
                f"url\t{args.url}\n"
                f"slider_backend\t{args.slider_backend}\n"
                f"drag_boost\t{args.drag_boost}\n"
                f"drag_extra_px\t{args.drag_extra_px}\n"
                f"aliyun_systematic_extra_px_effective\t{get_aliyun_systematic_drag_extra_px()}\n"
                f"aliyun_drag_scale_mult_effective\t{get_aliyun_drag_scale_mult()}\n"
                f"aliyun_slide_pre_release_extra_hold_sec_effective\t{aliyun_slide_pre_release_extra_hold_sec()}\n"
                f"aliyun_post_release_decoy_click_effective\t{int(aliyun_post_release_decoy_click_enabled())}\n"
                f"aliyun_slide_y_jitter_effective\t{int(aliyun_slide_y_jitter_enabled())}\n"
                f"aliyun_slide_y_jitter_cap_px_effective\t{aliyun_slide_y_jitter_cap_px()}\n"
                f"aliyun_slide_y_jitter_sigma_px_effective\t{aliyun_slide_y_jitter_sigma_px()}\n"
                f"aliyun_fail_same_image_settle_sec_effective\t{aliyun_fail_same_image_retry_settle_sec()}\n"
                f"aliyun_fail_same_image_wait_slider_sec_effective\t{aliyun_fail_same_image_retry_wait_slider_sec()}\n"
                f"aliyun_fail_same_image_mouse_wiggle_effective\t{int(aliyun_fail_same_image_retry_mouse_wiggle_enabled())}\n"
                f"aliyun_fail_src_churn_settle_sec_effective\t{aliyun_fail_src_churn_retry_settle_sec()}\n"
                f"env_ICGOO_ALIYUN_SYSTEMATIC_DRAG_EXTRA_PX\t{_aliyun_sys_env or '(unset)'}\n"
                f"env_ICGOO_ALIYUN_DRAG_SCALE_MULT\t{_aliyun_mult_env or '(unset)'}\n"
                f"env_ICGOO_ALIYUN_SLIDE_HUMAN_SLOW\t{_aliyun_hum_env or '(unset)'}\n"
                f"env_ICGOO_ALIYUN_SLIDE_PRE_RELEASE_EXTRA_HOLD_SEC\t{_aliyun_pre_hold_env or '(unset)'}\n"
                f"env_ICGOO_ALIYUN_POST_RELEASE_DECOY_CLICK\t{_aliyun_decoy_env or '(unset)'}\n"
                f"env_ICGOO_ALIYUN_POST_RELEASE_DECOY_RADIUS_PX\t{_aliyun_decoy_r_env or '(unset)'}\n"
                f"env_ICGOO_ALIYUN_SLIDE_Y_JITTER\t{_aliyun_yjit_env or '(unset)'}\n"
                f"env_ICGOO_ALIYUN_SLIDE_Y_JITTER_CAP_PX\t{_aliyun_yjit_cap_env or '(unset)'}\n"
                f"env_ICGOO_ALIYUN_SLIDE_Y_JITTER_SIGMA_PX\t{_aliyun_yjit_sig_env or '(unset)'}\n"
                f"env_ICGOO_ALIYUN_RAW_X_SEGMENT_ADJUST\t{_aliyun_rxseg_off_env or '(unset)'}\n"
                f"env_ICGOO_ALIYUN_RAW_X_SEGMENTS\t{_aliyun_rxseg_env or '(unset)'}\n"
                f"env_ICGOO_ALIYUN_FAIL_SAME_IMAGE_SETTLE_SEC\t{_aliyun_fsi_settle_env or '(unset)'}\n"
                f"env_ICGOO_ALIYUN_FAIL_SAME_IMAGE_WAIT_SLIDER_SEC\t{_aliyun_fsi_wait_env or '(unset)'}\n"
                f"env_ICGOO_ALIYUN_FAIL_SAME_IMAGE_MOUSE_WIGGLE\t{_aliyun_fsi_wig_env or '(unset)'}\n"
                f"env_ICGOO_ALIYUN_FAIL_SRC_CHURN_SETTLE_SEC\t{_aliyun_fsc_settle_env or '(unset)'}\n"
                f"env_ICGOO_ALIYUN_LOWCONF_SHADOW_OVERRIDE\t{_aliyun_lc_off_env or '(unset)'}\n"
                f"env_ICGOO_ALIYUN_LOWCONF_SHADOW_CONF_MAX\t{_aliyun_lc_conf_env or '(unset)'}\n"
                f"env_ICGOO_ALIYUN_LOWCONF_SHADOW_DIVERGE_MIN_PX\t{_aliyun_lc_div_env or '(unset)'}\n"
                f"time\t{time.strftime('%Y-%m-%d %H:%M:%S')}\n",
            )
            with open(os.path.join(captcha_run_dir, "attempts.tsv"), "w", encoding="utf-8") as log:
                log.write("attempt\td_try\tfactor\ttracks_sum\tverified_ok\n")
            _debug_write_text(
                os.path.join(captcha_run_dir, "00_README.txt"),
                "00_page_initial.html  拉取拼图时的整页 HTML\n"
                "00_background.png / 00_slider.png  当前轮识别用图（多轮时会覆盖）\n"
                "00_background_gap_marked.png  识别后在背景上圈出落点矩形（round_NN_ 前缀同 detect）\n"
                "round_NN_background.png / round_NN_slider.png  第 NN 轮图副本，与 round_NN_detect.txt 或首轮回填 00_detect 对齐\n"
                "00_detect.txt         第 1 轮缺口与拖动换算参数\n"
                "NN_viewport_release_rR_d*.png  松手后短延迟再截（默认~0.22s，早于校验长等待与换图；R=captcha_round）\n"
                "  可调 ICGOO_CAPTCHA_RELEASE_SCREENSHOT_DELAY_SEC（0~2.5）；过短 handle 易未就绪，过长可能已换新品图\n"
                "NN_viewport_release_*_geometry.json  与上一行同 stem：含 release_screenshot_delay_sec、DOM、残差估算\n"
                "NN_viewport_d*.png    第 N 次滑动在校验与等待之后的视口截图（可与 release 对照是否换图）\n"
                "NN_yidun_widget_*.png 第 N 次后 .yidun 区域截图（若可取）\n"
                "NN_page_after.html    第 N 次后整页 HTML\n"
                "NN_slide_meta.txt     第 N 次轨迹与是否判定成功\n"
                "attempts.tsv          各次汇总\n"
                "aliyun_calibration.jsonl  标定/复盘：每轮 gap_detected + slide_attempt（可含 release_* 松手残差 CSS px）\n"
                "fail_new_image_review.tsv  服务端未过且换题时追加一行，对照 gap 标注图人工判断识别/拖动\n",
            )
            solver.calibration_jsonl_path = os.path.join(
                captcha_run_dir,
                ALIYUN_CALIBRATION_JSONL_BASENAME,
            )
        else:
            solver.calibration_jsonl_path = None
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
                if captcha_run_dir:
                    shutil.copy2(
                        bg_path,
                        os.path.join(
                            captcha_run_dir,
                            f"round_{captcha_round:02d}_background.png",
                        ),
                    )
                    shutil.copy2(
                        slider_path,
                        os.path.join(
                            captcha_run_dir,
                            f"round_{captcha_round:02d}_slider.png",
                        ),
                    )
                was_aliyun = solver._aliyun_challenge_active(timeout=0.6)
                raw_x, distance = solver.detect_gap(
                    bg_path,
                    slider_path,
                    drag_scale_x,
                    drag_boost=args.drag_boost,
                    gap_offset_image_px=args.gap_offset_image_px,
                    drag_extra_px=args.drag_extra_px,
                )
                drag_extra_eff = getattr(
                    solver, "_effective_drag_extra_px", args.drag_extra_px
                )
                rd = getattr(solver, "_gap_raw_detected", None)
                if args.gap_offset_image_px != 0 and rd is not None:
                    print(
                        f"识别图内 x≈{rd}px，叠加 --gap-offset-image-px={args.gap_offset_image_px} → "
                        f"用于换算的 x≈{raw_x}px",
                        flush=True,
                    )
                aa = getattr(solver, "_aliyun_drag_extra_applied", 0)
                extra_note = f"{args.drag_extra_px}px"
                if aa:
                    extra_note = (
                        f"{args.drag_extra_px}px+阿里云默认{aa}px（合计拖动加量 {drag_extra_eff}px）"
                    )
                print(
                    f"检测到缺口: 图内 x≈{raw_x}px（主算法选用），换算比例 {drag_scale_x:.4f}，"
                    f"补偿系数 {args.drag_boost:.3f}，拖动加量 {extra_note}，基准拖动≈{distance}px",
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
                elif solver._last_gap_backend == "slidercracker":
                    _rx = solver._dddd_raw_left_simple_true
                    print(
                        "缺口识别: slidercracker → identify_w="
                        f"{_rx if _rx is not None else '（无）'}（库内 fixed simple_target=True）",
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
                if solver._last_gap_backend in ("ddddocr", "ddddocr_shim", "slidercracker"):
                    _rxf = solver._dddd_raw_left_simple_false
                    _rxt = solver._dddd_raw_left_simple_true
                    if _rxf is not None and _rxt is not None:
                        print(
                            f"slide_match 两路图内左缘 x：边缘(Canny)={_rxf}，灰度模板={_rxt}（|差|={abs(_rxf - _rxt)}px）",
                            flush=True,
                        )
                distance_alt = _compute_slide_distance_alt(
                    solver,
                    raw_x,
                    distance,
                    drag_scale_x,
                    args.drag_boost,
                    drag_extra_eff,
                )
                xg = solver._opencv_raw_x_gray
                xc = solver._opencv_raw_x_canny
                sg = solver._opencv_score_gray
                scn = solver._opencv_score_canny
                if distance_alt is not None and distance_alt != distance:
                    if (
                        xg is not None
                        and xc is not None
                        and sg is not None
                        and scn is not None
                        and abs(xg - xc) >= 6
                        and abs(xg - xc) <= 72
                    ):
                        print(
                            f"OpenCV 灰度 x={xg}（{sg:.3f}） Canny x={xc}（{scn:.3f}）相差 {abs(xg - xc)}px → "
                            f"第1/3/5…次用主拖动≈{distance}px，第2/4/6…次用辅拖动≈{distance_alt}px",
                            flush=True,
                        )
                    else:
                        xf = solver._dddd_raw_left_simple_false
                        xt = solver._dddd_raw_left_simple_true
                        ddx = abs(xf - xt) if xf is not None and xt is not None else 0
                        if (
                            solver._last_gap_backend in ("ddddocr", "ddddocr_shim", "slidercracker")
                            and xf is not None
                            and xt is not None
                            and 4 <= ddx <= _SLIDE_MATCH_ALT_MAX_RAW_DELTA_PX
                        ):
                            print(
                                f"slide_match 边缘 x={xf}px，灰度模板 x={xt}px（相差 {ddx}）→ "
                                f"第1/3/5…次主≈{distance}px，第2/4/6…次辅≈{distance_alt}px",
                                flush=True,
                            )
                if distance_alt is None and not getattr(solver, "_slide_match_blended", False):
                    xf = solver._dddd_raw_left_simple_false
                    xt = solver._dddd_raw_left_simple_true
                    ddx = abs(xf - xt) if xf is not None and xt is not None else 0
                    if (
                        solver._last_gap_backend in ("ddddocr", "ddddocr_shim", "slidercracker")
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
                gap_overlay_png = ""
                _need_gap_vis = bool(captcha_run_dir) or bool(args.show_gap_overlay)
                if _need_gap_vis:
                    if captcha_run_dir:
                        _gname = (
                            "00_background_gap_marked.png"
                            if captcha_round <= 1
                            else f"round_{captcha_round:02d}_background_gap_marked.png"
                        )
                        _gpath = os.path.join(captcha_run_dir, _gname)
                    else:
                        _gname = ""
                        _gpath = os.path.join(
                            tempfile.gettempdir(),
                            f"icgoo_gap_overlay_r{captcha_round}_{int(time.time())}.png",
                        )
                    if write_gap_overlay_png(
                        bg_path,
                        slider_path,
                        raw_x,
                        _gpath,
                        raw_detected=getattr(solver, "_gap_raw_detected", None),
                        gap_offset_image_px=args.gap_offset_image_px,
                        show_imshow=bool(args.show_gap_overlay),
                    ):
                        if captcha_run_dir:
                            gap_overlay_png = _gname
                        print(
                            f"已写出缺口标注图（矩形为识别落点，红字为 raw_x / detected+offset）: {_gpath}",
                            flush=True,
                        )
                        if args.show_gap_overlay:
                            print("（已关闭 gap 预览窗口，继续滑动流程）", flush=True)
                det_extra = f"captcha_round\t{captcha_round}\n"
                det_extra += f"gap_backend\t{solver._last_gap_backend or 'unknown'}\n"
                if solver._last_gap_backend in ("ddddocr", "ddddocr_shim", "slidercracker"):
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
                xam = getattr(solver, "_opencv_raw_x_alpha_mask", None)
                sam = getattr(solver, "_opencv_score_alpha_mask", None)
                if xam is not None and sam is not None:
                    det_extra += f"opencv_x_alpha_mask\t{xam}\nopencv_score_alpha_mask\t{sam}\n"
                xps = getattr(solver, "_opencv_raw_x_puzzle_psc", None)
                sps = getattr(solver, "_opencv_score_puzzle_psc", None)
                if xps is not None and sps is not None:
                    det_extra += f"opencv_x_puzzle_psc\t{xps}\nopencv_score_puzzle_psc\t{sps}\n"
                xsh = getattr(solver, "_opencv_raw_x_shadow_ms", None)
                ssh = getattr(solver, "_opencv_score_shadow_ms", None)
                if xsh is not None and ssh is not None:
                    det_extra += f"opencv_x_shadow_ms\t{xsh}\nopencv_score_shadow_ms\t{ssh}\n"
                xshe = getattr(solver, "_opencv_raw_x_shadow_ms_edges", None)
                sshe = getattr(solver, "_opencv_score_shadow_ms_edges", None)
                if xshe is not None and sshe is not None:
                    det_extra += (
                        f"opencv_x_shadow_ms_edges\t{xshe}\n"
                        f"opencv_score_shadow_ms_edges\t{sshe}\n"
                    )
                if distance_alt is not None:
                    det_extra += f"base_drag_alt_px\t{distance_alt}\n"
                if gap_overlay_png:
                    det_extra += f"gap_overlay_png\t{gap_overlay_png}\n"
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
                        f"drag_extra_effective_px\t{drag_extra_eff}\n"
                        f"aliyun_drag_extra_applied\t{aa}\n"
                        f"base_drag_px\t{distance}\n"
                        f"slide_attempts\t{args.slide_attempts}\n"
                        f"slide_factors\t{','.join(str(x) for x in _SLIDE_DISTANCE_FACTORS)}\n"
                        f"{det_extra}"
                        f"background\t00_background.png\n"
                        f"slider\t00_slider.png\n",
                    )
                solver._emit_calibration(
                    build_gap_calibration_record(
                        solver,
                        captcha_round=captcha_round,
                        drag_scale_x=drag_scale_x,
                        raw_x=raw_x,
                        base_drag_px=distance,
                        distance_alt_px=distance_alt,
                        page_url=args.url,
                        drag_boost=args.drag_boost,
                        gap_offset_image_px=args.gap_offset_image_px,
                        drag_extra_px_user=args.drag_extra_px,
                        bg_path=bg_path,
                        slider_path=slider_path,
                    )
                )
                solver._captcha_debug_residual_raw_x = int(raw_x)
                slide_ok, captcha_replaced = solver.slide_verification(
                    distance,
                    max_attempts=args.slide_attempts,
                    debug_run_dir=captcha_run_dir,
                    distance_alt=distance_alt,
                    captcha_round=captcha_round,
                )
                if args.aliyun_dataset_root and was_aliyun:
                    from icgoo_aliyun_gap import export_aliyun_dataset_sample

                    exp = export_aliyun_dataset_sample(
                        os.path.abspath(args.aliyun_dataset_root),
                        bg_src_path=bg_path,
                        slider_src_path=slider_path,
                        meta={
                            "is_aliyun": True,
                            "captcha_round": captcha_round,
                            "raw_x": raw_x,
                            "raw_x_detected": getattr(solver, "_gap_raw_detected", None),
                            "base_drag_px": distance,
                            "distance_alt_px": distance_alt,
                            "slide_ok": slide_ok,
                            "captcha_replaced": captcha_replaced,
                            "drag_scale_x": drag_scale_x,
                            "drag_extra_effective_px": drag_extra_eff,
                            "gap_backend": getattr(solver, "_last_gap_backend", ""),
                            "gap_ensemble_pick": getattr(solver, "_gap_ensemble_pick", None),
                            "url": args.url,
                            "time": time.strftime("%Y-%m-%d %H:%M:%S"),
                        },
                    )
                    if exp:
                        print(f"阿里云样本已写入: {exp}", flush=True)
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
                    print("服务端未返回成功。", flush=True)
                    print("已换图或整页刷新，将用新拼图重新识别缺口并再试滑动。", flush=True)
                    if captcha_run_dir:
                        _append_fail_new_image_review_row(
                            captcha_run_dir,
                            captcha_round=captcha_round,
                            gap_overlay_png=gap_overlay_png,
                            raw_x_px=raw_x,
                            base_drag_px=distance,
                            page_url=args.url,
                        )
                        print(
                            "已记入 fail_new_image_review.tsv（请对照本轮 *_background_gap_marked.png "
                            "判断红框是否对准真缺口；对准则优先查拖动/轨宽/轨迹）。",
                            flush=True,
                        )
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
                if captcha_run_dir:
                    _cal = os.path.join(captcha_run_dir, ALIYUN_CALIBRATION_JSONL_BASENAME)
                    if os.path.isfile(_cal):
                        _tuner = os.path.join(_script_dir, "tune_icgoo_captcha_from_jsonl.py")
                        print(
                            f"提示：可根据本目录标定 JSONL 生成调参建议："
                            f'python "{_tuner}" "{os.path.abspath(_cal)}"',
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
        if not wait_for_results(solver.page, timeout=10.0, quiet=False):
            print("等待表格超时，仍尝试解析当前 DOM…", flush=True)
            time.sleep(2)
        else:
            time.sleep(1)
    rows = parse_search_results(solver.page, query_model, quiet=False)
    print(json.dumps(rows, ensure_ascii=False, indent=2), flush=True)
    print(f"共解析 {len(rows)} 条（query_model={query_model!r}）", flush=True)
    print("脚本执行完毕，浏览器保持打开。如需关闭，请手动关闭。")


if __name__ == "__main__":
    main()