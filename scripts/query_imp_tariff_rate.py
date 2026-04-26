from DrissionPage import ChromiumPage, ChromiumOptions
import json
import time
import pandas as pd
from typing import Dict, List, Optional, Any


# 海关查询页多为「税则号 + 商品名称」双输入；placeholder / 组件库文案常与硬编码不一致
_INPUT_HINTS_CODE = (
    "税则",
    "号列",
    "税则号",
    "商品编码",
    "hs编码",
    "h.s",
    "codets",
    "code_ts",
)
_INPUT_HINTS_NAME = ("商品", "名称", "货品", "品名", "货物", "gname", "货品名称")


class CustomsTariffQuery:
    """海关关税税率查询类"""
    
    def __init__(self, headless: bool = False, proxy: str = None):
        """
        初始化查询器
        
        参数:
        headless: 是否无头模式（不显示浏览器界面）
        proxy: 代理服务器，格式如 'http://127.0.0.1:1080'
        """
        self.url = "https://online.customs.gov.cn/ociswebserver/pages/jckspsl/index.html"
        self.api_url = "https://online.customs.gov.cn/ociswebserver/ocis/taxRateQuery/query/queryImpTariffRate"
        
        # 配置浏览器选项
        co = ChromiumOptions()
        if headless:
            co.headless(True)
        if proxy:
            co.set_proxy(proxy)
        
        # 设置请求头
        co.set_user_agent('Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36')
        
        # 初始化页面对象
        self.page = ChromiumPage(addr_or_opts=co)
        
        # 设置超时
        self.page.set.timeouts(30)
        
    def login(self):
        """访问首页获取必要的Cookie"""
        print("正在访问查询页面...")
        self.page.get(self.url)
        
        # 等待页面加载
        self.page.wait.load_start()
        time.sleep(2)
        
        # 检查是否成功加载
        if "进口税则" in self.page.title or "进口" in self.page.title:
            print("页面加载成功")
        else:
            print(f"页面标题: {self.page.title}")
        
        # 获取Cookie
        cookies = self.page.cookies()
        print(f"已获取Cookie数量: {len(cookies)}")
        
        return True
    
    def get_cookie_str(self) -> str:
        """获取Cookie字符串"""
        cookies = self.page.cookies()
        cookie_str = '; '.join([f"{c['name']}={c['value']}" for c in cookies])
        return cookie_str
    
    def query_by_code(self, code_ts: str, page_size: int = 20) -> Dict:
        """
        通过税则号查询
        
        参数:
        code_ts: 税则号列，如'4805911000'
        page_size: 每页条数
        """
        # 构造请求参数
        params = {
            "pageNum": 1,
            "pageSize": page_size,
            "codeTs": code_ts
        }
        
        return self._make_api_request(params)
    
    def query_by_name(self, g_name: str, page_size: int = 20) -> Dict:
        """
        通过商品名称查询
        
        参数:
        g_name: 商品名称
        page_size: 每页条数
        """
        params = {
            "pageNum": 1,
            "pageSize": page_size,
            "gName": g_name
        }
        
        return self._make_api_request(params)
    
    def _make_api_request(self, params: Dict) -> Dict:
        """发起API请求"""
        # 设置请求头
        headers = {
            'Accept': 'application/json, text/plain, */*',
            'Accept-Encoding': 'gzip, deflate, br',
            'Accept-Language': 'zh-CN,zh;q=0.9',
            'Connection': 'keep-alive',
            'Content-Type': 'application/json;charset=UTF-8',
            'Host': 'online.customs.gov.cn',
            'Origin': 'https://online.customs.gov.cn',
            'Referer': 'https://online.customs.gov.cn/ociswebserver/pages/jckspsl/index.html',
            'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36',
            'X-Requested-With': 'XMLHttpRequest'
        }
        
        # 添加Cookie
        cookie_str = self.get_cookie_str()
        if cookie_str:
            headers['Cookie'] = cookie_str
        
        try:
            print(f"正在查询: {params}")
            
            # 使用DrissionPage的session模式发起请求
            response = self.page.session.post(
                self.api_url,
                headers=headers,
                json=params,
                timeout=30
            )
            
            print(f"响应状态码: {response.status_code}")
            
            if response.status_code == 200:
                data = response.json()
                return self._process_response(data)
            else:
                print(f"请求失败，状态码: {response.status_code}")
                return {"error": f"HTTP {response.status_code}", "response": response.text}
                
        except Exception as e:
            print(f"请求发生异常: {e}")
            return {"error": str(e)}
    
    def _process_response(self, data: Dict) -> Dict:
        """处理API响应"""
        result = {
            "success": False,
            "message": "",
            "data": [],
            "total": 0
        }
        
        # 检查响应结构
        if isinstance(data, dict):
            if data.get("statue") == "1":
                result["success"] = True
                result["message"] = "查询成功"
                result["data"] = data.get("data", [])
                result["total"] = data.get("totalCount", 0)
            elif "res" in data:
                # 处理嵌套的res字段
                res_str = data.get("res", "{}")
                try:
                    if isinstance(res_str, str):
                        res_data = json.loads(res_str)
                    else:
                        res_data = res_str
                    
                    if res_data.get("statue") == 1:
                        result["success"] = True
                        result["message"] = res_data.get("message", "查询成功")
                        result["data"] = res_data.get("data", [])
                        result["total"] = res_data.get("totalCount", 0)
                    else:
                        result["message"] = res_data.get("message", "查询失败")
                except json.JSONDecodeError:
                    result["message"] = f"JSON解析错误: {res_str}"
            else:
                result["message"] = f"未知响应格式: {data}"
        else:
            result["message"] = f"响应不是JSON: {type(data)}"
        
        return result

    def _usable_form_inputs(self) -> List[Any]:
        """可见、可用于填写的 input（排除 hidden/button 等）。"""
        usable: List[Any] = []
        for inp in self.page.eles("tag:input"):
            t = (inp.attr("type") or "text").lower()
            if t in ("hidden", "submit", "button", "image", "reset", "file", "checkbox", "radio"):
                continue
            try:
                if not inp.states.is_displayed:
                    continue
            except Exception:
                continue
            usable.append(inp)
        return usable

    def _input_hint_text(self, inp) -> str:
        return " ".join(
            [
                inp.attr("placeholder") or "",
                inp.attr("name") or "",
                inp.attr("id") or "",
                inp.attr("class") or "",
            ]
        ).lower()

    def _pick_search_input(self, inputs: List[Any], code_ts: Optional[str], g_name: Optional[str]):
        """
        在多个 input 中选出本次查询应对准的输入框。
        双字段页常见顺序：税则号列在前、商品名称在后。
        """
        if not inputs:
            return None

        def score_code(inp) -> int:
            h = self._input_hint_text(inp)
            return sum(1 for k in _INPUT_HINTS_CODE if k.lower() in h)

        def score_name(inp) -> int:
            h = self._input_hint_text(inp)
            return sum(1 for k in _INPUT_HINTS_NAME if k.lower() in h)

        # 旧逻辑：与历史文案完全匹配
        for inp in inputs:
            ph = inp.attr("placeholder") or ""
            if ph and ("税则号列" in ph or "货品名称" in ph):
                return inp

        if g_name and not code_ts:
            best = max(inputs, key=score_name)
            if score_name(best) > 0:
                print(f"按文案匹配到商品名称输入框: {self._input_hint_text(best)[:120]!r}")
                return best
            if len(inputs) >= 2:
                print("未匹配到商品名称占位文案，按双输入框约定使用第 2 个可见输入框")
                return inputs[1]
            print("仅 1 个可见输入框，用作商品名称输入")
            return inputs[0]

        if code_ts and not g_name:
            best = max(inputs, key=score_code)
            if score_code(best) > 0:
                print("按文案匹配到税则号输入框")
                return best
            if len(inputs) >= 2:
                print("未匹配到税则号占位文案，按双输入框约定使用第 1 个可见输入框")
                return inputs[0]
            print("仅 1 个可见输入框，用作税则号输入")
            return inputs[0]

        # 两者都未指定或都指定：优先税则号框，否则第一个
        best = max(inputs, key=score_code)
        if score_code(best) > 0:
            return best
        return inputs[0]

    def query_using_browser_automation(self, code_ts: str = None, g_name: str = None):
        """
        使用浏览器自动化模拟用户操作进行查询
        
        参数:
        code_ts: 税则号
        g_name: 商品名称
        """
        print("使用浏览器自动化查询...")
        
        try:
            # 查找输入框和查询按钮
            # 由于页面是动态生成的，我们需要等待元素加载
            self.page.wait.ele_displayed('input', timeout=10)

            raw_inputs = self.page.eles("tag:input")
            print(f"找到 {len(raw_inputs)} 个 input 节点")

            inputs = self._usable_form_inputs()
            if not inputs:
                # 个别页面 is_displayed 与真实可交互不一致，回退为仅按 type 过滤
                inputs = [
                    inp
                    for inp in raw_inputs
                    if (inp.attr("type") or "text").lower()
                    not in ("hidden", "submit", "button", "image", "reset", "file", "checkbox", "radio")
                ]
                print(f"可见性过滤后无可用框，回退得到 {len(inputs)} 个非 hidden 的 input")
            else:
                print(f"其中可见可编辑 {len(inputs)} 个（已排除 hidden 等）")
            for i, inp in enumerate(inputs):
                print(
                    f"  [{i}] type={inp.attr('type')!r} "
                    f"placeholder={inp.attr('placeholder')!r} "
                    f"name={inp.attr('name')!r} id={inp.attr('id')!r}"
                )

            search_input = self._pick_search_input(inputs, code_ts, g_name)
            if not search_input:
                cand = self.page.eles("#codeTs", timeout=2)
                search_input = cand[0] if cand else None
                if not search_input:
                    cand = self.page.eles('input[id*="code"]', timeout=2)
                    search_input = cand[0] if cand else None
                if not search_input:
                    for name_pat in ('input[name*="gName"]', 'input[name*="gname"]'):
                        cand = self.page.eles(name_pat, timeout=2)
                        if cand:
                            search_input = cand[0]
                            break

            # 查找所有按钮
            buttons = self.page.eles('tag:button')
            print(f"找到 {len(buttons)} 个按钮")
            
            # 查找查询按钮
            search_btn = None
            for btn in buttons:
                text = btn.text
                if text and ('查询' in text or '搜索' in text):
                    search_btn = btn
                    print(f"找到查询按钮: {text}")
                    break
            
            if not search_btn:
                cand = self.page.eles('button:contains(查询)', timeout=2)
                search_btn = cand[0] if cand else None
                if not search_btn:
                    cand = self.page.eles('button:contains(搜索)', timeout=2)
                    search_btn = cand[0] if cand else None
            
            if search_input and search_btn:
                # 清空输入框
                search_input.clear()
                
                # 输入查询条件
                if code_ts:
                    print(f"输入税则号: {code_ts}")
                    search_input.input(code_ts)
                elif g_name:
                    print(f"输入商品名称: {g_name}")
                    search_input.input(g_name)
                else:
                    print("未提供查询条件，使用示例查询")
                    search_input.input("4805911000")
                
                # 点击查询按钮
                search_btn.click()
                print("已点击查询按钮")
                
                # 等待查询结果
                time.sleep(3)
                
                # 尝试获取表格数据
                return self._extract_table_data()
            else:
                print("未找到查询输入框或按钮")
                return {
                    "success": False,
                    "message": "页面元素未找到（输入框或查询按钮）",
                    "error": "页面元素未找到",
                }
                
        except Exception as e:
            print(f"浏览器自动化查询失败: {e}")
            return {"error": str(e)}
    
    def _extract_table_data(self) -> Dict:
        """从页面提取表格数据"""
        result = {
            "success": False,
            "message": "",
            "data": []
        }
        
        try:
            # 等待表格加载
            self.page.wait.ele_displayed('table', timeout=10)
            
            # 查找所有表格
            tables = self.page.eles('tag:table')
            print(f"找到 {len(tables)} 个表格")
            
            for i, table in enumerate(tables):
                print(f"\n表格 {i+1}:")
                
                # 尝试获取表头
                headers = []
                th_elements = table.eles('tag:th')
                if th_elements:
                    headers = [th.text.strip() for th in th_elements]
                else:
                    # 如果没有th，尝试第一行作为表头
                    first_row = table.ele('tag:tr', timeout=0)
                    if first_row:
                        td_elements = first_row.eles('tag:td')
                        headers = [td.text.strip() for td in td_elements]
                
                if headers:
                    print(f"表头: {headers}")
                    
                    # 获取所有行
                    rows = table.eles('tag:tr')[1:]  # 跳过表头
                    
                    for row in rows:
                        cells = row.eles('tag:td')
                        if cells and len(cells) == len(headers):
                            row_data = {}
                            for j, cell in enumerate(cells):
                                row_data[headers[j]] = cell.text.strip()
                            result["data"].append(row_data)
            
            if result["data"]:
                result["success"] = True
                result["message"] = f"成功提取 {len(result['data'])} 条数据"
            else:
                result["message"] = "未提取到表格数据"
                
        except Exception as e:
            result["message"] = f"提取表格数据失败: {e}"
        
        return result
    
    def save_to_excel(self, data: List[Dict], filename: str = "关税查询结果.xlsx"):
        """保存结果到Excel文件"""
        if not data:
            print("没有数据可保存")
            return
        
        df = pd.DataFrame(data)
        df.to_excel(filename, index=False)
        print(f"结果已保存到: {filename}")
        return filename
    
    def close(self):
        """关闭浏览器"""
        self.page.quit()
        print("浏览器已关闭")


def main():
    """主函数"""
    print("=" * 50)
    print("海关关税税率查询工具 (DrissionPage实现)")
    print("=" * 50)
    
    # 创建查询器实例
    # 设置 headless=False 可以看到浏览器界面，方便调试
    # 设置 headless=True 在后台运行
    query = CustomsTariffQuery(headless=False)
    
    try:
        # 步骤1: 登录获取Cookie
        if not query.login():
            print("登录失败")
            return
        
        # 步骤2: 选择查询方式
        print("\n请选择查询方式:")
        print("1. 通过税则号查询")
        print("2. 通过商品名称查询")
        print("3. 使用浏览器自动化查询")
        
        choice = input("请输入选项 (1-3): ").strip()
        
        if choice == "1":
            # 通过税则号查询
            code_ts = input("请输入税则号列 (例如: 4805911000): ").strip()
            if not code_ts:
                code_ts = "4805911000"  # 默认值
            
            result = query.query_by_code(code_ts)
            
        elif choice == "2":
            # 通过商品名称查询
            g_name = input("请输入商品名称: ").strip()
            if not g_name:
                g_name = "电解电容器原纸"  # 默认值
            
            result = query.query_by_name(g_name)
            
        elif choice == "3":
            # 使用浏览器自动化
            sub_choice = input("1. 按税则号查询\n2. 按商品名称查询\n请选择: ").strip()
            
            if sub_choice == "1":
                code_ts = input("请输入税则号列: ").strip()
                result = query.query_using_browser_automation(code_ts=code_ts)
            else:
                g_name = input("请输入商品名称: ").strip()
                result = query.query_using_browser_automation(g_name=g_name)
        else:
            print("无效选择，使用默认税则号查询")
            result = query.query_by_code("4805911000")
        
        # 步骤3: 处理结果
        print("\n" + "=" * 50)
        print("查询结果:")
        print("=" * 50)
        
        if result.get("success"):
            data = result.get("data", [])
            total = result.get("total", 0)
            
            print(f"查询成功! 共找到 {total} 条记录")
            print(f"本次返回 {len(data)} 条记录")
            
            if data:
                # 显示前5条记录
                print("\n前5条记录:")
                for i, item in enumerate(data[:5]):
                    print(f"\n记录 {i+1}:")
                    for key, value in item.items():
                        print(f"  {key}: {value}")
                
                # 保存到Excel
                save_choice = input("\n是否保存到Excel文件? (y/n): ").strip().lower()
                if save_choice == 'y':
                    filename = input("请输入文件名 (默认: 关税查询结果.xlsx): ").strip()
                    if not filename:
                        filename = "关税查询结果.xlsx"
                    query.save_to_excel(data, filename)
            else:
                print("未找到相关记录")
        else:
            msg = result.get("message") or result.get("error") or repr(result)
            print(f"查询失败: {msg}")
            
    except KeyboardInterrupt:
        print("\n用户中断操作")
    except Exception as e:
        print(f"程序运行异常: {e}")
    finally:
        # 步骤4: 关闭浏览器
        close_choice = input("\n是否关闭浏览器? (y/n): ").strip().lower()
        if close_choice == 'y':
            query.close()
        else:
            print("浏览器保持打开状态，可继续操作")


def batch_query_example():
    """批量查询示例"""
    print("批量查询示例")
    
    # 税则号列表
    codes = [
        "4805911000",  # 电解电容器原纸
        "8504909000",  # 变压器零件
        "8543709990",  # 其他具有独立功能的电气设备
    ]
    
    query = CustomsTariffQuery(headless=True)  # 无头模式
    
    try:
        # 登录
        query.login()
        
        all_results = []
        
        for code in codes:
            print(f"\n查询税则号: {code}")
            result = query.query_by_code(code)
            
            if result.get("success"):
                data = result.get("data", [])
                all_results.extend(data)
                print(f"  找到 {len(data)} 条记录")
            else:
                print(f"  查询失败: {result.get('message')}")
            
            time.sleep(2)  # 避免请求过快
        
        # 保存所有结果
        if all_results:
            df = pd.DataFrame(all_results)
            filename = f"批量查询结果_{time.strftime('%Y%m%d_%H%M%S')}.xlsx"
            df.to_excel(filename, index=False)
            print(f"\n所有结果已保存到: {filename}")
            
    finally:
        query.close()


if __name__ == "__main__":
    # 运行主程序
    main()
    
    # 或者运行批量查询示例
    # batch_query_example()