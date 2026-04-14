# 电子元器件型号归类与税检判定方案（Top5 + LLM重排 + 人工复核）

**状态：** 草案（待评审）  
**日期：** 2026-04-13  
**适用范围：** 中国进出口场景（进口 + 出口）

## 1. 目标与边界

**目标**

1. 基于型号与补充描述，输出 `Top5` 税号候选（`hs_code`）并给出可追溯依据。
2. 判断是否涉及商检/监管条件，并给出进出口相关税率字段。
3. 在缺少完整参数时，支持“型号搜索补参”并量化置信度。
4. 通过人工复核闭环持续提升命中率，逐步从“辅助判定”走向“高自动化”。

**非目标**

- 不将 LLM 作为最终法律意义上的归类裁定主体。
- 不依赖单一电商页面直接产出最终税号结论。
- 不在缺失关键字段时强行自动通过。

## 2. 总体方案（推荐：方案2.5）

采用 **检索增强 + 规则引擎 + LLM重排序解释 + 人工复核**：

1. 规则召回：按品类、用途、参数、章注约束召回候选税号池（如 Top20）。
2. 检索增强：检索税则注释、历史改判案例、申报要素模板。
3. LLM 重排：仅对候选池内税号重排序，输出 Top5 与理由，不允许“出圈”。
4. 规则终裁：执行硬约束校验（监管条件、字段完整性、冲突规则）。
5. 人工复核：低置信或冲突单据进入复核，结果回流案例库。

## 3. 架构与数据流

```
输入(型号/描述/方向)
    -> 标准化(型号清洗、品牌别名、单位归一)
    -> 候选召回(规则召回 + 相似案例召回)
    -> 证据检索(税则注释/案例/要素模板)
    -> LLM重排序(Top5 + 解释 + 缺失字段)
    -> 规则终裁(硬规则、冲突检测、置信度分级)
    -> 输出建议(税号/商检监管/税率/依据链)
    -> 人工复核(确认/改判/原因)
    -> 样本回流(持续学习)
```

## 4. 权威数据接入原则

优先接入海关总署“我要查”能力，作为权威基座：  
[海关总署-我要查](https://online.customs.gov.cn/mySearch)

建议优先级：

- `P0`：海关权威数据（税率、税则、监管事项、注释）
- `P1`：企业内部历史复核案例
- `P2`：LLM重排与解释

冲突原则：`P0 > P1 > P2`。

## 5. 输出接口模型（MVP）

`POST /classify/by-model`

**输入最小集**

- `trade_direction`: `import | export`
- `declaration_date`（申报日期/拟归类生效日，`YYYY-MM-DD`）
- `model`
- `product_name_cn`
- `function_use`
- `category`
- `specs`（可空但影响置信度）

**输出核心**

- `candidates`: Top5（`hs_code`, `score`, `reason`, `evidence`, `required_elements_missing`）
- `final_suggestion`: `hs_code`, `confidence`, `review_required`
- `inspection_and_compliance`: `needs_ci`, `regulatory_conditions`, `license_requirements`
- `tax`:
  - `import`: `mfn_rate`, `provisional_rate`, `vat_rate`, `consumption_tax_rate`
  - `export`: `export_tariff_rate`, `rebate_hint`
- `trace`: `rule_hits`, `retrieval_refs`, `source_snapshot_time`, `llm_version`, `policy_version_id`

### 5.1 终裁判定矩阵（MVP强制口径）

为避免实现漂移，终裁动作统一按下表执行：

- **自动通过（`review_required=false`）**：需同时满足
  - 无硬冲突（品类冲突、关键字段冲突、监管冲突均为 0）
  - `confidence >= 85`
  - `completeness_ratio >= 0.9`
  - Top1-Top2 分差 `>= 8`
  - 关键参数来源等级不低于 `P1`
- **人工快审（`review_required=true`）**：任一满足
  - `70 <= confidence < 85`
  - `0.7 <= completeness_ratio < 0.9`
  - Top1-Top2 分差 `< 8`
- **强制人工（`review_required=true`）**：任一满足
  - 触发硬规则（见第8节）
  - `confidence < 70`
  - `completeness_ratio < 0.7`
  - 权威税则/税率版本不可用或超时

## 6. hs_code 生成机制

`hs_code` 来源不是“单次猜测”，而是决策链：

1. 型号要素化（品类/用途/参数/封装/材质）。
2. 规则召回候选税号（Top20）。
3. 权威税则有效性校验（无效码剔除）。
4. LLM 结合证据重排为 Top5。
5. 规则终裁输出建议码；不满足条件则强制人工复核。
6. 人工最终确认结果回流案例库，优化后续命中。

## 7. 缺参场景：型号搜索补参

数据源可信度建议：

- `P0`：原厂官网 / Datasheet
- `P1`：授权分销商
- `P2`：主流分销平台
- `P3`：论坛/聚合站（仅线索）

流程：

1. 型号检索多源参数。
2. 多源一致性校验与冲突标记。
3. 高置信参数回填归类引擎。
4. 缺关键参数则降级人工，不强判。

## 8. 置信度评分规则（补参）

总分公式：

`score = 来源分 + 型号匹配分 + 参数一致性分 + 数据完整度分 - 风险扣分`

权重：

- 来源分 30
- 型号匹配分 25
- 参数一致性分 25
- 数据完整度分 20
- 风险扣分最高 40

分级动作：

- `A >= 85`: 自动回填并参与自动判定
- `B 70-84`: 回填 + 人工快审
- `C 50-69`: 仅候选提示，不参与终裁
- `D < 50`: 丢弃并要求人工补参

硬规则（不受分数影响）：

- 缺 `trade_direction/model/product_name_cn/function_use/category` 强制人工。
- 品类冲突或关键字段冲突强制人工。
- 来源分过低可直接拒绝。

## 9. 必填字段模板（摘要）

全局必填：

- `trade_direction`
- `model`
- `product_name_cn`
- `function_use`
- `category`

按品类必填（示例）：

- `capacitor`: `capacitance`, `rated_voltage`, `dielectric_or_material`, `package`
- `resistor`: `resistance`, `power_rating`, `tolerance`, `package`
- `ic`: `ic_type`, `package`, `processor_or_memory_flag`, `application_use`
- `connector`: `connector_type`, `pin_count`, `pitch`, `rated_voltage_current`

完整度计算：

`completeness_ratio = (filled_global_required + filled_category_required) / (total_global_required + total_category_required)`

## 10. 人工复核闭环

复核台最小动作：

1. 采纳 Top1 / 切换 Top2~Top5 / 手工输入税号。
2. 改判原因必填。
3. 回写“最终税号 + 原因 + 证据”，进入案例库。

收益：

- 降低反复争议单据处理成本。
- 让模型从业务真实纠偏中持续学习。

## 11. MVP实施顺序（建议）

**阶段1（2周）**

- 打通输入标准化、Top20规则召回、Top5输出、人工复核台。
- 接入海关查询结果的最小字段缓存（税号、税率、监管条件）。
- 实现置信度评分与强制复核阈值。

**阶段2（2~4周）**

- 接入 LLM 重排序与解释（限制在候选池内）。
- 完善多源补参与冲突仲裁。
- 建立案例回流与离线评估面板。

## 12. 验收指标（MVP）

1. Top5 覆盖率（人工最终税号落在 Top5）>= 90%。
2. 自动通过单据误判率 < 1%。
3. 人工复核平均耗时下降 >= 40%。
4. 每条结论均可追溯（规则命中 + 数据源快照 + 证据片段）。

### 12.1 指标统计口径（必须固定）

- 统计窗口：按自然周滚动统计，发布验收采用最近连续 4 周。
- 样本范围：仅纳入“已有人工最终确认税号”的单据。
- 分层维度：`trade_direction`、`category`、是否缺参补参。
- 最小样本量：每个主要品类（周）不少于 100 单；不足时只做观察不做达标判定。
- 指标同时给出总体值与分层值，避免被单一高频品类掩盖。

## 13. 风险与控制

- **风险：** 仅依赖 LLM 可能幻觉。  
  **控制：** LLM 仅重排解释，规则终裁，冲突强制人工。

- **风险：** 外部来源参数冲突。  
  **控制：** 来源分层、多源一致性、关键冲突不自动通过。

- **风险：** 税率/监管规则时效变化。  
  **控制：** 日增量、周校验、按生效日版本化查询。

- **风险：** 权威数据源短时不可用（超时/限流/页面结构变更）。  
  **控制：** 启用降级策略：读取最近一次有效快照（带时间戳与版本号）并强制人工复核；同时触发告警与补采任务。

## 14. 测试与回归集（新增）

上线前与每次规则/模型更新后，均执行固定回归：

1. `golden_set`：人工一致性高的标准样本（覆盖进口/出口 + 主流品类）。
2. `hard_cases`：历史改判单、争议税号、边界品类样本。
3. `missing_spec_cases`：缺参与多源冲突样本（验证降级与强制人工路径）。
4. `recent_regressions`：最近两周线上错判/险判样本。

准入门槛：回归后不得出现 P0/P1 级别回归（错税号自动通过、监管条件漏判）；否则禁止发布。

## 15. JSON Schema（MVP实现稿）

以下 Schema 用于约束 `POST /classify/by-model` 的请求与响应。字段命名与第5节保持一致，便于服务端校验与联调。

### 15.1 请求体 Schema（Draft 2020-12）

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://caichip.local/schemas/hs-classify-request.json",
  "title": "HsClassifyRequest",
  "type": "object",
  "additionalProperties": false,
  "required": [
    "trade_direction",
    "declaration_date",
    "model",
    "product_name_cn",
    "function_use",
    "category"
  ],
  "properties": {
    "trade_direction": {
      "type": "string",
      "enum": ["import", "export"]
    },
    "declaration_date": {
      "type": "string",
      "format": "date"
    },
    "model": {
      "type": "string",
      "minLength": 1
    },
    "product_name_cn": {
      "type": "string",
      "minLength": 1
    },
    "function_use": {
      "type": "string",
      "minLength": 1
    },
    "category": {
      "type": "string",
      "minLength": 1
    },
    "specs": {
      "type": "object",
      "description": "按品类扩展键值，如 capacitance/rated_voltage/package 等",
      "additionalProperties": {
        "type": ["string", "number", "boolean", "null"]
      }
    },
    "context": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "brand": { "type": "string" },
        "origin_country": { "type": "string" },
        "declared_unit": { "type": "string" },
        "declared_quantity": { "type": "number", "minimum": 0 }
      }
    }
  }
}
```

### 15.2 响应体 Schema（Draft 2020-12）

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://caichip.local/schemas/hs-classify-response.json",
  "title": "HsClassifyResponse",
  "type": "object",
  "additionalProperties": false,
  "required": [
    "candidates",
    "final_suggestion",
    "inspection_and_compliance",
    "tax",
    "trace"
  ],
  "properties": {
    "candidates": {
      "type": "array",
      "minItems": 1,
      "maxItems": 5,
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": [
          "hs_code",
          "score",
          "reason",
          "evidence",
          "required_elements_missing"
        ],
        "properties": {
          "hs_code": {
            "type": "string",
            "pattern": "^[0-9]{10}$"
          },
          "score": {
            "type": "number",
            "minimum": 0,
            "maximum": 100
          },
          "reason": { "type": "string" },
          "evidence": {
            "type": "array",
            "items": { "type": "string" }
          },
          "required_elements_missing": {
            "type": "array",
            "items": { "type": "string" }
          }
        }
      }
    },
    "final_suggestion": {
      "type": "object",
      "additionalProperties": false,
      "required": ["hs_code", "confidence", "review_required"],
      "properties": {
        "hs_code": {
          "type": "string",
          "pattern": "^[0-9]{10}$"
        },
        "confidence": {
          "type": "number",
          "minimum": 0,
          "maximum": 100
        },
        "review_required": { "type": "boolean" },
        "review_reason_codes": {
          "type": "array",
          "items": { "type": "string" }
        }
      }
    },
    "inspection_and_compliance": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "needs_ci",
        "regulatory_conditions",
        "license_requirements"
      ],
      "properties": {
        "needs_ci": { "type": "boolean" },
        "regulatory_conditions": {
          "type": "array",
          "items": { "type": "string" }
        },
        "license_requirements": {
          "type": "array",
          "items": { "type": "string" }
        }
      }
    },
    "tax": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "import": {
          "type": "object",
          "additionalProperties": false,
          "properties": {
            "mfn_rate": { "type": "number", "minimum": 0 },
            "provisional_rate": { "type": "number", "minimum": 0 },
            "vat_rate": { "type": "number", "minimum": 0 },
            "consumption_tax_rate": { "type": "number", "minimum": 0 }
          }
        },
        "export": {
          "type": "object",
          "additionalProperties": false,
          "properties": {
            "export_tariff_rate": { "type": "number", "minimum": 0 },
            "rebate_hint": { "type": "string" }
          }
        }
      }
    },
    "trace": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "rule_hits",
        "retrieval_refs",
        "source_snapshot_time",
        "llm_version",
        "policy_version_id"
      ],
      "properties": {
        "rule_hits": {
          "type": "array",
          "items": { "type": "string" }
        },
        "retrieval_refs": {
          "type": "array",
          "items": { "type": "string" }
        },
        "source_snapshot_time": {
          "type": "string",
          "format": "date-time"
        },
        "llm_version": { "type": "string" },
        "policy_version_id": { "type": "string" }
      }
    }
  }
}
```

### 15.3 终裁规则配置 Schema（可热更新）

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://caichip.local/schemas/hs-final-decision-policy.json",
  "title": "HsFinalDecisionPolicy",
  "type": "object",
  "additionalProperties": false,
  "required": [
    "version",
    "auto_pass",
    "quick_review",
    "force_review",
    "hard_rules"
  ],
  "properties": {
    "version": { "type": "string" },
    "auto_pass": {
      "type": "object",
      "additionalProperties": false,
      "required": [
        "min_confidence",
        "min_completeness_ratio",
        "min_top1_top2_gap",
        "min_source_tier"
      ],
      "properties": {
        "min_confidence": { "type": "number", "minimum": 0, "maximum": 100 },
        "min_completeness_ratio": { "type": "number", "minimum": 0, "maximum": 1 },
        "min_top1_top2_gap": { "type": "number", "minimum": 0 },
        "min_source_tier": { "type": "string", "enum": ["P0", "P1", "P2", "P3"] }
      }
    },
    "quick_review": {
      "type": "object",
      "additionalProperties": false,
      "required": ["confidence_range", "completeness_range"],
      "properties": {
        "confidence_range": {
          "type": "array",
          "minItems": 2,
          "maxItems": 2,
          "items": { "type": "number", "minimum": 0, "maximum": 100 }
        },
        "completeness_range": {
          "type": "array",
          "minItems": 2,
          "maxItems": 2,
          "items": { "type": "number", "minimum": 0, "maximum": 1 }
        },
        "top1_top2_gap_lt": { "type": "number", "minimum": 0 }
      }
    },
    "force_review": {
      "type": "object",
      "additionalProperties": false,
      "required": ["max_confidence", "max_completeness_ratio"],
      "properties": {
        "max_confidence": { "type": "number", "minimum": 0, "maximum": 100 },
        "max_completeness_ratio": { "type": "number", "minimum": 0, "maximum": 1 },
        "when_policy_source_unavailable": { "type": "boolean" }
      }
    },
    "hard_rules": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["code", "description", "enabled"],
        "properties": {
          "code": { "type": "string" },
          "description": { "type": "string" },
          "enabled": { "type": "boolean" }
        }
      }
    }
  }
}
```

说明：

- 规则服务读取 `HsFinalDecisionPolicy.version` 并在 `trace.policy_version_id` 回传，保证判定可追溯。
- 推荐将 `hard_rules.code` 与 `final_suggestion.review_reason_codes` 对齐，便于复核台统计。

## 16. 默认策略配置示例（可直接落库）

建议将以下 JSON 作为 `final-decision-policy.json` 的初始版本：

```json
{
  "version": "mvp-2026-04-13-v1",
  "auto_pass": {
    "min_confidence": 85,
    "min_completeness_ratio": 0.9,
    "min_top1_top2_gap": 8,
    "min_source_tier": "P1"
  },
  "quick_review": {
    "confidence_range": [70, 85],
    "completeness_range": [0.7, 0.9],
    "top1_top2_gap_lt": 8
  },
  "force_review": {
    "max_confidence": 70,
    "max_completeness_ratio": 0.7,
    "when_policy_source_unavailable": true
  },
  "hard_rules": [
    {
      "code": "HR_MISSING_GLOBAL_REQUIRED",
      "description": "缺 trade_direction/model/product_name_cn/function_use/category",
      "enabled": true
    },
    {
      "code": "HR_CATEGORY_CONFLICT",
      "description": "品类冲突",
      "enabled": true
    },
    {
      "code": "HR_KEY_FIELD_CONFLICT",
      "description": "关键字段冲突",
      "enabled": true
    },
    {
      "code": "HR_REGULATORY_CONFLICT",
      "description": "监管条件冲突",
      "enabled": true
    },
    {
      "code": "HR_LOW_SOURCE_TRUST",
      "description": "来源分或来源等级低于阈值",
      "enabled": true
    }
  ]
}
```

## 17. 请求/响应样例（联调用）

### 17.1 自动通过样例（Auto Pass）

请求：

```json
{
  "trade_direction": "import",
  "declaration_date": "2026-04-14",
  "model": "CL10B104KB8NNNC",
  "product_name_cn": "多层片式陶瓷电容器",
  "function_use": "电源去耦与滤波",
  "category": "capacitor",
  "specs": {
    "capacitance": "0.1uF",
    "rated_voltage": "50V",
    "dielectric_or_material": "X7R",
    "package": "0603"
  },
  "context": {
    "brand": "Samsung",
    "origin_country": "KR",
    "declared_unit": "pcs",
    "declared_quantity": 50000
  }
}
```

响应（示例）：

```json
{
  "candidates": [
    {
      "hs_code": "8532241000",
      "score": 91.2,
      "reason": "材质、容量、电压与片式陶瓷电容归类要素高度匹配",
      "evidence": [
        "GACC_TARIFF_NOTE_2026Q2#853224",
        "CASE_LIB#IMP_CAP_202603_0012"
      ],
      "required_elements_missing": []
    },
    {
      "hs_code": "8532249000",
      "score": 80.4,
      "reason": "同章兜底项，参数匹配度低于Top1",
      "evidence": ["GACC_TARIFF_NOTE_2026Q2#853224"],
      "required_elements_missing": []
    }
  ],
  "final_suggestion": {
    "hs_code": "8532241000",
    "confidence": 89.5,
    "review_required": false,
    "review_reason_codes": []
  },
  "inspection_and_compliance": {
    "needs_ci": false,
    "regulatory_conditions": [],
    "license_requirements": []
  },
  "tax": {
    "import": {
      "mfn_rate": 8.0,
      "provisional_rate": 6.0,
      "vat_rate": 13.0,
      "consumption_tax_rate": 0
    }
  },
  "trace": {
    "rule_hits": [
      "RULE_CAP_CATEGORY_MATCH",
      "RULE_CAP_PARAM_COMPLETE"
    ],
    "retrieval_refs": [
      "GACC_TARIFF_NOTE_2026Q2#853224",
      "CASE_LIB#IMP_CAP_202603_0012"
    ],
    "source_snapshot_time": "2026-04-14T08:00:00Z",
    "llm_version": "reranker-1.0.0",
    "policy_version_id": "mvp-2026-04-13-v1"
  }
}
```

### 17.2 人工快审样例（Quick Review）

请求：

```json
{
  "trade_direction": "import",
  "declaration_date": "2026-04-14",
  "model": "STM32F103C8T6",
  "product_name_cn": "微控制器",
  "function_use": "通用控制",
  "category": "ic",
  "specs": {
    "ic_type": "MCU",
    "package": "LQFP48",
    "application_use": "工业控制"
  }
}
```

响应（示例）：

```json
{
  "candidates": [
    {
      "hs_code": "8542319000",
      "score": 82.1,
      "reason": "MCU用途与封装匹配，但处理器/存储归属字段未明确",
      "evidence": [
        "GACC_TARIFF_NOTE_2026Q2#854231",
        "CASE_LIB#IMP_IC_202602_0108"
      ],
      "required_elements_missing": ["processor_or_memory_flag"]
    },
    {
      "hs_code": "8542399000",
      "score": 76.8,
      "reason": "可归入其他集成电路项，证据弱于Top1",
      "evidence": ["GACC_TARIFF_NOTE_2026Q2#854239"],
      "required_elements_missing": ["processor_or_memory_flag"]
    }
  ],
  "final_suggestion": {
    "hs_code": "8542319000",
    "confidence": 78.9,
    "review_required": true,
    "review_reason_codes": [
      "QR_CONFIDENCE_RANGE",
      "QR_MISSING_CATEGORY_REQUIRED"
    ]
  },
  "inspection_and_compliance": {
    "needs_ci": false,
    "regulatory_conditions": [],
    "license_requirements": []
  },
  "tax": {
    "import": {
      "mfn_rate": 0,
      "provisional_rate": 0,
      "vat_rate": 13.0,
      "consumption_tax_rate": 0
    }
  },
  "trace": {
    "rule_hits": ["RULE_IC_CATEGORY_MATCH"],
    "retrieval_refs": [
      "GACC_TARIFF_NOTE_2026Q2#854231",
      "CASE_LIB#IMP_IC_202602_0108"
    ],
    "source_snapshot_time": "2026-04-14T08:00:00Z",
    "llm_version": "reranker-1.0.0",
    "policy_version_id": "mvp-2026-04-13-v1"
  }
}
```

### 17.3 强制人工样例（Force Review）

请求：

```json
{
  "trade_direction": "import",
  "declaration_date": "2026-04-14",
  "model": "ABC-UNKNOWN-001",
  "product_name_cn": "电子元器件",
  "function_use": "未明确",
  "category": "ic",
  "specs": {
    "package": "QFN32"
  }
}
```

响应（示例）：

```json
{
  "candidates": [
    {
      "hs_code": "8542399000",
      "score": 63.5,
      "reason": "仅有封装信息，关键归类要素缺失",
      "evidence": ["GACC_TARIFF_NOTE_2026Q2#854239"],
      "required_elements_missing": [
        "ic_type",
        "processor_or_memory_flag",
        "application_use"
      ]
    }
  ],
  "final_suggestion": {
    "hs_code": "8542399000",
    "confidence": 61.0,
    "review_required": true,
    "review_reason_codes": [
      "FR_LOW_CONFIDENCE",
      "HR_MISSING_GLOBAL_REQUIRED",
      "FR_POLICY_SOURCE_UNAVAILABLE"
    ]
  },
  "inspection_and_compliance": {
    "needs_ci": true,
    "regulatory_conditions": ["待人工确认监管条件"],
    "license_requirements": ["待人工确认许可证要求"]
  },
  "tax": {
    "import": {
      "mfn_rate": 0,
      "provisional_rate": 0,
      "vat_rate": 13.0,
      "consumption_tax_rate": 0
    }
  },
  "trace": {
    "rule_hits": [
      "RULE_FORCE_REVIEW_MISSING_REQUIRED",
      "RULE_FORCE_REVIEW_POLICY_UNAVAILABLE"
    ],
    "retrieval_refs": ["GACC_TARIFF_NOTE_2026Q2#854239"],
    "source_snapshot_time": "2026-04-14T08:00:00Z",
    "llm_version": "reranker-1.0.0",
    "policy_version_id": "mvp-2026-04-13-v1"
  }
}
```

## 18. 错误码与 review_reason_codes 词典

目标：统一服务端返回、前端提示、复核台统计口径。  
约定：`error_code` 用于接口失败；`review_reason_codes` 用于接口成功但需人工处理的原因标记。
机器可读词典：`docs/schema/review_reason_codes.json`（建议由前后端共享同一份常量源）。
Go 常量生成器：`cmd/tools/gen_review_codes/main.go`，输出 `pkg/hsclassifycodes/codes_gen.go`。

## 19. 运行时配置与联调样例

- 终裁策略配置：`docs/schema/hs_final_decision_policy.json`
- 请求样例：`docs/schema/hs_classify_request.example.json`
- 响应样例：
  - `docs/schema/hs_classify_response_auto_pass.example.json`
  - `docs/schema/hs_classify_response_quick_review.example.json`
  - `docs/schema/hs_classify_response_force_review.example.json`
- 发布前校验建议：
  1. 校验 `trace.policy_version_id` 与策略 `version` 一致
  2. 校验 `review_required=true` 时 `review_reason_codes` 非空
  3. 校验样例字段与 `ClassifyByModel` proto 字段一致

### 18.1 error_code（接口失败）

| error_code | HTTP | 触发条件 | 前端提示（建议） | 处理建议 |
|---|---:|---|---|---|
| `INVALID_ARGUMENT` | 400 | 必填字段缺失或格式非法（如 `declaration_date` 非日期） | 入参不完整或格式错误，请检查后重试 | 前端高亮字段；后端返回字段级错误明细 |
| `UNSUPPORTED_CATEGORY` | 400 | `category` 不在支持范围 | 当前品类暂不支持自动归类 | 引导人工归类，记录新增品类需求 |
| `POLICY_VERSION_NOT_FOUND` | 409 | 指定生效日找不到可用政策版本 | 未找到对应生效日税则版本 | 触发补采任务并转人工 |
| `POLICY_SOURCE_UNAVAILABLE` | 503 | 权威数据源超时/限流/不可用 | 权威数据暂不可用，已转人工复核 | 启动降级策略与告警 |
| `RETRIEVAL_TIMEOUT` | 504 | 检索证据超时 | 证据检索超时，请稍后重试 | 保留请求上下文，支持幂等重试 |
| `RERANKER_UNAVAILABLE` | 503 | LLM 重排服务不可用 | 重排服务暂不可用，已降级规则结果 | 使用纯规则候选并强制人工 |
| `INTERNAL_ERROR` | 500 | 未分类系统异常 | 系统异常，请稍后重试 | 记录 `trace_id` 并告警 |

### 18.2 review_reason_codes（成功返回但需人工）

#### A. Quick Review（前缀 `QR_`）

| code | 触发条件（示例） | 前端提示（建议） | 复核动作 |
|---|---|---|---|
| `QR_CONFIDENCE_RANGE` | `70 <= confidence < 85` | 置信度中等，建议人工快审 | 核对 Top1 与 Top2 差异证据 |
| `QR_COMPLETENESS_RANGE` | `0.7 <= completeness_ratio < 0.9` | 关键要素不完整，建议补充参数后确认 | 补参后重跑，若提升则采纳 |
| `QR_TOP_GAP_LOW` | Top1-Top2 分差 `< 8` | 候选分值接近，需人工判断 | 对比两候选章注与要素 |
| `QR_MISSING_CATEGORY_REQUIRED` | 品类必填缺失但未触发硬阻断 | 缺少品类关键字段，请补充 | 补齐后重新判定 |

#### B. Force Review（前缀 `FR_`）

| code | 触发条件（示例） | 前端提示（建议） | 复核动作 |
|---|---|---|---|
| `FR_LOW_CONFIDENCE` | `confidence < 70` | 置信度低，禁止自动通过 | 人工确认税号并记录原因 |
| `FR_LOW_COMPLETENESS` | `completeness_ratio < 0.7` | 信息不足，需人工补充后复核 | 补关键参数再判定 |
| `FR_POLICY_SOURCE_UNAVAILABLE` | 权威政策源不可用 | 权威数据不可用，已强制人工 | 使用最近有效快照 + 人工确认 |
| `FR_HARD_RULE_TRIGGERED` | 任一硬规则命中 | 触发硬规则，需人工处理 | 查看对应 `HR_*` 明细 |

#### C. Hard Rule（前缀 `HR_`，可与 `FR_` 同时出现）

| code | 触发条件 | 前端提示（建议） | 复核动作 |
|---|---|---|---|
| `HR_MISSING_GLOBAL_REQUIRED` | 缺全局必填字段 | 缺少核心字段，无法自动归类 | 必填补齐后重跑 |
| `HR_CATEGORY_CONFLICT` | 品类判断冲突 | 品类冲突，请人工确认品类 | 先定品类，再归类 |
| `HR_KEY_FIELD_CONFLICT` | 关键字段多源冲突 | 关键参数冲突，请确认权威来源 | 以 P0/P1 证据裁决 |
| `HR_REGULATORY_CONFLICT` | 监管条件冲突 | 监管条件存在冲突，需人工确认 | 核对监管条款与许可证 |
| `HR_LOW_SOURCE_TRUST` | 来源等级低于阈值 | 参数来源可信度不足 | 补充原厂或授权源证据 |

### 18.3 返回格式约定

- `error_code` 仅出现在非 2xx 响应体中，且必须配套 `message` 与 `trace_id`。
- 2xx 响应中，若 `final_suggestion.review_required=true`，必须返回 `review_reason_codes`（至少 1 个）。
- `review_reason_codes` 可多值并存，建议按优先级排序：`HR_* > FR_* > QR_*`。

推荐错误响应示例：

```json
{
  "error_code": "INVALID_ARGUMENT",
  "message": "declaration_date format invalid, expected YYYY-MM-DD",
  "details": [
    {
      "field": "declaration_date",
      "reason": "invalid_format"
    }
  ],
  "trace_id": "3f0f7a4b7efb4e13a8e6f2d7c8bb6f2a"
}
```

## 19. 结论

在当前“权威结构化库不完整”的前提下，采用“Top5 + 规则终裁 + LLM重排解释 + 人工复核”是可上线、可扩展且合规风险可控的路径。随着海关权威数据与内部案例库沉淀，可逐步提升自动通过比例。
