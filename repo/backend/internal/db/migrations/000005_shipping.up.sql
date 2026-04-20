-- 000005: Shipping regions and fee templates

CREATE TABLE IF NOT EXISTS shipping_regions (
    id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    name         VARCHAR(100) NOT NULL UNIQUE,
    cutoff_time  TIME         NOT NULL DEFAULT '17:00:00',
    timezone     VARCHAR(50)  NOT NULL DEFAULT 'America/New_York',
    created_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS shipping_templates (
    id               BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    region_id        BIGINT UNSIGNED NOT NULL,
    delivery_method  ENUM('pickup','courier') NOT NULL DEFAULT 'courier',
    min_weight_kg    DECIMAL(8,3)    NOT NULL DEFAULT 0,
    max_weight_kg    DECIMAL(8,3)    NOT NULL DEFAULT 999,
    min_quantity     INT UNSIGNED    NOT NULL DEFAULT 1,
    max_quantity     INT UNSIGNED    NOT NULL DEFAULT 9999,
    fee_amount       DECIMAL(10,2)   NOT NULL DEFAULT 0.00,
    currency         CHAR(3)         NOT NULL DEFAULT 'USD',
    lead_time_hours  INT UNSIGNED    NOT NULL DEFAULT 24,
    window_hours     INT UNSIGNED    NOT NULL DEFAULT 4,
    created_at       DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_st_region (region_id),
    CONSTRAINT fk_st_region FOREIGN KEY (region_id) REFERENCES shipping_regions (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
