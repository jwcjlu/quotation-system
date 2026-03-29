-- Agent × script_id 站点登录凭据（与 GORM CaichipAgentScriptAuth / t_caichip_agent_script_auth 一致）
CREATE TABLE IF NOT EXISTS t_caichip_agent_script_auth (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    agent_id VARCHAR(64) NOT NULL COMMENT 'Agent 标识',
    script_id VARCHAR(128) NOT NULL COMMENT '脚本/站点标识',
    username VARCHAR(256) NOT NULL COMMENT '明文用户名',
    password_cipher TEXT NOT NULL COMMENT 'AES-256-GCM 密文（nonce+ciphertext，base64）',
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    UNIQUE KEY uk_agent_script_auth (agent_id, script_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
    COMMENT='Agent 按脚本维度的平台登录凭据';
