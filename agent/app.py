"""
Agent 主循环：任务心跳（长轮询）与脚本安装心跳分线程；任务执行与心跳异步。

同类任务（同一 script_id）串行：全局每 script_id 一把锁。
不同 script_id 并行受 max_parallel_scripts 限制。
"""
from __future__ import annotations

import logging
import os
import threading
import time
from concurrent.futures import ThreadPoolExecutor

from .client import CaichipAgentClient, LeaseConflictError
from .config import Config
from . import manifest
from . import runner

log = logging.getLogger("caichip.agent")


class AgentApp:
    def __init__(self, cfg: Config) -> None:
        self.cfg = cfg
        self.client = CaichipAgentClient(
            cfg.base_url, cfg.api_key, timeout_sec=cfg.http_timeout_sec
        )
        self._stop = threading.Event()
        self._script_locks: dict[str, threading.Lock] = {}
        self._locks_guard = threading.Lock()
        self._pool = ThreadPoolExecutor(
            max_workers=max(1, cfg.max_parallel_scripts), thread_name_prefix="task"
        )

    def stop(self) -> None:
        self._stop.set()
        self._pool.shutdown(wait=False, cancel_futures=True)

    def _lock_for_script(self, script_id: str) -> threading.Lock:
        with self._locks_guard:
            if script_id not in self._script_locks:
                self._script_locks[script_id] = threading.Lock()
            return self._script_locks[script_id]

    def run(self) -> None:
        os.makedirs(self.cfg.data_dir, exist_ok=True)
        th_task = threading.Thread(target=self._loop_task_heartbeat, name="task_hb", daemon=True)
        th_sync = threading.Thread(target=self._loop_script_sync, name="script_sync", daemon=True)
        th_task.start()
        th_sync.start()
        log.info(
            "caichip agent started agent_id=%s base=%s data_dir=%s",
            self.cfg.agent_id,
            self.cfg.base_url,
            self.cfg.data_dir,
        )
        try:
            while not self._stop.is_set():
                time.sleep(0.5)
        except KeyboardInterrupt:
            self.stop()

    def _loop_task_heartbeat(self) -> None:
        while not self._stop.is_set():
            try:
                scripts = manifest.scan_installed_scripts(self.cfg.data_dir)
                body = {
                    "protocol_version": "1.0",
                    "agent_id": self.cfg.agent_id,
                    "queue": self.cfg.queue,
                    "tags": self.cfg.tags,
                    "installed_scripts": scripts,
                    "long_poll_timeout_sec": self.cfg.long_poll_sec,
                }
                data = self.client.task_heartbeat(body)
                tasks = data.get("tasks") or []
                for t in tasks:
                    self._pool.submit(self._run_one_task, t)
            except Exception as e:
                log.exception("task heartbeat error: %s", e)
                time.sleep(3)
            time.sleep(0.05)

    def _loop_script_sync(self) -> None:
        while not self._stop.is_set():
            try:
                scripts = manifest.scan_installed_scripts(self.cfg.data_dir)
                body = {
                    "protocol_version": "1.0",
                    "agent_id": self.cfg.agent_id,
                    "queue": self.cfg.queue,
                    "tags": self.cfg.tags,
                    "scripts": scripts,
                    "long_poll_timeout_sec": min(self.cfg.long_poll_sec, 55),
                }
                self.client.script_sync_heartbeat(body)
            except Exception as e:
                log.exception("script sync heartbeat error: %s", e)
            if self._stop.wait(timeout=self.cfg.script_sync_sec):
                break

    def _run_one_task(self, task: dict) -> None:
        script_id = task.get("script_id") or ""
        lock = self._lock_for_script(script_id)
        lock.acquire()
        try:
            status, code, so, se = runner.run_task(data_dir=self.cfg.data_dir, task=task)
            payload = {
                "protocol_version": "1.0",
                "agent_id": self.cfg.agent_id,
                "task_id": task.get("task_id"),
                "status": status,
                "lease_id": task.get("lease_id") or "",
                "attempt": 1,
                "stdout_tail": so,
                "stderr_tail": se,
                "result": {
                    "exit_code": code,
                    "script_id": script_id,
                    "version": task.get("version"),
                },
            }
            try:
                self.client.task_result(payload)
            except LeaseConflictError as e:
                log.warning("task result rejected (409): %s", e)
        finally:
            lock.release()
