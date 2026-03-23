"""
最小 Agent 客户端：任务心跳 + 结果上报（用于验证 caichip Agent HTTP API）。

用法（需先启动 caichip 且 configs/config.yaml 中 agent.enabled=true）:
  set CAICHIP_API_KEY=change-me-in-production
  python scripts/agent_heartbeat_smoke.py http://127.0.0.1:18080

环境变量:
  CAICHIP_API_KEY  必填，与配置 agent.api_keys 一致
  AGENT_ID         可选，默认 aabbccddeeff
"""
from __future__ import annotations

import json
import os
import sys
import time

import requests

DEFAULT_AGENT_ID = "aabbccddeeff"


def main() -> None:
    base = sys.argv[1] if len(sys.argv) > 1 else "http://127.0.0.1:18080"
    base = base.rstrip("/")
    key = os.environ.get("CAICHIP_API_KEY", "")
    if not key:
        print("请设置环境变量 CAICHIP_API_KEY", file=sys.stderr)
        sys.exit(1)
    agent_id = os.environ.get("AGENT_ID", DEFAULT_AGENT_ID)
    headers = {"Authorization": f"Bearer {key}", "Content-Type": "application/json"}
    body = {
        "protocol_version": "1.0",
        "agent_id": agent_id,
        "queue": "default",
        "tags": [],
        "installed_scripts": [
            {"script_id": "demo", "version": "1.0.0", "env_status": "ready"},
        ],
        "long_poll_timeout_sec": 5,
    }
    url = f"{base}/api/v1/agent/task/heartbeat"
    r = requests.post(url, headers=headers, data=json.dumps(body), timeout=30)
    print("status", r.status_code, r.text[:500])
    r.raise_for_status()
    data = r.json()
    tasks = data.get("tasks") or []
    print("tasks:", len(tasks))
    if tasks:
        t = tasks[0]
        res_url = f"{base}/api/v1/agent/task/result"
        res_body = {
            "protocol_version": "1.0",
            "agent_id": agent_id,
            "task_id": t["task_id"],
            "status": "success",
            "lease_id": t.get("lease_id") or "",
            "attempt": 1,
            "stdout_tail": "",
            "stderr_tail": "",
            "result": {"ok": True, "smoke": True},
        }
        r2 = requests.post(res_url, headers=headers, data=json.dumps(res_body), timeout=30)
        print("result status", r2.status_code, r2.text)
        r2.raise_for_status()


if __name__ == "__main__":
    main()
