# 背景图缺口坐标识别说明

本文说明本仓库如何估计「滑块拼图」背景图上缺口（目标槽）的**像素坐标**，对应脚本主要为 `match_back_shadow.py`、`merge_captcha_pair.py`，以及辅助的 `preview_captcha_masks.py` / `validation_report.py`。

---

## 1. 问题在算什么

- **输入**：通常为一对资源——**背景图 `back`**（带浅色缺口）与 **滑块图 `shadow` / `slider`**（可含透明区域）。
- **输出**：在**背景图像素坐标系**下，缺口与滑块对齐时的一个参考位置，例如：
  - **`x_left`、`y_top`**：将**与匹配时同尺寸**的滑块图左上角放在背景上的坐标；
  - **`x_center`、`y_center`**：上述矩形区域的中心（便于画辅助线或粗对齐）。

> 浏览器里控件的 `left` 往往按 **CSS 显示宽度** 计量，若图片被缩放显示，需再乘以 `naturalWidth / 显示宽度` 等比例换算，本文只讨论**图像像素坐标**。

---

## 2. 主方法：模板匹配（推荐，需 `back` + `shadow`）

### 2.1 原理

滑块小图在视觉上对应「要嵌入缺口」的那一块。将背景与滑块转为**灰度**，把滑块当作**模板**，在背景的指定**纵向条带**内做滑动窗口匹配；**归一化相关系数（TM_CCOEFF_NORMED）** 越大，表示该位置越相似。取**全局最大响应**对应的位置作为缺口对齐位置。

### 2.2 实现要点（与本仓库一致）

1. **滑块预处理**（`shadow_bgra_cropped_for_match`）  
   - 若有 Alpha，先裁掉近似全透明的外边，只保留不透明内容的外接矩形，减少灰边参与匹配。  
   - 匹配与 `merge_captcha_pair.py` 里叠加使用**同一裁剪结果**，保证「算出来的位置」与「贴图位置」一致。

2. **灰度化**  
   - `cv2.cvtColor(..., COLOR_BGR2GRAY)`，降低通道差异对匹配的干扰。

3. **可选高斯模糊**  
   - 对背景搜索条带与模板同时模糊（参数 `--blur`），减轻锯齿与噪声对 NCC 的影响。

4. **多尺度**（`--scales`）  
   - 对模板按多个比例缩放后再匹配，应对前端缩放或资源与显示比例略有偏差。

5. **纵向搜索范围**（`--search-y0`、`--search-y1`，0～1 相对高度）  
   - 只在 `back_gray[y0:y1, :]` 内匹配，减少计算量并限制误匹配区域。

6. **核心 API**  
   - `cv2.matchTemplate(roi, template, cv2.TM_CCOEFF_NORMED)`  
   - `cv2.minMaxLoc` 取最大值位置 `(mx_x, mx_y)`，再换算到整图：`y_top = mx_y + y0`。

### 2.3 脚本与产物

- **命令行**：`python match_back_shadow.py --back <路径或URL> --shadow <路径或URL>`  
- **JSON 字段**：`match.x_left`、`match.y_top`、`match.template_w`、`match.template_h`、`match.score`、`match.scale` 等。  
- **可视化**：默认 `imshow` 或 `--viz` 保存「橙框 + 过 `x_center` 的竖线」图，与 `draw_match_visualization` 一致。

---

## 3. 辅方法：颜色分割（仅背景、或作校验）

### 3.1 原理

缺口区域常为**浅灰、低饱和**。在 **Lab** 与 **HSV** 下用阈值得到二值掩码，取**交集**后做**形态学**（闭运算再开运算），再取**外轮廓**中面积落在给定占图比例范围内的候选；通常取**面积最大**者，用其**包围框左缘 `bbox_x`** 或**质心 `cx`** 作为缺口水平位置的估计。

### 3.2 代码位置

- 掩码与形态学：`preview_captcha_masks.py` 中 `build_masks` 等。  
- 指标与消融：`validation_report.py` 中 `masks_lab_hsv_combo`、`contour_metrics`。  
- 与模板匹配对比：`match_back_shadow.py` 中 `--segment-check`，比较 `x_left` 与分割得到的 `segment_bbox_x`。

### 3.3 局限

强纹理背景易产生多个浅灰假区域，**Top1 轮廓不一定是真缺口**；适合调参、对照，**有 `shadow` 时仍以模板匹配为主**更稳。

---

## 4. 与合成图脚本的关系（`merge_captcha_pair.py`）

1. 使用与 `match_back_shadow` **相同的**裁剪 BGRA、灰度模板、多尺度匹配逻辑，得到 `x_left`、`y_top`、`template_w`、`template_h`。  
2. 将滑块 BGRA **缩放到 `(template_w, template_h)`**。  
3. 调用 `paste_bgra_on_bgr`，以 **`(x_left, y_top)`** 为左上角做 Alpha 合成。  

因此，合成图上的贴图位置**就是**模板匹配给出的缺口对齐位置，而非另一套独立坐标。

---

## 5. 参数与调试建议

| 现象 | 可调整项 |
|------|-----------|
| 水平偏差较大 | `--scales` 范围、是否关闭或减小 `--blur` |
| 纵向搜错行 | 缩小 `--search-y0`～`--search-y1`，使条带盖住缺口纵范围 |
| 与分割结果差很多 | `--segment-check` 看 `diff`；以匹配分为准或检查阈值 |
| 与网页滑块不一致 | 检查图片 **natural 尺寸** 与 **CSS 显示尺寸** 的比例换算 |

---

## 7. ICGOO 仓库内落地（阿里云 ensemble）

本仓库在 **`icgoo_aliyun_gap.aliyun_shadow_multiscale_match_x`** 中实现了与上文 **§2** 对齐的一条候选路：

- 背景使用 **`aliyun_prepare_match_bytes` 后的预处理 PNG**（与 dddd/OpenCV 主路径一致）；  
- 滑块使用 **磁盘上原始 RGBA**（`00_slider.png`），**Alpha 紧裁**后按背景四角色做 RGB 合成，再转灰度；  
- 可选 **高斯模糊**（`ICGOO_ALIYUN_SHADOW_BLUR`，默认 3，设为 `0` 关闭）；  
- **多尺度**模板（默认 `0.92,0.96,1.0,1.03,1.07`，可用 `ICGOO_ALIYUN_SHADOW_SCALES` 覆盖，逗号分隔）；  
- **纵向 ROI**（`ICGOO_ALIYUN_SHADOW_Y0_FRAC` / `Y1_FRAC`，默认约 `0.06`～`0.995`）；  
- **`cv2.matchTemplate(..., TM_CCOEFF_NORMED)`** 取最大响应的 **`x` 左缘** 与 `score`。

另有一条 **`aliyun_shadow_multiscale_match_x_edges`**：与上相同的 ROI / 多尺度 / 预处理，但在 **Canny 边缘图** 上做 `TM_CCOEFF_NORMED`（与常见开源阿里云滑块脚本「边缘 + 模板匹配」同思路），ensemble 标签 **`aliyun_shadow_ms_edges`**；权重 **`ICGOO_ALIYUN_SHADOW_MS_EDGES_ENSEMBLE_WEIGHT`**（默认 `0.62`）；仅关闭边缘路而不关灰度多尺度：`ICGOO_ALIYUN_SHADOW_MS_EDGES=0`；Canny 阈值组可用 **`ICGOO_ALIYUN_SHADOW_MS_EDGES_CANNY`**（如 `50:150;40:120`）。

上述两路与 dddd、整图灰度/Canny、Alpha-mask、PSC 等一并进入 **`IcgooYidunSliderSolver._ensemble_auto_best_gap`**，按分数决选。若需关闭灰度多尺度路：`ICGOO_ALIYUN_SHADOW_MS=0`（关闭时边缘路亦不跑）。

**Ensemble 注意（避免识别率假象下降）**：`TM_CCOEFF_NORMED` 的峰值与 dddd `confidence`、灰度模板峰值**量纲不同**，不宜无校准地取 `max`；否则 shadow 路常因 0.5+ 的 NCC 压过 0.2 左右的灰度/Canny，在纹理重复时拖出 **x≈10/22** 等明显假峰。实现上已对 **`aliyun_shadow_ms` 乘权重**（`ICGOO_ALIYUN_SHADOW_ENSEMBLE_WEIGHT`，默认 `0.72`），且在**灰度与 Canny 分歧 ≥22px** 且 **shadow 偏离二者中点过远** 时**不采纳** shadow 候选。修复 **onnxruntime / ddddocr** 后应恢复 dddd 参与决选，整体会更稳。

**离线自调参**：对快照目录或合并多份 ``aliyun_calibration.jsonl`` 运行 ``python scripts/tune_icgoo_captcha_from_jsonl.py <...>``，按成败统计给出 ``ICGOO_ALIYUN_DRAG_SCALE_MULT``、``ICGOO_ALIYUN_SHADOW_ENSEMBLE_WEIGHT`` 等建议；可选 ``--write-env-cmd`` 写出 ``.cmd``。

---

## 6. 用到的库

### 6.1 需通过 pip 安装的第三方库

与 `requirements.txt` 一致，安装命令：

```bash
pip install -r requirements.txt
```

| 库名 | 典型 import | 在本流程中的作用 |
|------|-------------|------------------|
| **OpenCV**（`opencv-python-headless`） | `import cv2` | 读/写图、`cvtColor`（BGR/Lab/HSV/灰度）、`matchTemplate`、`minMaxLoc`、`GaussianBlur`、`resize`、`findContours`、`morphologyEx`、`imshow`（与 `cv_show` 配合）、`imencode`/`imdecode` 等 |
| **NumPy** | `import numpy as np` | 图像即 `ndarray`；掩码布尔运算、`where`、裁剪、Alpha 混合时的浮点运算、`frombuffer` 配合解码网络图片字节 |

说明：

- **`opencv-python-headless`**：无 Qt 等 GUI 依赖，体积相对小，适合服务器；一般仍可使用 `cv2.imshow`（具体取决于系统显示环境）。若需完整桌面集成包，可改用 `opencv-python`（功能类似，多带 GUI 后端）。
- 版本建议以 `requirements.txt` 为准：`opencv-python-headless>=4.8.0`，`numpy>=1.24.0`。
