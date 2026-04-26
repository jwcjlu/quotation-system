# Specs 索引（需求 / 设计）

本目录存放 **可版本化** 的需求与设计说明，供实现与 Code Review 引用。  
**说明：** `using-superpowers` 等技能描述的是 **工作方式**（何时 brainstorm、何时 writing-plans），**不**替代仓库内的需求文档；BOM 相关请从下表进入。

## BOM 货源搜索与配单

| 顺序 | 文档 | 内容 |
|------|------|------|
| 1 | [2026-03-27-bom-sourcing-requirements.md](./2026-03-27-bom-sourcing-requirements.md) | 产品需求要点（WHAT） |
| 2 | [2026-03-27-bom-sourcing-design.md](./2026-03-27-bom-sourcing-design.md) | 失败/跳过策略、任务状态机、Excel 映射、就绪判定（HOW） |
| 3 | [../plans/2026-03-27-bom-sourcing-implementation.md](../plans/2026-03-27-bom-sourcing-implementation.md) | 实现计划（任务拆解） |
| 4 | [2026-04-19-bom-line-hs-customs-tax-design.md](./2026-04-19-bom-line-hs-customs-tax-design.md) | 配单行 HS、商检、`t_hs_tax_rate_daily` 关税缓存与「一键找 HS」 |
| 5 | [../plans/2026-04-19-bom-line-hs-customs-tax-implementation.md](../plans/2026-04-19-bom-line-hs-customs-tax-implementation.md) | 上述设计的实现计划（proto、biz、data、Wire、Resolve 空厂牌对齐） |
| 6 | [2026-04-19-hs-resolve-manual-datasheet-design.md](./2026-04-19-hs-resolve-manual-datasheet-design.md) | HS Resolve：无 datasheet 时手动描述 + 上传手册（PDF）、优先级与 API |
| 7 | [2026-04-21-bom-llm-async-import-design.md](./2026-04-21-bom-llm-async-import-design.md) | BOM 导入（LLM）异步解析 + 进度条：状态机、分块策略、门禁与兼容性 |

## 数据模型（DDL 草案）

- [../../schema/bom_mysql.sql](../../schema/bom_mysql.sql)

## 其他计划（非 BOM）

- [../plans/](../plans/) 目录下按日期前缀的 `*-implementation.md` 文件。
