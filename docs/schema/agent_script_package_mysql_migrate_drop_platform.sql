-- 从旧版「platform_id + script_id」迁移到仅 script_id（执行前请备份）
-- 1) 若 platform_id 与 script_id 不一致，需先人工合并数据再执行。

ALTER TABLE agent_script_package DROP INDEX uk_pkg_platform_script_version;
ALTER TABLE agent_script_package DROP INDEX idx_pkg_platform_script_status;
ALTER TABLE agent_script_package DROP COLUMN platform_id;
ALTER TABLE agent_script_package ADD UNIQUE KEY uk_pkg_script_version (script_id, version);
ALTER TABLE agent_script_package ADD KEY idx_pkg_script_status (script_id, status);
