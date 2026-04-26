"""
ICGOO 站点上的 **阿里云 aliyunCaptcha** 拼图滑块：iframe 文档上下文、可见性探测、拉图与轨道比例。

与 ``icgoo_yidun_solver``（网易易盾 + 共用缺口识别引擎）分文件维护；缺口匹配（ddddocr / OpenCV）
仍在 ``IcgooYidunSliderSolver`` 内完成。

本模块不配置独立日志文件；需要时由调用方传入 ``log_warning`` / ``log_info``。

拖动链路标定与按行复盘见 ``icgoo_aliyun_gap.append_aliyun_calibration_jsonl`` 及
``IcgooYidunSliderSolver.calibration_jsonl_path`` / 环境变量 ``ICGOO_CAPTCHA_CALIBRATION_JSONL``。

**仅阿里云** 微调（易盾逻辑不变）：``ICGOO_ALIYUN_SYSTEMATIC_DRAG_EXTRA_PX``、
``ICGOO_ALIYUN_DRAG_SCALE_MULT``；或 ``icgoo_crawler_dev.py`` 的 ``--aliyun-systematic-extra-px`` /
``--aliyun-drag-scale-mult``。

**按图内缺口 x 分段再调拖动**（仅阿里云）：在基准 ``drag_x`` 算出后乘 ``mult`` 并加 ``extra`` CSS 像素。
默认三段（标定 run_223618：高 ``raw_x`` 易顶轨/多拖，低 x 不宜再压）：
``[0,180)×1.0+0``、``[180,220)×0.97+0``、``[220,∞)×0.925+0``。
关闭：``ICGOO_ALIYUN_RAW_X_SEGMENT_ADJUST=0``。自定义：``ICGOO_ALIYUN_RAW_X_SEGMENTS``，格式
``lo-hi:mult:extra`` 多条用 ``;`` 分隔，左闭右开，例如 ``0-180:1:0;180-220:0.97:0;220-9999:0.925:0``。

落点已准仍被拒（风控/轨迹）时，可试 ``ICGOO_ALIYUN_SLIDE_HUMAN_SLOW=1``（或 dev 的 ``--aliyun-slide-human-slow``）：
放慢单步拖动与松手前停顿，并略延长松手后再轮询成功态。

进一步模拟「到位停一下再松手、松手后点在旁边」：``ICGOO_ALIYUN_SLIDE_PRE_RELEASE_EXTRA_HOLD_SEC``（秒，叠加在随机 pre_release 之后）、
``ICGOO_ALIYUN_POST_RELEASE_DECOY_CLICK=1``（松开后相对当前指针极坐标随机偏移再左键点击；可选
``ICGOO_ALIYUN_POST_RELEASE_DECOY_RADIUS_PX=min,max`` 控制偏移半径 CSS 像素，默认 ``8,40``）。易盾忽略。

**验证失败但拼图未换图**（``fail_same_image``，或 URL 变但图像指纹仍相同）时，下一轮试拖前可加长等待与轻交互，减轻手柄短暂 ``rect`` 为 0、下一轮误判「无手柄」：
``ICGOO_ALIYUN_FAIL_SAME_IMAGE_SETTLE_SEC``（额外静止秒数，默认 ``1.35``，``<=0`` 关闭）、
``ICGOO_ALIYUN_FAIL_SAME_IMAGE_WAIT_SLIDER_SEC``（调用与易盾共用的手柄就绪轮询上限，默认 ``8``，``0`` 跳过）、
``ICGOO_ALIYUN_FAIL_SAME_IMAGE_MOUSE_WIGGLE=0`` 可关闭「仅平移、不点击」的微小鼠标扰动（默认开启）。
URL 变、指纹仍同图继续试系数前短停：``ICGOO_ALIYUN_FAIL_SRC_CHURN_SETTLE_SEC``（默认 ``0.55``）。

**低置信 dddd 与 shadow 大分歧**：ensemble 决选为 ``dddd_true`` / ``dddd_false`` 且分数低于阈值、同时
``aliyun_shadow_ms``（或 edges）与决选 x 相差足够大时，改采 shadow 路（标定里 ``gap_ensemble_pick`` 为
``aliyun_lowconf_shadow_ms*``）。开关与阈值：
``ICGOO_ALIYUN_LOWCONF_SHADOW_OVERRIDE=0`` 关闭；
``ICGOO_ALIYUN_LOWCONF_SHADOW_CONF_MAX``（默认 ``0.45``，仅当决选分数 **低于** 此值才考虑覆盖）；
``ICGOO_ALIYUN_LOWCONF_SHADOW_DIVERGE_MIN_PX``（默认 ``32``，shadow 与决选 x 至少相差此像素）。

拖动段纵向微动（轨迹 Y 维）：``ICGOO_ALIYUN_SLIDE_Y_JITTER=1`` 时在 **仅阿里云** 路径对每步 ``ac.move`` 叠加有界随机游走
``dy``（默认 cap ``4`` CSS px、步长 sigma ``0.65``）；可选 ``ICGOO_ALIYUN_SLIDE_Y_JITTER_CAP_PX``、
``ICGOO_ALIYUN_SLIDE_Y_JITTER_SIGMA_PX``。建议与 ``ICGOO_ALIYUN_SLIDE_HUMAN_SLOW=1`` 同开以拉长总时长。易盾忽略。
"""
from __future__ import annotations

import base64
import binascii
import hashlib
import math
import os
import random
import re
import time
import urllib.parse
import urllib.error
import urllib.request
from collections import deque
from collections.abc import Callable
from io import BytesIO

from DrissionPage import ChromiumPage
from PIL import Image

_XP_ALIYUN_WINDOW = "css:#aliyunCaptcha-window-embed"
_XP_ALIYUN_BG_IMG = "css:#aliyunCaptcha-img"
_XP_ALIYUN_PUZZLE_IMG = "css:#aliyunCaptcha-puzzle"
_XP_ALIYUN_SLIDER_HANDLE = "css:#aliyunCaptcha-sliding-slider"
_XP_ALIYUN_SLIDING_BODY = "css:#aliyunCaptcha-sliding-body"
_XP_ALIYUN_SLIDING_TEXT_BOX = "css:#aliyunCaptcha-sliding-text-box"
_XP_ALIYUN_SLIDING_TEXT = "css:#aliyunCaptcha-sliding-text"
_XP_ALIYUN_REFRESH = "css:#aliyunCaptcha-btn-refresh"

# 与易盾共用同一套 slide_match + 轨宽换算时叠加的阿里云常量加量（像素）。
# 标定：run_220202 偏少拖；run_223618 在 22px + mult1.03 下多见正残差/顶轨 885。取 **19** 与 mult **1.02** 作默认折中。
ALIYUN_SYSTEMATIC_DRAG_EXTRA_PX = 19
# 手柄在个别时刻 ``rect`` 宽高为 0 时，勿把可行程算成整条轨道宽（会抬高 ``drag_scale_x``、加剧多拖）。
_ALIYUN_SLIDER_HANDLE_FALLBACK_CSS_W = 40
_ALIYUN_SLIDER_HANDLE_MIN_W = 4
# 缺口/拼块在验证码图像中的特征尺寸：阿里云下发 PNG 实测约 **52×52**（与槽位一致；标定 JSONL 的
# slider_nat_w / slider_nat_h）。找缺口时模板即此矩形区域；勿与易盾拼块混用。
ALIYUN_CAPTCHA_PUZZLE_SIZE_NAT_PX: tuple[int, int] = (52, 52)
ALIYUN_CAPTCHA_PUZZLE_WIDTH_NAT_PX, ALIYUN_CAPTCHA_PUZZLE_HEIGHT_NAT_PX = ALIYUN_CAPTCHA_PUZZLE_SIZE_NAT_PX


def get_aliyun_systematic_drag_extra_px() -> int:
    """
    仅 **阿里云** 拖动加量（像素）；默认 ``ALIYUN_SYSTEMATIC_DRAG_EXTRA_PX``。
    环境变量 ``ICGOO_ALIYUN_SYSTEMATIC_DRAG_EXTRA_PX`` 可覆盖（易盾不受影响）。
    """
    v = (os.environ.get("ICGOO_ALIYUN_SYSTEMATIC_DRAG_EXTRA_PX") or "").strip()
    if v:
        try:
            return int(v)
        except ValueError:
            pass
    return ALIYUN_SYSTEMATIC_DRAG_EXTRA_PX


# 默认 [lo, hi) → (scale_mult, extra_css_px)；hi 用大数表示「以上」
_ALIYUN_RAW_X_SEGMENTS_DEFAULT: tuple[tuple[int, int, float, int], ...] = (
    (0, 180, 1.0, 0),
    (180, 220, 0.97, 0),
    (220, 1_000_000, 0.925, 0),
)


def _aliyun_raw_x_segment_adjust_disabled() -> bool:
    v = (os.environ.get("ICGOO_ALIYUN_RAW_X_SEGMENT_ADJUST") or "").strip().lower()
    return v in ("0", "false", "no", "off")


def _parse_aliyun_raw_x_segments_env(raw: str) -> list[tuple[int, int, float, int]] | None:
    """
    ``lo-hi:mult:extra``，多条 ``;`` 分隔；区间为左闭右开。
    """
    out: list[tuple[int, int, float, int]] = []
    for part in raw.split(";"):
        part = part.strip()
        if not part:
            continue
        try:
            range_part, mult_s, ex_s = part.split(":")
            lo_s, hi_s = range_part.split("-", 1)
            lo, hi = int(lo_s.strip()), int(hi_s.strip())
            mult = float(mult_s.strip())
            ex = int(ex_s.strip())
        except ValueError:
            return None
        if lo >= hi or not (0.5 <= mult <= 1.5) or not (-80 <= ex <= 80):
            return None
        out.append((lo, hi, mult, ex))
    return out or None


def _aliyun_raw_x_segments_table() -> tuple[tuple[int, int, float, int], ...]:
    raw = (os.environ.get("ICGOO_ALIYUN_RAW_X_SEGMENTS") or "").strip()
    if raw:
        parsed = _parse_aliyun_raw_x_segments_env(raw)
        if parsed is not None:
            return tuple(sorted(parsed, key=lambda t: t[0]))
    return _ALIYUN_RAW_X_SEGMENTS_DEFAULT


def aliyun_raw_x_segment_drag_adjust(raw_x_image_px: int) -> tuple[float, int, str]:
    """
    在阿里云路径、已含 systematic extra 与 drag_scale_mult 的 **页面拖动像素基准** 上再调整。

    返回 ``(scale_mult, extra_css_px, label)``：
    ``final_px = max(1, int(round(base_px * scale_mult)) + extra_css_px)``。

    ``raw_x_image_px`` 为用于换算的图内 x（与 ``detect_gap`` 返回的第一项一致，已含 gap_offset）。
    """
    if _aliyun_raw_x_segment_adjust_disabled():
        return 1.0, 0, "off"
    x = max(0, int(raw_x_image_px))
    for lo, hi, mult, ex in _aliyun_raw_x_segments_table():
        if lo <= x < hi:
            return float(mult), int(ex), f"x[{lo},{hi})"
    return 1.0, 0, "fallback"


def get_aliyun_drag_scale_mult() -> float:
    """
    仅 **阿里云**：对 ``get_images`` 返回的 ``drag_scale_x`` 再乘此系数（轨宽/图宽比仍不准时微调）。
    环境变量 ``ICGOO_ALIYUN_DRAG_SCALE_MULT``；未设置时默认 ``1.02``；
    合法范围 ``[0.5, 2.0]``（易盾不乘）。
    """
    v = (os.environ.get("ICGOO_ALIYUN_DRAG_SCALE_MULT") or "").strip()
    if not v:
        return 1.02
    try:
        m = float(v)
        if 0.5 <= m <= 2.0:
            return m
    except ValueError:
        pass
    return 1.0


def aliyun_slide_human_slow_enabled() -> bool:
    """
    阿里云滑块：放慢 ``ac.move`` 单步时长、步间 sleep、松手前静候，并略延长松手后首次校验等待，
    减轻「轨迹过快、过机械」导致的几何已准仍失败。易盾路径不读取此开关。
    """
    v = (os.environ.get("ICGOO_ALIYUN_SLIDE_HUMAN_SLOW") or "").strip().lower()
    return v in ("1", "true", "yes", "on")


def aliyun_slide_pre_release_extra_hold_sec() -> float:
    """
    仅阿里云：轨迹结束后、``release()`` 之前，在已有随机 pre_release sleep **之后**再额外静止的秒数。
    环境变量 ``ICGOO_ALIYUN_SLIDE_PRE_RELEASE_EXTRA_HOLD_SEC``；未设置或非正数则不加。
    """
    v = (os.environ.get("ICGOO_ALIYUN_SLIDE_PRE_RELEASE_EXTRA_HOLD_SEC") or "").strip()
    if not v:
        return 0.0
    try:
        x = float(v)
        return x if x > 0 else 0.0
    except ValueError:
        return 0.0


def aliyun_post_release_decoy_click_enabled() -> bool:
    """仅阿里云：松开后是否在指针附近做一次偏移左键点击。``ICGOO_ALIYUN_POST_RELEASE_DECOY_CLICK``。"""
    v = (os.environ.get("ICGOO_ALIYUN_POST_RELEASE_DECOY_CLICK") or "").strip().lower()
    return v in ("1", "true", "yes", "on")


def aliyun_post_release_decoy_radius_px() -> tuple[float, float]:
    """
    偏移点击相对松手点的极坐标半径范围 ``(min, max)``，视口 CSS 像素。
    ``ICGOO_ALIYUN_POST_RELEASE_DECOY_RADIUS_PX`` 格式 ``min,max`` 或 ``min;max``；非法则 ``(8, 40)``。
    """
    raw = (os.environ.get("ICGOO_ALIYUN_POST_RELEASE_DECOY_RADIUS_PX") or "").strip()
    if raw:
        parts = [p.strip() for p in raw.replace(";", ",").split(",") if p.strip()]
        if len(parts) >= 2:
            try:
                lo, hi = float(parts[0]), float(parts[1])
                if 0.0 <= lo < hi <= 200.0:
                    return (lo, hi)
            except ValueError:
                pass
    return (8.0, 40.0)


def aliyun_post_release_decoy_click(page: ChromiumPage) -> None:
    """
    假定指针仍在刚 ``release()`` 的视口坐标：先 ``move`` 到周边一点（非 (0,0)），再 ``click()``。
    仅应在阿里云滑块流程、且已开启 ``aliyun_post_release_decoy_click_enabled()`` 时调用。
    """
    r_lo, r_hi = aliyun_post_release_decoy_radius_px()
    r = random.uniform(r_lo, r_hi)
    th = random.uniform(0.0, 2.0 * math.pi)
    dx = int(round(r * math.cos(th)))
    dy = int(round(r * math.sin(th)))
    if dx == 0 and dy == 0:
        dx = max(8, int(round(r_lo)))
    ac = page.actions
    ac.move(float(dx), float(dy), duration=random.uniform(0.04, 0.14))
    time.sleep(random.uniform(0.02, 0.08))
    ac.click()


def aliyun_fail_same_image_retry_settle_sec() -> float:
    """
    阿里云：校验未通过且拼图 **src 字符串未变**（同图换系数重试）前，额外静止秒数。
    ``ICGOO_ALIYUN_FAIL_SAME_IMAGE_SETTLE_SEC``；未设置则默认 ``1.35``；``<=0`` 表示不加。
    """
    v = (os.environ.get("ICGOO_ALIYUN_FAIL_SAME_IMAGE_SETTLE_SEC") or "").strip()
    if v:
        try:
            x = float(v)
            return x if x > 0 else 0.0
        except ValueError:
            return 0.0
    return 1.35


def aliyun_fail_same_image_retry_wait_slider_sec() -> float:
    """
    同图重试前：对滑块手柄做就绪轮询的最长等待（秒），与 ``_wait_yidun_slider_ready`` 一致。
    ``ICGOO_ALIYUN_FAIL_SAME_IMAGE_WAIT_SLIDER_SEC``；默认 ``8.0``；``0`` 跳过。
    """
    v = (os.environ.get("ICGOO_ALIYUN_FAIL_SAME_IMAGE_WAIT_SLIDER_SEC") or "").strip()
    if v:
        try:
            x = float(v)
            return x if x > 0 else 0.0
        except ValueError:
            return 0.0
    return 8.0


def aliyun_fail_same_image_retry_mouse_wiggle_enabled() -> bool:
    """
    同图重试前是否在视口内做一次微小鼠标平移（不点击）。
    ``ICGOO_ALIYUN_FAIL_SAME_IMAGE_MOUSE_WIGGLE``；未设置或非 ``0/false`` 则 **默认开启**。
    """
    v = (os.environ.get("ICGOO_ALIYUN_FAIL_SAME_IMAGE_MOUSE_WIGGLE") or "").strip().lower()
    if not v:
        return True
    return v not in ("0", "false", "no", "off")


def aliyun_fail_src_churn_retry_settle_sec() -> float:
    """
    阿里云：滑动后拼图 **URL 键已变** 但 **图像指纹仍与滑前相同** 时，继续同图试系数前的短停。
    ``ICGOO_ALIYUN_FAIL_SRC_CHURN_SETTLE_SEC``；默认 ``0.55``；``<=0`` 关闭。
    """
    v = (os.environ.get("ICGOO_ALIYUN_FAIL_SRC_CHURN_SETTLE_SEC") or "").strip()
    if v:
        try:
            x = float(v)
            return x if x > 0 else 0.0
        except ValueError:
            return 0.0
    return 0.55


def aliyun_slide_y_jitter_enabled() -> bool:
    """仅阿里云拖动段：是否对每步水平 move 叠加纵向微抖动。``ICGOO_ALIYUN_SLIDE_Y_JITTER``。"""
    v = (os.environ.get("ICGOO_ALIYUN_SLIDE_Y_JITTER") or "").strip().lower()
    return v in ("1", "true", "yes", "on")


def aliyun_slide_y_jitter_cap_px() -> float:
    """
    纵向随机游走目标相对零位的软上限（CSS 像素）；实际每步 ``dy`` 为整数差分，幅度受 sigma 约束。
    ``ICGOO_ALIYUN_SLIDE_Y_JITTER_CAP_PX``；合法约 ``[0.5, 12]``，默认 ``4``。
    """
    v = (os.environ.get("ICGOO_ALIYUN_SLIDE_Y_JITTER_CAP_PX") or "").strip()
    if v:
        try:
            c = float(v)
            if 0.5 <= c <= 12.0:
                return c
        except ValueError:
            pass
    return 4.0


def aliyun_slide_y_jitter_sigma_px() -> float:
    """
    每发生一次水平拖动步时，对内部纵向目标加的均匀扰动幅度（单边尺度，非标准差名仅为习惯）。
    ``ICGOO_ALIYUN_SLIDE_Y_JITTER_SIGMA_PX``；合法约 ``[0.1, 3]``，默认 ``0.65``。
    """
    v = (os.environ.get("ICGOO_ALIYUN_SLIDE_Y_JITTER_SIGMA_PX") or "").strip()
    if v:
        try:
            s = float(v)
            if 0.1 <= s <= 3.0:
                return s
        except ValueError:
            pass
    return 0.65


def aliyun_slide_y_jitter_new_state() -> dict[str, float | int]:
    """供 ``aliyun_slide_y_jitter_step`` 复用的可变状态（每轮滑动新建一次）。"""
    return {"target": 0.0, "last_i": 0}


def aliyun_slide_y_jitter_step(state: dict[str, float | int], *, human_slow: bool) -> int:
    """
    计算本步应叠加的相对纵向像素 ``dy``（整数），并更新 ``state``。
    未开启 ``aliyun_slide_y_jitter_enabled()`` 时返回 ``0`` 且不修改状态。
    """
    if not aliyun_slide_y_jitter_enabled():
        return 0
    cap = aliyun_slide_y_jitter_cap_px()
    sig = aliyun_slide_y_jitter_sigma_px()
    if human_slow:
        sig *= 1.18
        cap = min(12.0, cap * 1.08)
    tgt = float(state.get("target", 0.0))
    tgt += random.uniform(-sig, sig)
    tgt = max(-cap, min(cap, tgt))
    state["target"] = tgt
    y_int = int(round(tgt))
    last_i = int(state.get("last_i", 0))
    dy = y_int - last_i
    state["last_i"] = y_int
    return dy


def aliyun_puzzle_src_compare_key(src: str | None) -> str:
    """
    滑动前后比对「是否换题」时用：阿里云 CDN 地址可能仅 query（如防缓存）变化，路径未变应视为同一题。

    ``data:`` 类地址按 **解码后图像字节** 的 SHA256 归一化，避免 SPA 把同一 PNG 重写成另一串
    base64 时误判为换题。非阿里云 http(s)、其它 scheme 仍用原串（易盾等）。
    """
    s = (src or "").strip()
    if not s:
        return ""
    low = s.lower()
    if "base64," in low or (low.startswith("data:") and "," in s):
        try:
            pl = _data_url_base64_payload(s)
            if pl:
                raw = _decode_base64_image_payload(pl)
                if raw:
                    return "data-img:" + hashlib.sha256(raw).hexdigest()
        except Exception:
            pass
        return s
    if not (low.startswith("http://") or low.startswith("https://")):
        return s
    try:
        p = urllib.parse.urlparse(s)
        host = (p.netloc or "").lower()
        if "aliyuncs.com" in host:
            return urllib.parse.urlunparse(
                (p.scheme.lower(), host, p.path, "", "", "")
            )
    except Exception:
        pass
    return s


def aliyun_puzzle_image_fingerprint(src: str | None, referer: str) -> str:
    """
    拼图 ``src`` 对应图像内容的 SHA256（十六进制），用于 URL 字符串已变但像素未变时的二次确认。

    失败（空 src、下载/解码失败）时返回 ``""``。
    """
    s = (src or "").strip()
    if not s:
        return ""
    try:
        b64 = _captcha_img_src_to_b64_payload(s, (referer or "").strip() or "https://www.icgoo.net/")
        raw = _decode_base64_image_payload(b64)
        if raw:
            return hashlib.sha256(raw).hexdigest()
    except Exception:
        pass
    return ""


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
    解码验证码图片 data URL 中的 base64：去空白、支持 url-safe、补 padding。
    优先尝试标准解码，若失败则尝试去除末尾一个字符再解码（应对站点偶发的异常长度）。
    """
    s = re.sub(r"\s+", "", (b64 or "").strip())
    if not s:
        raise ValueError("base64 为空")
    s = s.replace("-", "+").replace("_", "/")
    rem = len(s) % 4
    if rem:
        s += "=" * (4 - rem)
    try:
        return base64.b64decode(s, validate=True)
    except (binascii.Error, ValueError):
        if len(s) > 1:
            s_stripped = s[:-1]
            rem2 = len(s_stripped) % 4
            if rem2:
                s_stripped += "=" * (4 - rem2)
            try:
                return base64.b64decode(s_stripped, validate=False)
            except Exception:
                pass
        return base64.b64decode(s, validate=False)


def _captcha_img_src_to_b64_payload(src: str | None, referer: str) -> str:
    """
    将背景/拼图 img 的 src 转为可解码的 base64 字符串。
    站点可能使用 data: URL，也可能使用 https 直链。
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


def _read_captcha_img_src(ele) -> str:
    """
    拼图 img 的地址：DOM 可能先出现节点再异步写 src，或放在 data-src；
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


def _aliyun_host_context(page: ChromiumPage, *, depth_max: int = 8):
    """
    BFS 查找包含阿里云验证码节点的文档上下文（拼图 / 弹层 / 轨道 / 「拖动滑块完成拼图」文案区）。
    文案条往往先于拼图 ``src`` 出现，需一并作为锚点。主页面或 **跨域 iframe** 均可。
    找不到返回 None。
    """

    def _fid(obj) -> int:
        return int(getattr(obj, "_frame_id", None) or id(obj))

    _ALIYUN_CTX_MARKERS = (
        _XP_ALIYUN_PUZZLE_IMG,
        _XP_ALIYUN_WINDOW,
        _XP_ALIYUN_SLIDING_BODY,
        _XP_ALIYUN_SLIDING_TEXT_BOX,
        _XP_ALIYUN_SLIDING_TEXT,
        _XP_ALIYUN_BG_IMG,
        _XP_ALIYUN_SLIDER_HANDLE,
    )

    q: deque = deque([(page, 0)])
    seen: set[int] = set()
    while q:
        ctx, depth = q.popleft()
        if depth > depth_max:
            continue
        for xp in _ALIYUN_CTX_MARKERS:
            try:
                if ctx.ele(xp, timeout=0.14):
                    return ctx
            except Exception:
                pass
        if depth >= depth_max:
            continue
        try:
            frames = ctx.get_frames(timeout=0.55)
        except Exception:
            frames = []
        for fr in frames:
            try:
                if getattr(fr, "_type", None) == "ChromiumFrame":
                    sub = fr
                else:
                    sub = ctx.get_frame(fr, timeout=1.2)
            except Exception:
                continue
            fid = _fid(sub)
            if fid in seen:
                continue
            seen.add(fid)
            q.append((sub, depth + 1))
    return None


def _aliyun_captcha_layer_visible(page: ChromiumPage) -> bool | None:
    """
    是否存在 **需用户操作的** 阿里云验证码层。
    在 ``_aliyun_host_context`` 对应文档内探测（含跨域 iframe）；不再依赖可跨域的 contentDocument。
    True：拼图可见且有 src；False：弹层/拼图已隐藏（多已通过）；None：无结构、或窗口尚在展开（小尺寸且无
    ``aliyunCaptcha-show``）等**不明状态**——勿当作已通过。
    """
    ctx = _aliyun_host_context(page)
    if ctx is None:
        return None
    js = """
    return (function(){
      var w = document.querySelector('#aliyunCaptcha-window-embed');
      var p = document.querySelector('#aliyunCaptcha-puzzle');
      var tb = document.querySelector('#aliyunCaptcha-sliding-text-box');
      var stxt = document.querySelector('#aliyunCaptcha-sliding-text');
      function visibleBox(el) {
        if (!el) return false;
        var s = getComputedStyle(el);
        if (s.display === 'none' || s.visibility === 'hidden') return false;
        var op = parseFloat(s.opacity);
        if (!isNaN(op) && op < 0.04) return false;
        var r = el.getBoundingClientRect();
        return r.width >= 6 && r.height >= 3;
      }
      var hintText = '';
      try {
        hintText = ((stxt && (stxt.innerText || stxt.textContent)) ||
            (tb && (tb.innerText || tb.textContent)) || '').trim();
      } catch (e) {}
      var hintOk = (visibleBox(tb) || visibleBox(stxt)) &&
          /滑块|拼图|拖动|slider|puzzle/i.test(hintText);

      if (p) {
        var src = (p.getAttribute('src') || '').trim();
        if (src) {
          var pst = getComputedStyle(p);
          if (pst.display === 'none' || pst.visibility === 'hidden') return false;
          var pop = parseFloat(pst.opacity);
          if (!isNaN(pop) && pop < 0.04) return false;
          var pr = p.getBoundingClientRect();
          if (pr.width < 2 || pr.height < 2) return false;
          if (w) {
            var st = getComputedStyle(w);
            var whidden = (st.display === 'none' || st.visibility === 'hidden');
            var opw = parseFloat(st.opacity);
            if (!isNaN(opw) && opw < 0.04) whidden = true;
            var wr = w.getBoundingClientRect();
            if (whidden || wr.width < 4 || wr.height < 4) return false;
            var cls = (w.getAttribute('class') || '');
            if (cls.indexOf('aliyunCaptcha-show') < 0 && wr.width < 20 && wr.height < 20) return null;
          }
          return true;
        }
        if (hintOk) return true;
      } else if (hintOk) {
        return true;
      }
      return null;
    })();
    """
    try:
        raw = ctx.run_js(js)
        if raw is True:
            return True
        if raw is False:
            return False
    except Exception:
        pass
    return None


def aliyun_captcha_cleared_for_success(page: ChromiumPage) -> bool:
    """
    当拼图节点探测（``captcha_exists``）已为假时的二次确认。

    若页内仍存在阿里云验证码宿主，仅当拼图层 **明确已收起**（``_aliyun_captcha_layer_visible`` 为
    False）才认为验证码真的消失；``True`` / ``None`` 表示仍可能在换图、动画或加载中，**不得**当作通过。

    无阿里云上下文时返回 True（由易盾等其它分支负责）。
    """
    if _aliyun_host_context(page) is None:
        return True
    av = _aliyun_captcha_layer_visible(page)
    return av is False


def aliyun_verification_success_visible(page: ChromiumPage) -> bool | None:
    """
    阿里云滑块松手后轮询用：是否出现明确「通过」态（文案 / 弹层收起）。
    True：可视为验证成功；False：仍提示拖动或失败重试；None：无阿里云上下文或无法判断。
    """
    ctx = _aliyun_host_context(page)
    if ctx is None:
        return None
    js = """
    return (function(){
      var stxt = document.querySelector('#aliyunCaptcha-sliding-text');
      var tb = document.querySelector('#aliyunCaptcha-sliding-text-box');
      var w = document.querySelector('#aliyunCaptcha-window-embed');
      function txt(el){
        try { return ((el && (el.innerText || el.textContent)) || '').trim(); } catch(e){ return ''; }
      }
      var t = txt(stxt) || txt(tb);
      if (/验证通过|验证成功|通过验证|校验成功|验证完成|拼图验证成功|校验完成/.test(t)) return true;
      if (/验证失败|请重试|再试一次|拖动滑块完成拼图|不匹配|未通过/.test(t)) return false;
      if (!w) return null;
      var st = getComputedStyle(w);
      var hidden = (st.display === 'none' || st.visibility === 'hidden');
      var op = parseFloat(st.opacity);
      if (!isNaN(op) && op < 0.04) hidden = true;
      if (hidden) {
        if (/拖动|拼图|滑块|完成验证/i.test(t)) return false;
        return true;
      }
      var r = w.getBoundingClientRect();
      if (r.width < 6 && r.height < 6) {
        if (/拖动|拼图|滑块|完成验证/i.test(t)) return false;
        return true;
      }
      return null;
    })();
    """
    try:
        raw = ctx.run_js(js)
        if raw is True:
            return True
        if raw is False:
            return False
    except Exception:
        pass
    return None


def aliyun_challenge_active(
    page: ChromiumPage,
    timeout: float = 0.85,
    *,
    read_img_src: Callable[[object], str] | None = None,
) -> bool:
    """当前是否处于阿里云滑块挑战（与 ``_aliyun_captcha_layer_visible`` 一致，带 iframe 内 ele 兜底）。"""
    av = _aliyun_captcha_layer_visible(page)
    if av is True:
        return True
    if av is False:
        return False
    ctx = _aliyun_host_context(page)
    if ctx is None:
        return False
    reader = read_img_src or _read_captcha_img_src
    try:
        p = ctx.ele(_XP_ALIYUN_PUZZLE_IMG, timeout=min(timeout, 0.55))
        if not p:
            return False
        if reader(p).strip():
            return True
        # 节点已挂载但 src 尚未异步写入：仍视为挑战进行中，避免 captcha_exists/try_auto_solve 误判为无验证码
        return True
    except Exception:
        return False


def aliyun_drag_scale_x(
    page: ChromiumPage,
    bg_ele,
    natural_width: int,
    *,
    fallback_display_scale: Callable[[object, int], float],
) -> float:
    """阿里云：轨道可行程 / 背景图自然宽度。"""
    if natural_width <= 0:
        return float(fallback_display_scale(bg_ele, natural_width))
    ctx = _aliyun_host_context(page) or page
    try:
        body = ctx.ele(_XP_ALIYUN_SLIDING_BODY, timeout=0.85)
        handle = ctx.ele(_XP_ALIYUN_SLIDER_HANDLE, timeout=0.5)
        if not body or not handle:
            raise ValueError("no aliyun track")
        w_ctrl, _ = body.rect.size
        w_h, _ = handle.rect.size
        w_handle = float(w_h)
        if w_handle < float(_ALIYUN_SLIDER_HANDLE_MIN_W):
            w_handle = float(_ALIYUN_SLIDER_HANDLE_FALLBACK_CSS_W)
        travel = float(w_ctrl) - w_handle
        if travel < 1.0:
            travel = float(w_ctrl) * 0.88
        s = travel / float(natural_width)
        if 0.04 <= s <= 4.0:
            return s
    except Exception:
        pass
    return float(fallback_display_scale(bg_ele, natural_width))


def aliyun_get_images(
    page: ChromiumPage,
    *,
    fallback_display_scale: Callable[[object, int], float],
    log_warning: Callable[[str], None] | None = None,
    log_info: Callable[[str], None] | None = None,
    read_img_src: Callable[[object], str] | None = None,
    err_not_found_nodes: str = "未找到阿里云验证码图片节点，请确认弹层已显示",
    err_src_not_ready: str = "阿里云验证码图片 src 尚未就绪（请确认未禁用图片加载）",
) -> tuple[str, str, float]:
    """拉取阿里云背景图、拼图块；返回 (bg_b64, puzzle_b64, drag_scale_x)。"""
    referer = (page.url or "").strip() or "https://www.icgoo.net/"
    ctx = _aliyun_host_context(page) or page
    reader = read_img_src or _read_captcha_img_src
    t1 = time.time() + 22.0
    bg_ele = None
    puzzle_ele = None
    while time.time() < t1:
        try:
            ctx.wait.eles_loaded(_XP_ALIYUN_PUZZLE_IMG, timeout=2)
        except Exception:
            pass
        puzzle_ele = ctx.ele(_XP_ALIYUN_PUZZLE_IMG, timeout=2)
        bg_ele = ctx.ele(_XP_ALIYUN_BG_IMG, timeout=2)
        if puzzle_ele and bg_ele:
            break
        time.sleep(0.28)
    if not puzzle_ele or not bg_ele:
        if log_warning:
            log_warning("aliyun get_images: 未找到 #aliyunCaptcha-puzzle / #aliyunCaptcha-img")
        raise ValueError(err_not_found_nodes)

    t2 = time.time() + 40.0
    puzzle_src, bg_src = "", ""
    n = 0
    while time.time() < t2:
        try:
            puzzle_ele = ctx.ele(_XP_ALIYUN_PUZZLE_IMG, timeout=1.5)
            bg_ele = ctx.ele(_XP_ALIYUN_BG_IMG, timeout=1.5)
        except Exception:
            puzzle_ele, bg_ele = None, None
        if not puzzle_ele or not bg_ele:
            time.sleep(0.35)
            continue
        puzzle_src = reader(puzzle_ele)
        bg_src = reader(bg_ele)
        if puzzle_src and bg_src:
            break
        n += 1
        if n % 5 == 0:
            try:
                bg_ele.scroll.to_see(center=True)
                time.sleep(0.15)
            except Exception:
                pass
        time.sleep(0.35)
    if not puzzle_src or not bg_src:
        raise ValueError(err_src_not_ready)

    puzzle_b64 = _captcha_img_src_to_b64_payload(puzzle_src, referer)
    bg_b64 = _captcha_img_src_to_b64_payload(bg_src, referer)
    nat_w = 0
    try:
        raw_bg = _decode_base64_image_payload(bg_b64)
        im_bg = Image.open(BytesIO(raw_bg))
        nat_w = int(im_bg.size[0])
    except Exception:
        nat_w = 0
    drag_scale_x = aliyun_drag_scale_x(
        page,
        bg_ele,
        nat_w,
        fallback_display_scale=fallback_display_scale,
    )
    if log_info:
        log_info(
            f"aliyun get_images ok: nat_bg_w={nat_w} drag_scale_x={drag_scale_x:.4f}"
        )
    return bg_b64, puzzle_b64, drag_scale_x
