"""
ICGOO (www.icgoo.net) 搜索页爬虫 - DrissionPage，与 ickey_crawler 输出格式对齐供 Go 调用

搜索 URL: https://www.icgoo.net/search/{型号}/1
站点为 SPA，需 Chromium 渲染后再解析表格；搜索数据通常需登录后可见。

账号密码（按需登录）:
  - 环境变量: ICGOO_USERNAME / ICGOO_PASSWORD（推荐，避免出现在命令行历史）
  - 或命令行: --user / --password
  - **默认**：先打开搜索页，仅当页面出现「请登录」等提示时才走登录页（减少无谓登录与易盾）。
  - 需要旧版「无 Cookie 时先登录再打开搜索」时加 ``--login-before-search``。

**减少易盾 / 验证码触发（推荐）**: 每次走登录页容易弹出网易易盾或阿里云滑块。可 **先人工登录一次** 后保存 Cookie，
之后用 ``--cookies-file`` 加载会话 **跳过登录**（仅打开搜索页，验证码出现概率低很多）::

  python icgoo_crawler.py --model XXX --no-headless --save-cookies icgoo_cookies.json
  # 登录并完成易盾后，下次：
  python icgoo_crawler.py --model XXX --cookies-file icgoo_cookies.json

环境变量 ``ICGOO_COOKIES_FILE`` 可指定默认 Cookie 文件路径。

**临时不用 Cookie 文件**：命令行 ``--no-cookies``，或环境变量 ``ICGOO_NO_COOKIES=1``（``true``/``yes``/``on`` 亦可），
本进程内不加载 ``--cookies-file`` / ``ICGOO_COOKIES_FILE``；有账号密码时仍会在**站点提示需登录**时再登录。与 ``--force-login`` 不同：后者为「忽略已存 Cookie 文件」；
``--no-cookies`` 侧重「本次不要读 Cookie 文件」，适合全局配置了 Cookie 路径但某次想干净会话。

**刷新/多次打开页面也会触发易盾**：站点对「访问 icgoo.net」本身做风控，并非只有登录页。
脚本已尽量 **不在加载 Cookie 时先打开首页**（改为空白页注入 Cookie，再直达搜索 URL），
并支持 **批量型号之间的请求间隔**（``--request-delay``）以降低「过于频繁」概率；仍无法完全消除对方策略。

**浏览器指纹 / 风控**：站点若采集 Canvas、WebGL、navigator、无头等特征，无法「完全伪装成另一台真机」，
只能降低自动化痕迹：**优先有界面（``--no-headless``）**、**出口 IP 与真人一致（如住宅/隧道代理）**、
**请求节奏放慢**、**复用 Cookie**。脚本会默认加上 ``--disable-blink-features=AutomationControlled``；
**勿再硬编码与当前 Chromium 主版本不一致的 UA**（除非用环境变量 ``ICGOO_USER_AGENT`` 显式指定）。

**出口 IP / 代理**：Chromium 经 HTTP 代理访问站点，便于与办公网络一致、降低风控概率。

1. **Agent 下发（快代理私密代理等）**：环境变量 ``CAICHIP_TASK_PARAMS`` 为 JSON，可含
   ``proxy_host``、``proxy_port``、可选 ``proxy_user``、``proxy_password``（与中心调度
   ``params`` 一致）。有用户名密码时按快代理官方 **Http 代理 · DrissionPage 用户名密码认证**
   方式加载 Chromium 扩展（见 https://www.kuaidaili.com/doc/dev/sdk_http/ ）；仅 host/port 时等价白名单 ``http://host:port``。
   **带账密的扩展在默认无头模式下常无法加载**，出口可能仍直连；请 ``--no-headless`` 或在快代理侧做 **白名单 IP** 后只下发无账密代理。

2. **本地调试**：命令行 ``--proxy URL``，或环境变量（优先级从高到低）``ICGOO_PROXY``、
   ``ICGOO_HTTP_PROXY``、``HTTPS_PROXY``、``HTTP_PROXY``、``ALL_PROXY``。
   示例：``http://127.0.0.1:7890``。

**优先级**：若 ``CAICHIP_TASK_PARAMS`` 中同时给出 ``proxy_host`` 与 ``proxy_port``，优先用其
配置浏览器代理；否则再解析 ``--proxy`` / 上述环境变量。``SOCKS`` 仍走 URL 解析；DrissionPage
对 **URL 内嵌账号密码** 可能不支持，请优先用任务参数中的分立字段或本地无认证 HTTP 代理。

**代理是否生效**：启动 Chromium 时会打印 **实际写入的代理方式**（``set_proxy`` 或 **账密扩展**）。
若需 **实测出口 IP**，设置环境变量 ``ICGOO_PROXY_VERIFY=1``（或 ``true``），会在打开业务页面前访问 ipify/httpbin 并打一条探测日志（需当前环境能访问外网探测站）。

CLI:
  set ICGOO_USERNAME=xxx
  set ICGOO_PASSWORD=xxx
  python icgoo_crawler.py --model LAN8720AI-CP-TR

**浏览器模式**：命令行未加 ``--no-headless`` 时 **默认无头**；``run_search`` / ``run_search_batch`` 的 ``headless`` 形参默认亦为 ``True``（无头），与 CLI 一致。需要可见窗口时请 ``--no-headless`` 或传入 ``headless=False``。

若站点弹出验证码/滑块，无头模式可能失败，请使用 ``--no-headless`` 在可见窗口中手动完成
易盾或阿里云滑块/短信验证；脚本会轮询直到验证消失或出现成功态。

**阿里云滑块（自动）**：搜索流程在加载 ``icgoo_yidun_solver`` 时，与 ``icgoo_crawler_dev`` 共用同一套
``IcgooYidunSliderSolver``（拉图、缺口识别、拖动、失败重试）；**不**包含 dev 的调试图目录与标定 JSONL 收集。
阿里云专用微调（拖动加量、scale 乘子、轨迹与风控相关开关等）见同目录 ``icgoo_aliyun_captcha.py`` 顶部说明，
通过环境变量 ``ICGOO_ALIYUN_*`` 生效（与 dev 的 ``--aliyun-*`` 写入的变量一致）。
可选地，还可设置 ``ICGOO_SLIDER_GAP_BACKEND``、``ICGOO_SLIDE_ATTEMPTS``、``ICGOO_DRAG_DISTANCE_BOOST``、
``ICGOO_DRAG_EXTRA_PX``、``ICGOO_GAP_OFFSET_IMAGE_PX`` 覆盖自动破解的缺口后端与拖动参数（见 ``try_auto_solve_icgoo_yidun_slider``）。

「您的搜索过于频繁」为服务端限流，需降低请求频率、换网络/账号，或隔一段时间再试；
无法通过纯代码绕过，亦不建议对接打码平台（合规与稳定性风险）。

依赖: ``pip install -r requirements.txt``（详见 requirements.txt）。若求解器导入失败，进程启动时会在 stderr 打一行原因；可用环境变量 ``ICGOO_QUIET_YIDUN_IMPORT=1`` 关闭该提示。

运行日志写入当前工作目录 ``icgoo_crawler.log``；需控制台提示时同时输出到 stderr。
"""

from __future__ import annotations

import argparse
import json
import logging
import os
import re
import shutil
import sys
import tempfile
import time
from dataclasses import dataclass
from typing import Any
from urllib.parse import quote, urlparse

from DrissionPage import ChromiumPage, ChromiumOptions

# 保证与本文件同目录的 icgoo_yidun_solver / icgoo_aliyun_* 可被导入（勿依赖当前工作目录）
_SCRIPTS_DIR = os.path.dirname(os.path.abspath(__file__))
if _SCRIPTS_DIR not in sys.path:
    sys.path.insert(0, _SCRIPTS_DIR)

try:
    from crawler_cli import emit_json_stderr_error, emit_json_stdout
except ImportError:
    from crawler_cli import emit_json_stderr_error, emit_json_stdout

try:
    from icgoo_aliyun_captcha import _aliyun_host_context
except ImportError:
    _aliyun_host_context = None  # type: ignore[assignment, misc]

_ICGOO_YIDUN_SOLVER_IMPORT_ERROR: str | None = None
try:
    from icgoo_yidun_solver import (
        IcgooBrowserDisconnectedError,
        try_auto_solve_icgoo_yidun_slider,
        yidun_still_requires_user_action,
        YidunAutoSolveResult,
    )
except Exception as _e:
    # 除 ModuleNotFoundError 外，Windows 上 cv2/onnx 等也可能以 OSError、RuntimeError 等形式失败
    try_auto_solve_icgoo_yidun_slider = None  # type: ignore[assignment, misc]
    yidun_still_requires_user_action = None  # type: ignore[assignment, misc]
    IcgooBrowserDisconnectedError = RuntimeError  # type: ignore[misc, assignment]
    _ICGOO_YIDUN_SOLVER_IMPORT_ERROR = f"{type(_e).__name__}: {_e}"
    if (os.environ.get("ICGOO_QUIET_YIDUN_IMPORT") or "").strip().lower() not in (
        "1",
        "true",
        "yes",
        "on",
    ):
        print(
            "[icgoo_crawler] icgoo_yidun_solver 未加载，自动滑块将不可用。原因: "
            f"{_ICGOO_YIDUN_SOLVER_IMPORT_ERROR}",
            file=sys.stderr,
        )

# 登录页（SPA，需等待 JS 渲染出表单）
DEFAULT_LOGIN_URL = "https://www.icgoo.net/login"
# 加载 Cookie 时优先用空白页，避免多一次打开 icgoo 首页触发易盾（失败再回退首页）
ICGOO_ORIGIN = "https://www.icgoo.net/"
_BLANK_PAGE = "about:blank"

_LOG = logging.getLogger("icgoo_crawler")


class _EchoConsoleFilter(logging.Filter):
    """为 StreamHandler 使用：``extra={"echo_console": False}`` 时仅写文件、不刷 stderr。"""

    def filter(self, record: logging.LogRecord) -> bool:
        return getattr(record, "echo_console", True)


def _ensure_icgoo_logging() -> None:
    if _LOG.handlers:
        return
    _LOG.setLevel(logging.INFO)
    _LOG.propagate = False
    fmt = logging.Formatter(
        "%(asctime)s [%(levelname)s] %(message)s",
        datefmt="%Y-%m-%d %H:%M:%S",
    )
    log_path = os.path.join(os.getcwd(), "icgoo_crawler.log")
    fh = logging.FileHandler(log_path, encoding="utf-8")
    fh.setLevel(logging.INFO)
    fh.setFormatter(fmt)
    sh = logging.StreamHandler(sys.stderr)
    sh.setLevel(logging.INFO)
    sh.setFormatter(fmt)
    sh.addFilter(_EchoConsoleFilter())
    _LOG.addHandler(fh)
    _LOG.addHandler(sh)


def _icgoo_log(
    level: int,
    msg: str,
    *,
    echo_console: bool = True,
) -> None:
    _ensure_icgoo_logging()
    _LOG.log(level, msg, extra={"echo_console": echo_console})


def _try_auto_solve_kwargs_from_env() -> dict[str, Any]:
    """
    可选环境变量，用于覆盖 ``try_auto_solve_icgoo_yidun_slider`` 的缺口后端与拖动参数；
    与 ``icgoo_crawler_dev`` 命令行中的 ``--slider-backend`` / ``--drag-boost`` 等对齐（此处用 ICGOO_ 前缀）。
    阿里云专用 ``ICGOO_ALIYUN_*`` 仍由 ``icgoo_aliyun_captcha`` 在运行时读取，无需在此列出。
    """
    out: dict[str, Any] = {}
    b = (os.environ.get("ICGOO_SLIDER_GAP_BACKEND") or "").strip().lower()
    if b in ("auto", "ddddocr", "opencv", "slidercracker"):
        out["slider_gap_backend"] = b
    sa = (os.environ.get("ICGOO_SLIDE_ATTEMPTS") or "").strip()
    if sa:
        try:
            v = int(sa)
            if v >= 1:
                out["slide_attempts"] = v
        except ValueError:
            pass
    db = (os.environ.get("ICGOO_DRAG_DISTANCE_BOOST") or "").strip()
    if db:
        try:
            out["drag_boost"] = float(db)
        except ValueError:
            pass
    de = (os.environ.get("ICGOO_DRAG_EXTRA_PX") or "").strip()
    if de:
        try:
            out["drag_extra_px"] = int(de)
        except ValueError:
            pass
    go = (os.environ.get("ICGOO_GAP_OFFSET_IMAGE_PX") or "").strip()
    if go:
        try:
            out["gap_offset_image_px"] = int(go)
        except ValueError:
            pass
    return out


def _get_credentials(
    user: str | None, password: str | None
) -> tuple[str | None, str | None]:
    u = (user or os.environ.get("ICGOO_USERNAME") or "").strip() or None
    p = (password or os.environ.get("ICGOO_PASSWORD") or "").strip() or None
    return u, p


def _default_cookies_file_path(explicit: str | None) -> str | None:
    if explicit:
        return explicit
    env = (os.environ.get("ICGOO_COOKIES_FILE") or "").strip()
    return env or None


def _icgoo_no_cookies_env() -> bool:
    v = (os.environ.get("ICGOO_NO_COOKIES") or "").strip().lower()
    return v in ("1", "true", "yes", "on")


@dataclass
class BrowserProxyConfig:
    """浏览器出口代理：URL（含无认证 HTTP）或快代理式 host/port/可选账号（与 CAICHIP_TASK_PARAMS 对齐）。"""

    url: str | None = None
    host: str | None = None
    port: int | None = None
    user: str | None = None
    password: str | None = None

    def effective(self) -> bool:
        if self.url:
            return True
        return bool(self.host and self.port is not None)


def _load_caichip_task_params() -> dict[str, Any] | None:
    raw = (os.environ.get("CAICHIP_TASK_PARAMS") or "").strip()
    if not raw:
        return None
    try:
        obj = json.loads(raw)
    except json.JSONDecodeError:
        return None
    return obj if isinstance(obj, dict) else None


def _coerce_proxy_port(v: Any) -> int | None:
    if v is None:
        return None
    try:
        if isinstance(v, bool):
            return None
        if isinstance(v, int):
            return v if 1 <= v <= 65535 else None
        if isinstance(v, float):
            p = int(v)
            return p if 1 <= p <= 65535 else None
        s = str(v).strip()
        if not s:
            return None
        p = int(s)
        return p if 1 <= p <= 65535 else None
    except (TypeError, ValueError):
        return None


def _proxy_from_caichip_params(p: dict[str, Any]) -> BrowserProxyConfig | None:
    host = (p.get("proxy_host") or "").strip() or None
    port = _coerce_proxy_port(p.get("proxy_port"))
    if not host or port is None:
        return None
    user = (p.get("proxy_user") or "").strip() or None
    pw = (p.get("proxy_password") or "").strip() or None
    return BrowserProxyConfig(host=host, port=port, user=user, password=pw)


def _resolve_browser_proxy(cli_proxy: str | None) -> BrowserProxyConfig:
    """
    合并 CAICHIP_TASK_PARAMS 中的 proxy_*（Agent 下发、快代理 HTTP 用法）与 CLI/环境变量 URL。
    """
    tp = _load_caichip_task_params()
    if tp:
        k = _proxy_from_caichip_params(tp)
        if k:
            return k
    url = _resolve_chromium_proxy(cli_proxy)
    return BrowserProxyConfig(url=url) if url else BrowserProxyConfig()


def _create_kuaidaili_drission_proxy_extension(
    proxy_host: str,
    proxy_port: int,
    proxy_username: str,
    proxy_password: str,
    plugin_folder: str | None = None,
) -> str:
    """
    快代理文档「代码样例 - Http 代理 · DrissionPage · 用户名密码认证」中的 Chromium 扩展目录。
    参考: https://www.kuaidaili.com/doc/dev/sdk_http/
    """
    if plugin_folder is None:
        plugin_folder = tempfile.mkdtemp(prefix="icgoo_kdl_chromium_proxy_")
    os.makedirs(plugin_folder, exist_ok=True)
    manifest_json = """
        {
            "version": "1.0.0",
            "manifest_version": 2,
            "name": "kdl_Chromium_Proxy",
            "permissions": [
                "proxy",
                "tabs",
                "unlimitedStorage",
                "storage",
                "<all_urls>",
                "webRequest",
                "webRequestBlocking",
                "browsingData"
            ],
            "background": {
                "scripts": ["background.js"]
            },
            "minimum_chrome_version":"22.0.0"
        }
    """
    h = json.dumps(proxy_host)
    p = int(proxy_port)
    u = json.dumps(proxy_username)
    w = json.dumps(proxy_password)
    background_js = f"""
        var config = {{
            mode: "fixed_servers",
            rules: {{
                singleProxy: {{
                    scheme: "http",
                    host: {h},
                    port: parseInt({p})
                }},
                bypassList: []
            }}
        }};

        chrome.proxy.settings.set({{value: config, scope: "regular"}}, function() {{}});

        function callbackFn(details) {{
            return {{
                authCredentials: {{
                    username: {u},
                    password: {w}
                }}
            }};
        }}

        chrome.webRequest.onAuthRequired.addListener(
            callbackFn,
            {{urls: ["<all_urls>"]}},
            ['blocking']
        );
    """
    with open(os.path.join(plugin_folder, "manifest.json"), "w", encoding="utf-8") as f:
        f.write(manifest_json.strip())
    with open(os.path.join(plugin_folder, "background.js"), "w", encoding="utf-8") as f:
        f.write(background_js.strip())
    return os.path.abspath(plugin_folder)


def _resolve_chromium_proxy(cli_proxy: str | None) -> str | None:
    """
    解析 Chromium ``--proxy-server`` 用的代理串。
    CLI 优先；否则依次读 ICGOO_PROXY、ICGOO_HTTP_PROXY、HTTPS_PROXY、HTTP_PROXY、ALL_PROXY。
    仅 ``host:port`` 时补 ``http://`` 前缀。
    """
    raw = (cli_proxy or "").strip()
    if not raw:
        for key in (
            "ICGOO_PROXY",
            "ICGOO_HTTP_PROXY",
            "HTTPS_PROXY",
            "HTTP_PROXY",
            "ALL_PROXY",
        ):
            raw = (os.environ.get(key) or "").strip()
            if raw:
                break
    if not raw:
        return None
    low = raw.lower()
    if low.startswith("socks"):
        return raw
    if "://" not in raw:
        if re.match(r"^[\w.-]+:\d{1,5}$", raw):
            raw = f"http://{raw}"
    return raw


def _cookie_dict_is_icgoo(c: dict) -> bool:
    d = (c.get("domain") or "").lower()
    return "icgoo.net" in d


def load_icgoo_cookies_from_file(page: ChromiumPage, path: str) -> bool:
    """
    从 JSON 恢复会话 Cookie（仅含 icgoo.net 相关），成功则返回 True。
    文件格式: page.cookies(all_domains=True, all_info=True) 过滤后的 list[dict]。

    优先在 about:blank 注入 Cookie，避免先导航到 icgoo 首页（多一次页面加载易触发易盾）。
    """
    if not path or not os.path.isfile(path):
        return False
    try:
        with open(path, encoding="utf-8") as f:
            raw = json.load(f)
    except (OSError, json.JSONDecodeError):
        return False
    if not isinstance(raw, list) or not raw:
        return False

    def _try_set(after_get: str) -> bool:
        page.get(after_get)
        time.sleep(0.35 if after_get == _BLANK_PAGE else 0.55)
        try:
            page.set.cookies(raw)
            return True
        except Exception:
            return False

    if _try_set(_BLANK_PAGE):
        return True
    # 部分环境下需在真实站点页上 set；仅作兜底（多一次 icgoo 导航）
    return _try_set(ICGOO_ORIGIN)


def save_icgoo_cookies_to_file(page: ChromiumPage, path: str, quiet: bool = False) -> bool:
    """将当前浏览器中 icgoo.net 相关 Cookie 写入 JSON。"""
    try:
        all_c = page.cookies(all_domains=True, all_info=True)
        filtered = [dict(c) for c in all_c if _cookie_dict_is_icgoo(c)]
        if not filtered:
            _icgoo_log(
                logging.INFO,
                "未找到 icgoo.net 相关 Cookie，跳过保存",
                echo_console=not quiet,
            )
            return False
        parent = os.path.dirname(os.path.abspath(path))
        if parent and not os.path.isdir(parent):
            os.makedirs(parent, exist_ok=True)
        with open(path, "w", encoding="utf-8") as f:
            json.dump(filtered, f, ensure_ascii=False, indent=2)
        _icgoo_log(
            logging.INFO,
            f"已保存 {len(filtered)} 条 Cookie 到: {path}",
            echo_console=not quiet,
        )
        return True
    except OSError as e:
        _icgoo_log(
            logging.ERROR,
            f"保存 Cookie 失败: {e}",
            echo_console=not quiet,
        )
        return False


def _icgoo_rate_limit_ui_visible(page: ChromiumPage) -> bool:
    """
    「搜索过于频繁」是否对用户可见。

    勿用 ``\"slide-frequently\" in page.html``：SPA 打包脚本、隐藏模板里常含该子串，
    易盾已过后仍会误报，从而进 ``wait_for_manual``。
    """
    js = """
    return (function(){
      function visible(el){
        if (!el) return false;
        var st = getComputedStyle(el);
        if (st.display === 'none' || st.visibility === 'hidden') return false;
        var op = parseFloat(st.opacity);
        if (!isNaN(op) && op < 0.05) return false;
        var r = el.getBoundingClientRect();
        return r.width > 8 && r.height > 8;
      }
      var box = document.getElementById('slide-frequently');
      if (box && visible(box)) return true;
      try {
        var nodes = document.querySelectorAll('.frequently-text');
        for (var i = 0; i < nodes.length; i++) {
          if (visible(nodes[i])) return true;
        }
      } catch (e) {}
      try {
        var bt = (document.body && document.body.innerText) || '';
        if (/您的搜索过于频繁/.test(bt)) return true;
      } catch (e) {}
      return false;
    })();
    """
    try:
        return page.run_js(js) is True
    except Exception:
        return False


def _has_icgoo_yidun_or_frequent_block(page: ChromiumPage) -> bool:
    """
    是否仍存在需处理的「搜索过于频繁」或 **易盾 / 阿里云** 滑块可见交互层。

    易盾通过后页面常保留隐藏 ``.yidun`` / ``img.yidun_jigsaw``，仅用 ``ele('.yidun')`` 会误报；
    有 ``icgoo_yidun_solver`` 时改用与自动破解相同的 JS 探测（含阿里云 iframe / 成功态）。
    若求解器未加载，则回退为 DOM 选择器；其中须包含阿里云嵌入层，避免仅命中 ``.yidun`` 时漏检。
    """
    if _icgoo_rate_limit_ui_visible(page):
        return True

    try:
        html = page.html or ""
    except Exception:
        html = ""

    low = html.lower()
    has_yidun_trace = "yidun" in low or "nc-container" in html
    if not has_yidun_trace and "aliyuncaptcha" in low:
        has_yidun_trace = True
    if not has_yidun_trace:
        try:
            if page.ele("css:.yidun", timeout=0.25):
                has_yidun_trace = True
        except Exception:
            pass
        if not has_yidun_trace:
            try:
                if page.ele("css:.nc-container", timeout=0.2):
                    has_yidun_trace = True
            except Exception:
                pass
        if not has_yidun_trace:
            try:
                if page.ele("css:#aliyunCaptcha-window-embed", timeout=0.2):
                    has_yidun_trace = True
            except Exception:
                pass
        if not has_yidun_trace and _aliyun_host_context is not None:
            try:
                if _aliyun_host_context(page) is not None:
                    has_yidun_trace = True
            except Exception:
                pass
    if not has_yidun_trace:
        return False

    if yidun_still_requires_user_action is not None:
        try:
            return yidun_still_requires_user_action(page)
        except Exception:
            pass

    for sel in (
        "css:.yidun",
        "css:.nc-container",
        "css:#aliyunCaptcha-window-embed",
    ):
        try:
            if page.ele(sel, timeout=0.4):
                return True
        except Exception:
            continue
    return False


def wait_for_manual_captcha_or_rate_limit(
    page: ChromiumPage,
    timeout_sec: float = 300.0,
    quiet: bool = False,
    poll_interval: float = 1.2,
) -> bool:
    """
    若页面存在易盾/频繁限制，提示用户在浏览器中手动完成验证，并轮询直到消失或成功态。

    返回 True 表示已无障碍（或本来就没有）；False 表示超时仍未消除。
    """
    if not _has_icgoo_yidun_or_frequent_block(page):
        return True
    msg = (
        "检测到验证码（易盾/阿里云滑块）或「搜索过于频繁」提示：请在浏览器窗口内完成验证；"
        "完成后脚本会自动继续…"
    )
    _icgoo_log(logging.WARNING, msg, echo_console=True)
    if quiet:
        _icgoo_log(
            logging.WARNING,
            f"（最长等待 {timeout_sec:.0f} 秒；请加 --no-headless 以便操作浏览器）",
            echo_console=True,
        )
    else:
        _icgoo_log(
            logging.WARNING,
            f"（最长等待 {timeout_sec:.0f} 秒，可按 Ctrl+C 中断）",
            echo_console=True,
        )
    deadline = time.time() + timeout_sec
    while time.time() < deadline:
        time.sleep(poll_interval)
        # 易盾成功态（阿里云无此类 class，依赖下方 _has_icgoo_yidun_or_frequent_block / yidun_still_requires_user_action）
        try:
            if page.ele("css:.yidun.yidun--success", timeout=0.3):
                time.sleep(0.8)
                if not _has_icgoo_yidun_or_frequent_block(page):
                    return True
                continue
        except Exception:
            pass
        if not _has_icgoo_yidun_or_frequent_block(page):
            return True
    err = "等待验证/限流解除超时，后续步骤可能仍被拦截。"
    _icgoo_log(logging.ERROR, err, echo_console=True)
    return False


def icgoo_suggests_not_logged_in(page: ChromiumPage) -> bool:
    """
    根据当前页 URL / 文案判断是否像未登录访客（Cookie 失效、被重定向到登录等）。
    用于在打开搜索页后决定是否需要走账号密码登录。
    """
    try:
        path = (urlparse(page.url or "").path or "").lower().rstrip("/")
        if path.endswith("/login"):
            return True
    except Exception:
        pass
    try:
        html = page.html or ""
    except Exception:
        html = ""
    if len(html) < 200:
        return False
    for needle in (
        "请登录",
        "登录后查看",
        "登录后可见",
        "登录后可",
        "请先登录",
        "立即登录查看",
    ):
        if needle in html:
            return True
    if re.search(r"登录\s*/\s*注册", html):
        return True
    return False


def login_icgoo(
    page: ChromiumPage,
    username: str,
    password: str,
    login_url: str = DEFAULT_LOGIN_URL,
    quiet: bool = False,
    captcha_wait_sec: float = 300.0,
    wait_captcha_after_login: bool = True,
) -> None:
    """
    打开登录页并提交账号密码。站点为 Vue/Element UI，选择器多路兜底。

    wait_captcha_after_login:
      登录成功跳转后是否在**当前 URL**（常为首页 ``/?code=...``）上调用易盾/限流等待。
      开发脚本若随后会 ``get`` 到搜索页，可设为 False，只在业务页面上做一次等待，避免在回调首页误拦。
    """
    if not quiet:
        _icgoo_log(logging.INFO, f"登录页: {login_url}", echo_console=True)
    page.get(login_url)
    time.sleep(2)

    try:
        page.wait.ele_displayed("css:input[type=password]", timeout=25)
    except Exception as e:
        raise RuntimeError("登录页未出现密码输入框（可能网络异常或页面改版）") from e

    time.sleep(0.5)

    # 密码框
    pwd_el = page.ele("css:input[type=password]", timeout=5)
    # 账号：同一表单内、密码之前的第一个可见文本/手机输入框
    user_el = None
    for xpath in (
        'xpath://input[@type="password"]/preceding::input[(@type="text" or @type="tel")][1]',
        "xpath://form//input[not(@type) or @type='text' or @type='tel'][1]",
        "css:input.el-input__inner",
    ):
        try:
            cand = page.ele(xpath, timeout=2)
            if cand and cand != pwd_el:
                t = (cand.attr("type") or "").lower()
                if t in ("", "text", "tel", "email"):
                    user_el = cand
                    break
        except Exception:
            continue

    if not user_el:
        # 取页面上第一个非 password 的 input（排除 hidden）
        try:
            for inp in page.eles("tag:input"):
                t = (inp.attr("type") or "").lower()
                if t in ("hidden", "submit", "button", "checkbox", "radio", "password"):
                    continue
                if inp == pwd_el:
                    continue
                user_el = inp
                break
        except Exception:
            pass

    if not user_el:
        raise RuntimeError("未找到账号/手机输入框")

    try:
        user_el.clear()
    except Exception:
        pass
    user_el.input(username)
    time.sleep(0.2)
    try:
        pwd_el.clear()
    except Exception:
        pass
    pwd_el.input(password)
    time.sleep(0.3)

    # 登录按钮
    clicked = False
    for sel in (
        'xpath://button[contains(.,"登录") and not(contains(.,"注册"))]',
        "text:登录",
        'css:button[type=submit]',
        'xpath://span[contains(.,"登录")]/ancestor::button[1]',
    ):
        try:
            btn = page.ele(sel, timeout=2)
            if btn:
                btn.click()
                clicked = True
                break
        except Exception:
            continue

    if not clicked:
        # 回车提交
        try:
            pwd_el.input("\n")
        except Exception:
            raise RuntimeError("未找到登录按钮且无法提交表单")

    time.sleep(2)
    # 等待跳转或首页/搜索页加载（失败时常仍停在 login）
    deadline = time.time() + 15
    while time.time() < deadline:
        u = page.url or ""
        if "login" not in u.lower() and u.startswith("http"):
            break
        time.sleep(0.5)

    # 简单校验：若仍在纯登录 URL 且能看到错误提示，可再判断
    if (page.url or "").rstrip("/").endswith("/login"):
        # 再等一会 SPA 路由
        time.sleep(2)
    if not quiet:
        _icgoo_log(logging.INFO, f"登录后 URL: {page.url}", echo_console=True)

    if wait_captcha_after_login:
        wait_for_manual_captcha_or_rate_limit(
            page, timeout_sec=captcha_wait_sec, quiet=quiet
        )


def _cleanup_icgoo_proxy_extension(page: ChromiumPage | None) -> None:
    d = getattr(page, "_icgoo_proxy_extension_dir", None) if page else None
    if d and os.path.isdir(d):
        shutil.rmtree(d, ignore_errors=True)


def _icgoo_proxy_verify_env() -> bool:
    v = (os.environ.get("ICGOO_PROXY_VERIFY") or "").strip().lower()
    return v in ("1", "true", "yes", "on")


def _proxy_url_for_log(url: str) -> str:
    """日志用：隐去 URL 中的用户名密码。"""
    try:
        p = urlparse(url.strip())
        if p.username or p.password:
            port = f":{p.port}" if p.port else ""
            host = p.hostname or ""
            scheme = p.scheme or "http"
            return f"{scheme}://***:***@{host}{port}"
        return url.strip()
    except Exception:
        return url.strip()[:120]


def _log_chromium_outgoing_ip_probe(page: ChromiumPage) -> None:
    """可选：用外网服务探测当前 Chromium 出口 IP（验证代理是否真在用）。"""
    if not _icgoo_proxy_verify_env():
        return
    probes: list[tuple[str, str]] = [
        ("https://api.ipify.org?format=json", "ipify"),
        ("https://httpbin.org/ip", "httpbin"),
    ]
    for url, name in probes:
        try:
            page.get(url)
            time.sleep(1.0)
            raw = (page.html or "").strip()
            if not raw:
                continue
            ip = ""
            if "{" in raw:
                try:
                    obj = json.loads(raw)
                    if isinstance(obj, dict):
                        ip = str(obj.get("ip") or obj.get("origin") or "").strip()
                except json.JSONDecodeError:
                    pass
            if not ip:
                m = re.search(
                    r"\b(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}"
                    r"(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\b",
                    raw,
                )
                if m:
                    ip = m.group(0)
            if ip:
                _icgoo_log(
                    logging.INFO,
                    f"代理出口探测 OK（{name}）: 本浏览器当前出口 IP 为 {ip}",
                    echo_console=True,
                )
                return
        except Exception as e:
            _icgoo_log(
                logging.WARNING,
                f"代理出口探测失败（{name}）: {type(e).__name__}: {e}",
                echo_console=True,
            )
    _icgoo_log(
        logging.WARNING,
        "代理出口探测：ipify/httpbin 均未得到 IP，请检查网络、代理或关闭探测",
        echo_console=True,
    )


def setup_browser(
    headless: bool = True, proxy_cfg: BrowserProxyConfig | None = None
) -> ChromiumPage:
    """
    配置 Chromium；快代理带账号密码时按官方 Http 代理文档加载扩展（DrissionPage 用户名密码样例）。
    """
    co = ChromiumOptions()
    co.auto_port()
    # UA 与真实 Chromium 主版本不一致时易被归一化为「伪造指纹」；默认不覆盖，由浏览器自带 UA。
    ua = (os.environ.get("ICGOO_USER_AGENT") or "").strip()
    if ua:
        co.set_user_agent(ua)
    co.headless(headless)
    co.set_argument("--remote-allow-origins=*")
    co.set_argument("--disable-blink-features=AutomationControlled")
    if headless:
        co.set_argument("--headless=new")
    co.set_argument("--window-size=1400,900")
    cfg = proxy_cfg or BrowserProxyConfig()
    ext_dir: str | None = None
    if cfg.host and cfg.port is not None:
        if cfg.user and cfg.password:
            ext_dir = _create_kuaidaili_drission_proxy_extension(
                cfg.host, cfg.port, cfg.user, cfg.password
            )
            co.add_extension(ext_dir)
            _icgoo_log(
                logging.INFO,
                "Chromium 代理 — 已注入启动项: 账密扩展 (chrome.proxy + onAuthRequired)，"
                f"目标 {cfg.host}:{cfg.port}；headless={headless}。"
                "（无头时扩展常未加载，流量可能仍直连。）",
                echo_console=True,
            )
        else:
            pu = f"http://{cfg.host}:{cfg.port}"
            co.set_proxy(pu)
            _icgoo_log(
                logging.INFO,
                f"Chromium 代理 — 已注入启动项: set_proxy → {pu}（不依赖扩展，无头可用）",
                echo_console=True,
            )
    elif (cfg.url or "").strip():
        u = cfg.url.strip()
        co.set_proxy(u)
        _icgoo_log(
            logging.INFO,
            f"Chromium 代理 — 已注入启动项: set_proxy → {_proxy_url_for_log(u)}",
            echo_console=True,
        )
    else:
        _icgoo_log(
            logging.INFO,
            "Chromium 网络 — 未配置代理，浏览器直连",
            echo_console=True,
        )
    if ext_dir and headless:
        _icgoo_log(
            logging.WARNING,
            "无头模式 + 代理账密扩展：Chromium 往往不加载扩展，代理可能未生效（流量仍可能直连）。"
            "请使用 --no-headless，或快代理白名单仅用 host:port（无用户名密码）。",
            echo_console=True,
        )
    # 勿关闭图片：网易易盾验证码依赖 <img> 正常加载，imagesEnabled=false 会导致拼图 src 长期为空、自动/手动滑块均失败。
    page = ChromiumPage(co)
    if ext_dir:
        setattr(page, "_icgoo_proxy_extension_dir", ext_dir)
    _log_chromium_outgoing_ip_probe(page)
    return page


def search_url(keyword: str) -> str:
    # 路径中型号做百分号编码，斜杠等需编码；t= 为当前毫秒时间戳（与站点防缓存参数一致）
    ts = int(time.time() * 1000)
    return f"https://www.icgoo.net/search/{quote(keyword, safe='')}/1?t={ts}"


def _maybe_auto_solve_yidun(
    page: ChromiumPage, quiet: bool
) -> "YidunAutoSolveResult | None":
    """
    若存在易盾拼图 img，尝试自动滑动；依赖 opencv / Pillow / 可选 ddddocr。
    返回 ``None`` 表示未加载求解器或调用抛错（已记日志）；否则返回 ``YidunAutoSolveResult``。
    """
    if try_auto_solve_icgoo_yidun_slider is None:
        if _has_icgoo_yidun_or_frequent_block(page):
            _why = _ICGOO_YIDUN_SOLVER_IMPORT_ERROR or (
                "（未记录：请更新 icgoo_crawler.py 或检查是否用了非本仓库脚本副本）"
            )
            _icgoo_log(
                logging.WARNING,
                "页面存在易盾/阿里云滑块或限流提示，但未加载 icgoo_yidun_solver。"
                f" 原因: {_why} "
                "请在本机执行: pip install -r requirements.txt（与运行爬虫同一 Python），"
                "并在仓库 scripts 目录下自检: python -c \"import icgoo_yidun_solver\"。"
                " 环境变量 ICGOO_QUIET_YIDUN_IMPORT=1 可关闭启动时关于求解器导入的 stderr 提示。",
                echo_console=not quiet,
            )
        return None
    try:
        res = try_auto_solve_icgoo_yidun_slider(
            page,
            quiet=quiet,
            **_try_auto_solve_kwargs_from_env(),
        )
        if res.auto_solved:
            _icgoo_log(
                logging.INFO,
                "滑块验证码（易盾或阿里云）已由脚本自动通过，继续执行搜索与解析。",
                echo_console=not quiet,
            )
        if not res:
            _icgoo_log(
                logging.WARNING,
                "滑块自动破解已执行但未通过（返回失败），将进入手动等待。"
                "请查看当前目录 icgoo_yidun_solver.log 中的缺口识别与滑动记录；"
                "无头模式易失败时请使用 --no-headless 人工完成滑块。",
                echo_console=not quiet,
            )
        return res
    except IcgooBrowserDisconnectedError:
        raise
    except Exception as e:
        _ensure_icgoo_logging()
        _LOG.warning(
            "滑块自动识别抛错 %s: %s，将进入手动等待。"
            "请核对依赖（opencv-python-headless、Pillow、可选 ddddocr）与同目录 icgoo_yidun_solver.log。",
            type(e).__name__,
            e,
            exc_info=True,
            extra={"echo_console": not quiet},
        )
        return None


def wait_for_results(page: ChromiumPage, timeout: float = 25.0, quiet: bool = False) -> bool:
    """等待列表区域出现（多选择器兜底：新版商品卡片 + 旧版表格）。"""
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            cards = page.eles("css:.product-list-wrapper .product-card-item", timeout=1)
            if not cards:
                cards = page.eles("css:.product-list-wrapper [class*='ProductCard_container']", timeout=1)
            if cards and len(cards) >= 1:
                return True
        except Exception:
            pass
        selectors = [
            "css:.el-table__body tbody tr",
            "css:table tbody tr",
            "xpath://table//tbody//tr[td]",
            "css:[class*='table'] tbody tr",
        ]
        for sel in selectors:
            try:
                rows = page.eles(sel, timeout=1)
                if rows and len(rows) >= 1:
                    for r in rows[:3]:
                        try:
                            tds = r.eles("tag:td", timeout=0.2)
                            if len(tds) >= 3:
                                return True
                        except Exception:
                            continue
            except Exception:
                continue
        time.sleep(0.5)
    return False


def _extract_prices_line(text: str) -> str:
    """从单元格文本拼价格梯度: 1+ ￥x | 10+ ￥y"""
    parts = []
    for m in re.finditer(r"(\d+)\s*\+\s*[￥¥]?\s*([\d.]+)", text):
        parts.append(f"{m.group(1)}+ ￥{m.group(2)}")
    if parts:
        return " | ".join(parts)
    m = re.search(r"[￥¥]\s*([\d.]+)", text)
    if m:
        return f"1+ ￥{m.group(1)}"
    return "N/A"


def _digits_stock(text: str) -> str:
    t = re.sub(r"[,\s]", "", text)
    m = re.search(r"(\d+)", t)
    return m.group(1) if m else "0"


def _pick_model(cells: list[str], query_hint: str) -> str:
    """在单元格中选最像型号的串。"""
    best = ""
    for c in cells:
        c = (c or "").strip()
        if not c or len(c) < 3:
            continue
        if re.match(r"^[\d\s,]+$", c):
            continue
        if "型号" in c or "品牌" in c or "封装" in c or "库存" in c:
            continue
        if query_hint and query_hint.upper() in c.upper():
            return c
        if re.search(r"[A-Za-z]", c) and len(c) > len(best):
            best = c
    return best or (cells[1] if len(cells) > 1 else cells[0] if cells else "N/A")


def parse_product_cards_js(page: ChromiumPage, quiet: bool = True) -> list[dict]:
    """解析新版搜索页 ``product-list-wrapper`` 内商品卡片（与 ickey 字段对齐）。

    站点由表格改为 Vue 卡片后，类名带 CSS Modules 哈希；选择器用 ``[class*=\"...\"]`` 与结构兜底。

    说明：DrissionPage 会把脚本包成 ``function(){ ... }`` 再执行（见 ``is_js_func`` / ``callFunctionOn``）。
    若脚本写成 ``(function(){ return x; })();``，内层返回值不会传给外层，外层隐式 ``undefined``，
    Python 侧得到 ``None``。因此这里只用「函数体 + ``return JSON.stringify(out)``」，不要外包 IIFE。

    另：对象数组经 CDP 反序列化不稳定，故用 ``JSON.stringify`` + ``json.loads``。
    """
    js = r"""
      function text(el){ return el ? (el.innerText || '').trim().replace(/\s+/g, ' ') : ''; }
      function findSpec(card, keyword) {
        var labels = card.querySelectorAll('[class*="specLabel"], [class*="SpecLabel"]');
        for (var i = 0; i < labels.length; i++) {
          var lab = labels[i];
          if ((lab.textContent || '').indexOf(keyword) < 0) continue;
          var par = lab.closest('[class*="specItem"]') || lab.closest('[class*="descriptionItem"]');
          if (!par) continue;
          var val = par.querySelector('[class*="specValue"], [class*="SpecValue"]');
          return val ? text(val) : '';
        }
        return '';
      }
      function stockField(card, keyword) {
        var labels = card.querySelectorAll('[class*="stockLabel"]');
        for (var i = 0; i < labels.length; i++) {
          if ((labels[i].textContent || '').indexOf(keyword) < 0) continue;
          var par = labels[i].closest('[class*="stockItem"]');
          if (!par) continue;
          var val = par.querySelector('[class*="stockValue"]');
          return val ? text(val).replace(/,/g, '') : '';
        }
        return '';
      }
      function parseRowPrice(row) {
        var t = text(row);
        if (t === '-' || !t) return {usd: '', cny: ''};
        var m = t.match(/\$\s*([\d.]+)/);
        var usd = m ? ('$' + m[1]) : '';
        var m2 = t.match(/[￥¥]\s*([\d.]+)/);
        var cny = m2 ? ('￥' + m2[1]) : '';
        return {usd: usd, cny: cny};
      }
      function parsePrices(card) {
        var tierEls = card.querySelectorAll('[class*="priceTierLabel"]');
        var labels = [];
        for (var i = 0; i < tierEls.length; i++) labels.push(text(tierEls[i]));
        var valuesRoot = card.querySelector('[class*="priceValues"]');
        if (!valuesRoot) return {hk_price: 'N/A', mainland_price: 'N/A', price_tiers: 'N/A'};
        var cols = valuesRoot.querySelectorAll(':scope > [class*="priceValue"]');
        var usdRows = [], cnyRows = [];
        if (cols.length >= 2) {
          usdRows = cols[0].querySelectorAll('[class*="priceRow"]');
          cnyRows = cols[1].querySelectorAll('[class*="priceRow"]');
        } else if (cols.length === 1) {
          cnyRows = cols[0].querySelectorAll('[class*="priceRow"]');
        }
        var hkParts = [], cnParts = [];
        var n = Math.max(labels.length, Math.max(usdRows.length, cnyRows.length));
        for (var j = 0; j < n; j++) {
          var lbl = labels[j] || '';
          var u = '', c = '';
          if (usdRows[j]) {
            var pr = parseRowPrice(usdRows[j]);
            u = pr.usd;
            if (!c && pr.cny) c = pr.cny;
          }
          if (cnyRows[j]) {
            var pr2 = parseRowPrice(cnyRows[j]);
            if (!c && pr2.cny) c = pr2.cny;
            if (!u && pr2.usd) u = pr2.usd;
          }
          if (u) hkParts.push(lbl + ' ' + u);
          if (c) cnParts.push(lbl + ' ' + c);
        }
        var hk = hkParts.length ? hkParts.join(' | ') : 'N/A';
        var cn = cnParts.length ? cnParts.join(' | ') : 'N/A';
        var tiers = cn !== 'N/A' ? cn : hk;
        return {hk_price: hk, mainland_price: cn, price_tiers: tiers};
      }
      function leadTime(card) {
        var sec = card.querySelector('[class*="deliveryTimeSection"]');
        if (!sec) return 'N/A';
        var picked = '';
        var infos = sec.querySelectorAll('[class*="deliveryInfo"]');
        for (var i = 0; i < infos.length; i++) {
          var info = infos[i];
          var dot = info.querySelector('[class*="deliveryDot"]');
          var selected = dot && dot.className.indexOf('selected') >= 0;
          if (!selected) continue;
          var sp = info.querySelector('[class*="deliveryText"]');
          if (sp) { picked = text(sp); break; }
        }
        if (!picked && infos.length) {
          var sp2 = infos[0].querySelector('[class*="deliveryText"]');
          if (sp2) picked = text(sp2);
        }
        return picked || 'N/A';
      }
      var wrap = document.querySelector('.product-list-wrapper') ||
        document.querySelector('[class*="product-list-wrapper"]');
      var cards = [];
      if (wrap) {
        cards = wrap.querySelectorAll('.product-card-item');
        if (!cards.length) cards = wrap.querySelectorAll('[class*="ProductCard_container"]');
      }
      if (!cards.length) {
        cards = document.querySelectorAll('.product-card-item, [class*="ProductCard_container"]');
      }
      var out = [];
      for (var k = 0; k < cards.length; k++) {
        var card = cards[k];
        var a = card.querySelector('a[href*="partno-detail"]');
        var model = a ? text(a) : '';
        if (!model) continue;
        var manufacturer = findSpec(card, '品牌') || 'N/A';
        var pkg = findSpec(card, '封装') || 'N/A';
        if (pkg === '') pkg = 'N/A';
        var desc = findSpec(card, '描述') || 'N/A';
        if (desc === '') desc = 'N/A';
        var batch = stockField(card, '批次');
        if (batch) {
          if (desc !== 'N/A') desc = desc + ' | 批次:' + batch;
          else desc = '批次:' + batch;
        }
        var stock = stockField(card, '库存') || '0';
        stock = stock.replace(/\s/g, '');
        var moq = stockField(card, '起订量') || 'N/A';
        var prices = parsePrices(card);
        out.push({
          model: model,
          manufacturer: manufacturer,
          package: pkg,
          desc: desc,
          stock: stock,
          moq: moq,
          price_tiers: prices.price_tiers,
          hk_price: prices.hk_price,
          mainland_price: prices.mainland_price,
          lead_time: leadTime(card)
        });
      }
      return JSON.stringify(out);
    """
    try:
        raw = page.run_js(js)
        if raw is None:
            if not quiet:
                _ensure_icgoo_logging()
                _icgoo_log(logging.WARNING, "parse_product_cards_js: run_js 返回 None", echo_console=True)
            return []
        if isinstance(raw, str):
            s = raw.strip()
            if not s:
                return []
            try:
                parsed = json.loads(s)
            except json.JSONDecodeError as e:
                if not quiet:
                    _ensure_icgoo_logging()
                    _icgoo_log(
                        logging.WARNING,
                        f"parse_product_cards_js: JSON 解析失败 {e}，片段={s[:200]!r}",
                        echo_console=True,
                    )
                return []
            raw = parsed
        if not isinstance(raw, list):
            if not quiet:
                _ensure_icgoo_logging()
                _icgoo_log(
                    logging.WARNING,
                    f"parse_product_cards_js: 期望 list，实际 {type(raw).__name__}",
                    echo_console=True,
                )
            return []
        out: list[dict] = []
        for row in raw:
            if isinstance(row, dict):
                out.append(dict(row))
        if not out and not quiet:
            try:
                n_wrap = page.run_js(
                    "return document.querySelectorAll('.product-list-wrapper .product-card-item').length"
                )
                n_a = page.run_js('return document.querySelectorAll(\'a[href*="partno-detail"]\').length')
                _ensure_icgoo_logging()
                _icgoo_log(
                    logging.WARNING,
                    f"parse_product_cards_js: 解析 0 条；DOM 中 wrapper 下 .product-card-item={n_wrap}，"
                    f"partno-detail 链接数={n_a}",
                    echo_console=True,
                )
            except Exception:
                pass
        return out
    except Exception as e:
        if not quiet:
            _ensure_icgoo_logging()
            _icgoo_log(logging.WARNING, f"parse_product_cards_js: {e}", echo_console=True)
        return []


def parse_table_rows_js(page: ChromiumPage) -> list[list[str]]:
    """用 JS 抽取页面中最大表格的 tbody 行文本。"""
    js = """
      var tables = document.querySelectorAll('table');
      var bestRows = [];
      for (var i = 0; i < tables.length; i++) {
        var trs = tables[i].querySelectorAll('tbody tr');
        if (trs.length > bestRows.length) bestRows = trs;
      }
      if (bestRows.length === 0) {
        var alt = document.querySelectorAll('.el-table__body tbody tr');
        if (alt.length) bestRows = alt;
      }
      return Array.prototype.map.call(bestRows, function(tr){
        return Array.prototype.map.call(tr.querySelectorAll('td'), function(td){
          return (td.innerText || '').trim().replace(/\\s+/g, ' ');
        });
      });
    """
    try:
        raw = page.run_js(js)
        if not raw or not isinstance(raw, list):
            return []
        return [list(r) for r in raw if r]
    except Exception:
        return []


def cells_to_item(seq: int, cells: list[str], query_model: str) -> dict | None:
    if len(cells) < 3:
        return None
    joined = " ".join(cells)
    if "型号" in joined and ("品牌" in joined or "厂牌" in joined or "制造商" in joined):
        return None

    model = _pick_model(cells, query_model)
    manufacturer = "N/A"
    package = "N/A"
    stock_s = "0"
    lead_time = "N/A"
    price_cell = ""

    # 常见列序： [勾选/图] 型号 品牌 封装 库存 价格... 或 型号 品牌 ...
    for i, c in enumerate(cells):
        c = (c or "").strip()
        if not c:
            continue
        if "工作日" in c or "周" in c and "货" in c:
            lead_time = c
        if "￥" in c or "¥" in c or re.search(r"\d+\s*\+", c):
            price_cell = price_cell + " " + c if price_cell else c
        if re.match(r"^[\d,]+$", re.sub(r"\s", "", c)) and int(re.sub(r"\D", "", c) or 0) < 10**9:
            if len(c) <= 12 and "￥" not in c:
                stock_s = _digits_stock(c)

    if len(cells) >= 4:
        manufacturer = cells[2] if len(cells[2]) < 80 else manufacturer
    if len(cells) >= 5:
        maybe_pkg = cells[3]
        if re.search(r"(SOT|QFN|BGA|LQFP|TSSOP|SOP|DIP|LGA|QFP|SON|DFN)", maybe_pkg, re.I):
            package = maybe_pkg
        elif len(maybe_pkg) <= 24 and "￥" not in maybe_pkg:
            package = maybe_pkg

    mainland = _extract_prices_line(price_cell) if price_cell else _extract_prices_line(joined)

    return {
        "seq": seq,
        "model": model,
        "manufacturer": manufacturer,
        "package": package if package and package != "N/A" else "N/A",
        "desc": "N/A",
        "stock": stock_s,
        "moq": "N/A",
        "price_tiers": mainland,
        "hk_price": "N/A",
        "mainland_price": mainland,
        "lead_time": lead_time,
        "query_model": query_model,
    }


def parse_search_results(page: ChromiumPage, query_model: str, quiet: bool = False) -> list[dict]:
    # 新版：商品卡片列表（product-list-wrapper）
    card_rows = parse_product_cards_js(page, quiet=quiet)
    if card_rows:
        results: list[dict] = []
        seq = 0
        for it in card_rows:
            model = (it.get("model") or "").strip()
            if not model or model == "N/A":
                continue
            seq += 1
            it["seq"] = seq
            it["query_model"] = query_model
            results.append(it)
        if results:
            return results

    rows = parse_table_rows_js(page)
    if not rows and not quiet:
        try:
            trs = page.eles("css:.el-table__body tbody tr", timeout=2) or page.eles(
                "css:table tbody tr", timeout=2
            )
            for tr in trs or []:
                tds = tr.eles("tag:td", timeout=0.2)
                rows.append([(td.text or "").strip() for td in tds])
        except Exception:
            pass

    results = []
    seq = 0
    for cells in rows:
        item = cells_to_item(seq + 1, cells, query_model)
        if item and item.get("model") and item["model"] != "N/A":
            seq += 1
            item["seq"] = seq
            results.append(item)
    return results


def run_search(
    keyword: str,
    headless: bool = True,
    quiet: bool = False,
    query_model: str | None = None,
    page: ChromiumPage | None = None,
    username: str | None = None,
    password: str | None = None,
    login_url: str = DEFAULT_LOGIN_URL,
    skip_login: bool = False,
    captcha_wait_sec: float = 300.0,
    cookies_file: str | None = None,
    save_cookies_to: str | None = None,
    force_login: bool = False,
    no_cookies: bool = False,
    login_before_search: bool = False,
    proxy: str | None = None,
) -> list[dict]:
    resolved_cfg = _resolve_browser_proxy(proxy)
    own = page is None
    if own:
        page = setup_browser(headless=headless, proxy_cfg=resolved_cfg)
    qm = query_model if query_model is not None else keyword
    try:
        u, p = _get_credentials(username, password)
        cookies_path = (
            None
            if no_cookies
            else _default_cookies_file_path(cookies_file)
        )
        cookies_ok = False
        if cookies_path and not force_login and not skip_login:
            cookies_ok = load_icgoo_cookies_from_file(page, cookies_path)
            if cookies_ok:
                msg = "已加载 Cookie，跳过登录（减少登录页易盾触发）"
                _icgoo_log(logging.INFO, msg, echo_console=True)

        did_login = False
        # 旧行为：无可用 Cookie 时先打开登录页（易盾概率更高）；默认关闭，改在搜索页按需登录
        if (
            login_before_search
            and not cookies_ok
            and u
            and p
            and not skip_login
        ):
            login_icgoo(
                page,
                u,
                p,
                login_url=login_url,
                quiet=quiet,
                captcha_wait_sec=captcha_wait_sec,
            )
            did_login = True

        url = search_url(keyword)
        if not quiet:
            _icgoo_log(logging.INFO, f"打开: {url}", echo_console=True)
        page.get(url)
        time.sleep(2)

        if not skip_login and u and p and icgoo_suggests_not_logged_in(page):
            if not quiet:
                _icgoo_log(
                    logging.INFO,
                    "搜索页提示需登录，正在使用账号登录…",
                    echo_console=True,
                )
            login_icgoo(
                page,
                u,
                p,
                login_url=login_url,
                quiet=quiet,
                captcha_wait_sec=captcha_wait_sec,
                wait_captcha_after_login=False,
            )
            did_login = True
            page.get(url)
            time.sleep(2)
        elif not skip_login and icgoo_suggests_not_logged_in(page) and not (u and p):
            _icgoo_log(
                logging.WARNING,
                "站点要求登录但未设置 ICGOO_USERNAME/ICGOO_PASSWORD，搜索可能无数据",
                echo_console=True,
            )

        if did_login and save_cookies_to:
            save_icgoo_cookies_to_file(page, save_cookies_to, quiet=quiet)

        yidun_res = _maybe_auto_solve_yidun(page, quiet=quiet)
        if yidun_res is not None and yidun_res.auto_solved:
            # 自动通过后易盾 success 类 / 阿里云成功态或隐藏拼图可能晚半拍才写入，避免立刻误判需手动等待
            time.sleep(2)
        wait_for_manual_captcha_or_rate_limit(
            page, timeout_sec=captcha_wait_sec, quiet=quiet
        )
        if not wait_for_results(page, timeout=22, quiet=quiet):
            if not quiet:
                _icgoo_log(
                    logging.WARNING,
                    "等待结果超时，尝试继续解析当前 DOM",
                    echo_console=True,
                )
            time.sleep(3)
        else:
            time.sleep(1)
        return parse_search_results(page, qm, quiet=quiet)
    finally:
        if own:
            try:
                page.quit()
            except Exception:
                pass
            _cleanup_icgoo_proxy_extension(page)


def run_search_batch(
    models: list[str],
    headless: bool = True,
    quiet: bool = False,
    parse_workers: int = 1,
    username: str | None = None,
    password: str | None = None,
    login_url: str = DEFAULT_LOGIN_URL,
    captcha_wait_sec: float = 300.0,
    cookies_file: str | None = None,
    save_cookies_to: str | None = None,
    force_login: bool = False,
    no_cookies: bool = False,
    login_before_search: bool = False,
    request_delay_sec: float = 5.0,
    proxy: str | None = None,
) -> list[dict]:
    """串流依次搜索；同一浏览器内仅在需要时登录。多个型号之间可插入间隔以降低限流。"""
    if not models:
        return []
    if len(models) == 1:
        return run_search(
            models[0],
            headless=headless,
            quiet=quiet,
            query_model=models[0],
            username=username,
            password=password,
            login_url=login_url,
            captcha_wait_sec=captcha_wait_sec,
            cookies_file=cookies_file,
            save_cookies_to=save_cookies_to,
            force_login=force_login,
            no_cookies=no_cookies,
            login_before_search=login_before_search,
            proxy=proxy,
        )

    resolved_cfg = _resolve_browser_proxy(proxy)
    all_results: list[dict] = []
    page = setup_browser(headless=headless, proxy_cfg=resolved_cfg)
    try:
        for i, model in enumerate(models):
            if i > 0 and request_delay_sec > 0:
                if quiet:
                    _icgoo_log(
                        logging.INFO,
                        f"型号间隔 {request_delay_sec:.1f}s（降低频繁访问触发易盾/限流）",
                        echo_console=True,
                    )
                else:
                    _icgoo_log(
                        logging.INFO,
                        f"等待 {request_delay_sec:.1f}s 再搜下一型号…",
                        echo_console=True,
                    )
                time.sleep(request_delay_sec)
            chunk = run_search(
                model,
                headless=headless,
                quiet=quiet,
                query_model=model,
                page=page,
                username=username,
                password=password,
                login_url=login_url,
                skip_login=(i > 0),
                captcha_wait_sec=captcha_wait_sec,
                cookies_file=cookies_file,
                save_cookies_to=save_cookies_to if i == 0 else None,
                force_login=force_login,
                no_cookies=no_cookies,
                login_before_search=login_before_search,
                proxy=proxy,
            )
            all_results.extend(chunk)
    finally:
        try:
            page.quit()
        except Exception:
            pass
        _cleanup_icgoo_proxy_extension(page)
    return all_results


def main() -> None:
    parser = argparse.ArgumentParser(description="ICGOO 元器件搜索爬虫（JSON 输出与 ickey_crawler 对齐）")
    parser.add_argument("--model", "-m", type=str, help="搜索型号，逗号分隔多个")
    parser.add_argument(
        "--parse-workers",
        "-w",
        type=int,
        default=8,
        help="预留，与 ickey 对齐（当前未使用）",
    )
    parser.add_argument(
        "--user",
        type=str,
        default=None,
        help="登录账号（也可用环境变量 ICGOO_USERNAME）",
    )
    parser.add_argument(
        "--password",
        "-p",
        type=str,
        default=None,
        help="登录密码（也可用环境变量 ICGOO_PASSWORD）",
    )
    parser.add_argument(
        "--login-url",
        type=str,
        default=DEFAULT_LOGIN_URL,
        help=f"登录页 URL，默认 {DEFAULT_LOGIN_URL}",
    )
    parser.add_argument(
        "--no-headless",
        action="store_true",
        help="有界面浏览器，便于手动完成滑块/短信验证（易盾或阿里云，推荐遇验证码时使用）",
    )
    parser.add_argument(
        "--captcha-wait",
        type=float,
        default=300.0,
        metavar="SEC",
        help="出现验证码/频繁提示时最长等待秒数（默认 300）",
    )
    parser.add_argument(
        "--cookies-file",
        type=str,
        default=None,
        help="从 JSON 加载会话 Cookie，成功则跳过登录（也可用环境变量 ICGOO_COOKIES_FILE）",
    )
    parser.add_argument(
        "--save-cookies",
        type=str,
        nargs="?",
        const="__default__",
        default=None,
        metavar="PATH",
        help="登录成功后保存 icgoo.net Cookie 到 JSON；省略 PATH 时与 --cookies-file 或 icgoo_cookies.json",
    )
    parser.add_argument(
        "--force-login",
        action="store_true",
        help="忽略 Cookie 文件（不加载本地 JSON）；登录仍默认在搜索页提示需登录时再执行，除非加 --login-before-search",
    )
    parser.add_argument(
        "--login-before-search",
        action="store_true",
        help="无可用 Cookie 时先打开登录页再搜（旧行为，易盾概率更高）；默认为先搜再按需登录",
    )
    parser.add_argument(
        "--no-cookies",
        action="store_true",
        help="本次不读取 --cookies-file / ICGOO_COOKIES_FILE（可与环境 ICGOO_NO_COOKIES=1 等价）；有账号仍会登录",
    )
    parser.add_argument(
        "--request-delay",
        type=float,
        default=5.0,
        metavar="SEC",
        help="逗号分隔多型号时，两次搜索之间的间隔秒数，减轻「过于频繁」与易盾（默认 5，可加大）",
    )
    parser.add_argument(
        "--proxy",
        type=str,
        default=None,
        metavar="URL",
        help="Chromium HTTP 代理，如 http://127.0.0.1:7890；不设则用 ICGOO_PROXY / HTTPS_PROXY 等环境变量",
    )
    args = parser.parse_args()

    no_cookies_run = bool(args.no_cookies) or _icgoo_no_cookies_env()

    cookies_file = _default_cookies_file_path(args.cookies_file)
    save_to: str | None = None
    if args.save_cookies is not None:
        if args.save_cookies == "__default__":
            save_to = cookies_file or os.environ.get("ICGOO_COOKIES_FILE", "").strip() or "icgoo_cookies.json"
        else:
            save_to = args.save_cookies

    if args.model:
        try:
            models = [m.strip() for m in args.model.split(",") if m.strip()]
            if not models:
                raise ValueError("--model 不能为空")
            results = run_search_batch(
                models,
                headless=not args.no_headless,
                quiet=True,
                parse_workers=args.parse_workers,
                username=args.user,
                password=args.password,
                login_url=args.login_url or DEFAULT_LOGIN_URL,
                captcha_wait_sec=args.captcha_wait,
                cookies_file=args.cookies_file,
                save_cookies_to=save_to,
                force_login=args.force_login,
                no_cookies=no_cookies_run,
                login_before_search=args.login_before_search,
                request_delay_sec=max(0.0, args.request_delay),
                proxy=args.proxy,
            )
            emit_json_stdout(results)
        except Exception as e:
            emit_json_stderr_error(str(e))
            sys.exit(1)
        return

    # 交互调试
    kw = "LAN8720AI-CP-TR"
    _icgoo_log(logging.INFO, f"ICGOO 爬虫调试模式，关键词: {kw}", echo_console=True)
    rs = run_search(kw, headless=True, quiet=False)
    _icgoo_log(logging.INFO, json.dumps(rs, ensure_ascii=False, indent=2), echo_console=True)


if __name__ == "__main__":
    main()
