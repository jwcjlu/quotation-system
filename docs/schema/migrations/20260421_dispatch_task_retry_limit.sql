ALTER TABLE t_caichip_dispatch_task
  ADD COLUMN retry_max INT NOT NULL DEFAULT 3 AFTER attempt,
  ADD COLUMN retry_backoff_json JSON NULL AFTER retry_max;
