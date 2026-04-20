-- 000012: Data export requests, deletion requests, and audit logs

CREATE TABLE IF NOT EXISTS data_export_requests (
    id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id       BIGINT UNSIGNED NOT NULL,
    status        ENUM('pending','processing','ready','downloaded','expired') NOT NULL DEFAULT 'pending',
    file_path     VARCHAR(512)    NULL,
    requested_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ready_at      DATETIME        NULL,
    downloaded_at DATETIME        NULL,
    expires_at    DATETIME        NULL,
    INDEX idx_der_user   (user_id),
    INDEX idx_der_status (status),
    CONSTRAINT fk_der_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS data_deletion_requests (
    id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id       BIGINT UNSIGNED NOT NULL,
    status        ENUM('pending','anonymized','cancelled') NOT NULL DEFAULT 'pending',
    requested_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    scheduled_for DATETIME        NOT NULL,
    completed_at  DATETIME        NULL,
    INDEX idx_ddr_user      (user_id),
    INDEX idx_ddr_scheduled (scheduled_for, status),
    CONSTRAINT fk_ddr_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Append-only audit log — no UPDATE or DELETE allowed at application layer
CREATE TABLE IF NOT EXISTS audit_logs (
    id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id     BIGINT UNSIGNED  NULL,
    action      VARCHAR(100)     NOT NULL,
    entity_type VARCHAR(50)      NULL,
    entity_id   BIGINT UNSIGNED  NULL,
    ip_address  VARCHAR(45)      NULL,
    user_agent  TEXT             NULL,
    metadata    JSON             NULL,
    created_at  DATETIME         NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_al_user    (user_id),
    INDEX idx_al_action  (action),
    INDEX idx_al_entity  (entity_type, entity_id),
    INDEX idx_al_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
