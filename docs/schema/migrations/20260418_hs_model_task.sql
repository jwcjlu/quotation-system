-- HS 型号解析任务快照（跨进程可轮询，与 design §6 / §12.1 对齐）
CREATE TABLE IF NOT EXISTS `t_hs_model_task` (
  `run_id` varchar(512) NOT NULL,
  `model` varchar(128) NOT NULL,
  `manufacturer` varchar(128) NOT NULL,
  `request_trace_id` varchar(256) NOT NULL DEFAULT '',
  `task_status` varchar(32) NOT NULL,
  `result_status` varchar(32) NOT NULL,
  `stage` varchar(64) NOT NULL DEFAULT '',
  `attempt_count` int NOT NULL DEFAULT 0,
  `last_error` text,
  `best_score` decimal(8,4) NOT NULL DEFAULT 0.0000,
  `best_code_ts` char(10) NOT NULL DEFAULT '',
  `updated_at` datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`run_id`),
  KEY `idx_hs_model_task_model_mfr_updated` (`model`, `manufacturer`, `updated_at`),
  KEY `idx_hs_model_task_req` (`model`, `manufacturer`, `request_trace_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
