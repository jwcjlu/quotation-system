-- BOM 合并键 require_proxy 且 getdps 失败时的退避队列（策略 B）
CREATE TABLE IF NOT EXISTS t_bom_merge_proxy_wait (
  mpn_norm     VARCHAR(256) NOT NULL,
  platform_id  VARCHAR(32)  NOT NULL,
  biz_date     DATE         NOT NULL,
  next_retry_at DATETIME(3) NOT NULL COMMENT '到期后由 worker 再次 TryDispatchMergeKey',
  attempt      INT          NOT NULL DEFAULT 0 COMMENT '已连续失败次数',
  last_error   TEXT         NULL,
  first_failed_at DATETIME(3) NULL COMMENT '首次失败时间，用于 wall_clock 上限',
  created_at   DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at   DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (mpn_norm, platform_id, biz_date),
  KEY idx_bom_merge_proxy_wait_next (next_retry_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
