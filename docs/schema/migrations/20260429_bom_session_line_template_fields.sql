-- BOM 模板字段：统一型号、位号、替代型号、备注、描述
ALTER TABLE `t_bom_session_line`
  ADD COLUMN IF NOT EXISTS `unified_mpn` varchar(256) NULL COMMENT '统一型号' AFTER `mpn`,
  ADD COLUMN IF NOT EXISTS `reference_designator` text NULL COMMENT '位号' AFTER `unified_mpn`,
  ADD COLUMN IF NOT EXISTS `substitute_mpn` varchar(256) NULL COMMENT '替代型号' AFTER `reference_designator`,
  ADD COLUMN IF NOT EXISTS `remark` text NULL COMMENT '备注' AFTER `substitute_mpn`,
  ADD COLUMN IF NOT EXISTS `description` text NULL COMMENT '描述/规格' AFTER `remark`;
