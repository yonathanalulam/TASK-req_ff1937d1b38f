-- 000010: Notification templates, notifications, and outbox

CREATE TABLE IF NOT EXISTS notification_templates (
    id             BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    code           VARCHAR(100)  NOT NULL UNIQUE,
    title_template TEXT          NOT NULL,
    body_template  TEXT          NOT NULL,
    created_at     DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS notifications (
    id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id       BIGINT UNSIGNED NOT NULL,
    template_code VARCHAR(100)    NULL,
    title         VARCHAR(500)    NOT NULL,
    body          TEXT            NOT NULL,
    is_read       TINYINT(1)      NOT NULL DEFAULT 0,
    created_at    DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_notif_user   (user_id, is_read),
    INDEX idx_notif_created (created_at),
    CONSTRAINT fk_notif_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS notification_outbox (
    id               BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id          BIGINT UNSIGNED NOT NULL,
    notification_id  BIGINT UNSIGNED NOT NULL,
    status           ENUM('pending','sent','failed') NOT NULL DEFAULT 'pending',
    attempts         TINYINT UNSIGNED NOT NULL DEFAULT 0,
    last_attempt_at  DATETIME         NULL,
    created_at       DATETIME         NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_outbox_user   (user_id),
    INDEX idx_outbox_status (status),
    CONSTRAINT fk_outbox_user  FOREIGN KEY (user_id)          REFERENCES users          (id) ON DELETE CASCADE,
    CONSTRAINT fk_outbox_notif FOREIGN KEY (notification_id)  REFERENCES notifications  (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
