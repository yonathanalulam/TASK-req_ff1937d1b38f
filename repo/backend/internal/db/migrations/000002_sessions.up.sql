-- 000002: Sessions and login attempt tracking

CREATE TABLE IF NOT EXISTS sessions (
    id             VARCHAR(64)      NOT NULL PRIMARY KEY,
    user_id        BIGINT UNSIGNED  NOT NULL,
    csrf_token     VARCHAR(64)      NOT NULL,
    ip_address     VARCHAR(45)      NULL,
    user_agent     TEXT             NULL,
    last_active_at DATETIME         NOT NULL,
    expires_at     DATETIME         NOT NULL,
    created_at     DATETIME         NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_sessions_user    (user_id),
    INDEX idx_sessions_expires (expires_at),
    CONSTRAINT fk_session_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS login_attempts (
    id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    username     VARCHAR(50)  NOT NULL,
    ip_address   VARCHAR(45)  NOT NULL,
    success      TINYINT(1)   NOT NULL DEFAULT 0,
    attempted_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_la_username_time (username, attempted_at),
    INDEX idx_la_ip_time       (ip_address, attempted_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
