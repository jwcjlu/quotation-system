"""
立创商城搜索页爬虫（DrissionPage）。

CLI 模式（与 ickey_crawler.py 一致，供 Go / Agent 调用）:
  python szlcsc_crawler.py --model LAN8720AI-CP-TR
  python szlcsc_crawler.py -m A,B,C   # 多型号逗号分隔，串流依次搜索
  成功：stdout 仅输出 JSON 数组（UTF-8）；失败：stderr 输出 {"error":"...","results":[]}

**解析方式：仅 DOM** — 不从 ``<script id="__NEXT_DATA__">`` / soData 等内嵌 JSON 取数。
新版列表为主卡片 ``section[data-spm="zy"]``（或同结构），内含 ``span.LUCENE_HIGHLIGHT_CLASS`` 型号、
``dl`` 键值（品牌/封装/编号/现货等）、``item.szlcsc.com`` 商品链；旧版仍支持「供应商编号」与 ``/product/`` 链。

自动登录（可选，与 icgoo 用法一致）:
  - 环境变量: ``SZLCSC_USERNAME`` / ``SZLCSC_PASSWORD``（推荐）
  - 或命令行: ``--user`` / ``--password``
  - 登录页默认: https://passport.szlcsc.com/ （嘉立创统一登录，可用 ``--login-url`` 覆盖）

若出现验证码/滑块，无头模式可能失败，可先去掉无头或人工登录一次。
"""
from DrissionPage import ChromiumPage, ChromiumOptions
from datetime import datetime
import argparse
import re
import sys
import time
import json
import os
from urllib.parse import quote

try:
    from crawler_cli import UNIFIED_RESULT_KEYS
except ImportError:
    _root = os.path.dirname(os.path.abspath(__file__))
    if _root not in sys.path:
        sys.path.insert(0, _root)
    from crawler_cli import UNIFIED_RESULT_KEYS

# 不参与商品行的 data-spm 值（如页脚 ft）；若误过滤可再缩小集合
_SPM_SKIP = frozenset({"ft"})

# 嘉立创/立创统一登录中心（实际以页面为准，可 --login-url 覆盖）
DEFAULT_LOGIN_URL = "https://passport.szlcsc.com/"


def lcsc_search_url(keyword: str) -> str:
    return f"https://so.szlcsc.com/global.html?k={quote(keyword, safe='')}"


def _sz_field(v: str | None) -> str:
    if not v or (isinstance(v, str) and v.strip() == "未提取到"):
        return "N/A"
    return str(v).strip()


def _normalize_stock_for_ickey_json(stock_txt: str) -> str:
    """尽量输出纯数字，对齐 ickey data-stock，便于 Go strconv.ParseInt。"""
    s = (stock_txt or "").strip().replace("\n", " ")
    if not s or s.upper() in ("N/A", "—", "-", "未提取到"):
        return "0"
    if re.fullmatch(r"\d+", s):
        return s
    m = re.search(r"([\d][\d,\s]*)", s.replace("，", ","))
    if m:
        digits = re.sub(r"[^\d]", "", m.group(1))
        if digits:
            return digits
    return "0"


def _moq_from_price_gradient_str(price_str: str) -> str:
    """从梯度串中取最小起订量（如 1+、10+）。"""
    ps = (price_str or "").strip()
    if not ps or ps == "N/A":
        return "N/A"
    mins: list[int] = []
    for m in re.finditer(r"(\d+)\s*\+", ps):
        try:
            mins.append(int(m.group(1)))
        except ValueError:
            continue
    if mins:
        return str(min(mins))
    m = re.search(r"(?:^|\|)\s*(\d+)\s+(?:[￥¥$]|\d)", ps)
    if m:
        return m.group(1)
    return "N/A"


def _normalize_moq_field(moq_txt: str) -> str:
    """起订量字段：纯数字或从文本抽数字，否则 N/A。"""
    s = (moq_txt or "").strip()
    if not s or s == "N/A" or s == "未提取到":
        return "N/A"
    if re.fullmatch(r"\d+", s):
        return s
    m = re.search(r"\d+", s)
    return m.group(0) if m else "N/A"


def lcsc_product_to_unified(p: dict, seq: int, query_model: str) -> dict:
    """build_product_from_lcsc_row 结果 → 与 ickey_crawler.py 完全一致键名与顺序。"""
    raw_pt = (p.get("价格梯度") or "").strip()
    pt = _sz_field(p.get("价格梯度"))
    if pt == "N/A":
        pt = "N/A"

    st_disp = _sz_field(p.get("库存"))
    stock = "0" if st_disp == "N/A" else _normalize_stock_for_ickey_json(st_disp)

    moq = _normalize_moq_field(_sz_field(p.get("起订量")))
    if moq == "N/A":
        moq = _moq_from_price_gradient_str(raw_pt)

    desc = _sz_field(p.get("描述"))
    if desc != "N/A" and len(desc) > 200:
        desc = desc[:200]

    row = {
        "seq": seq,
        "model": _sz_field(p.get("型号")),
        "manufacturer": _sz_field(p.get("品牌")),
        "package": _sz_field(p.get("封装")),
        "desc": desc,
        "stock": stock,
        "moq": moq,
        "price_tiers": pt,
        "hk_price": "N/A",
        "mainland_price": pt,
        "lead_time": "N/A",
        "query_model": query_model,
    }
    return {k: row[k] for k in UNIFIED_RESULT_KEYS}


def _get_credentials(
    user: str | None, password: str | None
) -> tuple[str | None, str | None]:
    u = (user or os.environ.get("SZLCSC_USERNAME") or "").strip() or None
    p = (password or os.environ.get("SZLCSC_PASSWORD") or "").strip() or None
    return u, p


def login_szlcsc(
    page: ChromiumPage,
    username: str,
    password: str,
    login_url: str = DEFAULT_LOGIN_URL,
    quiet: bool = False,
) -> None:
    """
    打开立创/嘉立创统一登录页并填写账号密码（手机号或邮箱 + 密码）。
    选择器多路兜底，适配常见 SPA 表单。
    """
    if not quiet:
        print(f"登录页: {login_url}", flush=True)
    page.get(login_url)
    time.sleep(2.5)

    try:
        page.wait.ele_displayed("css:input[type=password]", timeout=25)
    except Exception as e:
        raise RuntimeError(
            "登录页未出现密码输入框（网络异常、URL 变更或需验证码）"
        ) from e

    time.sleep(0.5)
    pwd_el = page.ele("css:input[type=password]", timeout=5)
    user_el = None
    for xpath in (
        'xpath://input[@type="password"]/preceding::input[(@type="text" or @type="tel")][1]',
        'xpath://input[@type="password"]/ancestor::form//input[@type="text" or @type="tel"][1]',
        "css:input.el-input__inner",
        'xpath://input[contains(@placeholder,"手机") or contains(@placeholder,"账号") or contains(@placeholder,"邮箱")]',
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
        raise RuntimeError("未找到账号/手机/邮箱输入框")

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
    time.sleep(0.35)

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
        try:
            pwd_el.input("\n")
        except Exception as e:
            raise RuntimeError("未找到登录按钮") from e

    time.sleep(2)
    deadline = time.time() + 18
    while time.time() < deadline:
        u = page.url or ""
        if u and "passport" not in u.lower() and "login" not in u.lower():
            break
        time.sleep(0.5)
    time.sleep(1)
    if not quiet:
        print(f"登录后 URL: {page.url}", flush=True)


def setup_browser(headless: bool = False) -> ChromiumPage:
    """与 icgoo/ickey 类似：稳定 UA、无头、禁图。"""
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


def save_page_html(page, prefix: str = "szlcsc_page", out_dir: str | None = None) -> str:
    """
    将当前页面完整 HTML 保存为 UTF-8 文件，便于离线查看与后续用 BeautifulSoup/正则分析。
    返回保存路径。
    """
    ts = datetime.now().strftime("%Y%m%d_%H%M%S")
    name = f"{prefix}_{ts}.html"
    path = os.path.join(out_dir, name) if out_dir else name
    if out_dir:
        os.makedirs(out_dir, exist_ok=True)
    html = page.html or ""
    with open(path, "w", encoding="utf-8") as f:
        f.write(html)
    return path


def extract_dl_fields(item) -> dict[str, str]:
    """
    从单条商品根节点解析 <dl><dt>键</dt>…<dd>值</dd></dl>（中间可有 Colon 占位 span）。
    另解析「描述」所在 <p>（非 dl 结构）。
    """
    fields: dict[str, str] = {}
    try:
        dls = item.eles("tag:dl", timeout=2)
    except Exception:
        dls = []
    for dl in dls or []:
        try:
            dt = dl.ele("tag:dt", timeout=0.3)
            dd = dl.ele("tag:dd", timeout=0.3)
            if not dt or not dd:
                continue
            key = (dt.text or "").strip()
            tit_dt = (dt.attr("title") or "").strip()
            if tit_dt and len(tit_dt) > len(key):
                key = tit_dt
            val = (dd.text or "").strip()
            tit_dd = (dd.attr("title") or "").strip()
            if tit_dd and len(tit_dd) > len(val):
                val = tit_dd
            if key:
                fields[key] = val
        except Exception:
            continue

    # 描述：<p>…<span>描述</span>…<span class="…text-[#333333]…" title="长文">…</span>
    if "描述" not in fields:
        try:
            sp = item.ele(
                'xpath:.//p[.//span[normalize-space()="描述"]]'
                '//span[contains(@class,"333333")][@title!="" or normalize-space()!=""]',
                timeout=0.6,
            )
            if sp:
                t = (sp.attr("title") or "").strip() or (sp.text or "").strip()
                if t and t != "描述":
                    fields["描述"] = t
        except Exception:
            pass

    return fields


def _parse_first_data_custom_data(item) -> dict | None:
    """从 a[data-custom-data] 取 JSON（含 productCode）。"""
    if not item:
        return None
    try:
        for a in item.eles("css:a[data-custom-data]", timeout=0.4) or []:
            raw = (a.attr("data-custom-data") or "").strip()
            if not raw:
                continue
            try:
                return json.loads(raw)
            except Exception:
                continue
    except Exception:
        pass
    return None


def _item_page_id_from_href(href: str) -> str:
    m = re.search(r"item\.szlcsc\.com/(\d+)\.html", href or "", re.I)
    return m.group(1) if m else ""


def _first_product_like_anchor(el):
    """卡片内第一条更像「商品详情」的 <a>（顺序：立创 item 站 > /product/ > href 含 product）。"""
    if not el:
        return None
    for xp, tmo in (
        ('xpath:.//a[contains(@href, "item.szlcsc.com")]', 0.45),
        ('xpath:.//a[contains(@href, "/product/")]', 0.35),
        ('xpath:.//a[contains(@href, "product")]', 0.35),
    ):
        try:
            a = el.ele(xp, timeout=tmo)
            if a:
                return a
        except Exception:
            continue
    return None


def _dedup_key_for_product_root(el, fields: dict[str, str]) -> str:
    """
    列表收集阶段：为「同一张商品卡片」生成稳定键，避免 XPath 命中重复节点时输出多份。

    优先级（先命中先用，前缀仅便于排查）:
    1. sup:{供应商编号}     — 旧版列表 dl 里的供应商编号（若存在通常唯一）。
    2. code:{C 码}         — dl「编号」，若无则读 a[data-custom-data] 的 productCode（新版常见）。
    3. item:{数字 id}      — item.szlcsc.com/{id}.html 的路径 id，与商品详情页一一对应。
    4. href:{无 query 路径} — 上面解析不出数字 id 时，用去掉 ? 后的链接路径兜底。

    若仍得空串，可配合高亮型号判断是否为有效商品行（见 collect_product_roots_under_data_spm）。

    说明：此键表示「商品身份」，**不含价格**；若按此键对列表去重，会把同料号不同价的行压成一条。
    当前收集逻辑**不再**用该键去重，仅用于判断是否具备可解析身份（与高亮二选一）。
    """
    sup = (fields.get("供应商编号") or "").strip()
    if sup:
        return f"sup:{sup}"

    code = (fields.get("编号") or "").strip()
    if not code:
        custom = _parse_first_data_custom_data(el) or {}
        if isinstance(custom, dict):
            code = (custom.get("productCode") or "").strip()
    if code:
        return f"code:{code}"

    try:
        link = _first_product_like_anchor(el)
        if link:
            href = (link.attr("href") or "").strip().split("?")[0]
            if href:
                pid = _item_page_id_from_href(href)
                if pid:
                    return f"item:{pid}"
                return f"href:{href}"
    except Exception:
        pass
    return ""


def _warehouse_subdict(fields: dict[str, str]) -> dict[str, str]:
    """各 *仓 库存（如 广东仓）。"""
    return {k: v for k, v in fields.items() if k.endswith("仓")}


def _price_tiers_from_block_text(block_text: str) -> str:
    if not block_text:
        return "未提取到"
    parts = []
    for m in re.finditer(r"(\d+\s*\+?\s*[^￥\s]*[￥¥]\s*[\d.]+)", block_text):
        parts.append(m.group(1).strip().replace("\n", " "))
    if parts:
        return " | ".join(parts[:30])
    return "未提取到"


def _model_from_lucene_highlight(item) -> str:
    """搜索高亮里的完整型号，如 <span class=\"LUCENE_HIGHLIGHT_CLASS\">LAN8720AI-CP-TR</span>。"""
    if not item:
        return ""
    for sel in (
        "css:span.LUCENE_HIGHLIGHT_CLASS",
        'xpath:.//span[contains(@class,"LUCENE_HIGHLIGHT_CLASS")]',
    ):
        try:
            for sp in item.eles(sel, timeout=0.35) or []:
                t = (sp.text or "").strip()
                if t:
                    return t
            el = item.ele(sel, timeout=0.2)
            if el:
                t = (el.text or "").strip()
                if t:
                    return t
        except Exception:
            continue
    return ""


def _packaging_moq_line_from_item(item) -> str:
    """如「3000个/圆盘」类文案。"""
    if not item:
        return ""
    try:
        for p in item.eles("tag:p", timeout=0.5) or []:
            t = re.sub(r"\s+", "", (p.text or "").strip())
            if re.search(r"^\d+.*个.*/", t):
                return (p.text or "").strip()
    except Exception:
        pass
    return ""


def build_product_from_lcsc_row(item, fields: dict[str, str], data_spm: str) -> dict:
    """按样本 HTML：高亮型号 → 链接型号 → dt 编号/供应商编号。"""
    try:
        block_text = (item.text or "") if item else ""
    except Exception:
        block_text = ""

    custom = _parse_first_data_custom_data(item) or {}
    pcode = (custom.get("productCode") or "").strip() if isinstance(custom, dict) else ""

    model = _model_from_lucene_highlight(item) or "未提取到"
    if model == "未提取到":
        try:
            for xp in (
                'xpath:.//a[contains(@href, "item.szlcsc.com")]',
                'xpath:.//a[contains(@href, "/product/")]',
                'xpath:.//a[contains(@href, "product")]',
                'xpath:.//a[contains(@href, "szlcsc.com")]',
            ):
                a = item.ele(xp, timeout=0.4)
                if a:
                    t = (a.text or "").strip()
                    if t:
                        model = t
                        break
        except Exception:
            pass

    if model == "未提取到" and fields.get("编号"):
        model = fields["编号"].strip()
    if model == "未提取到" and fields.get("供应商编号"):
        sup = fields["供应商编号"].strip()
        if "-" in sup:
            model = sup.split("-", 1)[-1].strip()
        else:
            model = sup

    supplier_no = (fields.get("供应商编号") or "").strip()
    code = (fields.get("编号") or pcode or supplier_no or "").strip() or "未提取到"
    if supplier_no == "":
        supplier_no = code if code != "未提取到" else ""

    pack_line = _packaging_moq_line_from_item(item)
    moq_field = fields.get("最小包装") or fields.get("增量") or pack_line or "未提取到"

    product: dict = {
        "data_spm": data_spm,
        "型号": model,
        "编号": code,
        "供应商编号": supplier_no or "未提取到",
        "品牌": fields.get("品牌", "未提取到"),
        "封装": fields.get("封装", "未提取到"),
        "描述": fields.get("描述", "未提取到"),
        "库存": fields.get("库存") or fields.get("现货") or "未提取到",
        "参数": fields.get("参数", "未提取到"),
        "批次": fields.get("批次", "未提取到"),
        "类目": fields.get("类目", "未提取到"),
        "最小包装": fields.get("最小包装", "未提取到"),
        "增量": fields.get("增量", "未提取到"),
        "产地": fields.get("产地", "未提取到"),
        "包装": fields.get("包装", "未提取到"),
        "价格梯度": _price_tiers_from_block_text(block_text),
        "起订量": moq_field,
    }
    wh = _warehouse_subdict(fields)
    if wh:
        product["区域仓库存"] = wh
    # 保留完整 dl 字段，便于对照样本页调试
    product["dl字段"] = fields
    return product


def collect_product_roots_under_data_spm(page) -> list:
    """
    选取商品卡片根节点：优先新版 section + LUCENE 高亮；其次旧版供应商编号 / 商品链。

    不按「商品身份」去重：同一 C 码 / 同一 item 若页面有多行（例如价格梯度、渠道不同），会各输出一条。
    仅对「策略 XPath 重复命中同一 DOM 节点」用 id(el) 去重，避免同一张卡片进列表两次。
    """
    strategies = (
        'xpath://section[@data-spm][.//span[contains(@class,"LUCENE_HIGHLIGHT_CLASS")]]',
        'xpath://*[contains(@class,"MainCard")][@data-spm][.//span[contains(@class,"LUCENE_HIGHLIGHT_CLASS")]]',
        'xpath://*[@data-spm][.//dl[.//dt[contains(., "供应商编号")]]]',
        'xpath://*[@data-spm][.//a[contains(@href, "item.szlcsc.com")]]',
        'xpath://*[@data-spm][.//a[contains(@href, "/product/")]]',
        'xpath://*[@data-spm][.//a[contains(@href, "product")]]',
    )

    candidates: list = []
    seen_el: set[int] = set()
    for xp in strategies:
        try:
            found = page.eles(xp, timeout=4) or []
        except Exception:
            found = []
        if not found:
            continue
        for el in found:
            eid = id(el)
            if eid in seen_el:
                continue
            seen_el.add(eid)
            candidates.append(el)
        break

    roots: list = []
    for el in candidates:
        try:
            spm = (el.attr("data-spm") or "").strip()
            if spm in _SPM_SKIP:
                continue

            fields = extract_dl_fields(el)
            hl = _model_from_lucene_highlight(el)
            if not hl:
                if not fields.get("供应商编号") and not fields.get("编号"):
                    if not el.ele(
                        'xpath:.//a[contains(@href, "item.szlcsc.com")]', timeout=0.35
                    ):
                        continue

            identity = _dedup_key_for_product_root(el, fields)
            if not identity and not hl:
                continue
            roots.append(el)
        except Exception:
            continue
    return roots


def parse_lcsc_search_page_dom(page: ChromiumPage, quiet: bool = False) -> list[dict]:
    """
    从当前立创搜索结果页用 DOM 解析商品（data-spm 根节点 + dl 键值 + 高亮/链接型号）。
    唯一解析路径；不读取 __NEXT_DATA__。
    """
    product_items = collect_product_roots_under_data_spm(page)
    if not product_items:
        product_items = page.eles('xpath://div[contains(@class, "product-item")]')
    if not product_items:
        product_items = page.eles('xpath://tr[contains(@class, "product")]')

    if not product_items:
        if not quiet:
            print("未找到产品项，请检查选择器", flush=True)
            try:
                print(f"页面标题: {page.title}", flush=True)
            except Exception:
                print("页面标题: (无法获取)", flush=True)
        return []

    products: list[dict] = []
    for idx, item in enumerate(product_items, 1):
        try:
            spm = (item.attr("data-spm") or "").strip()
            fields = extract_dl_fields(item)
            if not fields and not _model_from_lucene_highlight(item):
                if not quiet:
                    print(f"第 {idx} 条无 dl 且无型号高亮，跳过（可对照已保存 HTML）", flush=True)
                continue
            product = build_product_from_lcsc_row(item, fields, spm)
            products.append(product)
            if not quiet:
                print(f"成功提取第 {idx} 个产品: {product.get('型号', 'Unknown')}", flush=True)
        except Exception as e:
            if not quiet:
                print(f"提取第 {idx} 个产品时出错: {e}", flush=True)
            continue
    return products


def fetch_lcsc_product_data(
    url: str | None = None,
    username: str | None = None,
    password: str | None = None,
    login_url: str = DEFAULT_LOGIN_URL,
    skip_login: bool = False,
    headless: bool = False,
    quiet: bool = False,
    page: ChromiumPage | None = None,
    keyword: str | None = None,
):
    """
    获取立创商城搜索结果页产品数据（build_product_from_lcsc_row 原始结构）。
    仅通过 DOM 解析（parse_lcsc_search_page_dom），不使用内嵌 JSON。
    若提供 keyword 则自动构造搜索 URL；否则使用 url。
    quiet: True 时不 print、不落盘 HTML（CLI JSON 模式）。
    page: 传入则复用浏览器（多型号串流）。
    """
    if keyword:
        url = lcsc_search_url(keyword)
    if not url:
        raise ValueError("需要 url 或 keyword")

    own_page = page is None
    if own_page:
        page = setup_browser(headless=headless)

    try:
        u, p = _get_credentials(username, password)
        if u and p and not skip_login:
            login_szlcsc(page, u, p, login_url=login_url, quiet=quiet)
        elif (not u or not p) and not quiet:
            print("未设置 SZLCSC_USERNAME/SZLCSC_PASSWORD，跳过登录", flush=True)

        if not quiet:
            print("正在访问页面...", flush=True)
        page.get(url)

        # 等待带 data-spm 的列表区域（立创 SPM 埋点，商品块在此属性节点下）
        try:
            page.wait.ele_displayed("css:[data-spm]", timeout=15)
        except Exception:
            try:
                page.wait.ele_displayed(".product-list", timeout=8)
            except Exception:
                page.wait.ele_displayed('[class*="product-item"]', timeout=8)

        time.sleep(2)

        if not quiet:
            saved_path = save_page_html(page, prefix="szlcsc_page")
            print(f"页面已保存: {saved_path}（请先对照此文件调整选择器）")

        return parse_lcsc_search_page_dom(page, quiet=quiet)

    except Exception as e:
        if not quiet:
            print(f"访问或解析页面时发生错误: {e}")
        return []
    finally:
        if own_page:
            page.quit()


def run_search(
    keyword: str,
    headless: bool = True,
    quiet: bool = False,
    query_model: str | None = None,
    username: str | None = None,
    password: str | None = None,
    login_url: str = DEFAULT_LOGIN_URL,
    skip_login: bool = False,
) -> list[dict]:
    qm = query_model if query_model is not None else keyword
    raw = fetch_lcsc_product_data(
        keyword=keyword,
        username=username,
        password=password,
        login_url=login_url,
        skip_login=skip_login,
        headless=headless,
        quiet=quiet,
        page=None,
    )
    return [lcsc_product_to_unified(p, i + 1, qm) for i, p in enumerate(raw)]


def run_search_batch(
    models: list[str],
    headless: bool = True,
    quiet: bool = False,
    parse_workers: int = 8,
    username: str | None = None,
    password: str | None = None,
    login_url: str = DEFAULT_LOGIN_URL,
    skip_login: bool = False,
) -> list[dict]:
    """多型号串流搜索；parse_workers 与 ickey 参数兼容（本脚本以串行解析为主）。"""
    _ = parse_workers
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
            skip_login=skip_login,
        )

    all_results: list[dict] = []
    page = setup_browser(headless=headless)
    try:
        for i, model in enumerate(models):
            # 仅首次按 skip_login 决定是否登录；后续会话语境已建立
            sk = skip_login if i == 0 else True
            raw = fetch_lcsc_product_data(
                keyword=model,
                username=username,
                password=password,
                login_url=login_url,
                skip_login=sk,
                headless=headless,
                quiet=quiet,
                page=page,
            )
            for j, prod in enumerate(raw):
                all_results.append(lcsc_product_to_unified(prod, j + 1, model))
    finally:
        try:
            page.quit()
        except Exception:
            pass
    return all_results

def main():
    parser = argparse.ArgumentParser(
        description=(
            "立创商城搜索页爬虫（CLI JSON 与 ickey_crawler.py 对齐，可选自动登录）。"
            "商品数据仅通过 DOM（data-spm + dl）解析，不使用 __NEXT_DATA__/soData。"
        )
    )
    parser.add_argument(
        "--model",
        "-m",
        type=str,
        help="搜索型号，逗号分隔可传多个，串流依次搜索",
    )
    parser.add_argument(
        "--parse-workers",
        "-w",
        type=int,
        default=8,
        help="与 ickey 一致；当前以串行解析为主，参数保留兼容",
    )
    parser.add_argument(
        "--url",
        "-u",
        type=str,
        default="https://so.szlcsc.com/global.html?k=LAN8720AI-CP-TR",
        help="搜索页 URL",
    )
    parser.add_argument("--user", type=str, default=None, help="登录账号（或环境变量 SZLCSC_USERNAME）")
    parser.add_argument("--password", type=str, default=None, help="登录密码（或环境变量 SZLCSC_PASSWORD）")
    parser.add_argument(
        "--login-url",
        type=str,
        default=DEFAULT_LOGIN_URL,
        help=f"登录页 URL，默认 {DEFAULT_LOGIN_URL}",
    )
    parser.add_argument(
        "--headless",
        action="store_true",
        help="无头浏览器（登录遇验证码时可去掉此项）",
    )
    parser.add_argument(
        "--skip-login",
        action="store_true",
        help="即使配置了账号也跳过登录（仅抓公开页）",
    )
    args = parser.parse_args()

    if args.model:
        # 与 ickey_crawler.py main 一致：stdout 仅 JSON，错误写 stderr
        try:
            models = [m.strip() for m in args.model.split(",") if m.strip()]
            if not models:
                raise ValueError("--model 不能为空")
            results = run_search_batch(
                models,
                headless=True,
                quiet=True,
                parse_workers=args.parse_workers,
                username=args.user,
                password=args.password,
                login_url=args.login_url or DEFAULT_LOGIN_URL,
                skip_login=args.skip_login,
            )
            out = json.dumps(results, ensure_ascii=False, indent=0)
            try:
                sys.stdout.reconfigure(encoding="utf-8")
            except AttributeError:
                pass
            sys.stdout.buffer.write((out + "\n").encode("utf-8"))
        except Exception as e:
            err = json.dumps({"error": str(e), "results": []}, ensure_ascii=False)
            try:
                sys.stderr.buffer.write((err + "\n").encode("utf-8"))
            except Exception:
                sys.stderr.write(err + "\n")
            sys.exit(1)
        return

    products = fetch_lcsc_product_data(
        url=args.url,
        username=args.user,
        password=args.password,
        login_url=args.login_url or DEFAULT_LOGIN_URL,
        skip_login=args.skip_login,
        headless=args.headless,
        quiet=False,
        page=None,
        keyword=None,
    )
    
    if products:
        print(f"\n共提取到 {len(products)} 个产品:\n")
        for i, product in enumerate(products, 1):
            print(f"--- 产品 {i} ---")
            for key, value in product.items():
                print(f"{key}: {value}")
            print()
        
        # 可选：保存为 JSON 文件
        with open("lcsc_products.json", "w", encoding="utf-8") as f:
            json.dump(products, f, ensure_ascii=False, indent=2)
        print("数据已保存至 lcsc_products.json")
    else:
        print("未提取到任何产品数据")

if __name__ == "__main__":
    main()