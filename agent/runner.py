"""任务子进程执行：超时、进程组终止、stdout/stderr 采集。"""
from __future__ import annotations

import json
import logging
import os
import signal
import subprocess
import sys
import threading
import time
from pathlib import Path
from typing import Any

from . import versionutil

log = logging.getLogger("caichip.agent.runner")

# 默认任务超时（秒），与需求 §6.2 一致
DEFAULT_TIMEOUT_SEC = 300


def _find_version_root(data_dir: str, script_id: str, version: str) -> Path | None:
    """匹配 script_id 下与 task.version 规范化一致的目录名（支持 v 前缀目录）。"""
    base = Path(data_dir) / script_id
    if not base.is_dir():
        return None
    for name in os.listdir(base):
        p = base / name
        if not p.is_dir():
            continue
        if versionutil.equal(name, version):
            return p
    return None


def resolve_entry_script(root: Path, entry_file: str | None) -> Path | None:
    if entry_file:
        p = (root / entry_file).resolve()
        try:
            p.relative_to(root.resolve())
        except ValueError:
            return None
        if p.is_file():
            return p
        return None
    for name in ("main.py", "run.py"):
        p = root / name
        if p.is_file():
            return p
    return None


def run_task(
    *,
    data_dir: str,
    task: dict[str, Any],
) -> tuple[str, int | None, str, str]:
    """
    返回 (status, exit_code, stdout_tail, stderr_tail)
    status: success | failed | timeout | skipped
    """
    task_id = task.get("task_id") or ""
    script_id = task.get("script_id") or ""
    version = task.get("version") or ""
    root = _find_version_root(data_dir, script_id, version)
    if root is None:
        log.warning(
            "task skip task_id=%s script_id=%s reason=no_package_dir",
            task_id,
            script_id,
        )
        return "skipped", None, "", "script package directory not found"
    vf = root / "version.txt"
    if not vf.is_file():
        log.warning(
            "task skip task_id=%s script_id=%s reason=no_version_txt path=%s",
            task_id,
            script_id,
            root,
        )
        return "skipped", None, "", "local package missing"
    try:
        local_ver = vf.read_text(encoding="utf-8").strip()
    except OSError as e:
        log.warning(
            "task skip task_id=%s script_id=%s reason=read_version_txt err=%s",
            task_id,
            script_id,
            e,
        )
        return "skipped", None, "", str(e)
    if not versionutil.equal(local_ver, version):
        log.warning(
            "task skip task_id=%s script_id=%s reason=version_mismatch local=%s want=%s",
            task_id,
            script_id,
            local_ver,
            version,
        )
        return "skipped", None, "", "version mismatch with version.txt"

    entry = task.get("entry_file")
    entry_str = None if entry is None else str(entry)
    py_file = resolve_entry_script(root, entry_str)
    if py_file is None:
        log.error(
            "task fail task_id=%s script_id=%s reason=no_entry_script root=%s",
            task_id,
            script_id,
            root,
        )
        return "failed", None, "", "no entry script (main.py/run.py or entry_file)"

    timeout_sec = int(task.get("timeout_sec") or 0)
    if timeout_sec <= 0:
        timeout_sec = DEFAULT_TIMEOUT_SEC

    params = task.get("params") or {}
    env = os.environ.copy()
    env["CAICHIP_TASK_PARAMS"] = json.dumps(params, ensure_ascii=False)
    env["PYTHONUNBUFFERED"] = "1"

    py = sys.executable
    cmd = [py, str(py_file)]
    argv = task.get("argv") or []
    if isinstance(argv, list):
        cmd.extend(str(a) for a in argv)

    try:
        entry_display = str(py_file.resolve().relative_to(root.resolve()))
    except ValueError:
        entry_display = py_file.name

    log.info(
        "task exec start task_id=%s script_id=%s version=%s cwd=%s entry=%s timeout_sec=%s cmd=%s",
        task_id,
        script_id,
        version,
        root,
        entry_display,
        timeout_sec,
        cmd,
    )

    stdout_chunks: list[str] = []
    stderr_chunks: list[str] = []

    creationflags = 0
    preexec_fn = None
    if sys.platform == "win32":
        creationflags = getattr(subprocess, "CREATE_NEW_PROCESS_GROUP", 0)
    else:
        preexec_fn = os.setsid  # type: ignore[assignment]

    t0 = time.monotonic()
    proc = subprocess.Popen(
        cmd,
        cwd=str(root),
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        encoding="utf-8",
        errors="replace",
        env=env,
        creationflags=creationflags,
        preexec_fn=preexec_fn,
    )

    out_t = threading.Thread(target=_drain, args=(proc.stdout, stdout_chunks))
    err_t = threading.Thread(target=_drain, args=(proc.stderr, stderr_chunks))
    out_t.start()
    err_t.start()

    try:
        proc.wait(timeout=timeout_sec)
        rc = proc.returncode
        status = "success" if rc == 0 else "failed"
    except subprocess.TimeoutExpired:
        log.warning(
            "task exec timeout task_id=%s script_id=%s after_sec=%s",
            task_id,
            script_id,
            timeout_sec,
        )
        _kill_tree(proc.pid)
        status = "timeout"
        rc = None
    finally:
        try:
            if proc.poll() is None:
                _kill_tree(proc.pid)
        except Exception:
            pass
        out_t.join(timeout=5)
        err_t.join(timeout=5)

    so = "".join(stdout_chunks)
    se = "".join(stderr_chunks)
    elapsed_ms = int((time.monotonic() - t0) * 1000)
    log.info(
        "task exec end task_id=%s script_id=%s status=%s exit_code=%s duration_ms=%s stdout_bytes=%s stderr_bytes=%s",
        task_id,
        script_id,
        status,
        rc,
        elapsed_ms,
        len(so.encode("utf-8", errors="replace")),
        len(se.encode("utf-8", errors="replace")),
    )
    if status != "success" and se.strip():
        log.warning(
            "task stderr tail task_id=%s: %s",
            task_id,
            _tail(se.strip().replace("\n", " "), 500),
        )
    return status, rc, _tail(so, 32000), _tail(se, 32000)


def _drain(pipe: Any, buf: list[str]) -> None:
    if not pipe:
        return
    for line in iter(pipe.readline, ""):
        buf.append(line)
    try:
        pipe.close()
    except Exception:
        pass


def _tail(s: str, max_len: int) -> str:
    if len(s) <= max_len:
        return s
    return s[-max_len:]


def _kill_tree(pid: int) -> None:
    if pid <= 0:
        return
    if sys.platform == "win32":
        subprocess.run(
            ["taskkill", "/F", "/T", "/PID", str(pid)],
            capture_output=True,
            text=True,
            timeout=30,
        )
    else:
        try:
            os.killpg(os.getpgid(pid), signal.SIGTERM)
        except ProcessLookupError:
            return
        time.sleep(0.3)
        try:
            os.killpg(os.getpgid(pid), signal.SIGKILL)
        except ProcessLookupError:
            pass
