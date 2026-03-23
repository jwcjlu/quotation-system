"""caichip Agent HTTP 客户端（与 API 协议对齐）。"""
from __future__ import annotations

import json
from typing import Any
from urllib.parse import urljoin

import requests


class CaichipAgentClient:
    def __init__(self, base_url: str, api_key: str, timeout_sec: int = 120) -> None:
        self.base_url = base_url.rstrip("/") + "/"
        self.timeout_sec = timeout_sec
        self._session = requests.Session()
        self._session.headers.update(
            {
                "Authorization": f"Bearer {api_key}",
                "Content-Type": "application/json",
            }
        )

    def task_heartbeat(self, body: dict[str, Any]) -> dict[str, Any]:
        url = urljoin(self.base_url, "api/v1/agent/task/heartbeat")
        r = self._session.post(url, data=json.dumps(body), timeout=self.timeout_sec)
        r.raise_for_status()
        return r.json()

    def script_sync_heartbeat(self, body: dict[str, Any]) -> dict[str, Any]:
        url = urljoin(self.base_url, "api/v1/agent/script-sync/heartbeat")
        r = self._session.post(url, data=json.dumps(body), timeout=self.timeout_sec)
        r.raise_for_status()
        return r.json()

    def task_result(self, body: dict[str, Any]) -> dict[str, Any]:
        url = urljoin(self.base_url, "api/v1/agent/task/result")
        r = self._session.post(url, data=json.dumps(body), timeout=self.timeout_sec)
        if r.status_code == 409:
            raise LeaseConflictError(r.text)
        r.raise_for_status()
        return r.json()


class LeaseConflictError(Exception):
    """HTTP 409 租约冲突。"""

    pass
