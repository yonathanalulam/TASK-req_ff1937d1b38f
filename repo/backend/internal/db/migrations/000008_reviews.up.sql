-- 000008: Reviews, review images, and review reports

CREATE TABLE IF NOT EXISTS reviews (
    id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    ticket_id   BIGINT UNSIGNED NOT NULL UNIQUE,
    user_id     BIGINT UNSIGNED NOT NULL,
    offering_id BIGINT UNSIGNED NOT NULL,
    rating      TINYINT UNSIGNED NOT NULL CHECK (rating BETWEEN 1 AND 5),
    text        TEXT             NULL,
    status      ENUM('published','pending_moderation','rejected') NOT NULL DEFAULT 'published',
    created_at  DATETIME         NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME         NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_rev_offering (offering_id),
    INDEX idx_rev_user     (user_id),
    INDEX idx_rev_status   (status),
    CONSTRAINT fk_rev_ticket   FOREIGN KEY (ticket_id)   REFERENCES tickets           (id) ON DELETE CASCADE,
    CONSTRAINT fk_rev_user     FOREIGN KEY (user_id)     REFERENCES users             (id) ON DELETE RESTRICT,
    CONSTRAINT fk_rev_offering FOREIGN KEY (offering_id) REFERENCES service_offerings (id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS review_images (
    id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    review_id    BIGINT UNSIGNED NOT NULL,
    filename     VARCHAR(255)    NOT NULL,
    storage_path VARCHAR(512)    NOT NULL,
    created_at   DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_ri_review (review_id),
    CONSTRAINT fk_ri_review FOREIGN KEY (review_id) REFERENCES reviews (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS review_reports (
    id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    review_id   BIGINT UNSIGNED NOT NULL,
    reporter_id BIGINT UNSIGNED NOT NULL,
    reason      ENUM('spam','abusive','irrelevant') NOT NULL,
    details     TEXT            NULL,
    created_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_rr_review   (review_id),
    INDEX idx_rr_reporter (reporter_id),
    CONSTRAINT fk_rr_review   FOREIGN KEY (review_id)   REFERENCES reviews (id) ON DELETE CASCADE,
    CONSTRAINT fk_rr_reporter FOREIGN KEY (reporter_id) REFERENCES users   (id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
