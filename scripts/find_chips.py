from DrissionPage import ChromiumPage, ChromiumOptions
from DrissionPage.errors import PageDisconnectedError
from datetime import datetime
from html import unescape
import argparse
import json
import os
import re
import time

FINDCHIPS_ORIGIN = "https://www.findchips.com"


def _normalize_findchips_url(href: str | None) -> str | None:
    """站内相对路径、//analytics 等转为可访问 URL。"""
    if not href:
        return None
    h = href.strip()
    if h.startswith("//"):
        return "https:" + h
    if h.startswith("/") and not h.startswith("//"):
        return FINDCHIPS_ORIGIN + h
    return h


def _parse_data_price_json(raw: str) -> tuple[str, list[dict]]:
    """
    data-price 形如 [[1,&quot;USD&quot;,&quot;1.27&quot;], ...] → 可读梯度 + 结构化档位。
    """
    if not raw:
        return "", []
    try:
        s = unescape(raw)
        arr = json.loads(s)
    except (json.JSONDecodeError, TypeError):
        return raw, []
    parts: list[str] = []
    breaks: list[dict] = []
    for it in arr:
        if isinstance(it, (list, tuple)) and len(it) >= 3:
            qty, cur, price = it[0], it[1], it[2]
            parts.append(f"{qty}+ {cur} {price}")
            breaks.append(
                {"起订量": qty, "币种": cur, "单价": str(price).strip()}
            )
    return (" | ".join(parts), breaks)


def setup_findchips_browser() -> ChromiumPage:
    """与 icgoo/szlcsc 类似：独立端口、允许 CDP 来源，降低连接异常概率。"""
    co = ChromiumOptions()
    co.auto_port()
    co.set_argument("--remote-allow-origins=*")
    co.set_argument("--disable-blink-features=AutomationControlled")
    co.set_user_agent(
        "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 "
        "(KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
    )
    co.set_argument("--window-size=1400,900")
    co.set_load_mode("normal")
    return ChromiumPage(co)


def _safe_quit(page: ChromiumPage | None) -> None:
    if page is None:
        return
    try:
        page.quit()
    except Exception:
        pass


def save_findchips_page_html(
    page: ChromiumPage, prefix: str = "findchips_page", out_dir: str | None = None
) -> str:
    """将当前页面完整 HTML 落盘（UTF-8），便于对照 DOM 调试。返回保存路径。"""
    ts = datetime.now().strftime("%Y%m%d_%H%M%S")
    name = f"{prefix}_{ts}.html"
    path = os.path.join(out_dir, name) if out_dir else name
    if out_dir:
        os.makedirs(out_dir, exist_ok=True)
    html = page.html or ""
    with open(path, "w", encoding="utf-8") as f:
        f.write(html)
    return path


def accept_findchips_consent_if_present(page: ChromiumPage, timeout: float = 10.0) -> bool:
    """
    FindChips（SupplyFrame）使用 Usercentrics CMP，常见 #uc-main-dialog。
    自动点击「Accept All」关闭隐私横幅，避免遮挡后续解析。
    """
    selectors = (
        "css:#accept",
        "css:button.uc-accept-button",
        'css:button[data-action-type="accept"]',
        'xpath://button[contains(@class,"uc-accept-button")]',
        'xpath://button[contains(normalize-space(.),"Accept All")]',
        'xpath://*[@id="uc-main-dialog"]//button[contains(@aria-label,"Accept")]',
    )
    deadline = time.time() + timeout
    while time.time() < deadline:
        for sel in selectors:
            try:
                btn = page.ele(sel, timeout=0.35)
                if btn:
                    btn.click()
                    time.sleep(0.8)
                    return True
            except Exception:
                continue
        time.sleep(0.45)
    return False


def _attr(el, name: str, default: str = "") -> str:
    try:
        v = el.attr(name)
        return (v or "").strip() if v is not None else default
    except Exception:
        return default


def _clean_stock_display(raw: str) -> str:
    """data-stock 可能含 <nobr><b> 等 HTML 片段。"""
    if not raw:
        return ""
    t = unescape(raw)
    t = re.sub(r"<[^>]+>", " ", t)
    return " ".join(t.split())


def _collect_distributor_tables(page: ChromiumPage) -> list[tuple[str, object]]:
    """
    与 findchips 保存页一致：
    - 有机：div.distributor-results[data-distributor_name] > table
    - 赞助：div.premiumFrame[data-distributor_name] > table.premiumAdTable
    """
    blocks: list[tuple[str, object]] = []
    for div in page.eles("css:.distributor-results"):
        dist = _attr(div, "data-distributor_name")
        if not dist:
            h2 = div.ele("css:h2.distributor-title", timeout=0.5)
            if h2:
                dist = (h2.text or "").strip()
                dist = re.split(r"[\n\r]", dist, 1)[0].strip()
        for table in div.eles("tag:table"):
            blocks.append((dist, table))

    for frame in page.eles("css:.premiumFrame"):
        dist = _attr(frame, "data-distributor_name")
        for table in frame.eles("css:table.premiumAdTable"):
            blocks.append((dist, table))

    return blocks


def parse_findchips_table_row(row) -> dict | None:
    """
    解析 FindChips 搜索结果表中的一行（SupplyFrame 标准结构）。

    典型 DOM::

        <tr class="row" data-distributor_name="..." data-mfr="..." data-mfrpartnumber="..."
            data-stock="..." data-price='[[...]]' data-distino="...">
            <td class="td-part">...</td>
            <td class="td-mfg">...</td>
            <td class="td-desc">...</td>
            <td class="td-stock">...</td>
            <td class="td-price"><ul class="price-list">...</ul></td>
            <td class="td-price-range">...</td>
            <td class="td-buy"><a class="buy-button">...</a></td>
        </tr>
    """
    try:
        has_organic = bool(row.ele("css:td.td-part", timeout=0.2))
        has_premium = bool(row.ele("css:td.mfrPartNumber", timeout=0.2))
        cls = _attr(row, "class")
        has_row_class = "row" in cls.split() if cls else False
        has_data = bool(_attr(row, "data-distributor_name") or _attr(row, "data-mfrpartnumber"))
        if not has_row_class and not has_data and not has_organic and not has_premium:
            return None
    except Exception:
        return None

    dist = _attr(row, "data-distributor_name")
    mfr_attr = _attr(row, "data-mfr")
    mpn_attr = _attr(row, "data-mfrpartnumber")
    stock_attr = _attr(row, "data-stock") or _attr(row, "data-instock")
    distino = _attr(row, "data-distino")
    price_data = _attr(row, "data-price")

    # 有机：td-part / td-mfg / …；赞助位 premiumAdTable：mfrPartNumber / mfrName / …
    td_part = row.ele("css:td.td-part", timeout=0.35)
    td_mfg = row.ele("css:td.td-mfg", timeout=0.35)
    td_desc = row.ele("css:td.td-desc", timeout=0.35)
    td_stock = row.ele("css:td.td-stock", timeout=0.35)
    td_price = row.ele("css:td.td-price", timeout=0.35)
    td_range = row.ele("css:td.td-price-range", timeout=0.35)
    td_buy = row.ele("css:td.td-buy", timeout=0.35)

    if not td_part:
        td_part = row.ele("css:td.mfrPartNumber", timeout=0.35)
    if not td_mfg:
        td_mfg = row.ele("css:td.mfrName", timeout=0.35)
    if not td_desc:
        td_desc = row.ele("css:td.td-desc", timeout=0.35)
    if not td_desc:
        td_desc = row.ele("css:td.description", timeout=0.35)
    if not td_stock:
        td_stock = row.ele("css:td.td-stock", timeout=0.35)
    if not td_stock:
        td_stock = row.ele("css:td.mfrStock", timeout=0.35)
    if not td_price:
        td_price = row.ele("css:td.td-price", timeout=0.35)
    if not td_price:
        td_price = row.ele("css:td.mfrPrice", timeout=0.35)
    if not td_range:
        td_range = row.ele("css:td.td-price-range", timeout=0.35)
    if not td_buy:
        td_buy = row.ele("css:td.td-buy", timeout=0.35)
    if not td_buy:
        td_buy = row.ele("css:td.buyNow", timeout=0.35)

    mpn = mpn_attr
    dist_sku_from_span = ""
    if td_part:
        a = td_part.ele("css:.part-name a", timeout=0.22)
        if not a:
            a = td_part.ele("css:a", timeout=0.22)
        if a:
            t = (a.text or "").strip()
            if t:
                mpn = t
        av = td_part.ele("css:.td-desc-distributor .additional-value", timeout=0.2)
        if not av:
            av = td_part.ele("css:.additional-description .additional-value", timeout=0.2)
        if av:
            dist_sku_from_span = (av.text or "").strip()

    desc = ""
    rohs = ""
    detail_link = None
    if td_desc:
        sp = td_desc.ele("css:span.td-description", timeout=0.2)
        desc = (sp.text or "").strip() if sp else (td_desc.text or "").strip()
        rv = td_desc.ele("css:.td-desc-rohs .additional-value", timeout=0.2)
        if rv:
            rohs = (rv.text or "").strip()
        for pdp_sel in ("css:a.pdp-link-desktop", "css:a.pdp-link-mobile"):
            pdp = td_desc.ele(pdp_sel, timeout=0.2)
            if pdp:
                detail_link = _normalize_findchips_url(pdp.link)
                break

    mfr = (td_mfg.text or "").strip() if td_mfg else ""
    if not mfr:
        mfr = mfr_attr

    stock_txt = (td_stock.text or "").strip() if td_stock else ""
    if not stock_txt and stock_attr:
        stock_txt = _clean_stock_display(stock_attr)
    elif stock_txt and "<" in stock_txt:
        stock_txt = _clean_stock_display(stock_txt)

    price_txt = ""
    if td_price:
        price_txt = " ".join((td_price.text or "").split())

    price_range_txt = (td_range.text or "").strip() if td_range else ""

    buy_link = None
    if td_buy:
        buy_btn = td_buy.ele("css:a.buy-button", timeout=0.25)
        if not buy_btn:
            buy_btn = td_buy.ele("css:a.buy-now", timeout=0.25)
        if not buy_btn:
            buy_btn = td_buy.ele("css:a.buy-now-click-tracking", timeout=0.25)
        if not buy_btn:
            buy_btn = td_buy.ele("css:a", timeout=0.25)
        if buy_btn:
            buy_link = _normalize_findchips_url(buy_btn.link)

    if not mpn and not mpn_attr:
        return None

    price_grad_str, price_breaks = _parse_data_price_json(price_data)

    dist_sku = distino or dist_sku_from_span

    out: dict = {
        "分销商": dist,
        "部件编号": mpn or mpn_attr,
        "制造商": mfr,
        "说明": desc,
        "库存": stock_txt,
        "价格": price_txt,
        "价格区间": price_range_txt,
        "购买链接": buy_link,
    }
    if dist_sku:
        out["分销商编号"] = dist_sku
    if rohs:
        out["RoHS"] = rohs
    if detail_link:
        out["详情链接"] = detail_link
    if price_grad_str:
        out["价格梯度"] = price_grad_str
    if price_breaks:
        out["价格档位"] = price_breaks
    if price_data:
        out["price_data"] = price_data
    if _attr(row, "data-sponsored") == "true":
        out["sponsored"] = True
    return out


def _collect_rows_from_table(table) -> list:
    """优先 tbody tr.row，否则带 td.td-part / td.mfrPartNumber 的数据行。"""
    rows = table.eles("css:tbody tr.row")
    if rows:
        return list(rows)
    out = []
    try:
        for r in table.eles("css:tbody tr"):
            try:
                if r.ele("css:td.td-part", timeout=0.22):
                    out.append(r)
                elif r.ele("css:td.mfrPartNumber", timeout=0.22):
                    out.append(r)
            except Exception:
                continue
    except Exception:
        pass
    return out


def _fetch_findchips_once(page: ChromiumPage, url: str) -> list:
    """
    单次会话内抓取 FindChips 搜索页；可能抛出 PageDisconnectedError。
    """
    print("正在访问页面...")
    page.get(url)
    try:
        page.wait.doc_loaded(timeout=60)
    except Exception:
        pass

    # Usercentrics GDPR：自动接受，避免遮罩挡住表格
    if accept_findchips_consent_if_present(page, timeout=12):
        print("已自动接受 Cookie 同意（Accept All）")
    else:
        print("未检测到隐私弹层或已关闭，继续…")

    # 等待主内容（table 或典型结果区；站点可能用 div 模拟表格）
    waited = False
    for sel in (
        "css:table",
        "css:tbody tr",
        "css:[class*='result']",
        "css:main",
    ):
        try:
            page.wait.ele_displayed(sel, timeout=15)
            waited = True
            break
        except Exception:
            continue
    if not waited:
        print("等待主内容超时，尝试继续...")

    time.sleep(2)

    # 检测是否存在验证码（若有易盾特征，可手动处理）
    try:
        if page.ele(".yidun_control", timeout=2):
            print("检测到验证码，请手动完成...")
            input("完成验证后按回车继续...")
            page.refresh()
            time.sleep(3)
    except PageDisconnectedError:
        raise
    except Exception:
        pass

    # 与离线 HTML 一致：.distributor-results、.premiumFrame + premiumAdTable
    blocks = _collect_distributor_tables(page)
    if not blocks:
        print("未通过 .distributor-results/.premiumFrame 找到表格，回退 h2/h3/h4 …")
        for tag in ("h2", "h3", "h4"):
            for el in page.eles(f"tag:{tag}"):
                dist_name = (el.text or "").strip()
                if not dist_name or "登录" in dist_name:
                    continue
                dist_name = re.split(r"[\n\r]", dist_name, 1)[0].strip()
                table = el.next("table")
                if table:
                    blocks.append((dist_name, table))

    if not blocks:
        print("回退：遍历页面内 table …")
        for table in page.eles("tag:table"):
            prev = table.prev("h2") or table.prev("h3") or table.prev("h4")
            hint = (prev.text or "").strip() if prev else ""
            if hint:
                hint = re.split(r"[\n\r]", hint, 1)[0].strip()
            blocks.append((hint, table))

    products: list[dict] = []
    for block_hint, table in blocks:
        block_hint = (block_hint or "").strip()
        for row in _collect_rows_from_table(table):
            item = parse_findchips_table_row(row)
            if not item:
                continue
            if block_hint and not (item.get("分销商") or "").strip():
                item["分销商"] = block_hint
            products.append(item)

    if products:
        try:
            saved = save_findchips_page_html(page)
            print(f"已保存页面 HTML: {saved}")
        except OSError as ex:
            print(f"保存页面 HTML 失败: {ex}")

    return products


def fetch_findchips_data(url: str, retries: int = 2) -> list:
    """
    获取 FindChips 搜索页面的产品数据。
    遇「与页面的连接已断开」时自动重开浏览器重试（默认 2 次）。
    """
    last_disc: PageDisconnectedError | None = None
    for attempt in range(1, retries + 1):
        page = setup_findchips_browser()
        try:
            return _fetch_findchips_once(page, url)
        except PageDisconnectedError as e:
            last_disc = e
            print(
                f"与页面连接已断开（{attempt}/{retries}），"
                f"可能是标签页崩溃或页面过重；将重试…",
                flush=True,
            )
        except Exception as e:
            print(f"发生错误: {e}")
            return []
        finally:
            _safe_quit(page)
        if attempt < retries:
            time.sleep(2.5)

    if last_disc is not None:
        print(f"发生错误: {last_disc}")
    return []


def _parse_findchips_row_bs4(tr) -> dict | None:
    """与 parse_findchips_table_row 字段对齐，供离线 HTML 验证。"""
    cls = tr.get("class") or []
    if isinstance(cls, str):
        cls = cls.split()
    has_row = "row" in cls
    has_data = tr.get("data-distributor_name") or tr.get("data-mfrpartnumber")
    has_td = tr.select_one("td.td-part") or tr.select_one("td.mfrPartNumber")
    if not has_row and not has_data and not has_td:
        return None

    dist = (tr.get("data-distributor_name") or "").strip()
    mfr_attr = (tr.get("data-mfr") or "").strip()
    mpn_attr = (tr.get("data-mfrpartnumber") or "").strip()
    stock_attr = (tr.get("data-stock") or tr.get("data-instock") or "").strip()
    distino = (tr.get("data-distino") or "").strip()
    price_data = (tr.get("data-price") or "").strip()

    td_part = tr.select_one("td.td-part") or tr.select_one("td.mfrPartNumber")
    td_mfg = tr.select_one("td.td-mfg") or tr.select_one("td.mfrName")
    td_desc = tr.select_one("td.td-desc") or tr.select_one("td.description")
    td_stock = tr.select_one("td.td-stock") or tr.select_one("td.mfrStock")
    td_price = tr.select_one("td.td-price") or tr.select_one("td.mfrPrice")
    td_range = tr.select_one("td.td-price-range")
    td_buy = tr.select_one("td.td-buy") or tr.select_one("td.buyNow")

    mpn = mpn_attr
    dist_sku_from_span = ""
    if td_part:
        a = td_part.select_one(".part-name a") or td_part.select_one("a")
        if a:
            t = a.get_text(strip=True)
            if t:
                mpn = t
        av = td_part.select_one(".td-desc-distributor .additional-value")
        if not av:
            av = td_part.select_one(".additional-description .additional-value")
        if av:
            dist_sku_from_span = av.get_text(strip=True)

    desc = ""
    rohs = ""
    detail_link = None
    if td_desc:
        sp = td_desc.select_one("span.td-description")
        desc = sp.get_text(" ", strip=True) if sp else td_desc.get_text(" ", strip=True)
        rv = td_desc.select_one(".td-desc-rohs .additional-value")
        if rv:
            rohs = rv.get_text(strip=True)
        for sel in ("a.pdp-link-desktop", "a.pdp-link-mobile"):
            pdp = td_desc.select_one(sel)
            if pdp and pdp.get("href"):
                detail_link = _normalize_findchips_url(pdp.get("href"))
                break

    mfr = td_mfg.get_text(" ", strip=True) if td_mfg else ""
    if not mfr:
        mfr = mfr_attr

    stock_txt = td_stock.get_text(" ", strip=True) if td_stock else ""
    if not stock_txt and stock_attr:
        stock_txt = _clean_stock_display(stock_attr)
    elif stock_txt and "<" in stock_txt:
        stock_txt = _clean_stock_display(stock_txt)

    price_txt = ""
    if td_price:
        price_txt = " ".join(td_price.get_text(" ", strip=True).split())
    price_range_txt = td_range.get_text(" ", strip=True) if td_range else ""

    buy_link = None
    if td_buy:
        buy_btn = (
            td_buy.select_one("a.buy-button")
            or td_buy.select_one("a.buy-now")
            or td_buy.select_one("a.buy-now-click-tracking")
            or td_buy.select_one("a")
        )
        if buy_btn and buy_btn.get("href"):
            buy_link = _normalize_findchips_url(buy_btn.get("href"))

    if not mpn and not mpn_attr:
        return None

    price_grad_str, price_breaks = _parse_data_price_json(price_data)
    dist_sku = distino or dist_sku_from_span

    out: dict = {
        "分销商": dist,
        "部件编号": mpn or mpn_attr,
        "制造商": mfr,
        "说明": desc,
        "库存": stock_txt,
        "价格": price_txt,
        "价格区间": price_range_txt,
        "购买链接": buy_link,
    }
    if dist_sku:
        out["分销商编号"] = dist_sku
    if rohs:
        out["RoHS"] = rohs
    if detail_link:
        out["详情链接"] = detail_link
    if price_grad_str:
        out["价格梯度"] = price_grad_str
    if price_breaks:
        out["价格档位"] = price_breaks
    if price_data:
        out["price_data"] = price_data
    if tr.get("data-sponsored") == "true":
        out["sponsored"] = True
    return out


def parse_findchips_html_file(path: str) -> list[dict]:
    """离线解析已保存的搜索页 HTML（与线上 DOM 一致时可用于回归验证）。"""
    try:
        from bs4 import BeautifulSoup
    except ImportError as e:
        raise ImportError("请 pip install beautifulsoup4 后再使用 --validate-html") from e
    with open(path, encoding="utf-8") as f:
        soup = BeautifulSoup(f.read(), "html.parser")
    products = []
    for tr in soup.select("tbody tr.row"):
        item = _parse_findchips_row_bs4(tr)
        if item:
            products.append(item)
    return products


def main() -> None:
    parser = argparse.ArgumentParser(description="FindChips 搜索页爬虫")
    parser.add_argument(
        "--validate-html",
        metavar="PATH",
        help="离线验证：解析已保存的 HTML（不启动浏览器），需 beautifulsoup4",
    )
    parser.add_argument(
        "--url",
        default="https://www.findchips.com/search/LAN8720AI-CP-TR",
        help="搜索页 URL",
    )
    args = parser.parse_args()

    if args.validate_html:
        p = os.path.abspath(args.validate_html)
        data = parse_findchips_html_file(p)
        print(f"[validate-html] {p}")
        print(f"[validate-html] 解析到 {len(data)} 条 tr.row 产品行")
        if not data:
            print("[validate-html] 失败：0 条，请检查 HTML 是否完整或选择器是否变更")
            raise SystemExit(1)
        out_json = os.path.join(os.path.dirname(p) or ".", "findchips_products.validate.json")
        with open(out_json, "w", encoding="utf-8") as f:
            json.dump(data, f, ensure_ascii=False, indent=2)
        print(f"[validate-html] 已写入 {out_json}")
        for idx, item in enumerate(data[:3], 1):
            print(f"\n样例 {idx}: {item.get('部件编号')} @ {item.get('分销商')}")
        raise SystemExit(0)

    data = fetch_findchips_data(args.url)

    if data:
        print(f"共提取到 {len(data)} 条产品记录")
        for idx, item in enumerate(data[:5], 1):
            print(f"\n记录 {idx}:")
            for k, v in item.items():
                print(f"  {k}: {v}")

        with open("findchips_products.json", "w", encoding="utf-8") as f:
            json.dump(data, f, ensure_ascii=False, indent=2)
        print("\n数据已保存至 findchips_products.json")
    else:
        print("未提取到任何产品数据")


if __name__ == "__main__":
    main()