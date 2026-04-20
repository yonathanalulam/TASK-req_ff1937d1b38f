-- 000003: User preferences, favorites, and browsing history

CREATE TABLE IF NOT EXISTS user_preferences (
    user_id        BIGINT UNSIGNED NOT NULL PRIMARY KEY,
    notify_in_app  TINYINT(1)      NOT NULL DEFAULT 1,
    muted_tags     JSON            NULL,
    muted_authors  JSON            NULL,
    updated_at     DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_pref_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS user_favorites (
    id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id     BIGINT UNSIGNED NOT NULL,
    offering_id BIGINT UNSIGNED NOT NULL,
    created_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY uq_fav (user_id, offering_id),
    INDEX idx_fav_user (user_id),
    CONSTRAINT fk_fav_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS user_browsing_history (
    id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id     BIGINT UNSIGNED NOT NULL,
    offering_id BIGINT UNSIGNED NOT NULL,
    viewed_at   DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_history_user (user_id),
    INDEX idx_history_time (user_id, viewed_at),
    CONSTRAINT fk_hist_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
