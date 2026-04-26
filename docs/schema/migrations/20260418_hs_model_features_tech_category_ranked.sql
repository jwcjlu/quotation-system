-- 设计 §5.2：有序备选类目 JSON
ALTER TABLE `t_hs_model_features`
  ADD COLUMN `tech_category_ranked_json` json NULL AFTER `tech_category`;
