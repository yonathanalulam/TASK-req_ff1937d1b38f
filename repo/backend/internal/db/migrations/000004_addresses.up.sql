-- 000004: US-style address book

CREATE TABLE IF NOT EXISTS addresses (
    id                    BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id               BIGINT UNSIGNED NOT NULL,
    label                 VARCHAR(100)    NOT NULL DEFAULT 'Home',
    address_line1_encrypted VARBINARY(512) NOT NULL,
    address_line2_encrypted VARBINARY(512) NULL,
    city                  VARCHAR(100)    NOT NULL,
    state                 CHAR(2)         NOT NULL,
    zip                   VARCHAR(10)     NOT NULL,
    is_default            TINYINT(1)      NOT NULL DEFAULT 0,
    created_at            DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at            DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_addr_user    (user_id),
    INDEX idx_addr_default (user_id, is_default),
    CONSTRAINT fk_addr_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
