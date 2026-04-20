-- 000015: Lakehouse metadata, lineage, schema evolution, lifecycle policies, legal holds

CREATE TABLE IF NOT EXISTS lakehouse_metadata (
    id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    source_id   BIGINT UNSIGNED NOT NULL,
    layer       ENUM('bronze','silver','gold') NOT NULL,
    file_path   VARCHAR(512)    NOT NULL,
    row_count   BIGINT UNSIGNED NOT NULL DEFAULT 0,
    schema_hash VARCHAR(64)     NULL,
    ingested_at DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    archived_at DATETIME        NULL,
    purged_at   DATETIME        NULL,
    INDEX idx_lm_source  (source_id),
    INDEX idx_lm_layer   (layer),
    INDEX idx_lm_created (ingested_at),
    CONSTRAINT fk_lm_source FOREIGN KEY (source_id) REFERENCES ingest_sources (id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS lakehouse_lineage (
    id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    output_id  BIGINT UNSIGNED NOT NULL,
    input_id   BIGINT UNSIGNED NOT NULL,
    created_at DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY uq_lineage (output_id, input_id),
    CONSTRAINT fk_ll_output FOREIGN KEY (output_id) REFERENCES lakehouse_metadata (id) ON DELETE CASCADE,
    CONSTRAINT fk_ll_input  FOREIGN KEY (input_id)  REFERENCES lakehouse_metadata (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS lakehouse_schema_versions (
    id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    source_id   BIGINT UNSIGNED NOT NULL,
    version     INT UNSIGNED    NOT NULL,
    schema_json JSON            NOT NULL,
    is_breaking TINYINT(1)      NOT NULL DEFAULT 0,
    created_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY uq_schema_version (source_id, version),
    CONSTRAINT fk_lsv_source FOREIGN KEY (source_id) REFERENCES ingest_sources (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS lakehouse_lifecycle_policies (
    id                BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    source_id         BIGINT UNSIGNED NULL,
    layer             ENUM('bronze','silver','gold') NOT NULL,
    archive_after_days INT UNSIGNED   NOT NULL DEFAULT 90,
    purge_after_days  INT UNSIGNED    NOT NULL DEFAULT 548,
    created_at        DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_llp_source FOREIGN KEY (source_id) REFERENCES ingest_sources (id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS legal_holds (
    id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    source_id   BIGINT UNSIGNED NULL,
    job_id      BIGINT UNSIGNED NULL,
    reason      TEXT            NOT NULL,
    placed_by   BIGINT UNSIGNED NOT NULL,
    placed_at   DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    released_at DATETIME        NULL,
    INDEX idx_lh_source (source_id),
    INDEX idx_lh_active (released_at),
    CONSTRAINT fk_lh_source    FOREIGN KEY (source_id) REFERENCES ingest_sources (id) ON DELETE SET NULL,
    CONSTRAINT fk_lh_job       FOREIGN KEY (job_id)    REFERENCES ingest_jobs    (id) ON DELETE SET NULL,
    CONSTRAINT fk_lh_placed_by FOREIGN KEY (placed_by) REFERENCES users          (id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
