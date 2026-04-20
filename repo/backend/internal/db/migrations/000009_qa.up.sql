-- 000009: Q&A threads and posts

CREATE TABLE IF NOT EXISTS qa_threads (
    id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    offering_id BIGINT UNSIGNED NOT NULL,
    author_id   BIGINT UNSIGNED NOT NULL,
    question    TEXT            NOT NULL,
    status      ENUM('published','pending_moderation','closed') NOT NULL DEFAULT 'published',
    created_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_qa_offering (offering_id),
    INDEX idx_qa_author   (author_id),
    CONSTRAINT fk_qa_offering FOREIGN KEY (offering_id) REFERENCES service_offerings (id) ON DELETE CASCADE,
    CONSTRAINT fk_qa_author   FOREIGN KEY (author_id)   REFERENCES users             (id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS qa_posts (
    id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    thread_id  BIGINT UNSIGNED NOT NULL,
    author_id  BIGINT UNSIGNED NOT NULL,
    content    TEXT            NOT NULL,
    status     ENUM('published','pending_moderation','removed') NOT NULL DEFAULT 'published',
    created_at DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_qp_thread (thread_id),
    CONSTRAINT fk_qp_thread FOREIGN KEY (thread_id) REFERENCES qa_threads (id) ON DELETE CASCADE,
    CONSTRAINT fk_qp_author FOREIGN KEY (author_id) REFERENCES users      (id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
