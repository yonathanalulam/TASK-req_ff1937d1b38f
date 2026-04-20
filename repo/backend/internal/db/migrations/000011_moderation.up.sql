-- 000011: Content moderation — sensitive terms, queue, actions, violations

CREATE TABLE IF NOT EXISTS sensitive_terms (
    id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    term       VARCHAR(255)                          NOT NULL UNIQUE,
    class      ENUM('prohibited','borderline')       NOT NULL DEFAULT 'borderline',
    created_at DATETIME                              NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS moderation_queue (
    id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    content_type  ENUM('review','qa_thread','qa_post','ticket_note') NOT NULL,
    content_id    BIGINT UNSIGNED NOT NULL,
    content_text  TEXT            NOT NULL,
    flagged_terms JSON            NULL,
    status        ENUM('pending','approved','rejected') NOT NULL DEFAULT 'pending',
    moderator_id  BIGINT UNSIGNED  NULL,
    reviewed_at   DATETIME         NULL,
    created_at    DATETIME         NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_mq_status  (status),
    INDEX idx_mq_content (content_type, content_id),
    CONSTRAINT fk_mq_moderator FOREIGN KEY (moderator_id) REFERENCES users (id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS moderation_actions (
    id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    moderator_id BIGINT UNSIGNED NOT NULL,
    action_type  VARCHAR(50)     NOT NULL,
    content_type VARCHAR(50)     NOT NULL,
    content_id   BIGINT UNSIGNED NOT NULL,
    reason       TEXT            NULL,
    created_at   DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_ma_moderator (moderator_id),
    CONSTRAINT fk_ma_moderator FOREIGN KEY (moderator_id) REFERENCES users (id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS violation_records (
    id                    BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id               BIGINT UNSIGNED NOT NULL,
    content_type          VARCHAR(50)     NOT NULL,
    content_id            BIGINT UNSIGNED NOT NULL,
    violation_at          DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    freeze_applied        TINYINT(1)      NOT NULL DEFAULT 0,
    freeze_duration_hours INT UNSIGNED    NOT NULL DEFAULT 0,
    created_at            DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_vr_user (user_id),
    CONSTRAINT fk_vr_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
