-- HS 型号到 code_ts 映射能力：4 张核心表（MySQL 8+）

CREATE TABLE IF NOT EXISTS t_hs_datasheet_asset (
    id              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
    model           VARCHAR(128) NOT NULL COMMENT '型号',
    manufacturer    VARCHAR(128) NOT NULL COMMENT '厂牌',
    datasheet_url   VARCHAR(1024) NOT NULL DEFAULT '' COMMENT '数据手册 URL',
    local_path      VARCHAR(512) NOT NULL DEFAULT '' COMMENT '本地落盘路径',
    sha256          CHAR(64) NOT NULL DEFAULT '' COMMENT '文件 SHA256',
    download_status ENUM('ok','failed') NOT NULL DEFAULT 'failed' COMMENT '下载状态',
    error_msg       VARCHAR(512) NOT NULL DEFAULT '' COMMENT '失败原因',
    updated_at      DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '更新时间',
    PRIMARY KEY (id),
    KEY idx_hs_datasheet_asset_model_mfr (model, manufacturer)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='datasheet 资产事实表';

CREATE TABLE IF NOT EXISTS t_hs_model_mapping (
    id                      BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
    model                   VARCHAR(128) NOT NULL COMMENT '型号',
    manufacturer            VARCHAR(128) NOT NULL COMMENT '厂牌',
    code_ts                 CHAR(10) NOT NULL COMMENT '10 位数字编码（保留前导 0）',
    source                  ENUM('manual','llm_auto') NOT NULL DEFAULT 'llm_auto' COMMENT '映射来源',
    confidence              DECIMAL(5,4) NULL COMMENT '置信度',
    status                  ENUM('confirmed','pending_review','rejected') NOT NULL DEFAULT 'pending_review' COMMENT '结果状态',
    features_version        VARCHAR(64) NOT NULL DEFAULT '' COMMENT '抽取版本',
    recommendation_version  VARCHAR(64) NOT NULL DEFAULT '' COMMENT '推荐版本',
    created_at              DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
    updated_at              DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '更新时间',
    PRIMARY KEY (id),
    UNIQUE KEY uk_hs_model_mapping_model_mfr (model, manufacturer),
    KEY idx_hs_model_mapping_code_ts (code_ts),
    KEY idx_hs_model_mapping_status (status),
    CONSTRAINT chk_hs_model_mapping_code_ts CHECK (code_ts REGEXP '^[0-9]{10}$')
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='型号 + 厂牌 到 HS 编码最终映射';

CREATE TABLE IF NOT EXISTS t_hs_model_features (
    id              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
    model           VARCHAR(128) NOT NULL COMMENT '型号',
    manufacturer    VARCHAR(128) NOT NULL COMMENT '厂牌',
    asset_id        BIGINT UNSIGNED NOT NULL COMMENT '关联 datasheet 资产',
    tech_category   VARCHAR(64) NOT NULL DEFAULT '' COMMENT '技术类别',
    component_name  VARCHAR(128) NOT NULL DEFAULT '' COMMENT '元器件名称',
    package_form    VARCHAR(64) NOT NULL DEFAULT '' COMMENT '封装形式',
    key_specs_json  JSON NULL COMMENT '关键参数 JSON',
    raw_extract_json JSON NULL COMMENT '原始抽取 JSON',
    extract_model   VARCHAR(64) NOT NULL DEFAULT '' COMMENT '抽取模型名',
    extract_version VARCHAR(64) NOT NULL DEFAULT '' COMMENT '抽取版本',
    created_at      DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
    PRIMARY KEY (id),
    KEY idx_hs_model_features_model_mfr (model, manufacturer),
    KEY idx_hs_model_features_asset_id (asset_id),
    CONSTRAINT fk_hs_model_features_asset_id FOREIGN KEY (asset_id) REFERENCES t_hs_datasheet_asset(id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='datasheet 结构化特征';

CREATE TABLE IF NOT EXISTS t_hs_model_recommendation (
    id                  BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
    model               VARCHAR(128) NOT NULL COMMENT '型号',
    manufacturer        VARCHAR(128) NOT NULL COMMENT '厂牌',
    run_id              CHAR(36) NOT NULL COMMENT '推荐批次 ID',
    candidate_rank      TINYINT UNSIGNED NOT NULL COMMENT '候选排序位次',
    code_ts             CHAR(10) NOT NULL COMMENT '候选 10 位 code_ts',
    g_name              VARCHAR(512) NOT NULL DEFAULT '' COMMENT '候选商品名称',
    score               DECIMAL(5,4) NULL COMMENT '分值',
    reason              VARCHAR(1024) NOT NULL DEFAULT '' COMMENT '推荐理由',
    input_snapshot_json JSON NULL COMMENT '输入快照',
    recommend_model     VARCHAR(64) NOT NULL DEFAULT '' COMMENT '推荐模型名',
    recommend_version   VARCHAR(64) NOT NULL DEFAULT '' COMMENT '推荐版本',
    created_at          DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
    PRIMARY KEY (id),
    UNIQUE KEY uk_hs_model_reco_run_rank (run_id, candidate_rank),
    KEY idx_hs_model_reco_model_mfr_created (model, manufacturer, created_at),
    KEY idx_hs_model_reco_run_id (run_id),
    CONSTRAINT chk_hs_model_reco_code_ts CHECK (code_ts REGEXP '^[0-9]{10}$')
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='候选推荐审计轨迹';
