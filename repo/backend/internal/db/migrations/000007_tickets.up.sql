-- 000007: Service request tickets, notes, and attachments

CREATE TABLE IF NOT EXISTS tickets (
    id               BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id          BIGINT UNSIGNED NOT NULL,
    offering_id      BIGINT UNSIGNED NOT NULL,
    category_id      BIGINT UNSIGNED NOT NULL,
    address_id       BIGINT UNSIGNED NOT NULL,
    preferred_start  DATETIME        NOT NULL,
    preferred_end    DATETIME        NOT NULL,
    delivery_method  ENUM('pickup','courier') NOT NULL DEFAULT 'pickup',
    shipping_fee     DECIMAL(10,2)   NOT NULL DEFAULT 0.00,
    status           ENUM('Accepted','Dispatched','In Service','Completed','Closed','Cancelled')
                                     NOT NULL DEFAULT 'Accepted',
    sla_deadline     DATETIME        NULL,
    sla_breached     TINYINT(1)      NOT NULL DEFAULT 0,
    cancel_reason    TEXT            NULL,
    created_at       DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_ticket_user     (user_id),
    INDEX idx_ticket_status   (status),
    INDEX idx_ticket_offering (offering_id),
    INDEX idx_ticket_sla      (sla_deadline, sla_breached),
    CONSTRAINT fk_ticket_user     FOREIGN KEY (user_id)     REFERENCES users              (id) ON DELETE RESTRICT,
    CONSTRAINT fk_ticket_offering FOREIGN KEY (offering_id) REFERENCES service_offerings  (id) ON DELETE RESTRICT,
    CONSTRAINT fk_ticket_category FOREIGN KEY (category_id) REFERENCES service_categories (id) ON DELETE RESTRICT,
    CONSTRAINT fk_ticket_address  FOREIGN KEY (address_id)  REFERENCES addresses          (id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS ticket_notes (
    id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    ticket_id  BIGINT UNSIGNED NOT NULL,
    author_id  BIGINT UNSIGNED NOT NULL,
    content    TEXT            NOT NULL,
    created_at DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_tn_ticket (ticket_id),
    CONSTRAINT fk_tn_ticket FOREIGN KEY (ticket_id) REFERENCES tickets (id) ON DELETE CASCADE,
    CONSTRAINT fk_tn_author FOREIGN KEY (author_id) REFERENCES users   (id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS ticket_attachments (
    id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    ticket_id     BIGINT UNSIGNED NOT NULL,
    filename      VARCHAR(255)    NOT NULL,
    original_name VARCHAR(255)    NOT NULL,
    mime_type     VARCHAR(100)    NOT NULL,
    size_bytes    INT UNSIGNED    NOT NULL,
    storage_path  VARCHAR(512)    NOT NULL,
    created_at    DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_ta_ticket (ticket_id),
    CONSTRAINT fk_ta_ticket FOREIGN KEY (ticket_id) REFERENCES tickets (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
