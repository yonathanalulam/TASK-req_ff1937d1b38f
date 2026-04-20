-- 000006: Service categories and offerings

CREATE TABLE IF NOT EXISTS service_categories (
    id                       BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    name                     VARCHAR(100) NOT NULL UNIQUE,
    slug                     VARCHAR(100) NOT NULL UNIQUE,
    description              TEXT         NULL,
    response_time_minutes    INT UNSIGNED NOT NULL DEFAULT 60,
    completion_time_minutes  INT UNSIGNED NOT NULL DEFAULT 480,
    created_at               DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at               DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS service_offerings (
    id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    agent_id        BIGINT UNSIGNED NOT NULL,
    category_id     BIGINT UNSIGNED NOT NULL,
    name            VARCHAR(200)    NOT NULL,
    description     TEXT            NULL,
    base_price      DECIMAL(10,2)   NOT NULL DEFAULT 0.00,
    duration_minutes INT UNSIGNED   NOT NULL DEFAULT 60,
    active_status   TINYINT(1)      NOT NULL DEFAULT 1,
    created_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_so_agent    (agent_id),
    INDEX idx_so_category (category_id),
    INDEX idx_so_active   (active_status),
    CONSTRAINT fk_so_agent    FOREIGN KEY (agent_id)    REFERENCES users              (id) ON DELETE RESTRICT,
    CONSTRAINT fk_so_category FOREIGN KEY (category_id) REFERENCES service_categories (id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
