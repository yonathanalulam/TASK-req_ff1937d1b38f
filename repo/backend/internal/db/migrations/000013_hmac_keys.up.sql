-- 000013: HMAC signing keys for internal client authentication

CREATE TABLE IF NOT EXISTS hmac_keys (
    id               BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    key_id           VARCHAR(64)    NOT NULL UNIQUE,
    secret_encrypted VARBINARY(512) NOT NULL,
    is_active        TINYINT(1)     NOT NULL DEFAULT 1,
    created_at       DATETIME       NOT NULL DEFAULT CURRENT_TIMESTAMP,
    rotated_at       DATETIME       NULL,
    INDEX idx_hk_active (is_active)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
