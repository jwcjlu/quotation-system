-- run_id 与 service 层 model|manufacturer|request_trace_id 对齐（设计 P2-5）
ALTER TABLE `t_hs_model_recommendation`
  MODIFY COLUMN `run_id` varchar(384) NOT NULL;
