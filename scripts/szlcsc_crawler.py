"""
立创商城搜索页爬虫：建议先落盘 HTML 再分析，便于对照真实 DOM 调整选择器。

CLI（与 ickey_crawler 一致）:
  python szlcsc_crawler.py --model LAN8720AI-CP-TR
  python szlcsc_crawler.py -m A,B,C   # 串流依次搜索，stdout 仅 JSON

商品数据均在带 **data-spm** 的节点下；单条样本结构为多个
``<dl><dt>标签</dt><dd>值</dd></dl>``（如：供应商编号、品牌、封装、库存、广东仓…）。
解析时优先按该结构抽取，与样本页 ``szlcsc_page_*.html`` 一致。

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
    from crawler_cli import emit_json_stderr_error, emit_json_stdout
except ImportError:
    _root = os.path.dirname(os.path.abspath(__file__))
    if _root not in sys.path:
        sys.path.insert(0, _root)
    from crawler_cli import emit_json_stderr_error, emit_json_stdout

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


def lcsc_product_to_unified(p: dict, seq: int, query_model: str) -> dict:
    """build_product_from_lcsc_row 结果 → 与 ickey_crawler 一致字段。"""
    pt = _sz_field(p.get("价格梯度"))
    if pt == "N/A":
        pt = "N/A"
    moq = _sz_field(p.get("起订量"))
    return {
        "seq": seq,
        "model": _sz_field(p.get("型号")),
        "manufacturer": _sz_field(p.get("品牌")),
        "package": _sz_field(p.get("封装")),
        "desc": _sz_field(p.get("描述"))[:100],
        "stock": _sz_field(p.get("库存")),
        "moq": moq,
        "price_tiers": pt,
        "hk_price": "N/A",
        "mainland_price": pt,
        "lead_time": "N/A",
        "query_model": query_model,
    }


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
    从单条商品根节点解析 <dl><dt>标签</dt><dd>值</dd></dl>（立创搜索列表样本格式）。
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
            val = (dd.text or "").strip()
            if key:
                fields[key] = val
        except Exception:
            continue
    return fields


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


def build_product_from_lcsc_row(item, fields: dict[str, str], data_spm: str) -> dict:
    """按样本 HTML：dt 键名 + 链接型号。"""
    try:
        block_text = (item.text or "") if item else ""
    except Exception:
        block_text = ""

    model = "未提取到"
    try:
        for xp in (
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

    supplier_no = fields.get("供应商编号", "")
    code = (fields.get("编号") or supplier_no or "").strip() or "未提取到"

    product: dict = {
        "data_spm": data_spm,
        "型号": model,
        "编号": code,
        "供应商编号": supplier_no or "未提取到",
        "品牌": fields.get("品牌", "未提取到"),
        "封装": fields.get("封装", "未提取到"),
        "描述": fields.get("描述", "未提取到"),
        "库存": fields.get("库存", "未提取到"),
        "参数": fields.get("参数", "未提取到"),
        "批次": fields.get("批次", "未提取到"),
        "类目": fields.get("类目", "未提取到"),
        "最小包装": fields.get("最小包装", "未提取到"),
        "增量": fields.get("增量", "未提取到"),
        "产地": fields.get("产地", "未提取到"),
        "包装": fields.get("包装", "未提取到"),
        "价格梯度": _price_tiers_from_block_text(block_text),
        "起订量": fields.get("最小包装", fields.get("增量", "未提取到")),
    }
    wh = _warehouse_subdict(fields)
    if wh:
        product["区域仓库存"] = wh
    # 保留完整 dl 字段，便于对照样本页调试
    product["dl字段"] = fields
    return product


def collect_product_roots_under_data_spm(page) -> list:
    """
    选取带 data-spm 且含立创商品 dl（样本中含「供应商编号」）的根节点；
    按「供应商编号」或商品链接去重。
    """
    candidates = page.eles(
        'xpath://*[@data-spm][.//dl[.//dt[contains(., "供应商编号")]]]'
    )
    if not candidates:
        candidates = page.eles(
            'xpath://*[@data-spm][.//a[contains(@href, "/product/") or contains(@href, "product")]]'
        )

    seen: set[str] = set()
    roots: list = []
    for el in candidates:
        try:
            spm = (el.attr("data-spm") or "").strip()
            if spm in _SPM_SKIP:
                continue

            fields = extract_dl_fields(el)
            dedup_key = (fields.get("供应商编号") or "").strip()
            if not dedup_key:
                link = el.ele('xpath:.//a[contains(@href, "product")]', timeout=0.5)
                if link:
                    dedup_key = (link.attr("href") or "").split("?")[0]
            if not dedup_key:
                continue
            if dedup_key in seen:
                continue
            seen.add(dedup_key)
            roots.append(el)
        except Exception:
            continue
    return roots


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
    获取立创商城搜索结果页面的产品数据（build_product_from_lcsc_row 原始结构）。
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

        product_items = collect_product_roots_under_data_spm(page)
        if not product_items:
            product_items = page.eles('xpath://div[contains(@class, "product-item")]')
        if not product_items:
            product_items = page.eles('xpath://tr[contains(@class, "product")]')

        if not product_items:
            if not quiet:
                print("未找到产品项，请检查选择器")
                try:
                    print(f"页面标题: {page.title}")
                except Exception:
                    print("页面标题: (无法获取)")
            return []

        products = []

        for idx, item in enumerate(product_items, 1):
            try:
                spm = (item.attr("data-spm") or "").strip()
                fields = extract_dl_fields(item)
                if not fields:
                    if not quiet:
                        print(f"第 {idx} 条未解析到 dl 字段，跳过（可对照已保存 HTML）")
                    continue
                product = build_product_from_lcsc_row(item, fields, spm)
                products.append(product)
                if not quiet:
                    print(f"成功提取第 {idx} 个产品: {product.get('型号', 'Unknown')}")
            except Exception as e:
                if not quiet:
                    print(f"提取第 {idx} 个产品时出错: {e}")
                continue

        return products

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
    parser = argparse.ArgumentParser(description="立创商城搜索页爬虫（可选自动登录）")
    parser.add_argument(
        "--model",
        "-m",
        type=str,
        help="搜索型号，逗号分隔；输出与 ickey_crawler 一致的 JSON 到 stdout",
    )
    parser.add_argument(
        "--parse-workers",
        "-w",
        type=int,
        default=8,
        help="预留，与 ickey 对齐",
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
            emit_json_stdout(results)
        except Exception as e:
            emit_json_stderr_error(str(e))
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