"""
云汉芯城元器件搜索页面爬虫 - DrissionPage实现
目标URL: https://search.ickey.cn/?keyword=SN74HC595PWR
提取数据：供应商、厂牌、封装、描述、库存、起订量、价格梯度、货期

CLI 模式（供 Go 后端调用）:
  python ickey_crawler.py --model SN74HC595PWR
  python ickey_crawler.py --model SN74HC595PWR,ABC123,XYZ456   # 多型号逗号分隔，串流依次搜索
  输出 JSON 到 stdout，无交互

要求：
  1. 型号串流执行（依次搜索，不并行）
  2. 解析页面数据时并行解析（每个型号的 result-list 行并行提取）
"""

import argparse
import json
import os
import re
import sys
import csv
import time
import tkinter as tk
from concurrent.futures import ThreadPoolExecutor, as_completed
from datetime import datetime
from DrissionPage import ChromiumPage, ChromiumOptions
from DrissionPage.common import By


def setup_browser(headless=False):
    """
    配置并初始化浏览器（关键：修改UA绕过反爬）
    headless: True 时无头模式，供 CLI 调用
    """
    co = ChromiumOptions()

    # 使用自动端口，避免 9222 被占用导致 Handshake 404
    co.auto_port()

    # 重要：设置常规浏览器User-Agent，避免被识别为爬虫
    co.set_user_agent('Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36')

    co.headless(headless)

    # Chrome 110+ 连接 CDP 需此参数，否则 Handshake 404/403
    co.set_argument('--remote-allow-origins=*')
    # 无界面系统或 Chrome 136+ 需用新 headless 模式
    if headless:
        co.set_argument('--headless=new')

    # 设置窗口大小
    co.set_argument('--window-size=1400,900')

    # 禁用图片加载，提高速度
    co.set_argument('--blink-settings=imagesEnabled=false')

    # 初始化浏览器
    page = ChromiumPage(co)
    return page


def wait_for_page_load(page, keyword, timeout=15, quiet=False):
    """
    等待页面加载完成
    quiet: True 时不输出日志（CLI 模式，避免污染 stdout）
    """
    if not quiet:
        print(f"正在加载页面，关键词: {keyword}")
    page.get(f'https://search.ickey.cn/?keyword={keyword}')
    try:
        page.wait.ele_displayed('.result-list', timeout=timeout)
        if not quiet:
            print("页面加载成功")
        time.sleep(2)
        return True
    except Exception:
        if not quiet:
            print("页面加载超时或未找到结果")
        return False


def _extract_qty_price_pairs(qty_elems, price_elems) -> list:
    """将数量与价格配对，返回 ['5+ ￥0.88', ...]"""
    pairs = []
    for i in range(min(len(qty_elems), len(price_elems))):
        qty = (qty_elems[i].text or '').strip()
        pr = (price_elems[i].text or '').strip()
        if qty and pr:
            pairs.append(f"{qty} {pr}")
    return pairs


def extract_price_gradients_from_row(row) -> dict:
    """
    从商品行中提取价格梯度
    云汉芯城结构：价格块在 div[data-sku] 内，含 search-w-price / search-w-hk / search-w-home
    """
    out = {'price_tiers': 'N/A', 'hk_price': 'N/A', 'mainland_price': 'N/A'}
    # 优先在 row 内查找，若无则在其父级查找（价格块可能在父容器内）
    def _find(sel):
        el = row.ele(sel, timeout=0.3)
        if not el and hasattr(row, 'parent'):
            try:
                p = row.parent()
                if p:
                    el = p.ele(sel, timeout=0.2)
            except Exception:
                pass
        return el
    qty_container = _find('.search-w-price')
    hk_container = _find('.search-w-hk')
    home_container = _find('.search-w-home')
    if not qty_container:
        return out
    # search-result-bor mt8 为数量/价格单元格
    qty_elems = qty_container.eles('.search-result-bor')
    # 内地交货 = search-w-home 价格
    if home_container:
        home_elems = home_container.eles('.search-result-bor')
        pairs = _extract_qty_price_pairs(qty_elems, home_elems)
        if pairs:
            out['price_tiers'] = out['mainland_price'] = ' | '.join(pairs)
    # 中国香港交货 = search-w-hk 价格
    if hk_container:
        hk_elems = hk_container.eles('.search-result-bor')
        pairs = _extract_qty_price_pairs(qty_elems, hk_elems)
        if pairs:
            out['hk_price'] = ' | '.join(pairs)
    return out


def get_clipboard_text():
    """
    使用tkinter读取系统剪贴板（备用方案，暂未使用）
    """
    try:
        root = tk.Tk()
        root.withdraw()
        text = root.clipboard_get()
        root.destroy()
        return text.strip()
    except:
        return ""


def _extract_package_from_block(block: str) -> str:
    """从 HTML 块中提取封装，如 LGA-16、TSSOP-16"""
    # 匹配 封装：</span> <span class="..."> LGA-16 </span>
    m = re.search(r'封装[：:]\s*</span>\s*<span[^>]*>\s*([^<]+)\s*</span>', block)
    if m:
        return m.group(1).strip()
    # 匹配 封装：xxx（简单格式）
    m = re.search(r'封装[：:]\s*([^<,，\s]+)', block)
    if m:
        return m.group(1).strip()
    m = re.search(r'data-package="([^"]+)"', block)
    if m:
        return m.group(1).strip()
    return 'N/A'


def _extract_desc_from_block(block: str) -> str:
    """从 HTML 块中提取描述"""
    # 匹配 描述：</span> <span ...>xxx</span>
    m = re.search(r'描述[：:]\s*</span>\s*<span[^>]*>([^<]*)</span>', block)
    if m:
        return m.group(1).strip()[:100]
    m = re.search(r'描述[：:]\s*<span[^>]*>([^<]*)</span>', block)
    if m:
        return m.group(1).strip()[:100]
    return 'N/A'


def _extract_moq_from_block(block: str) -> str:
    """从 HTML 块中提取起订量"""
    m = re.search(r'起订量[：:]\s*(\d+)', block)
    return m.group(1) if m else 'N/A'


def _extract_delivery_from_block(block: str) -> str:
    """从 HTML 块中提取货期，如 7-9工作日"""
    m = re.search(r'内地\s*(\d+[-\d]*\s*工作日)', block)
    if m:
        return m.group(1).strip()
    m = re.search(r'(\d+[-\d]*\s*工作日)', block)
    return m.group(1).strip() if m else 'N/A'


def _extract_prices_from_section(section: str) -> list:
    """从 HTML 片段中提取价格列表，支持 ￥/¥ 和 $"""
    prices = []
    # 优先匹配 ￥<span>0.xx</span> 或 $<span>0.xx</span> 或 ¥<span>0.xx</span>
    for m in re.finditer(r'[￥¥\$]\s*<span>([^<]+)</span>', section):
        prices.append(m.group(1).strip())
    if not prices:
        # 备用：直接匹配 ￥0.xx 或 $0.xx
        for m in re.finditer(r'[￥¥\$]([\d.]+)', section):
            prices.append(m.group(1).strip())
    return prices


def _extract_price_gradients_from_block(block: str) -> dict:
    """
    从 result-list 的 HTML 块中提取价格梯度
    结构：search-result-bor mt8 为数量/价格单元格
      - price_tiers: search-w-price 下的 search-result-bor mt8
      - hk_price: fl search-w-hk 下的 search-result-bor mt8
      - mainland_price: fl search-w-home 下的 search-result-bor mt8
    """
    out = {'price_tiers': 'N/A', 'hk_price': 'N/A', 'mainland_price': 'N/A'}
    qtys = []
    qty_start = block.find('search-w-price')
    hk_start = block.find('search-w-hk')
    if qty_start >= 0 and hk_start > qty_start:
        qty_section = block[qty_start:hk_start]
        # search-result-bor mt8 匹配数量
        for m in re.finditer(r'search-result-bor(?:\s+mt8)?[^>]*>([^<]+)<', qty_section):
            t = m.group(1).strip()
            if t and re.search(r'\d', t):
                qtys.append(t)
    # 香港价格：search-w-hk 与 search-w-home 之间（使用 $）
    hk_prices = []
    if hk_start >= 0:
        home_start = block.find('search-w-home', hk_start)
        hk_section = block[hk_start:home_start if home_start >= 0 else hk_start + 2000]
        hk_prices = _extract_prices_from_section(hk_section)
    # 内地价格：search-w-home 段（使用 ￥/¥）
    home_prices = []
    home_start = block.find('search-w-home')
    if home_start >= 0:
        home_section = block[home_start:home_start + 2500]
        home_prices = _extract_prices_from_section(home_section)
    if qtys and home_prices:
        parts = [f"{qtys[i]} ￥{home_prices[i]}" for i in range(min(len(qtys), len(home_prices)))]
        out['price_tiers'] = out['mainland_price'] = ' | '.join(parts)
    if qtys and hk_prices:
        parts = [f"{qtys[i]} ${hk_prices[i]}" for i in range(min(len(qtys), len(hk_prices)))]
        out['hk_price'] = ' | '.join(parts)
    return out


def _parse_single_block(idx: int, block: str, tag: str) -> dict | None:
    """
    从单个 result-list 的 HTML 块解析一条商品（纯函数，可并行调用）
    """
    pro_name = re.search(r'data-pro-name="([^"]+)"', tag)
    maf = re.search(r'data-maf="([^"]+)"', tag)
    stock = re.search(r'data-stock="(\d+)"', tag)
    if not (pro_name and maf and stock):
        return None
    pn, ma, st = pro_name.group(1), maf.group(1), stock.group(1)
    if '{{' in pn:
        return None
    price_data = _extract_price_gradients_from_block(block)
    return {
        'seq': idx + 1,
        'model': pn,
        'manufacturer': ma,
        'package': _extract_package_from_block(block),
        'desc': _extract_desc_from_block(block),
        'stock': st,
        'moq': _extract_moq_from_block(block),
        'price_tiers': price_data['price_tiers'],
        'hk_price': price_data['hk_price'],
        'mainland_price': price_data['mainland_price'],
        'lead_time': _extract_delivery_from_block(block),
    }


def parse_results_from_html(html: str, max_workers=8) -> list:
    """
    从 HTML 源码中解析商品列表（当 DOM 选择器失效时的备用方案）
    并行解析每个 result-list 块
    """
    pattern = r'<div[^>]*class="[^"]*result-list\s+clearfix[^"]*"[^>]*>'
    matches = list(re.finditer(pattern, html))
    blocks = []
    for i, m in enumerate(matches):
        start = m.start()
        block_end = matches[i + 1].start() if i + 1 < len(matches) else start + 15000
        block = html[start:block_end]
        tag = html[start:html.find('>', start) + 1]
        blocks.append((i, block, tag))

    def parse_one(args):
        i, block, tag = args
        return _parse_single_block(i, block, tag)

    results = []
    with ThreadPoolExecutor(max_workers=min(max_workers, len(blocks) or 1)) as ex:
        for item in ex.map(parse_one, blocks):
            if item:
                results.append(item)
    return results


def _parse_single_row_from_html(idx: int, attrs: dict, html_block: str) -> dict:
    """
    从单行 HTML 块解析商品（纯函数，可并行调用）
    attrs: {'model','manufacturer','stock'} 来自 data 属性
    """
    item = {
        'seq': idx + 1,
        'model': attrs.get('model', 'N/A'),
        'manufacturer': attrs.get('manufacturer', 'N/A'),
        'stock': attrs.get('stock', 'N/A'),
        'package': _extract_package_from_block(html_block),
        'desc': _extract_desc_from_block(html_block),
        'moq': _extract_moq_from_block(html_block),
        'lead_time': _extract_delivery_from_block(html_block),
    }
    price_data = _extract_price_gradients_from_block(html_block)
    item['price_tiers'] = price_data['price_tiers']
    item['hk_price'] = price_data['hk_price']
    item['mainland_price'] = price_data['mainland_price']
    return item


def parse_search_results(page, keyword, quiet=False, max_workers=8):
    """
    解析搜索结果页面，提取所有供应商报价信息
    并行解析每个 result-list 行
    quiet: True 时不输出日志、不保存页面（CLI 模式）
    max_workers: 解析时的最大并行线程数
    """
    html = page.html

    if not quiet:
        page_file = f'ickey_page_{datetime.now().strftime("%Y%m%d_%H%M%S")}.html'
        with open(page_file, 'w', encoding='utf-8') as f:
            f.write(html)
        print(f"页面已保存至: {page_file}", flush=True)

    # 云汉芯城搜索页：每个商品是 div.result-list
    rows = page.eles('.result-list')
    if not rows:
        rows = page.eles('css:div.result-list')
    if not rows:
        rows = page.eles('tag:div@class:result-list')

    if not rows:
        if not quiet:
            print("DOM 选择器未找到商品，尝试从 HTML 源码解析...")
            with open('debug_page.html', 'w', encoding='utf-8') as f:
                f.write(html)
        return parse_results_from_html(html, max_workers=max_workers)

    if not quiet:
        print(f"找到 {len(rows)} 个商品条目")

    # Phase 1: 主线程收集每行原始数据（DOM 访问需串行）
    row_data = []
    for idx, row in enumerate(rows):
        try:
            attrs = {
                'model': row.attr('data-pro-name') or row.attr('data-sno') or 'N/A',
                'manufacturer': row.attr('data-maf') or 'N/A',
                'stock': row.attr('data-stock') or 'N/A',
            }
            h = getattr(row, 'html', None) or getattr(row, 'outer_html', None) or ''
            if not h and hasattr(row, 'parent') and row.parent():
                try:
                    p = row.parent()
                    if p:
                        h = getattr(p, 'html', None) or getattr(p, 'outer_html', None) or ''
                except Exception:
                    pass
            if not h or 'search-w-price' not in h:
                # DOM 取价成功时用 DOM 价格，其余仍从 HTML 解析
                price_data = extract_price_gradients_from_row(row)
                if price_data['price_tiers'] != 'N/A':
                    item = _parse_single_row_from_html(idx, attrs, h or '')
                    item['price_tiers'] = price_data['price_tiers']
                    item['hk_price'] = price_data['hk_price']
                    item['mainland_price'] = price_data['mainland_price']
                    row_data.append(('direct', idx, item, None))
                    continue
            row_data.append(('parse', idx, attrs, h or ''))
        except Exception as e:
            if not quiet:
                print(f"  第 {idx+1} 行收集出错: {e}")

    # Phase 2: 并行解析
    def parse_one(args):
        kind, idx, a, b = args
        if kind == 'direct':
            return a  # 已解析好的 item
        return _parse_single_row_from_html(idx, a, b)

    results = []
    with ThreadPoolExecutor(max_workers=min(max_workers, len(row_data) or 1)) as ex:
        for item in ex.map(parse_one, row_data):
            if item:
                results.append(item)
    return results


def save_to_csv(data, filename=None):
    """
    将结果保存为CSV文件
    """
    if not data:
        print("无数据可保存")
        return
    
    if not filename:
        timestamp = datetime.now().strftime('%Y%m%d_%H%M%S')
        filename = f'ickey_results_{timestamp}.csv'
    
    # 确定字段顺序（与 JSON 输出一致）
    fieldnames = ['seq', 'model', 'manufacturer', 'package', 'desc', 'stock', 'moq', 'price_tiers', 'hk_price', 'mainland_price', 'lead_time']
    
    with open(filename, 'w', newline='', encoding='utf-8-sig') as f:
        writer = csv.DictWriter(f, fieldnames=fieldnames)
        writer.writeheader()
        writer.writerows(data)
    
    print(f"✅ 数据已保存至: {filename}")
    return filename


def run_search(keyword, headless=True, quiet=False, query_model=None, parse_workers=8, page=None):
    """
    执行搜索并返回结果列表
    headless: 是否无头模式
    quiet: True 时不输出日志、不保存页面（CLI 模式，保证 stdout 仅输出 JSON）
    query_model: 搜索关键词，用于多型号时标记结果来源，默认用 keyword
    parse_workers: 解析页面时的最大并行线程数
    page: 可选，传入已有 page 则复用，不传入则新建并在返回前 quit
    """
    own_page = page is None
    if own_page:
        page = setup_browser(headless=headless)
    try:
        if not wait_for_page_load(page, keyword, quiet=quiet):
            return []
        results = parse_search_results(page, keyword, quiet=quiet, max_workers=parse_workers)
        qm = query_model if query_model is not None else keyword
        for r in results:
            r['query_model'] = qm
        return results
    finally:
        if own_page:
            try:
                page.quit()
            except Exception:
                pass


def run_search_batch(models, headless=True, quiet=False, parse_workers=8):
    """
    串流依次搜索多个型号，复用同一浏览器，每个型号的页面解析并行
    models: 型号列表
    parse_workers: 解析每个型号页面时的最大并行线程数
    返回: 扁平列表，每项含 query_model 字段
    """
    if not models:
        return []
    if len(models) == 1:
        return run_search(models[0], headless=headless, quiet=quiet, query_model=models[0], parse_workers=parse_workers)

    all_results = []
    page = setup_browser(headless=headless)
    try:
        for model in models:
            if not wait_for_page_load(page, model, quiet=quiet):
                continue
            results = parse_search_results(page, model, quiet=quiet, max_workers=parse_workers)
            for r in results:
                r['query_model'] = model
            all_results.extend(results)
    finally:
        try:
            page.quit()
        except Exception:
            pass
    return all_results


def main():
    """
    主函数
    支持 --model 参数供 Go 后端调用，输出 JSON 到 stdout
    --model 支持逗号分隔多型号，如 --model A,B,C，将多线程并行搜索
    """
    parser = argparse.ArgumentParser(description='云汉芯城元器件搜索爬虫')
    parser.add_argument('--model', '-m', type=str, help='搜索型号，逗号分隔可传多个，串流依次搜索')
    parser.add_argument('--parse-workers', '-w', type=int, default=8, help='解析页面时的最大并行线程数，默认8')
    args = parser.parse_args()

    if args.model:
        # CLI 模式：供 Go 调用，仅输出 JSON 到 stdout（UTF-8）
        try:
            models = [m.strip() for m in args.model.split(',') if m.strip()]
            if not models:
                raise ValueError("--model 不能为空")
            results = run_search_batch(models, headless=True, quiet=True, parse_workers=args.parse_workers)
            out = json.dumps(results, ensure_ascii=False, indent=0)
            try:
                sys.stdout.reconfigure(encoding='utf-8')
            except AttributeError:
                pass
            sys.stdout.buffer.write((out + '\n').encode('utf-8'))
        except Exception as e:
            err = json.dumps({'error': str(e), 'results': []}, ensure_ascii=False)
            try:
                sys.stderr.buffer.write((err + '\n').encode('utf-8'))
            except Exception:
                sys.stderr.write(err + '\n')
            sys.exit(1)
        return

    # 交互模式
    keyword = 'LIS331DLHTR'
    print("="*50)
    print("云汉芯城元器件爬虫")
    print(f"搜索关键词: {keyword}")
    print("="*50)

    page = setup_browser(headless=False)
    try:
        if not wait_for_page_load(page, keyword):
            print("❌ 页面加载失败，退出")
            return
        results = parse_search_results(page, keyword)
        if not results:
            print("❌ 未能提取到任何数据")
            return
        print(f"\n✅ 成功提取 {len(results)} 条供应商报价")
        print("\n前3条预览:")
        for i, item in enumerate(results[:3]):
            price_str = str(item.get('price_tiers', ''))[:50]
            print(f"  [{i+1}] {item['manufacturer']} | 库存: {item['stock']} | 价格: {price_str}")
        filename = save_to_csv(results)
        print(f"\n🎉 爬取完成！结果已保存至: {filename}")
    except Exception as e:
        print(f"\n❌ 程序运行出错: {e}")
        import traceback
        traceback.print_exc()
    finally:
        input("\n按回车键退出...")


if __name__ == '__main__':
    main()