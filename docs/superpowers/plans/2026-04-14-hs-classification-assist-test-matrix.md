# HS Classification Assist Test Matrix

- golden_set: 高置信样本应 `review_required=false`
- hard_cases: 关键字段冲突应命中 `HR_*`
- missing_spec: 缺字段应强制人工并返回 `review_reason_codes`
- recent_regressions: `policy_version_id` 必须回传且非空
