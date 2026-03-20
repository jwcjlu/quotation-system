# 云汉芯城 (Ickey) 平台搜索设计

## 概述

Ickey 平台搜索通过调用 Python 爬虫脚本 `ickey_crawler.py` 获取报价数据，Go 后端以子进程方式执行脚本并解析 JSON 输出。

## 调用流程

```
Go SearchUseCase
    → ickey.Client.Search(model, quantity)
    → exec: python ickey_crawler.py --model {model}
    → 解析 stdout JSON
    → 转换为 platform.Quote[]
```

## 爬虫 CLI 接口

```bash
python ickey_crawler.py --model SN74HC595PWR
```

**输出**：JSON 数组到 stdout，UTF-8 编码。单条结构：

```json
{
  "序号": 1,
  "型号": "SN74HC595PWR",
  "厂牌": "TI",
  "封装": "TSSOP-16",
  "描述": "...",
  "库存": "10000",
  "起订量": "1",
  "价格梯度": "1+ ￥0.88 | 10+ ￥0.75",
  "中国香港交货": "1+ $0.12",
  "内地交货(含增值税)": "1+ ￥0.88 | 10+ ￥0.75",
  "货期": "7-9工作日"
}
```

## 配置 (configs/config.yaml)

```yaml
platform:
  ickey:
    search_url: "https://search.ickey.cn/"
    timeout: 15
    crawler_path: "python"      # 或 python3
    crawler_script: "ickey_crawler.py"  # 相对路径以工作目录为准
```

## 依赖

- Python 3.7+
- DrissionPage
- 从项目根目录启动 Go 服务，或配置 `crawler_script` 为绝对路径
