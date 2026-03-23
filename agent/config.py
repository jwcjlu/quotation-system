"""Agent 配置：环境变量 + 可选 YAML（后续可扩展）。"""
from __future__ import annotations

import os
import uuid
from dataclasses import dataclass


def _default_agent_id() -> str:
    """优先 AGENT_ID；否则尝试读 MAC 简化形式（与需求 §4.1 一致）。"""
    env = os.environ.get("AGENT_ID", "").strip()
    if env:
        return env
    try:
        mac = uuid.getnode()
        return f"{mac:012x}"
    except Exception:
        return "unknown-agent"


@dataclass
class Config:
    base_url: str  # 如 http://127.0.0.1:18080，无尾部 /
    api_key: str
    agent_id: str
    queue: str
    tags: list[str]
    data_dir: str  # 脚本包根目录 script_id/version/
    task_heartbeat_sec: float  # 两次任务心跳发起间隔（长轮询在单次请求内）
    script_sync_sec: float
    long_poll_sec: int
    http_timeout_sec: int  # 须 > long_poll_sec
    max_parallel_scripts: int  # 不同 script_id 并行上限

    @staticmethod
    def from_env() -> "Config":
        base = os.environ.get("CAICHIP_BASE_URL", "http://127.0.0.1:18080").rstrip("/")
        key = os.environ.get("CAICHIP_API_KEY", "").strip()
        if not key:
            raise ValueError("环境变量 CAICHIP_API_KEY 未设置")
        tags_raw = os.environ.get("AGENT_TAGS", "")
        tags = [t.strip() for t in tags_raw.split(",") if t.strip()]
        data_dir = os.environ.get("AGENT_DATA_DIR", os.path.join(os.getcwd(), "agent_data"))
        return Config(
            base_url=base,
            api_key=key,
            agent_id=_default_agent_id(),
            queue=os.environ.get("AGENT_QUEUE", "default").strip() or "default",
            tags=tags,
            data_dir=os.path.abspath(data_dir),
            task_heartbeat_sec=float(os.environ.get("AGENT_TASK_HEARTBEAT_SEC", "10")),
            script_sync_sec=float(os.environ.get("AGENT_SCRIPT_SYNC_SEC", "600")),
            # 空闲时约等于请求周期；可改为 50 减少请求次数（与服务端长轮询上限对齐）
            long_poll_sec=int(os.environ.get("AGENT_LONG_POLL_SEC", "10")),
            http_timeout_sec=int(os.environ.get("AGENT_HTTP_TIMEOUT_SEC", "120")),
            max_parallel_scripts=int(os.environ.get("AGENT_MAX_PARALLEL", "4")),
        )
