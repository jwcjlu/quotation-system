"""
无浏览器：校验 ``icgoo_yidun_solver._ddd_auto_skip_opencv_ensemble`` 与 auto 下
``_auto_use_dddd_only`` 语义一致（易盾可仅 dddd；阿里云须让 OpenCV 参与 ensemble）。

运行: python scripts/test_icgoo_yidun_ddd_only_gate.py
"""
from __future__ import annotations

import os
import sys

_SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
if _SCRIPT_DIR not in sys.path:
    sys.path.insert(0, _SCRIPT_DIR)

from icgoo_yidun_solver import _ddd_auto_skip_opencv_ensemble  # noqa: E402


def main() -> None:
    # 易盾：不区分横差，恒跳过 ensemble 决选前的 OpenCV 比较
    assert _ddd_auto_skip_opencv_ensemble(72, 164, is_aliyun_preprocessed=False) is True
    assert _ddd_auto_skip_opencv_ensemble(0, 200, is_aliyun_preprocessed=False) is True

    # 阿里云：两路均有值时恒不跳过 ensemble（横差大或小都可能同错）
    assert _ddd_auto_skip_opencv_ensemble(72, 164, is_aliyun_preprocessed=True) is False
    assert _ddd_auto_skip_opencv_ensemble(164, 72, is_aliyun_preprocessed=True) is False
    assert _ddd_auto_skip_opencv_ensemble(127, 161, is_aliyun_preprocessed=True) is False
    assert _ddd_auto_skip_opencv_ensemble(150, 150, is_aliyun_preprocessed=True) is False

    # xf/xt 缺一则与 detect_gap 一致：不会形成「双 plausible + 阿里云」的 ddd-only
    assert _ddd_auto_skip_opencv_ensemble(None, 100, is_aliyun_preprocessed=True) is True

    def auto_use_dddd_only(xf_ok, xt_ok, xf, xt, tmp_aliyun):
        return xf_ok and xt_ok and _ddd_auto_skip_opencv_ensemble(
            xf, xt, is_aliyun_preprocessed=(tmp_aliyun is not None)
        )

    assert auto_use_dddd_only(True, True, 72, 164, None) is True
    assert auto_use_dddd_only(True, True, 127, 161, "/tmp/x") is False

    print("ok: test_icgoo_yidun_ddd_only_gate")


if __name__ == "__main__":
    main()
