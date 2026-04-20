-- 000001: Roles, Users, and User-Role assignments

CREATE TABLE IF NOT EXISTS roles (
    id          TINYINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    name        VARCHAR(30)  NOT NULL UNIQUE,
    description VARCHAR(255) NOT NULL DEFAULT ''
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT IGNORE INTO roles (name, description) VALUES
    ('regular_user',  'Standard authenticated user'),
    ('service_agent', 'Manages and fulfills service tickets'),
    ('moderator',     'Reviews and moderates user-generated content'),
    ('administrator', 'Full system access'),
    ('data_operator', 'Manages data ingestion and lakehouse pipeline');

CREATE TABLE IF NOT EXISTS users (
    id                   BIGINT UNSIGNED  AUTO_INCREMENT PRIMARY KEY,
    username             VARCHAR(50)      NOT NULL UNIQUE,
    email                VARCHAR(255)     NOT NULL UNIQUE,
    password_hash        VARCHAR(255)     NOT NULL,
    display_name         VARCHAR(100)     NOT NULL,
    phone_encrypted      VARBINARY(512)   NULL,
    avatar_url           VARCHAR(512)     NULL,
    bio                  TEXT             NULL,
    is_active            TINYINT(1)       NOT NULL DEFAULT 1,
    is_deleted           TINYINT(1)       NOT NULL DEFAULT 0,
    anonymized_at        DATETIME         NULL,
    posting_freeze_until DATETIME         NULL,
    created_at           DATETIME         NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at           DATETIME         NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_users_username (username),
    INDEX idx_users_email    (email),
    INDEX idx_users_active   (is_active)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS user_roles (
    user_id     BIGINT UNSIGNED  NOT NULL,
    role_id     TINYINT UNSIGNED NOT NULL,
    assigned_at DATETIME         NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, role_id),
    CONSTRAINT fk_ur_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT fk_ur_role FOREIGN KEY (role_id) REFERENCES roles (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
