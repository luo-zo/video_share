-- +goose Up
CREATE TABLE videos (
    id           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    user_id      BIGINT UNSIGNED NOT NULL,
    title        VARCHAR(100)    NOT NULL,
    description  TEXT            NOT NULL,
    object_key   VARCHAR(512)    NOT NULL,
    status       TINYINT         NOT NULL DEFAULT 1 COMMENT '1=uploading 2=ready 3=failed 4=deleted',
    file_size    BIGINT UNSIGNED NOT NULL DEFAULT 0,
    content_type VARCHAR(100)    NOT NULL DEFAULT 'video/mp4',
    created_at   DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at   DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uk_videos_object_key (object_key),
    KEY idx_videos_status_created (status, created_at, id),
    KEY idx_videos_user_created (user_id, created_at, id),
    CONSTRAINT fk_videos_user FOREIGN KEY (user_id) REFERENCES users (id),
    CONSTRAINT chk_videos_status CHECK (status IN (1, 2, 3, 4))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- +goose Down
-- 回滚会删除所有视频元数据；对象存储中的文件不会随之删除。
DROP TABLE IF EXISTS videos;
