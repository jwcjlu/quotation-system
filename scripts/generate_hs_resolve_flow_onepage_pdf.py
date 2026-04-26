# -*- coding: utf-8 -*-
"""生成「型号 → HS 编码」客户版单页 PDF（依赖 fpdf2，使用 Windows 自带 simkai.ttf）。"""

from __future__ import annotations

import sys
from pathlib import Path

from fpdf import FPDF


def _font_path() -> Path:
    for p in (
        Path(r"C:\Windows\Fonts\simkai.ttf"),
        Path(r"C:/Windows/Fonts/simkai.ttf"),
    ):
        if p.is_file():
            return p
    raise FileNotFoundError("未找到 C:\\Windows\\Fonts\\simkai.ttf，无法渲染中文。")


def main() -> int:
    repo = Path(__file__).resolve().parents[1]
    out = repo / "docs" / "hs-resolve-by-model-onepage.pdf"
    out.parent.mkdir(parents=True, exist_ok=True)

    font = str(_font_path())

    pdf = FPDF(unit="mm", format="A4")
    pdf.set_margins(12, 10, 12)
    pdf.set_auto_page_break(False)
    pdf.add_page()
    pdf.add_font("Kai", "", font)
    pdf.set_font("Kai", "", 15)
    w = pdf.epw
    pdf.multi_cell(w, 8, "元器件型号 → HS 编码 智能解析流程（客户版）", align="C")
    pdf.ln(2)

    pdf.set_font("Kai", "", 9)
    lh = 4.1

    blocks = [
        (
            "一、您需要提供",
            "型号（Model）、厂牌（Manufacturer）、请求追踪 ID（Request Trace ID，用于同一请求的幂等，"
            "避免重复解析）。可选「强制刷新」将重新跑全流程。",
        ),
        (
            "二、处理顺序（概要）",
            "1）发起解析，获得任务/运行 ID；若超时则返回已受理，请轮询查询任务接口。\n"
            "2）幂等：相同型号+厂牌+Trace ID 复用已有结果。\n"
            "3）快速路径：若该型号+厂牌已有已确认 HS 映射，直接返回 HS，不再下载规格书。\n"
            "4）规格书：从报价/资产等关联来源选取可下载且较新的规格书链接，下载并留存。\n"
            "5）特征抽取：从规格书得到技术品类与关键规格等结构化信息。\n"
            "6）税则匹配：在税则库中预筛候选，再打分排序；系统保留若干条 Top 候选供说明与复核。\n"
            "7）自动决策：以排名第 1 的候选为主结论；置信度达到阈值则自动标记「已确认」，"
            "否则为「待复核」。\n"
            "8）人工确认（可选）：对待复核结果，可通过确认接口选定最终 HS 并写入正式映射。\n"
            "9）常见失败：无可用规格书、无法识别技术品类、无合格候选或内部落库错误等。",
        ),
        (
            "三、产品侧决策口径",
            "当前实现为「首推 + Top 候选审计」模式（auto_top1_with_top3_audit）："
            "对外以 Top1 为主推荐，同时返回多条候选及分数/理由，便于复核与留痕。",
        ),
        (
            "四、合规提示",
            "HS 归类以海关及官方税则为准；本系统基于规格书与税则知识库提供辅助推荐，"
            "企业申报与海关认定具有最终效力。",
        ),
    ]

    for title, body in blocks:
        pdf.set_x(pdf.l_margin)
        pdf.set_font("Kai", "", 10)
        pdf.multi_cell(w, lh + 0.3, title)
        pdf.set_x(pdf.l_margin)
        pdf.set_font("Kai", "", 9)
        pdf.multi_cell(w, lh, body)
        pdf.ln(1.2)

    pdf.set_y(-18)
    pdf.set_x(pdf.l_margin)
    pdf.set_font("Kai", "", 8)
    pdf.set_text_color(80, 80, 80)
    pdf.multi_cell(w, 3.8, f"文档由仓库脚本生成：scripts/generate_hs_resolve_flow_onepage_pdf.py  →  {out.name}")

    pdf.output(str(out))
    print(out)
    return 0


if __name__ == "__main__":
    sys.exit(main())
