# ICGOO 阿里云 aliyunCaptcha 滑块：识别与拖动流程（调试对照）

本文档与仓库实现一致，供调试 `icgoo_crawler_dev.py` / `icgoo_crawler.py` 滑块逻辑时对照。涉及文件：

- `scripts/icgoo_aliyun_captcha.py` — iframe、拉图、`drag_scale_x`、成功态、`src` 键与图像指纹  
- `scripts/icgoo_aliyun_gap.py` — RGBA 合成预处理、Canny 共识、标定 JSONL 辅助  
- `scripts/icgoo_yidun_solver.py` — 缺口识别、拖动换算、`slide_verification`、外层多轮换图  

---

## 1. 模块分工

| 模块 | 职责 |
|------|------|
| `icgoo_aliyun_captcha.py` | 在宿主或 iframe 中定位验证码；读 `#aliyunCaptcha-img` / `#aliyunCaptcha-puzzle`；`data:` / HTTPS 转图像字节；**轨宽 ÷ 背景自然宽 → `drag_scale_x`**；`aliyun_puzzle_src_compare_key` / `aliyun_puzzle_image_fingerprint` |
| `icgoo_aliyun_gap.py` | **`aliyun_prepare_match_bytes`**：按背景四角 RGB 铺拼图透明底；临时 PNG；**`aliyun_try_consensus_median`**（两路 dddd 分歧大时） |
| `icgoo_yidun_solver.py` | **`IcgooYidunSliderSolver`**：`detect_gap_position` / `detect_gap` / `get_tracks` / **`slide_verification`**；**`try_auto_solve_icgoo_yidun_slider`** 入口 |

---

## 2. 入口与外层循环

- **入口**：`try_auto_solve_icgoo_yidun_slider`（`icgoo_yidun_solver.py`）。生产爬虫 `icgoo_crawler.py` 与调试脚本 `icgoo_crawler_dev.py` 均通过同一 Solver 类工作。  
- **流程**：  
  1. `captcha_exists`：短时内若存在易盾或阿里云拼图则进入自动破解。  
  2. 临时目录写入 `background.png`、`slider.png`。  
  3. 最多 **`_MAX_CAPTCHA_REIDENTIFY_ROUNDS`**（默认 8）轮：每轮 **拉图 → 识别 → 同图多次滑动**。  
  4. 若 `slide_verification` 返回 **已换题**（`captcha_replaced=True`），下一轮重新 `get_images` + `detect_gap`。

---

## 3. 判定阿里云与拉图

1. **`aliyun_challenge_active`**：弹层可见 + 能读到拼图 `src`（与易盾共用 `_read_yidun_img_src`：`src`、`data-src`、JS `currentSrc`）。  
2. **`_aliyun_host_context`**：BFS 查找含 `#aliyunCaptcha-puzzle` 等的文档（主页面或跨域 iframe）。  
3. **`get_images` 阿里云分支**（`aliyun_get_images`）：  
   - 等待 `#aliyunCaptcha-puzzle`、`#aliyunCaptcha-img` 且 `src` 就绪；  
   - 得到背景/拼图 **base64 载荷**；  
   - **`aliyun_drag_scale_x`**：`#aliyunCaptcha-sliding-body` 与 `#aliyunCaptcha-sliding-slider` 算轨道行程 ÷ 背景 **自然宽度** → **`drag_scale_x`**；阿里云再乘 **`get_aliyun_drag_scale_mult()`**。

---

## 4. 缺口识别前预处理（仅阿里云）

在 **`detect_gap_position`** 内调用 **`aliyun_resolve_detect_inputs`**：

1. 若 **`challenge_active_fn()`** 仍为 True：对本轮磁盘上的滑块/背景字节执行 **`aliyun_prepare_match_bytes`**（`icgoo_aliyun_gap.py`）：  
   - 背景规范为 RGB；  
   - 拼图 RGBA 按 **背景四角均值** 合成到背景色，减轻透明底假峰；  
   - 写入临时目录 `bg.png`、`sl.png`，返回 **`bg_eff, sl_eff`** 及新 bytes（供 `slide_match`）。  
2. 成功时第五项为临时目录路径 **`tmp_aliyun`**，在 `detect_gap_position` 的 **`finally` 中 `shutil.rmtree`**。  
3. **`tmp_aliyun is not None`** 时：  
   - `_fill_opencv_gap_scores(..., include_aliyun_aux=True)` 会算 **Alpha-mask**、可选 **`puzzle-slider-captcha`**、以及 **`aliyun_shadow_multiscale_match_x`**（与 `scripts/gap-position-detection.md` 一致：RGBA 紧裁、模糊、多尺度、纵向 ROI、`TM_CCOEFF_NORMED`）；  
   - **`_ensemble_auto_best_gap(..., aliyun_include_opencv_in_ensemble=True)`** 在候选中取 **置信度最高** 的路。

---

## 5. 图内缺口 x：`detect_gap_position`

依赖 **`slider_gap_backend`**（`auto` / `ddddocr` / `opencv` / `slidercracker` 等）：

1. **`auto` 且 dddd 可用**：`_fill_opencv_gap_scores` 后，对预处理 bytes 调用 **`slide_match(..., simple_target=False/True)`**，得两路左缘 x 与 confidence。  
2. **融合与 ensemble**：阈值控制是否仅用 dddd、是否引入 OpenCV 辅基准、阿里云 **共识** **`aliyun_try_consensus_median`**（原始图多 Canny，中位数）。  
3. dddd 不可用且非强制 dddd：**OpenCV** 灰度 / Canny 回退。  
4. 返回值：背景图坐标系下 **缺口左缘 x（像素）**。

---

## 6. 图内 x → 页面拖动像素：`detect_gap`

1. `raw_x = detect_gap_position 结果 + gap_offset_image_px`（下限 1）。  
2. 若仍为阿里云：`drag_scale_x *= get_aliyun_drag_scale_mult()`。  
3.  
   `drag_x = round(raw_x * drag_scale_x * drag_boost) + drag_extra_px + get_aliyun_systematic_drag_extra_px()`  
   - **`ICGOO_ALIYUN_SYSTEMATIC_DRAG_EXTRA_PX`**：默认约 +20，仅阿里云；  
   - **`drag_boost` / `drag_extra_px`**：与易盾共用，可由 CLI 传入。

---

## 7. 辅距离 `distance_alt`

当 dddd **Canny 与灰度** 两路图内 x 接近时，计算第二拖动距离，在 **`slide_verification`** 中 **奇数次主基准、偶数次辅基准** 交替尝试。

---

## 8. 滑动：`slide_verification`

1. 尝试前 **`_slide_passed_or_captcha_gone`**，已通过则直接成功。  
2. **`src_before = _puzzle_piece_src()`**（阿里云：`#aliyunCaptcha-puzzle`）。  
3. 手柄：**`#aliyunCaptcha-sliding-slider`**（`_find_yidun_drag_handle`）。  
4. **`get_tracks(d_try)`**：拟人步进，水平位移总和等于目标像素。  
5. `hold` → `move` → `release` → 等待 → **`check_success`**，必要时 **延迟复核**（成功 UI 晚于换图）。  
6. 失败后再读 **`src_after`**：  
   - **`aliyun_puzzle_src_compare_key`**：`https` 且 `*.aliyuncs.com` 时去掉 query；`data:` 用 **解码图像字节 SHA256** 键。  
   - 键不同：阿里云先 **延迟成功复核**；仍失败则 **`aliyun_puzzle_image_fingerprint`**，指纹相同视为 **同题改址**，`continue` 下一系数；否则 **换题** → `return False, True`。

---

## 9. 单轮数据流（便于打断点）

```
页面/iframe
  → aliyun_get_images → (bg_b64, puzzle_b64, drag_scale_x)
  → 落盘 background.png / slider.png
  → aliyun_resolve_detect_inputs → 预处理路径 + bytes（tmp_aliyun 或 None）
  → detect_gap_position（dddd + OpenCV / ensemble / aliyun_try_consensus_median）
  → detect_gap → 页面拖动像素
  → slide_verification（多系数 × 主/辅距离；换题 + 指纹兜底）
```

---

## 10. 调试时常用开关

| 项 | 说明 |
|----|------|
| `ICGOO_ALIYUN_SYSTEMATIC_DRAG_EXTRA_PX` | 阿里云额外加量像素 |
| `ICGOO_ALIYUN_DRAG_SCALE_MULT` | 对 `drag_scale_x` 再乘，范围约 0.5～2 |
| `ICGOO_CAPTCHA_CALIBRATION_JSONL` | 指向 JSONL 文件，记录每轮识别与滑动标定 |
| `icgoo_crawler_dev.py` | `--aliyun-systematic-extra-px`、`--aliyun-drag-scale-mult`、`--drag-boost`、`--drag-extra-px` 等 |
| 调试图目录 | dev 模式常写入 `scripts/icgoo_captcha_snapshots/run_*`（含 `00_background.png`、`00_slider.png`、`00_detect.txt` 等） |

可选依赖：**`puzzle-slider-captcha`**（`pip install`），用于阿里云 OpenCV 辅助一路；未安装则该路跳过。

---

## 11. 关键符号速查（grep 用）

- DOM：`#aliyunCaptcha-puzzle`、`#aliyunCaptcha-img`、`#aliyunCaptcha-sliding-slider`、`#aliyunCaptcha-sliding-body`  
- 函数：`aliyun_get_images`、`aliyun_resolve_detect_inputs`、`aliyun_prepare_match_bytes`、`detect_gap_position`、`detect_gap`、`slide_verification`、`try_auto_solve_icgoo_yidun_slider`  

文档版本：与仓库 `scripts/icgoo_aliyun_captcha.py`、`icgoo_aliyun_gap.py`、`icgoo_yidun_solver.py` 同步维护；若实现变更请同步更新本节与数据流。
