"""
华秋商城 hqchip.com 搜索页商品解析。

DOM 参考（与保存的 .mhtml 一致）:
  #resultTabBox / .res_tab_box
    .self_res / .other_res
      .self_res_box / .other_res_box
        div.tr[goods-id][data-suppname][goods-name]
          .col1  图/链接
          .col2  型号(em.light)、品牌、封装、描述(span.desc)…
          .col6 或 .col3 .dt_box  现货库存(strong)
          .col3 .price_list      梯度价(td.num / td.price)

验证: python hqchip.py --validate-mhtml path/to/snapshot.mhtml
"""

from __future__ import annotations

import argparse
import json
import os
import re
import sys
import time
from email import policy
from email.parser import BytesParser
from typing import Any
from urllib.parse import quote

from DrissionPage import ChromiumPage, ChromiumOptions

try:
    from crawler_cli import emit_json_stderr_error, emit_json_stdout
except ImportError:
    _root = os.path.dirname(os.path.abspath(__file__))
    if _root not in sys.path:
        sys.path.insert(0, _root)
    from crawler_cli import emit_json_stderr_error, emit_json_stdout


def setup_browser(headless: bool = False) -> ChromiumPage:
    co = ChromiumOptions()
    co.auto_port()
    co.headless(headless)
    if headless:
        co.set_argument("--headless=new")
    co.set_argument("--remote-allow-origins=*")
    co.set_user_agent(
        "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 "
        "(KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
    )
    co.set_argument("--window-size=1400,900")
    co.set_argument("--blink-settings=imagesEnabled=false")
    return ChromiumPage(co)


def hqchip_search_url(model: str) -> str:
    return f"https://www.hqchip.com/search/{quote(model, safe='')}.html"


def _hqchip_raw_to_unified(item: dict[str, Any], seq: int, query_model: str) -> dict[str, Any]:
    """与 ickey_crawler 输出字段一致。"""
    pt = (item.get("价格梯度") or "").strip() or "N/A"
    return {
        "seq": seq,
        "model": item.get("型号") or "N/A",
        "manufacturer": item.get("品牌") or "N/A",
        "package": (item.get("封装") or "N/A").strip() or "N/A",
        "desc": (item.get("描述") or "N/A")[:100],
        "stock": item.get("库存") or "N/A",
        "moq": "N/A",
        "price_tiers": pt,
        "hk_price": "N/A",
        "mainland_price": pt,
        "lead_time": "N/A",
        "query_model": query_model,
    }


def _attr(el, name: str, default: str = "") -> str:
    try:
        v = el.attr(name)
        return (v or "").strip() if v is not None else default
    except Exception:
        return default


def _text(el) -> str:
    try:
        return (el.text or "").strip()
    except Exception:
        return ""


def load_html_from_mhtml(path: str) -> str:
    """从 Chrome 保存的 .mhtml 中取出 text/html 部分（UTF-8）。"""
    with open(path, "rb") as f:
        msg = BytesParser(policy=policy.default).parse(f)
    for part in msg.walk():
        if part.get_content_type() == "text/html":
            payload = part.get_payload(decode=True)
            if payload:
                return payload.decode("utf-8", errors="replace")
    raise ValueError(f"未在 MHTML 中找到 text/html: {path}")


def parse_hqchip_row_fields(row) -> dict[str, Any] | None:
    """
    解析单行 div.tr（DrissionPage 元素或 BeautifulSoup Tag 均可传入）。
    通过 .col2 / .col3 / .col6 与属性 goods-id、data-suppname 抽取字段。
    """
    gid = _attr(row, "goods-id")
    if not gid and hasattr(row, "get"):
        gid = (row.get("goods-id") or "").strip()
    if not gid:
        return None

    supplier = _attr(row, "data-suppname")
    if not supplier and hasattr(row, "get"):
        supplier = (row.get("data-suppname") or "").strip()

    gname = _attr(row, "goods-name")
    if not gname and hasattr(row, "get"):
        gname = (row.get("goods-name") or "").strip()

    # col2
    model, brand, package, desc, supplier_sku = (
        gname or "",
        "",
        "",
        "",
        "",
    )
    try:
        col2 = row.ele("css:.col2", timeout=0.8)
    except Exception:
        col2 = None
    if col2:
        try:
            em = col2.ele("css:em.light", timeout=0.4)
            if em:
                t = _text(em)
                if t:
                    model = t
        except Exception:
            pass
        try:
            for li in col2.eles("tag:li"):
                txt = _text(li)
                sp = None
                try:
                    sp = li.ele("css:span.tag", timeout=0.15)
                except Exception:
                    pass
                label = _text(sp) if sp else ""
                if "型号" in label or label.startswith("型号"):
                    try:
                        em = li.ele("css:em.light", timeout=0.2)
                        if em and _text(em):
                            model = _text(em)
                    except Exception:
                        pass
                elif "品牌" in label or "牌" in label[:4]:
                    try:
                        a = li.ele("css:a", timeout=0.2)
                        brand = _text(a) if a else txt.split("：", 1)[-1].strip()
                    except Exception:
                        brand = re.split(r"品牌[：:]", txt, 1)[-1].strip() if txt else ""
                elif "封装" in label or "封装" in txt[:6]:
                    if "：" in txt:
                        package = txt.split("：", 1)[-1].strip()
                    else:
                        package = txt
                elif "描述" in label:
                    try:
                        d = li.ele("css:span.desc", timeout=0.2)
                        desc = _text(d) if d else ""
                    except Exception:
                        pass
                elif "供应商" in label or "编号" in label:
                    try:
                        em = li.ele("css:em.light", timeout=0.2)
                        supplier_sku = _text(em) if em else ""
                    except Exception:
                        pass
        except Exception:
            pass

    # 库存：自营常见 .col6 strong；订货/其他常见 .col3 .dt_box strong
    stock = ""
    try:
        col6 = row.ele("css:.col6", timeout=0.3)
        if col6:
            st = col6.ele("css:strong", timeout=0.3)
            if st:
                stock = _text(st)
    except Exception:
        pass
    if not stock:
        try:
            col3 = row.ele("css:.col3", timeout=0.5)
            if col3:
                st = col3.ele("css:.dt_box strong", timeout=0.35)
                if st:
                    stock = _text(st)
        except Exception:
            pass
    if not stock:
        try:
            col3 = row.ele("css:.col3", timeout=0.5)
            if col3:
                for strong in col3.eles("tag:strong"):
                    t = _text(strong)
                    if t and re.search(r"\d", t):
                        stock = t
                        break
        except Exception:
            pass

    # 价格梯度：.price_list table tr
    tiers: list[str] = []
    try:
        pl = row.ele("css:.price_list", timeout=0.5)
        if pl:
            for tr in pl.eles("css:table tbody tr"):
                try:
                    n = tr.ele("css:td.num", timeout=0.15)
                    p = tr.ele("css:td.price", timeout=0.15)
                    if n and p:
                        tiers.append(f"{_text(n)} {_text(p)}")
                except Exception:
                    continue
    except Exception:
        pass
    price_str = " | ".join(tiers) if tiers else ""

    item_link = ""
    try:
        a = row.ele('css:a[data-goodsid="' + gid + '"]', timeout=0.3)
        if not a:
            a = row.ele("css:a.ibox", timeout=0.3)
        if not a:
            a = row.ele("css:.col1 a[href*='item.hqchip.com']", timeout=0.3)
        if a:
            item_link = a.link or ""
    except Exception:
        pass

    return {
        "商品ID": gid,
        "型号": model or gname,
        "品牌": brand,
        "封装": package,
        "描述": desc,
        "供应商": supplier,
        "供应商型号": supplier_sku,
        "库存": stock,
        "价格梯度": price_str,
        "详情链接": item_link,
    }


def fetch_hqchip_products(
    url: str,
    quiet: bool = False,
    page: ChromiumPage | None = None,
    headless: bool = True,
) -> list[dict]:
    """
    返回华秋原始字段字典列表（非 unified）。
    page 传入则复用浏览器（多型号串流）；否则自建并在结束时关闭。
    """
    own = page is None
    if own:
        page = setup_browser(headless=headless)
    try:
        page.get(url)
        try:
            page.wait.doc_loaded(timeout=60)
        except Exception:
            pass
        for sel in (
            "css:#resultTabBox",
            "css:.res_tab_box",
            "css:div.tr[goods-id]",
        ):
            try:
                page.wait.ele_displayed(sel, timeout=25)
                break
            except Exception:
                continue
        time.sleep(1.5)

        rows = page.eles("css:#resultTabBox div[goods-id].tr")
        if not rows:
            rows = page.eles("css:.res_tab_box div[goods-id].tr")
        if not rows:
            rows = page.eles("css:div.tr[goods-id]")

        products: list[dict] = []
        seen: set[str] = set()
        for row in rows:
            try:
                item = parse_hqchip_row_fields(row)
                if not item:
                    continue
                key = item.get("商品ID", "")
                if key and key in seen:
                    continue
                if key:
                    seen.add(key)
                products.append(item)
            except Exception as e:
                if not quiet:
                    print(f"解析行失败: {e}", file=sys.stderr)
                continue
        return products
    finally:
        if own:
            try:
                page.quit()
            except Exception:
                pass


def run_search(
    keyword: str,
    headless: bool = True,
    quiet: bool = False,
    query_model: str | None = None,
) -> list[dict]:
    url = hqchip_search_url(keyword)
    raw = fetch_hqchip_products(url, quiet=quiet, page=None, headless=headless)
    qm = query_model if query_model is not None else keyword
    return [_hqchip_raw_to_unified(r, i + 1, qm) for i, r in enumerate(raw)]


def run_search_batch(
    models: list[str],
    headless: bool = True,
    quiet: bool = False,
    parse_workers: int = 8,
) -> list[dict]:
    _ = parse_workers
    if not models:
        return []
    if len(models) == 1:
        return run_search(models[0], headless=headless, quiet=quiet, query_model=models[0])

    all_results: list[dict] = []
    page = setup_browser(headless=headless)
    try:
        for model in models:
            url = hqchip_search_url(model)
            raw = fetch_hqchip_products(url, quiet=quiet, page=page, headless=headless)
            for i, r in enumerate(raw):
                all_results.append(_hqchip_raw_to_unified(r, i + 1, model))
    finally:
        try:
            page.quit()
        except Exception:
            pass
    return all_results


# ----- BeautifulSoup（离线验证） -----


def _parse_hqchip_row_bs4(div) -> dict[str, Any] | None:
    gid = (div.get("goods-id") or "").strip()
    if not gid:
        return None
    supplier = (div.get("data-suppname") or "").strip()
    gname = (div.get("goods-name") or "").strip()

    model, brand, package, desc, supplier_sku = gname, "", "", "", ""
    col2 = div.select_one(".col2")
    if col2:
        em = col2.select_one("em.light")
        if em:
            model = em.get_text(strip=True) or model
        for li in col2.select("li"):
            txt = li.get_text(" ", strip=True)
            sp = li.select_one("span.tag")
            label = sp.get_text(strip=True) if sp else ""
            if "型号" in label:
                em = li.select_one("em.light")
                if em and em.get_text(strip=True):
                    model = em.get_text(strip=True)
            elif "品牌" in label:
                a = li.select_one("a")
                brand = a.get_text(strip=True) if a else txt.split("：", 1)[-1].strip()
            elif "封装" in label or "封装" in txt[:8]:
                if "：" in txt:
                    package = txt.split("：", 1)[-1].strip()
                else:
                    package = txt
            elif "描述" in label:
                d = li.select_one("span.desc")
                desc = d.get_text(" ", strip=True) if d else ""
            elif "供应商" in label or "编号" in label:
                em = li.select_one("em.light")
                if em:
                    supplier_sku = em.get_text(strip=True)

    stock = ""
    col6 = div.select_one(".col6")
    if col6:
        st = col6.select_one("strong")
        if st:
            stock = st.get_text(strip=True)
    if not stock:
        col3 = div.select_one(".col3")
        if col3:
            st = col3.select_one(".dt_box strong")
            if st:
                stock = st.get_text(strip=True)
    if not stock and div.select_one(".col3"):
        for st in div.select(".col3 strong"):
            t = st.get_text(strip=True)
            if t and re.search(r"\d", t):
                stock = t
                break

    tiers: list[str] = []
    for tr in div.select(".price_list table tbody tr"):
        n = tr.select_one("td.num")
        p = tr.select_one("td.price")
        if n and p:
            tiers.append(f"{n.get_text(strip=True)} {p.get_text(strip=True)}")
    price_str = " | ".join(tiers)

    item_link = ""
    a = div.select_one(f'a[data-goodsid="{gid}"]') or div.select_one("a.ibox")
    if not a:
        a = div.select_one(".col1 a[href*='item.hqchip.com']")
    if a and a.get("href"):
        item_link = a["href"].strip()

    return {
        "商品ID": gid,
        "型号": model or gname,
        "品牌": brand,
        "封装": package,
        "描述": desc,
        "供应商": supplier,
        "供应商型号": supplier_sku,
        "库存": stock,
        "价格梯度": price_str,
        "详情链接": item_link,
    }


def parse_hqchip_html_string(html: str) -> list[dict]:
    try:
        from bs4 import BeautifulSoup
    except ImportError as e:
        raise ImportError("请 pip install beautifulsoup4") from e
    soup = BeautifulSoup(html, "html.parser")
    products: list[dict] = []
    seen: set[str] = set()
    for div in soup.select("#resultTabBox div.tr[goods-id], .res_tab_box div.tr[goods-id]"):
        item = _parse_hqchip_row_bs4(div)
        if not item:
            continue
        gid = item.get("商品ID", "")
        if gid in seen:
            continue
        seen.add(gid)
        products.append(item)
    if not products:
        for div in soup.select("div.tr[goods-id]"):
            item = _parse_hqchip_row_bs4(div)
            if not item:
                continue
            gid = item.get("商品ID", "")
            if gid in seen:
                continue
            seen.add(gid)
            products.append(item)
    return products


def validate_mhtml(path: str) -> list[dict]:
    html = load_html_from_mhtml(path)
    return parse_hqchip_html_string(html)


def main() -> None:
    parser = argparse.ArgumentParser(description="华秋商城搜索页爬虫")
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
        "--validate-mhtml",
        metavar="PATH",
        help="离线验证：从 Chrome 保存的 .mhtml 解析（不启动浏览器）",
    )
    parser.add_argument(
        "--url",
        default="https://www.hqchip.com/search/LAN8720AI-CP-TR.html",
        help="搜索页 URL",
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
            )
            emit_json_stdout(results)
        except Exception as e:
            emit_json_stderr_error(str(e))
            sys.exit(1)
        return

    if args.validate_mhtml:
        p = os.path.abspath(args.validate_mhtml)
        data = validate_mhtml(p)
        print(f"[validate-mhtml] {p}")
        print(f"[validate-mhtml] 解析到 {len(data)} 条商品")
        if not data:
            print("[validate-mhtml] 失败：0 条", file=sys.stderr)
            raise SystemExit(1)
        out = os.path.join(os.path.dirname(p), "hqchip_products.validate.json")
        with open(out, "w", encoding="utf-8") as f:
            json.dump(data, f, ensure_ascii=False, indent=2)
        print(f"[validate-mhtml] 已写入 {out}")
        for i, it in enumerate(data[:3], 1):
            print(f"  样例{i}: {it.get('型号')} | {it.get('供应商')} | 库存 {it.get('库存')}")
        raise SystemExit(0)

    data = fetch_hqchip_products(args.url, quiet=False, page=None, headless=False)
    print(f"共 {len(data)} 条")
    for it in data[:10]:
        print(json.dumps(it, ensure_ascii=False))
    out_path = "hqchip_products.json"
    with open(out_path, "w", encoding="utf-8") as f:
        json.dump(data, f, ensure_ascii=False, indent=2)
    print(f"已写入 {out_path}")


if __name__ == "__main__":
    main()
