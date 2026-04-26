-- BOM 相关表统一升级为 utf8mb4，修复 4-byte Unicode 入库失败（Error 1366）。
-- 适用：历史库/表仍为 utf8 或 utf8mb3 的场景。
-- 说明：本脚本幂等；仅做字符集与排序规则升级，不改业务语义。

SET @__bom_db := DATABASE();

-- 可选：先把当前库默认字符集切到 utf8mb4（仅影响后续新建对象）。
SET @__sql_alter_db := CONCAT(
  'ALTER DATABASE `', @__bom_db, '` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci'
);
PREPARE __stmt_alter_db FROM @__sql_alter_db;
EXECUTE __stmt_alter_db;
DEALLOCATE PREPARE __stmt_alter_db;

-- 统一转换 BOM 关键表字符集（含现有字符串列）。
-- 注意：CONVERT TO 会重建表，建议在低峰期执行。
SET @__tbl := 't_bom_quote_item';
SET @__has_tbl := (
  SELECT COUNT(*)
  FROM information_schema.TABLES
  WHERE TABLE_SCHEMA = @__bom_db
    AND TABLE_NAME = @__tbl
);
SET @__sql_convert_tbl := IF(
  @__has_tbl = 1,
  CONCAT('ALTER TABLE `', @__tbl, '` CONVERT TO CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci'),
  'SELECT ''t_bom_quote_item missing, skip'' AS bom_migration_msg'
);
PREPARE __stmt_convert_tbl FROM @__sql_convert_tbl;
EXECUTE __stmt_convert_tbl;
DEALLOCATE PREPARE __stmt_convert_tbl;

SET @__tbl := 't_bom_quote_cache';
SET @__has_tbl := (
  SELECT COUNT(*)
  FROM information_schema.TABLES
  WHERE TABLE_SCHEMA = @__bom_db
    AND TABLE_NAME = @__tbl
);
SET @__sql_convert_tbl := IF(
  @__has_tbl = 1,
  CONCAT('ALTER TABLE `', @__tbl, '` CONVERT TO CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci'),
  'SELECT ''t_bom_quote_cache missing, skip'' AS bom_migration_msg'
);
PREPARE __stmt_convert_tbl FROM @__sql_convert_tbl;
EXECUTE __stmt_convert_tbl;
DEALLOCATE PREPARE __stmt_convert_tbl;

SET @__tbl := 't_bom_search_task';
SET @__has_tbl := (
  SELECT COUNT(*)
  FROM information_schema.TABLES
  WHERE TABLE_SCHEMA = @__bom_db
    AND TABLE_NAME = @__tbl
);
SET @__sql_convert_tbl := IF(
  @__has_tbl = 1,
  CONCAT('ALTER TABLE `', @__tbl, '` CONVERT TO CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci'),
  'SELECT ''t_bom_search_task missing, skip'' AS bom_migration_msg'
);
PREPARE __stmt_convert_tbl FROM @__sql_convert_tbl;
EXECUTE __stmt_convert_tbl;
DEALLOCATE PREPARE __stmt_convert_tbl;

SET @__tbl := 't_bom_session';
SET @__has_tbl := (
  SELECT COUNT(*)
  FROM information_schema.TABLES
  WHERE TABLE_SCHEMA = @__bom_db
    AND TABLE_NAME = @__tbl
);
SET @__sql_convert_tbl := IF(
  @__has_tbl = 1,
  CONCAT('ALTER TABLE `', @__tbl, '` CONVERT TO CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci'),
  'SELECT ''t_bom_session missing, skip'' AS bom_migration_msg'
);
PREPARE __stmt_convert_tbl FROM @__sql_convert_tbl;
EXECUTE __stmt_convert_tbl;
DEALLOCATE PREPARE __stmt_convert_tbl;

SET @__tbl := 't_bom_session_line';
SET @__has_tbl := (
  SELECT COUNT(*)
  FROM information_schema.TABLES
  WHERE TABLE_SCHEMA = @__bom_db
    AND TABLE_NAME = @__tbl
);
SET @__sql_convert_tbl := IF(
  @__has_tbl = 1,
  CONCAT('ALTER TABLE `', @__tbl, '` CONVERT TO CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci'),
  'SELECT ''t_bom_session_line missing, skip'' AS bom_migration_msg'
);
PREPARE __stmt_convert_tbl FROM @__sql_convert_tbl;
EXECUTE __stmt_convert_tbl;
DEALLOCATE PREPARE __stmt_convert_tbl;

-- 显式兜底：确保 t_bom_quote_item.`desc` 是 utf8mb4 文本列。
SET @__has_desc := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = @__bom_db
    AND TABLE_NAME = 't_bom_quote_item'
    AND COLUMN_NAME = 'desc'
);
SET @__sql_fix_desc := IF(
  @__has_desc = 1,
  'ALTER TABLE t_bom_quote_item MODIFY COLUMN `desc` TEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NULL COMMENT ''描述原文''',
  'SELECT ''t_bom_quote_item.desc missing, skip'' AS bom_migration_msg'
);
PREPARE __stmt_fix_desc FROM @__sql_fix_desc;
EXECUTE __stmt_fix_desc;
DEALLOCATE PREPARE __stmt_fix_desc;

-- 检查建议（执行后可手动查询）：
-- SHOW CREATE TABLE t_bom_quote_item;
-- SHOW VARIABLES LIKE 'character_set%';
-- SHOW VARIABLES LIKE 'collation%';
