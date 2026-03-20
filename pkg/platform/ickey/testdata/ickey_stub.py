#!/usr/bin/env python3
"""测试用 stub：输出固定 JSON，不启动浏览器，支持多型号"""
import json
import sys

def stub_item(model):
    return {
        "seq": 1,
        "model": model,
        "manufacturer": "TI",
        "package": "TSSOP-16",
        "desc": "8-Bit Shift Registers",
        "stock": "10000",
        "moq": "1",
        "price_tiers": "1+ ￥0.88 | 10+ ￥0.75 | 100+ ￥0.65",
        "hk_price": "1+ $0.12",
        "mainland_price": "1+ ￥0.88 | 10+ ￥0.75 | 100+ ￥0.65",
        "lead_time": "7-9工作日",
        "query_model": model,
    }

if __name__ == "__main__":
    if "--model" not in sys.argv:
        sys.stderr.buffer.write(b"usage: ickey_stub.py --model <model>[,model2,...]\n")
        sys.exit(1)
    idx = sys.argv.index("--model")
    if idx + 1 >= len(sys.argv):
        sys.stderr.buffer.write(b"--model requires value\n")
        sys.exit(1)
    model_arg = sys.argv[idx + 1]
    models = [m.strip() for m in model_arg.split(",") if m.strip()]
    if not models:
        sys.stderr.buffer.write(b"no models\n")
        sys.exit(1)
    results = []
    for m in models:
        results.append(stub_item(m))
    sys.stdout.buffer.write((json.dumps(results, ensure_ascii=False) + "\n").encode("utf-8"))
