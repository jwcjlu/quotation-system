-- HS 手动上传 datasheet 暂存（消费后写入 t_hs_datasheet_asset）
CREATE TABLE IF NOT EXISTS t_hs_manual_datasheet_upload (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  upload_id VARCHAR(64) NOT NULL,
  local_path VARCHAR(512) NOT NULL,
  sha256 CHAR(64) NOT NULL,
  expires_at DATETIME(3) NOT NULL,
  owner_subject VARCHAR(128) NULL,
  consumed_at DATETIME(3) NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id),
  UNIQUE KEY uk_hs_manual_upload_id (upload_id),
  KEY idx_hs_manual_upload_expires (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
