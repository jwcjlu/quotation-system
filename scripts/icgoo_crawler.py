"""
ICGOO (www.icgoo.net) 搜索页爬虫 - DrissionPage，与 ickey_crawler 输出格式对齐供 Go 调用

搜索 URL: https://www.icgoo.net/search/{型号}/1
站点为 SPA，需 Chromium 渲染后再解析表格；搜索数据通常需登录后可见。

自动登录:
  - 环境变量: ICGOO_USERNAME / ICGOO_PASSWORD（推荐，避免出现在命令行历史）
  - 或命令行: --user / --password

**减少易盾触发（推荐）**: 每次走登录页容易弹出网易易盾。可 **先人工登录一次** 后保存 Cookie，
之后用 ``--cookies-file`` 加载会话 **跳过登录**（仅打开搜索页，易盾出现概率低很多）::

  python icgoo_crawler.py --model XXX --no-headless --save-cookies icgoo_cookies.json
  # 登录并完成易盾后，下次：
  python icgoo_crawler.py --model XXX --cookies-file icgoo_cookies.json

环境变量 ``ICGOO_COOKIES_FILE`` 可指定默认 Cookie 文件路径。

**刷新/多次打开页面也会触发易盾**：站点对「访问 icgoo.net」本身做风控，并非只有登录页。
脚本已尽量 **不在加载 Cookie 时先打开首页**（改为空白页注入 Cookie，再直达搜索 URL），
并支持 **批量型号之间的请求间隔**（``--request-delay``）以降低「过于频繁」概率；仍无法完全消除对方策略。

CLI:
  set ICGOO_USERNAME=xxx
  set ICGOO_PASSWORD=xxx
  python icgoo_crawler.py --model LAN8720AI-CP-TR

若站点弹出验证码/滑块，无头模式可能失败，请使用 ``--no-headless`` 在可见窗口中手动完成
易盾滑块/短信验证；脚本会轮询直到验证消失或出现 ``.yidun--success``。

「您的搜索过于频繁」为服务端限流，需降低请求频率、换网络/账号，或隔一段时间再试；
无法通过纯代码绕过，亦不建议对接打码平台（合规与稳定性风险）。

依赖: pip install DrissionPage
"""

from __future__ import annotations

import argparse
import json
import os
import re
import sys
import time
from urllib.parse import quote

from DrissionPage import ChromiumPage, ChromiumOptions

try:
    from crawler_cli import emit_json_stderr_error, emit_json_stdout
except ImportError:
    _root = os.path.dirname(os.path.abspath(__file__))
    if _root not in sys.path:
        sys.path.insert(0, _root)
    from crawler_cli import emit_json_stderr_error, emit_json_stdout

# 登录页（SPA，需等待 JS 渲染出表单）
DEFAULT_LOGIN_URL = "https://www.icgoo.net/login"
# 加载 Cookie 时优先用空白页，避免多一次打开 icgoo 首页触发易盾（失败再回退首页）
ICGOO_ORIGIN = "https://www.icgoo.net/"
_BLANK_PAGE = "about:blank"


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
            if not quiet:
                print("未找到 icgoo.net 相关 Cookie，跳过保存", flush=True)
            return False
        parent = os.path.dirname(os.path.abspath(path))
        if parent and not os.path.isdir(parent):
            os.makedirs(parent, exist_ok=True)
        with open(path, "w", encoding="utf-8") as f:
            json.dump(filtered, f, ensure_ascii=False, indent=2)
        if not quiet:
            print(f"已保存 {len(filtered)} 条 Cookie 到: {path}", flush=True)
        return True
    except OSError as e:
        if not quiet:
            print(f"保存 Cookie 失败: {e}", flush=True)
        return False


def _has_icgoo_yidun_or_frequent_block(page: ChromiumPage) -> bool:
    """是否出现网易易盾验证码或「搜索过于频繁」容器（#slide-frequently / .yidun）。"""
    try:
        html = page.html or ""
    except Exception:
        html = ""
    if "您的搜索过于频繁" in html or "slide-frequently" in html:
        return True
    for sel in ("css:#slide-frequently", "css:.frequently-text", "css:.yidun", "css:.nc-container"):
        try:
            el = page.ele(sel, timeout=0.4)
            if el:
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
        "检测到易盾验证码或「搜索过于频繁」提示：请在浏览器窗口内完成滑块/短信验证；"
        "完成后脚本会自动继续…"
    )
    if quiet:
        # JSON 模式勿写 stdout
        print(msg, file=sys.stderr, flush=True)
        print(
            f"（最长等待 {timeout_sec:.0f} 秒；请加 --no-headless 以便操作浏览器）",
            file=sys.stderr,
            flush=True,
        )
    else:
        print(msg, flush=True)
        print(f"（最长等待 {timeout_sec:.0f} 秒，可按 Ctrl+C 中断）", flush=True)
    deadline = time.time() + timeout_sec
    while time.time() < deadline:
        time.sleep(poll_interval)
        # 易盾成功态
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
    if quiet:
        print(err, file=sys.stderr, flush=True)
    else:
        print(err, flush=True)
    return False


def login_icgoo(
    page: ChromiumPage,
    username: str,
    password: str,
    login_url: str = DEFAULT_LOGIN_URL,
    quiet: bool = False,
    captcha_wait_sec: float = 300.0,
) -> None:
    """
    打开登录页并提交账号密码。站点为 Vue/Element UI，选择器多路兜底。
    """
    if not quiet:
        print(f"登录页: {login_url}", flush=True)
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
        print(f"登录后 URL: {page.url}", flush=True)

    wait_for_manual_captcha_or_rate_limit(
        page, timeout_sec=captcha_wait_sec, quiet=quiet
    )


def setup_browser(headless: bool = True) -> ChromiumPage:
    co = ChromiumOptions()
    co.auto_port()
    co.set_user_agent(
        "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 "
        "(KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
    )
    co.headless(headless)
    co.set_argument("--remote-allow-origins=*")
    if headless:
        co.set_argument("--headless=new")
    co.set_argument("--window-size=1400,900")
    co.set_argument("--blink-settings=imagesEnabled=false")
    return ChromiumPage(co)


def search_url(keyword: str) -> str:
    # 路径中型号做百分号编码，斜杠等需编码；t= 为当前毫秒时间戳（与站点防缓存参数一致）
    ts = int(time.time() * 1000)
    return f"https://www.icgoo.net/search/{quote(keyword, safe='')}/1?t={ts}"


def wait_for_results(page: ChromiumPage, timeout: float = 25.0, quiet: bool = False) -> bool:
    """等待列表区域出现（多选择器兜底）。"""
    selectors = [
        "css:.el-table__body tbody tr",
        "css:table tbody tr",
        "xpath://table//tbody//tr[td]",
        "css:[class*='table'] tbody tr",
    ]
    deadline = time.time() + timeout
    while time.time() < deadline:
        for sel in selectors:
            try:
                rows = page.eles(sel, timeout=1)
                if rows and len(rows) >= 1:
                    # 排除只有表头的一行情况：至少一行有多个单元格
                    for r in rows[:3]:
                        try:
                            tds = r.eles("tag:td", timeout=0.2)
                            if len(tds) >= 3:
                                if not quiet:
                                    pass
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


def parse_table_rows_js(page: ChromiumPage) -> list[list[str]]:
    """用 JS 抽取页面中最大表格的 tbody 行文本。"""
    js = """
    (function(){
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
    })();
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
    rows = parse_table_rows_js(page)
    if not rows and not quiet:
        # 再试 DrissionPage 直接取行
        try:
            trs = page.eles("css:.el-table__body tbody tr", timeout=2) or page.eles(
                "css:table tbody tr", timeout=2
            )
            for tr in trs or []:
                tds = tr.eles("tag:td", timeout=0.2)
                rows.append([(td.text or "").strip() for td in tds])
        except Exception:
            pass

    results: list[dict] = []
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
    headless: bool = False,
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
) -> list[dict]:
    own = page is None
    if own:
        page = setup_browser(headless=headless)
    qm = query_model if query_model is not None else keyword
    try:
        u, p = _get_credentials(username, password)
        cookies_path = _default_cookies_file_path(cookies_file)
        cookies_ok = False
        if cookies_path and not force_login and not skip_login:
            cookies_ok = load_icgoo_cookies_from_file(page, cookies_path)
            if cookies_ok:
                msg = "已加载 Cookie，跳过登录（减少登录页易盾触发）"
                if quiet:
                    print(msg, file=sys.stderr, flush=True)
                else:
                    print(msg, flush=True)

        did_login = False
        if not cookies_ok and u and p and not skip_login:
            login_icgoo(
                page,
                u,
                p,
                login_url=login_url,
                quiet=quiet,
                captcha_wait_sec=captcha_wait_sec,
            )
            did_login = True
        elif not cookies_ok and not quiet and not (u and p) and not skip_login:
            print("未设置 ICGOO_USERNAME/ICGOO_PASSWORD，跳过登录（搜索可能无数据）", flush=True)

        if did_login and save_cookies_to:
            save_icgoo_cookies_to_file(page, save_cookies_to, quiet=quiet)

        url = search_url(keyword)
        if not quiet:
            print(f"打开: {url}", flush=True)
        page.get(url)
        time.sleep(2)
        wait_for_manual_captcha_or_rate_limit(
            page, timeout_sec=captcha_wait_sec, quiet=quiet
        )
        if not wait_for_results(page, timeout=22, quiet=quiet):
            if not quiet:
                print("等待结果超时，尝试继续解析当前 DOM", flush=True)
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
    request_delay_sec: float = 5.0,
) -> list[dict]:
    """串流依次搜索；仅第一次打开浏览器时执行登录。多个型号之间可插入间隔以降低限流。"""
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
        )

    all_results: list[dict] = []
    page = setup_browser(headless=headless)
    try:
        for i, model in enumerate(models):
            if i > 0 and request_delay_sec > 0:
                if quiet:
                    print(
                        f"型号间隔 {request_delay_sec:.1f}s（降低频繁访问触发易盾/限流）",
                        file=sys.stderr,
                        flush=True,
                    )
                else:
                    print(
                        f"等待 {request_delay_sec:.1f}s 再搜下一型号…",
                        flush=True,
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
            )
            all_results.extend(chunk)
    finally:
        try:
            page.quit()
        except Exception:
            pass
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
        help="有界面浏览器，便于手动完成易盾滑块/短信验证（推荐遇验证码时使用）",
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
        help="忽略 Cookie 文件，始终走账号密码登录（用于刷新会话）",
    )
    parser.add_argument(
        "--request-delay",
        type=float,
        default=5.0,
        metavar="SEC",
        help="逗号分隔多型号时，两次搜索之间的间隔秒数，减轻「过于频繁」与易盾（默认 5，可加大）",
    )
    args = parser.parse_args()

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
                request_delay_sec=max(0.0, args.request_delay),
            )
            emit_json_stdout(results)
        except Exception as e:
            emit_json_stderr_error(str(e))
            sys.exit(1)
        return

    # 交互调试
    kw = "LAN8720AI-CP-TR"
    print("ICGOO 爬虫调试模式，关键词:", kw)
    rs = run_search(kw, headless=True, quiet=False)
    print(json.dumps(rs, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
