-- +goose Up
ALTER TABLE videos
    DROP CHECK chk_videos_status,
    ADD COLUMN hls_master_key VARCHAR(512) NULL AFTER content_type,
    ADD COLUMN cover_object_key VARCHAR(512) NULL AFTER hls_master_key,
    ADD COLUMN duration_ms BIGINT UNSIGNED NULL AFTER cover_object_key,
    ADD COLUMN width INT UNSIGNED NULL AFTER duration_ms,
    ADD COLUMN height INT UNSIGNED NULL AFTER width,
    ADD COLUMN processing_progress TINYINT UNSIGNED NOT NULL DEFAULT 0 AFTER height,
    ADD COLUMN processing_error VARCHAR(1000) NULL AFTER processing_progress,
    ADD COLUMN processed_at DATETIME(3) NULL AFTER processing_error,
    ADD CONSTRAINT chk_videos_status CHECK (status IN (1, 2, 3, 4, 5)),
    ADD CONSTRAINT chk_videos_processing_progress CHECK (processing_progress BETWEEN 0 AND 100);

CREATE TABLE video_transcode_jobs (
    id              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    job_id          CHAR(36)        NOT NULL,
    video_id        BIGINT UNSIGNED NOT NULL,
    idempotency_key VARCHAR(128)    NOT NULL,
    status          TINYINT         NOT NULL DEFAULT 1 COMMENT '1=pending 2=queued 3=running 4=succeeded 5=failed',
    attempts        INT UNSIGNED    NOT NULL DEFAULT 0,
    max_attempts    INT UNSIGNED    NOT NULL DEFAULT 3,
    next_retry_at   DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    worker_id       VARCHAR(128)    NULL,
    last_error      VARCHAR(1000)   NULL,
    started_at      DATETIME(3)     NULL,
    finished_at     DATETIME(3)     NULL,
    created_at      DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at      DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uk_transcode_jobs_job_id (job_id),
    UNIQUE KEY uk_transcode_jobs_idempotency (idempotency_key),
    KEY idx_transcode_jobs_video (video_id, created_at),
    KEY idx_transcode_jobs_dispatch (status, next_retry_at, id),
    CONSTRAINT fk_transcode_jobs_video FOREIGN KEY (video_id) REFERENCES videos (id),
    CONSTRAINT chk_transcode_jobs_status CHECK (status IN (1, 2, 3, 4, 5))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE outbox_events (
    id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    event_id      CHAR(36)        NOT NULL,
    topic         VARCHAR(255)    NOT NULL,
    event_key     VARCHAR(255)    NOT NULL,
    event_type    VARCHAR(100)    NOT NULL,
    payload       JSON            NOT NULL,
    status        TINYINT         NOT NULL DEFAULT 1 COMMENT '1=pending 2=published',
    attempts      INT UNSIGNED    NOT NULL DEFAULT 0,
    next_retry_at DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    published_at  DATETIME(3)     NULL,
    last_error    VARCHAR(1000)   NULL,
    created_at    DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at    DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uk_outbox_events_event_id (event_id),
    KEY idx_outbox_events_pending (status, next_retry_at, id),
    CONSTRAINT chk_outbox_events_status CHECK (status IN (1, 2))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- +goose Down
-- 回滚会删除全部待发布事件、转码任务和已经生成的处理元数据；MinIO 对象不会自动删除。
DROP TABLE IF EXISTS outbox_events;
DROP TABLE IF EXISTS video_transcode_jobs;

ALTER TABLE videos
    DROP CHECK chk_videos_processing_progress,
    DROP CHECK chk_videos_status,
    DROP COLUMN processed_at,
    DROP COLUMN processing_error,
    DROP COLUMN processing_progress,
    DROP COLUMN height,
    DROP COLUMN width,
    DROP COLUMN duration_ms,
    DROP COLUMN cover_object_key,
    DROP COLUMN hls_master_key,
    ADD CONSTRAINT chk_videos_status CHECK (status IN (1, 2, 3, 4));
