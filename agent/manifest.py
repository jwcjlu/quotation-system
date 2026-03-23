"""扫描 data_dir 下 script_id/version/ 与 version.txt，生成 installed_scripts。"""
from __future__ import annotations

import os

from . import versionutil


def scan_installed_scripts(data_dir: str) -> list[dict]:
    """
    目录结构: data_dir/<script_id>/<version>/version.txt
    返回 [{"script_id","version","env_status"}, ...]
    """
    out: list[dict] = []
    if not os.path.isdir(data_dir):
        return out
    for script_id in os.listdir(data_dir):
        sd = os.path.join(data_dir, script_id)
        if not os.path.isdir(sd):
            continue
        for ver in os.listdir(sd):
            vd = os.path.join(sd, ver)
            vf = os.path.join(vd, "version.txt")
            if not os.path.isfile(vf):
                continue
            try:
                raw = open(vf, encoding="utf-8").read().strip()
            except OSError:
                continue
            nv = versionutil.normalize(raw)
            if not nv:
                continue
            # 简单认为有 version.txt 即 ready（未做 venv/pip 检测）
            out.append(
                {
                    "script_id": script_id,
                    "version": raw,
                    "env_status": "ready",
                }
            )
    return out
