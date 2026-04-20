-- 000014: Data ingestion sources, jobs, and checkpoints

CREATE TABLE IF NOT EXISTS ingest_sources (
    id                BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    name              VARCHAR(255)    NOT NULL UNIQUE,
    source_type       ENUM('db_table','log_file','filesystem_drop') NOT NULL,
    config_encrypted  VARBINARY(2048) NOT NULL,
    is_active         TINYINT(1)      NOT NULL DEFAULT 1,
    created_at        DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS ingest_jobs (
    id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    source_id       BIGINT UNSIGNED NOT NULL,
    status          ENUM('pending','running','completed','failed','paused') NOT NULL DEFAULT 'pending',
    rows_ingested   BIGINT UNSIGNED NOT NULL DEFAULT 0,
    rows_expected   BIGINT UNSIGNED NOT NULL DEFAULT 0,
    schema_valid    TINYINT(1)      NOT NULL DEFAULT 1,
    error_message   TEXT            NULL,
    started_at      DATETIME        NULL,
    completed_at    DATETIME        NULL,
    created_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_ij_source (source_id),
    INDEX idx_ij_status (status),
    CONSTRAINT fk_ij_source FOREIGN KEY (source_id) REFERENCES ingest_sources (id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS ingest_checkpoints (
    id                BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    source_id         BIGINT UNSIGNED NOT NULL,
    job_id            BIGINT UNSIGNED NOT NULL,
    checkpoint_type   ENUM('updated_at','offset') NOT NULL,
    checkpoint_value  TEXT           NOT NULL,
    created_at        DATETIME       NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        DATETIME       NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uq_checkpoint (source_id, job_id),
    CONSTRAINT fk_ic_source FOREIGN KEY (source_id) REFERENCES ingest_sources (id) ON DELETE CASCADE,
    CONSTRAINT fk_ic_job    FOREIGN KEY (job_id)    REFERENCES ingest_jobs    (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
