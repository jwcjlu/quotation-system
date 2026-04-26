# ICGOO 滑块（易盾 / 阿里云）：人工标定可行方案

本文约定**真值含义**、**在现有快照数据上如何标注**、**推荐工具与步骤**、**交付给开发/模型的数据格式**，以及**需要回传哪些参数**以便调 `gap_offset`、`drag_scale`、`ensemble` 等。

---

## 1. 标定分两层

| 层级 | 含义 | 真值从哪里来 |
|------|------|----------------|
| **A. 图内缺口 x** | 背景图**自然像素**下，拼图块**左缘**对准槽位时，左缘的横坐标 **x**（与 `detect_gap` / `raw_x` 同坐标系） | 人工看图标注 |
| **B. 页面拖动距离** | 浏览器里滑块手柄要水平移动多少 **CSS 像素** 才能过验 | 一般由 **A × drag_scale_x × drag_boost + extra** 换算；A 错了 B 必错；A 对仍失败时再单独扫 `drag_scale_mult` / `drag_boost` |

**优先做满 A**，再谈 B。下文默认「标定」首先指 **A**。

---

## 2. 现有数据里有什么（无需重新抓图）

验证码调试目录（`icgoo_crawler_dev` 默认在 `scripts/icgoo_captcha_snapshots/run_*`）通常含：

| 文件 | 用途 |
|------|------|
| `00_background.png` / `round_NN_background.png` | 标注主对象：背景自然尺寸下的槽位 |
| `00_slider.png` / `round_NN_slider.png` | 对照拼图形状（可选） |
| `00_detect.txt` / `round_NN_detect.txt` | 算法输出的 `raw_x_px`、`gap_ensemble_pick`、OpenCV 各路 x |
| `*_background_gap_marked.png` | 算法认为的落点矩形 + 红字 `raw_x`（**适合算修正量**） |
| `aliyun_calibration.jsonl` | 每轮 `gap_detected` + 每次 `slide_attempt`（成败、拖动像素等），供统计与 `tune_icgoo_captcha_from_jsonl.py` |

**样本 ID 建议**：`{run 目录名}_{round}`，例如 `run_20260405_090058_9403e5_round_03`。

---

## 3. 真值定义（必须与代码一致）

- **true_gap_left_x_px**：在**背景图**上，**槽位左边界**的 x（整数像素），原点左上角，与仓库里 `raw_x` / `write_gap_overlay_png` 矩形左缘同一含义。  
- 不要求标 y；本流水线主要用水平拖动。

---

## 4. 标注方法（三种，由易到难）

### 方法 M1：基于「已画框图」做差（推荐，快）

1. 打开同轮的 `*_background_gap_marked.png` 与 **未画框**的 `round_NN_background.png`（或 `00_background.png`）。  
2. 在**原背景**上目视确定**槽位左缘**相对**算法矩形左缘**偏左还是偏右多少像素（可用画图工具标尺、或截图到 PS / GIMP 看坐标差）。  
3. 记：  
   `true_gap_left_x_px = raw_x_from_red_text + delta_px`  
   其中 `raw_x_from_red_text` 与图上红字一致，`delta_px` 左负右正（槽在矩形右侧则 delta 为正）。

**优点**：不用从零找绝对坐标，只要会估差值。  
**缺点**：若算法框完全指到错误区域，差值仍要心里以「真槽」为准。

### 方法 M2：绝对坐标（看图软件 / 浏览器）

1. 仅用 `round_NN_background.png`。  
2. 使用能显示鼠标像素坐标的工具（如 IrfanView、部分看图软件、或自写 HTML Canvas 读 `offsetX`）。  
3. 鼠标移到**槽位左缘**（竖线与槽内浅色区域分界处），读取 **x** 即为 `true_gap_left_x_px`。

**优点**：直接真值。  
**缺点**：对「槽左缘」主观要统一口径（建议同一标注员先标 10 张定标准）。

### 方法 M3：简单标注工具（可选）

- 用 Label Studio、CVAT 画一条竖线或 bbox 左缘导出 x；或仓库内可后续加 `scripts/mark_gap_x_viewer.html`（非必须）。  
- 导出字段映射到下文 **JSONL** 的 `true_gap_left_x_px`。

---

## 5. 推荐工作流程（步骤清单）

1. **选批次**：选定若干 `run_*` 目录（成功少、失败多的优先）。  
2. **每轮一张**：对 `round_NN_background.png`（或 `00`）做 **M1 或 M2**，得到 `true_gap_left_x_px`。  
3. **抄算法值**：从同轮 `*_detect.txt` 或 `aliyun_calibration.jsonl` 里 `gap_detected` 取 `raw_x_image_px`、`gap_ensemble_pick`、`opencv_mode_primary`（便于分析哪路常错）。  
4. **算误差**：`err_px = raw_x_image_px - true_gap_left_x_px`（算法偏右为正）。  
5. **汇总**：生成 `manual_labels.jsonl` + `manual_labels_summary.csv`（见下节）。  
6. **交给开发/AI**：附上两文件 + 简短说明「希望优先调 gap_offset 还是 drag」。

**可选**：对同一批样本跑 `python scripts/replay_icgoo_gap_detect.py <目录>`，把离线重跑的 x 一并写入 summary，对比「线上逻辑 vs 离线」是否一致。

---

## 6. 输出数据格式（推荐）

### 6.1 主交付：`manual_labels.jsonl`（每行一条 JSON）

一行 = 一个标注样本，便于脚本合并与版本管理。

**必填字段：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `schema` | int | 固定 `1` |
| `sample_id` | string | 唯一 ID，如 `run_20260405_090058_9403e5_round_03` |
| `true_gap_left_x_px` | int | 人工真值 x |
| `annotator` | string | 标注人标识 |
| `annotation_method` | string | `gap_marked_delta` \| `absolute_cursor` \| `tool_export` |

**强烈建议填写（便于自动汇总）：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `source_run_dir` | string | 快照目录名或绝对路径 |
| `background_relpath` | string | 相对 `run_*` 的背景图，如 `round_03_background.png` |
| `algo_raw_x_px` | int | 算法该轮主选（与 detect 一致） |
| `algo_gap_ensemble_pick` | string | 如 `aliyun_shadow_ms`、`dddd_true` |
| `delta_from_marked_px` | int \| null | 若用 M1，填你用的 delta |
| `annotated_at` | string | ISO 日期 `2026-04-05` |
| `notes` | string | 模糊槽、缺角、备注 |

**示例行：**

```json
{"schema":1,"sample_id":"run_20260405_090058_9403e5_round_03","source_run_dir":"run_20260405_090058_9403e5","background_relpath":"round_03_background.png","true_gap_left_x_px":233,"annotation_method":"gap_marked_delta","delta_from_marked_px":-4,"algo_raw_x_px":237,"algo_gap_ensemble_pick":"dddd_true","annotator":"alice","annotated_at":"2026-04-05","notes":""}
```

### 6.2 汇总表：`manual_labels_summary.csv`（给 Excel / 复盘）

便于非程序员查看；可由 JSONL 脚本生成。

**建议列：**

```text
sample_id,true_gap_left_x_px,algo_raw_x_px,err_px,abs_err_px,gap_ensemble_pick,opencv_mode_primary,annotation_method,annotator,notes
```

其中 `err_px = algo_raw_x_px - true_gap_left_x_px`。

### 6.3 与 `aliyun_calibration.jsonl` 的关系

- **不修改**原始 `aliyun_calibration.jsonl`（保持现场可追溯）。  
- `manual_labels.jsonl` **并行存放**，通过 `sample_id` / `captcha_round` + `source_run_dir` 与现场日志关联即可。

---

## 7. 需要输出给开发 / AI 的参数清单（按优先级）

标定完成后，请至少提供：

1. **`manual_labels.jsonl`**（或等效完整字段的 CSV）。  
2. **`manual_labels_summary.csv`**（含 `err_px` 统计）。  
3. **聚合统计**（可手工或脚本）：  
   - `median(err_px)`、`p90(abs_err_px)`  
   - 按 `gap_ensemble_pick` 分组的平均 `abs_err_px`（哪路最不准）  
4. **调参建议请求**（一句话即可），例如：  
   - 「希望根据 median(err) 给建议的 `gap_offset_image_px`」  
   - 「shadow_ms 样本平均误差最大，是否降低 `ICGOO_ALIYUN_SHADOW_ENSEMBLE_WEIGHT`」  
   - 「图内 x 已对齐标注仍滑动失败，请给 `ICGOO_ALIYUN_DRAG_SCALE_MULT` 扫描步长建议」

开发侧可把 **`median(err_px)` 近似成 `gap_offset_image_px` 的修正**（注意符号：`raw_x` 要减 err 才靠近真值 → 环境变量里 offset 的正向定义以 `icgoo_crawler_dev --gap-offset-image-px` 为准）。

---

## 8. 仓库内已有相关工具

| 工具 | 作用 |
|------|------|
| `scripts/replay_icgoo_gap_detect.py` | 对快照目录离线重放缺口算法，对比打印 x |
| `scripts/tune_icgoo_captcha_from_jsonl.py` | **无真值**：仅根据成败给 `drag_mult` / shadow 权重等启发式建议 |
| `scripts/benchmark_aliyun_gap.py` | **有真值**（合成或 dataset meta）：离线命中率 |
| `docs/gap-position-detection.md` | 算法与 ensemble 说明 |

**有真值后**可扩展：用小脚本读 `manual_labels.jsonl` + 跑 `detect_gap_position`，自动算各算法 MAE（未在本文强制实现）。

---

## 9. 质量与规模建议

- **口径**：先固定 1 名主标注员标 20～30 张，二人复核 5 张对齐「槽左」定义。  
- **规模**：调 `gap_offset` / 单路权重，通常 **30～80 张** 现场图比纯合成更有用。  
- **难例**：故意包含 `|err_px|>40` 的样本，便于压 shadow 假峰。

---

## 10. 小结

| 你要做的 | 产出 |
|----------|------|
| 在现有 `run_*` 背景图上标真值 x | `manual_labels.jsonl` |
| 与算法 raw_x 对比 | `manual_labels_summary.csv` + err 统计 |
| 交给 AI / 开发 | 上述文件 + 希望的调参目标（offset / ensemble / drag） |

按本方案标注的数据**与** `tune_icgoo_captcha_from_jsonl.py`（成败驱动）**互补**：前者解决「**x 偏哪**」，后者解决「**拖距/权重往哪扫**」。

---

## 11. DeepSeek / 大模型能否自动完成本方案？

### 11.1 能力边界（必须先分清）

| 环节 | 纯文本大模型（如 DeepSeek Chat / R1 文本 API） | 多模态 / 视觉模型（须能读图：如 DeepSeek-VL、其它 VL API） |
|------|-----------------------------------------------|-------------------------------------------------------------|
| 从背景图估 **`true_gap_left_x_px`（槽左像素 x）** | **不能**（收不到像素） | **可尝试**：把 `background.png` 编码进请求，要求只输出结构化 JSON；**误差与稳定性需统计**，不能默认当金标准 |
| 解析 `detect.txt` / JSONL、合并行、算 `median(err)`、生成 `gap_offset` 建议文案 | **可以** | **可以** |
| 根据 `manual_labels.jsonl` 写 `summary.csv`、shell/cmd 环境变量片段 | **可以**（也可用小脚本更稳） | **可以** |

结论：**「整份方案全自动、零人眼」不现实**；**「VL 粗标 + 规则校验 + 人抽检」**或 **「人标 + DeepSeek 做统计与报告」** 可行。

### 11.2 推荐落地形态（与 DeepSeek 配合）

**形态 A — 半自动标 x（多模态 API）**

1. 脚本遍历 `run_*` 下 `round_NN_background.png`，转 **base64** 或 URL（须符合平台要求）。  
2. **Prompt 要点**（示例）：  
   - 说明坐标系：左上角为原点，x 向右增大；  
   - 只输出一行 JSON：`{"true_gap_left_x_px": <int>, "confidence": "high|medium|low", "brief": "槽在浅灰凹陷左侧"}`；  
   - 禁止废话，禁止 Markdown。  
3. **程序侧校验**：`0 <= x <= bg_width - 10`；与 `algo_raw_x_px` 差超过 120px 的样本打 `flag_outlier` 进人工队列。  
4. **人工**：只审核 `low confidence` 与 `outlier`（通常占 10%～30%），写入最终 `manual_labels.jsonl`（`annotation_method`: `vl_deepseek` + `reviewed_by_human`: true/false）。

**形态 B — 全自动后处理（仅文本 DeepSeek）**

1. 人或其它工具已产生 `manual_labels.jsonl`（或 CSV）。  
2. 把 **文件内容 + 聚合要求** 发给 DeepSeek：输出 `median(err_px)`、按 `gap_ensemble_pick` 分组的表格、建议的 `gap_offset_image_px` 符号说明、给 `icgoo_crawler_dev` 的示例命令行。  
3. **更稳做法**：聚合用 **Python 脚本**（确定性），DeepSeek 只生成 **解释与 commit message**；避免模型算错中位数。

**形态 C — 不与 VL 绑死**

- 用 **专用小模型 / 传统 CV**（边缘、颜色分割）先出候选 x，再可选送 **文本模型** 做「两候选选谁」的逻辑（成本与复杂度更高，本文不展开）。

### 11.3 合规与用途说明

- 自动化标定用于 **自有站点调试、安全研究与风控对齐** 时，须遵守 **平台服务条款** 与本地法律。  
- 不建议依赖大模型 **批量绕过** 第三方验证码作为对外服务；本文档面向 **工程标定与误差分析**。

### 11.4 若要让 Cursor / 脚本「调用 DeepSeek」

- 在环境配置 **API Key**（勿写入仓库），用 **HTTPS 调用官方 API**；请求体中带 `image_url` / `image_base64`（以当前 DeepSeek 开放平台文档为准，VL 能力以实际开放模型名为准）。  
- 仓库内可不内置 Key；可另加 `scripts/llm_assisted_gap_label.py`（读取 playbook 字段、调 API、写 JSONL）——需要时可单独提需求实现。

### 11.5 一行结论（对照表）

| 方式 | 是否可行 |
|------|----------|
| **纯 DeepSeek 文本** 自动从 PNG 读出槽位 x | 否 |
| **DeepSeek 多模态（若可用）** 产出 `true_gap_left_x_px` + **程序校验 + 人审尾批** | 是（推荐） |
| **DeepSeek 文本** 做统计、报告、调参建议叙述 | 是 |
| **全无人参与** 且 100% 当生产真值 | 不建议 |
