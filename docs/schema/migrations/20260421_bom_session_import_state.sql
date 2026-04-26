ALTER TABLE t_bom_session
  ADD COLUMN IF NOT EXISTS import_status VARCHAR(32) NOT NULL DEFAULT 'idle' AFTER storage_file_key,
  ADD COLUMN IF NOT EXISTS import_progress INT NOT NULL DEFAULT 0 AFTER import_status,
  ADD COLUMN IF NOT EXISTS import_stage VARCHAR(64) NOT NULL DEFAULT 'validating' AFTER import_progress,
  ADD COLUMN IF NOT EXISTS import_message TEXT NULL AFTER import_stage,
  ADD COLUMN IF NOT EXISTS import_error_code VARCHAR(64) NULL AFTER import_message,
  ADD COLUMN IF NOT EXISTS import_error TEXT NULL AFTER import_error_code,
  ADD COLUMN IF NOT EXISTS import_updated_at DATETIME(3) NULL AFTER import_error;
