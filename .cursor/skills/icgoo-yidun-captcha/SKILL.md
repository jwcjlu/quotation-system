---
name: icgoo-yidun-captcha
description: >-
  Interprets scripts/icgoo_captcha_snapshots runs (attempts.tsv, 00_detect.txt) for ICGOO
  NetEase YiDun slider automation. Root-cause analysis when verified_ok or gap detection
  seems poor: separate image-space x from drag_scale/drag_boost/factors, shim vs onnx,
  blend bugs, and post-failure puzzle refresh. Use when retroing low success rates or
  tuning icgoo_crawler_dev.py.
---

# ICGOO 易盾滑块：快照复盘与指标

## 复盘：为什么「识别率 / 通过率」看起来很低（self-improving 摘要）

**Trigger**：用户或快照显示 `attempts.tsv` 里 `verified_ok=True` 极少（例如单次滑动个位数百分比），或体感「ddddocr 识别很差」。

**Wrong pattern**：把低通过率**单一归因**为「缺口识别模型不行」；或只看 `confidence` 高低判断对错；或在**拼图已换**后仍用上一轮的 `raw_x` 试系数。

**Right pattern**：先拆成 **(A) 图内缺口 x**、**(B) 像素→拖动换算**、**(C) 同图多系数试探 vs 换图重识别**、**(D) 易盾风控**，再对 `00_detect.txt` / 成对 PNG 逐项排除。

| 环节 | 常见原因 | 在快照里看什么 | 对策方向 |
|------|----------|----------------|----------|
| A 图内 x | `ddddocr_shim`、假峰、错误 blend | `gap_backend`、`dddd_raw_*`、`slide_match_blended` | 修 onnx 用官方 SlideEngine；`test_ddddocr_slide.py --mode local` 离线验；blend 已要求两路左缘均 ≥2 |
| B 换算 | `drag_scale_x`、`drag_boost`、`tw=0` 未减半宽 | `drag_scale_x`、`raw_x_detected`、`base_drag_px` | `--drag-boost`、`--gap-offset-image-px`；代码已补 tw 宽高兜底与 shim 低分用 OpenCV 覆盖（auto） |
| C 流程 | 失败后易盾**自动换新图**，仍当同图试系数 | 日志是否应出现「拼图 src 已变」；多轮 `captcha_round` | 依赖 `slide_verification` 的 `src` 比对 → `captcha_replaced` 后重 `get_images`+`detect_gap` |
| D 风控 | 轨迹、环境、频率 | 同参数有时过有时不过 | 降低频率、拟人轨迹已有；非纯算法可解 |

**Scope**：仅本仓库 ICGOO/易盾调试链路；实现见 `scripts/icgoo_crawler_dev.py`、`scripts/test_ddddocr_slide.py`。

**Verify**：跑本节「仓库快照统计」脚本得 `True/False` 计数；任选一失败 run，用 `test_ddddocr_slide.py` 复现同一对图，对比「离线 x」与 `00_detect.txt` 的 `raw_x_detected` 是否一致，以区分 A 与 B。

## 指标不要混为一谈

- **`attempts.tsv` 的 `verified_ok`**：端到端结果（识别 raw_x × `drag_scale_x` × `drag_boost` × 当次系数 × 轨迹 × 易盾风控）。**不是**「缺口识别准确率」单指标。
- **缺口识别**：看 `00_detect.txt` 的 `gap_backend`、`dddd_raw_x_simple_false` / `dddd_raw_x_simple_true`、`slide_match_blended`、`dddd_confidence`；或用 `test_ddddocr_slide.py --mode local` 离线复现同一对 `00_slider.png` + `00_background.png`。

## 仓库快照统计（可复算）

在仓库根目录执行：

```bash
python -c "import os,glob;root=r'scripts/icgoo_captcha_snapshots';ok=fail=0
for p in glob.glob(os.path.join(root,'**/attempts.tsv'),recursive=True):
 import pathlib
 for line in open(p,encoding='utf-8'):
  line=line.strip()
  if not line or line.startswith('#') or line.startswith('attempt'): continue
  a=line.split('\t')
  if len(a)<5: continue
  ok+=a[-1]=='True'; fail+=a[-1]=='False'
print('True',ok,'False',fail,'rate',round(100*ok/(ok+fail),2) if ok+fail else 0,'%')"
```

（路径按实际 cwd 调整。）若 **单次滑动成功率仅百分之几**，先判断主因是 **图内 x** 还是 **拖动换算/系数**。

## 失败模式（来自快照样本）

1. **`gap_backend=ddddocr_shim` + 置信度 ~0.14–0.2**：Windows 上 onnx 不可用时走纯 OpenCV；数值偏低属常态，**不等于**必然失败，但比官方 onnx 更易边界 case。
2. **`slide_match_blended=True` 且一路 raw 异常**（如 `dddd_raw_x_simple_true` 为 `0` 或与另一路差极大）：中点融合会把距离拉到错误区间，**多试滑动也难救**。应优先修复 ddddocr/onnx 或收紧「仅当两路均 plausible 才融合」的逻辑（见代码审查）。
3. **`drag_scale_x` 固定约 0.646**：DOM 量轨与背景自然宽换算一环错则整体按比例偏；可调 `--drag-boost` 或 `gap_offset_image_px` 做系统补偿。
4. **成功 run 常见形态**：两路 raw 接近（如 298/299）、未融合或分歧策略合理；**第 6–7 次尝试**、`factor` 为 `1.0` 或 `1.14` 时出现 `True` 的样本较多（仍依赖当次 base_drag）。
5. **验证失败后易盾常自动换新拼图**：`icgoo_crawler_dev.slide_verification` 会比对拼图 `img` 的 `src`；变化则 `captcha_replaced=True`，外层须重新 `get_images` + `detect_gap`，勿再用上一轮 raw_x 试系数。
6. **`slide_verification` 已打印「验证成功」但主流程仍进手动等待**：历史上 `main` 在 `slide_ok` 后又要求「`captcha_exists` 为假或 `_yidun_success_visible`」，与残留隐藏 DOM 矛盾；**以 `slide_ok` 为准** 设 `auto_success`，勿二次否定。

7. **icgoo_crawler：日志已「易盾自动验证通过」仍立刻进「检测到易盾/频繁」300s 等待**：`_has_icgoo_yidun_or_frequent_block` 若用 ``"slide-frequently" in page.html`` 且判断在 `yidun_still_requires_user_action` **之前**，则与易盾是否通过无关——ICGOO 等 SPA 的 **JS bundle / 隐藏模板** 常含 `slide-frequently` 子串，导致整条链误判。**Right pattern**：限流只认 **可见 UI**（见 `icgoo_crawler._icgoo_rate_limit_ui_visible`：`#slide-frequently` / `.frequently-text` 的 `getComputedStyle` + 尺寸，或 `body.innerText` 含「您的搜索过于频繁」），**禁止**用裸子串扫整页 `html`。

8. **`verified_ok=False` 但人工确认已过（误判）**：`page.ele` 仍能找到 DOM 里的 `yidun_jigsaw`，但节点已为 `display:none` / 极低 `opacity` / `getBoundingClientRect` 近似 0（`captcha_exists` 仍真）；或 Vue/易盾 **晚于首轮轮询** 才挂 `yidun--success` / 撤拼图；或 **业务已收起弹层、表格已加载** 但隐藏节点仍在。代码侧：`check_success` 两阶段轮询 + `_yidun_passed_js_probe()` + **`_yidun_challenge_surface_visible_js()`**（同源内是否存在视口内足够大的可见 `.yidun`；`captcha_exists` 真且 `surf===false` 视为放行）；`slide_verification` 在 `check_success` 假后 **约 2s + 2.5s** 两次 `_slide_passed_or_captcha_gone()`。跨域 iframe 内验证码时主文档 `surf` 可能误判，需慎用。

## 建议调试顺序

1. 对失败 run：打开同目录 `00_background.png`、`00_slider.png`，本地跑 `scripts/test_ddddocr_slide.py --mode local --target ... --bg ...`（可加 `--slide-engine shim` 对齐爬虫）。
2. 读 `00_detect.txt`：是否 shim、是否 blended、两路 raw 是否离谱。
3. 再调 `--drag-boost`、`--gap-offset-image-px`、`--slide-attempts`，勿只调 `slide_factors`。

## Retro snapshot（粘贴给用户时可用的短模板）

- **Lesson**：易盾低通过率多为 **A×B×C** 链式问题，不是单一「识别率」；必须先分清图内 x、拖动换算与是否换图。**端到端「验证失败」误判**还要区分：真失败 vs DOM 仍含隐藏 `yidun_jigsaw` / 成功态晚挂载 / iframe。**爬虫侧**勿用整页 HTML 子串当「限流/易盾」信号（易与 SPA 脚本误匹配）；限流须看可见节点或 `innerText`。
- **Proposed persistence**：本文件 `.cursor/skills/icgoo-yidun-captcha/SKILL.md`（已含）。
- **Change summary**：用表格分解；用快照脚本 + local 测试验证；代码侧已加强 blend、tw、shim/OpenCV、拼图 `src` 换图检测、**JS DOM 探测通过态**。
- **Verify next time**：`attempts.tsv` 聚合脚本；单 run 的 `00_detect.txt` + 双图离线 `test_ddddocr_slide.py`；若疑误判，对照同目录 `NN_page_after.html` 与控制台 `run_js` 可见性。
